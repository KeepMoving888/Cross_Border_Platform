package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/logger"
	"gorm.io/gorm"
)

// RAGService RAG 检索服务
// 优先使用 pgvector 语义检索,PostgreSQL 不可用时降级到 TF-IDF 关键词检索
type RAGService struct {
	mysqlDB  *gorm.DB          // MySQL(存储文档元数据,必须)
	pgDB     *gorm.DB          // PostgreSQL + pgvector(可选,用于向量检索)
	embedder EmbeddingProvider // Embedding 生成器
}

// NewRAGService 创建 RAG 服务
// mysqlDB: 业务数据库(必须)
// pgDB: 向量数据库(可选,nil 时降级到 TF-IDF)
// cfg: LLM 配置(决定 Embedding provider)
func NewRAGService(mysqlDB *gorm.DB, pgDB *gorm.DB, cfg config.LLMConfig) *RAGService {
	return &RAGService{
		mysqlDB:  mysqlDB,
		pgDB:     pgDB,
		embedder: NewEmbeddingProvider(cfg),
	}
}

// RAGDocument RAG 检索结果
type RAGDocument struct {
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	Source   string  `json:"source"`
	Score    float64 `json:"score"`
	ChunkIdx int     `json:"chunk_idx"`
}

// Search 检索知识库
// 优先使用 pgvector 语义检索,降级到 TF-IDF 关键词检索
func (r *RAGService) Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	if topK <= 0 {
		topK = 5
	}
	if query == "" {
		return []RAGDocument{}, nil
	}

	// 优先向量检索
	if r.pgDB != nil {
		docs, err := r.vectorSearch(context.Background(), query, knowledgeBaseID, topK)
		if err != nil {
			logger.Get().Warnf("vector search failed, fallback to TF-IDF: %v", err)
		} else if len(docs) > 0 {
			return docs, nil
		}
	}

	// 降级:TF-IDF 关键词检索
	return r.tfidfSearch(query, knowledgeBaseID, topK)
}

// SearchAsContext 检索并格式化为 LLM 上下文字符串
func (r *RAGService) SearchAsContext(query string, knowledgeBaseID uint, topK int) string {
	docs, err := r.Search(query, knowledgeBaseID, topK)
	if err != nil || len(docs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("以下是相关知识库文档片段:\n\n")
	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("【文档 %d】%s (相关度: %.0f%%)\n", i+1, doc.Title, doc.Score*100))
		sb.WriteString(doc.Content)
		sb.WriteString("\n\n---\n\n")
	}
	return sb.String()
}

// ============== 向量检索(pgvector) ==============

// vectorSearch 使用 pgvector 进行语义检索
func (r *RAGService) vectorSearch(ctx context.Context, query string, kbID uint, topK int) ([]RAGDocument, error) {
	// 生成查询向量
	embeddings, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	queryVec := embeddings[0]
	vecStr := vectorToPgString(queryVec)

	// 使用 pgvector 的 <=> 运算符(余弦距离)检索
	// SQL: SELECT kc.content, kc.chunk_index, kd.title, kd.source,
	//             1 - (kc.embedding <=> '[...]') AS score
	//      FROM knowledge_chunks kc
	//      JOIN knowledge_documents kd ON kc.knowledge_doc_id = kd.id
	//      WHERE kd.status = 'ready' [AND kc.knowledge_base_id = ?]
	//      ORDER BY kc.embedding <=> '[...]'
	//      LIMIT ?
	sql := `
		SELECT kd.title, kd.source, kc.content, kc.chunk_index,
		       1 - (kc.embedding <=> ?) AS score
		FROM knowledge_chunks kc
		JOIN knowledge_documents kd ON kc.knowledge_doc_id = kd.id
		WHERE kd.status = 'ready' AND kc.embedding IS NOT NULL`
	args := []interface{}{vecStr}
	if kbID > 0 {
		sql += " AND kc.knowledge_base_id = ?"
		args = append(args, kbID)
	}
	sql += " ORDER BY kc.embedding <=> ? LIMIT ?"
	args = append(args, vecStr, topK)

	rows, err := r.pgDB.WithContext(ctx).Raw(sql, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("pgvector query: %w", err)
	}
	defer rows.Close()

	results := make([]RAGDocument, 0, topK)
	for rows.Next() {
		var doc RAGDocument
		if err := rows.Scan(&doc.Title, &doc.Source, &doc.Content, &doc.ChunkIdx, &doc.Score); err != nil {
			continue
		}
		// score 可能为负(余弦距离),归一化到 [0,1]
		if doc.Score < 0 {
			doc.Score = 0
		}
		doc.Content = truncate(doc.Content, 500)
		results = append(results, doc)
	}
	return results, rows.Err()
}

