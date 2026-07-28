package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// RAG 检索缓存 TTL:1 小时
// 相同 query 在窗口内复用 embedding 与检索结果,显著降低 Embedding API 成本与 P95 延迟
const ragCacheTTL = time.Hour

// RAGService RAG 检索服务
// 优先使用 pgvector 语义检索,PostgreSQL 不可用时降级到 TF-IDF 关键词检索
// Redis 可用时启用检索结果缓存,避免重复调 Embedding API
type RAGService struct {
	mysqlDB  *gorm.DB          // MySQL(存储文档元数据,必须)
	pgDB     *gorm.DB          // PostgreSQL + pgvector(可选,用于向量检索)
	embedder EmbeddingProvider // Embedding 生成器
	redis    *redis.Client     // Redis 缓存(可选,nil 时跳过缓存)
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
		redis:    nil, // 默认不开缓存,由 SetRedis 注入
	}
}

// SetRedis 注入 Redis 客户端以启用检索结果缓存
func (r *RAGService) SetRedis(client *redis.Client) {
	r.redis = client
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
// 优先查 Redis 缓存 -> pgvector 语义检索 -> TF-IDF 关键词检索
// 全程埋点 Prometheus 指标:延迟/分数/降级/缓存命中
func (r *RAGService) Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	if topK <= 0 {
		topK = 5
	}
	if query == "" {
		return []RAGDocument{}, nil
	}

	start := time.Now()

	// 1. 查 Redis 缓存(命中则直接返回,策略标签 cache_hit)
	if cached, hit := r.getCache(query, knowledgeBaseID, topK); hit {
		middleware.RAGSearchDuration.WithLabelValues("cache_hit").Observe(time.Since(start).Seconds())
		middleware.RAGSearchTotal.WithLabelValues("cache_hit", "success").Inc()
		middleware.RAGCacheHitsTotal.WithLabelValues(fmt.Sprintf("%d", knowledgeBaseID)).Inc()
		return cached, nil
	}

	// 2. 优先向量检索
	strategy := "tfidf"
	if r.pgDB != nil {
		docs, err := r.vectorSearch(context.Background(), query, knowledgeBaseID, topK)
		if err != nil {
			reason := "query_failed"
			if strings.Contains(err.Error(), "embed") {
				reason = "embed_failed"
			}
			middleware.RAGFallbackTotal.WithLabelValues(reason).Inc()
			logger.Get().Warnf("vector search failed, fallback to TF-IDF: %v", err)
		} else if len(docs) > 0 {
			strategy = "vector"
			middleware.RAGSearchDuration.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
			middleware.RAGSearchTotal.WithLabelValues(strategy, "success").Inc()
			if docs[0].Score > 0 {
				middleware.RAGSearchScore.WithLabelValues(strategy).Observe(docs[0].Score)
			}
			r.setCache(query, knowledgeBaseID, topK, docs)
			return docs, nil
		} else {
			middleware.RAGFallbackTotal.WithLabelValues("no_results").Inc()
		}
	} else {
		middleware.RAGFallbackTotal.WithLabelValues("pg_unavailable").Inc()
	}

	// 3. 降级:TF-IDF 关键词检索
	docs, err := r.tfidfSearch(query, knowledgeBaseID, topK)
	if err != nil {
		middleware.RAGSearchTotal.WithLabelValues(strategy, "failed").Inc()
		middleware.RAGSearchDuration.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
		return nil, err
	}
	if len(docs) == 0 {
		middleware.RAGSearchTotal.WithLabelValues(strategy, "empty").Inc()
	} else {
		middleware.RAGSearchTotal.WithLabelValues(strategy, "success").Inc()
		if docs[0].Score > 0 {
			middleware.RAGSearchScore.WithLabelValues(strategy).Observe(docs[0].Score)
		}
		r.setCache(query, knowledgeBaseID, topK, docs)
	}
	middleware.RAGSearchDuration.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
	return docs, nil
}

// ============== Redis 检索缓存 ==============

// cacheKey 生成 RAG 检索缓存 key
// 格式: rag:search:{kb_id}:{topk}:{query_hash}
// query_hash 使用 FNV-1a 64 位,避免长 query 占用 key 长度
func cacheKey(query string, knowledgeBaseID uint, topK int) string {
	h := fnv1a64(query)
	return fmt.Sprintf("rag:search:%d:%d:%x", knowledgeBaseID, topK, h)
}

// getCache 从 Redis 读取缓存的检索结果
// 未启用 Redis / 未命中均返回 hit=false
func (r *RAGService) getCache(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, bool) {
	if r.redis == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	val, err := r.redis.Get(ctx, cacheKey(query, knowledgeBaseID, topK)).Bytes()
	if err != nil {
		return nil, false
	}
	var docs []RAGDocument
	if err := json.Unmarshal(val, &docs); err != nil {
		return nil, false
	}
	return docs, true
}

// setCache 将检索结果写入 Redis(失败仅记日志,不影响主流程)
func (r *RAGService) setCache(query string, knowledgeBaseID uint, topK int, docs []RAGDocument) {
	if r.redis == nil || len(docs) == 0 {
		return
	}
	data, err := json.Marshal(docs)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := r.redis.Set(ctx, cacheKey(query, knowledgeBaseID, topK), data, ragCacheTTL).Err(); err != nil {
		logger.Get().Warnf("rag cache set failed: %v", err)
	}
}

// fnv1a64 FNV-1a 64 位哈希(用于缓存 key,避免引入额外依赖)
func fnv1a64(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
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
	if r.mysqlDB == nil {
		return nil, fmt.Errorf("rag service unavailable: mysql not initialized")
	}
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
		middleware.RAGIndexDocsTotal.WithLabelValues("failed").Inc()
		return fmt.Errorf("update document status: %w", err)
	}

	// 指标埋点
	middleware.RAGIndexDocsTotal.WithLabelValues("success").Inc()
	middleware.RAGIndexChunks.Observe(float64(len(chunks)))

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