// vectorToPgString 将 float64 切片转为 pgvector 字符串格式 "[0.1,0.2,0.3]"
func vectorToPgString(vec []float64) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%.6f", v))
	}
	sb.WriteByte(']')
	return sb.String()
}

// ============== TF-IDF 降级检索 ==============

// tfidfSearch 基于关键词的 TF-IDF 检索(PostgreSQL 不可用时使用)
func (r *RAGService) tfidfSearch(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	keywords := tokenize(query)
	if len(keywords) == 0 {
		return []RAGDocument{}, nil
	}

	var docs []models.KnowledgeDocument
	queryDB := r.mysqlDB.Where("status = ?", "ready")
	if knowledgeBaseID > 0 {
		queryDB = queryDB.Where("knowledge_base_id = ?", knowledgeBaseID)
	}
	if err := queryDB.Find(&docs).Error; err != nil {
		return nil, err
	}

	results := make([]RAGDocument, 0, len(docs))
	for _, doc := range docs {
		score := calculateScore(doc.Title+" "+doc.Content, keywords)
		if score > 0 {
			results = append(results, RAGDocument{
				Title:   doc.Title,
				Content: truncate(doc.Content, 500),
				Source:  doc.Source,
				Score:   score,
			})
		}
	}

	sortDocumentsByScore(results)
	if len(results) > topK {
		results = results[:topK]
	}

	topScore := 0.0
	if len(results) > 0 {
		topScore = results[0].Score
	}
	logger.Get().Infof("RAG tfidf search: query=%q, found %d docs (top score: %.4f)", query, len(results), topScore)
	return results, nil
}

// ============== 文档分块与向量化入库 ==============

// ChunkConfig 分块配置
type ChunkConfig struct {
	ChunkSize int // 单块最大字符数
	Overlap   int // 相邻块重叠字符数
}

// DefaultChunkConfig 默认分块配置(500 字符 + 50 重叠)
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{ChunkSize: 500, Overlap: 50}
}

// ChunkText 将长文本按段落 + 固定长度分块
func ChunkText(text string, cfg ChunkConfig) []string {
	if cfg.ChunkSize <= 0 {
		cfg = DefaultChunkConfig()
	}
	if text == "" {
		return nil
	}
	// 按段落分割
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var current strings.Builder
	currentLen := 0

	flushCurrent := func() {
		if current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentLen = 0
		}
	}

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		// 当前块 + 新段落超过阈值,先 flush
		if currentLen > 0 && currentLen+len(para)+1 > cfg.ChunkSize {
			flushCurrent()
		}
		// 单段落超过阈值,按字符切分
		if len(para) > cfg.ChunkSize {
			runes := []rune(para)
			for i := 0; i < len(runes); i += cfg.ChunkSize - cfg.Overlap {
				end := i + cfg.ChunkSize
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[i:end]))
				if end >= len(runes) {
					break
				}
			}
		} else {
			if currentLen > 0 {
				current.WriteString("\n\n")
				currentLen += 2
			}
			current.WriteString(para)
			currentLen += len(para)
		}
	}
	flushCurrent()

	if len(chunks) == 0 && strings.TrimSpace(text) != "" {
		chunks = append(chunks, strings.TrimSpace(text))
	}
	return chunks
}

// IndexDocument 将文档分块并向量化入库
// 需要 PostgreSQL + pgvector 可用,否则只存元数据不生成向量
func (r *RAGService) IndexDocument(ctx context.Context, doc *models.KnowledgeDocument, cfg ChunkConfig) error {
	// 1. 文本分块
	chunks := ChunkText(doc.Content, cfg)
	if len(chunks) == 0 {
		doc.Status = "ready"
		doc.ChunkCount = 0
		return r.mysqlDB.Save(doc).Error
	}

	// 2. 清理旧分块(若有)
	if r.pgDB != nil {
		r.pgDB.Where("knowledge_doc_id = ?", doc.ID).Delete(&models.KnowledgeChunk{})
	}

	// 3. 批量生成向量
	var embeddings [][]float64
	if r.pgDB != nil {
		emb, err := r.embedder.Embed(ctx, chunks)
		if err != nil {
			logger.Get().Warnf("embed chunks failed, storing without vectors: %v", err)
		} else {
			embeddings = emb
		}
	}

	// 4. 写入分块(优先写 PostgreSQL,降级写 MySQL)
	for i, content := range chunks {
		chunk := models.KnowledgeChunk{
			KnowledgeBaseID: doc.KnowledgeBaseID,
			KnowledgeDocID:  doc.ID,
			ChunkIndex:      i,
			Content:         content,
			TokenCount:      estimateTokens(content),
			EmbeddingModel:  r.embedder.Name(),
		}
		if r.pgDB != nil {
			if err := r.pgDB.Create(&chunk).Error; err != nil {
				logger.Get().Warnf("create chunk %d failed: %v", i, err)
				continue
			}
			// 写入 pgvector 列(原生 SQL)
			if i < len(embeddings) && len(embeddings[i]) > 0 {
				vecStr := vectorToPgString(embeddings[i])
				err := r.pgDB.Exec(
					"UPDATE knowledge_chunks SET embedding = ? WHERE id = ?",
					vecStr, chunk.ID,
				).Error
				if err != nil {
					logger.Get().Warnf("update chunk embedding %d failed: %v", chunk.ID, err)
				}
			}
		} else {
			// 降级:存 MySQL(无向量)
			if err := r.mysqlDB.Create(&chunk).Error; err != nil {
				logger.Get().Warnf("create chunk %d (mysql) failed: %v", i, err)
			}
		}
	}

	// 5. 更新文档状态
	doc.Status = "ready"
	doc.ChunkCount = len(chunks)
	if err := r.mysqlDB.Save(doc).Error; err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	logger.Get().Infof("RAG indexed doc %d: %d chunks, %d embeddings",
		doc.ID, len(chunks), len(embeddings))
	return nil
}

// estimateTokens 粗略估算 token 数(中文按 1 字 1 token,英文按 4 字符 1 token)
func estimateTokens(text string) int {
	cnCount := 0
	enLen := 0
	for _, r := range text {
		if r > 0x4e00 {
			cnCount++
		} else {
			enLen++
		}
	}
	return cnCount + enLen/4
}

// ============== 工具函数 ==============

// tokenize 简单分词(降级检索用)
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 0x4e00)
	}) {
		if len(w) >= 2 {
			tokens = append(tokens, w)
		}
	}
	for _, r := range text {
		if r > 0x4e00 {
			tokens = append(tokens, string(r))
		}
	}
	return tokens
}

// calculateScore 计算 TF 分数
func calculateScore(docText string, keywords []string) float64 {
	docLower := strings.ToLower(docText)
	totalScore := 0.0
	for _, kw := range keywords {
		count := strings.Count(docLower, strings.ToLower(kw))
		if count > 0 {
			totalScore += float64(count) / float64(len(docLower)+1) * 100
		}
	}
	// 归一化到 [0,1]
	return math.Min(totalScore/10.0, 1.0)
}

// sortDocumentsByScore 按分数降序排序
func sortDocumentsByScore(docs []RAGDocument) {
	for i := 0; i < len(docs)-1; i++ {
		for j := i + 1; j < len(docs); j++ {
			if docs[i].Score < docs[j].Score {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}
}

// EnsureJSONUsed 确保 json 包被引用(用于 RAG API 序列化)
var _ = json.Marshal
