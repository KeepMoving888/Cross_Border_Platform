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
// 检索链路: Redis 缓存 -> 向量+BM25 混合检索(RRF 融合) -> Reranker 重排序
// 降级链路: 无 VectorStore -> TF-IDF 关键词检索; 无 Rerank API -> 启发式重排
type RAGService struct {
	mysqlDB     *gorm.DB          // MySQL(存储文档元数据,必须)
	pgDB        *gorm.DB          // PostgreSQL + pgvector(可选,用于 BM25 全文检索)
	vectorStore VectorStore       // 向量存储抽象(pgvector/memory/milvus,可选)
	embedder    EmbeddingProvider // Embedding 生成器
	reranker    RerankerProvider  // Reranker 重排序器(可选,nil 时跳过重排)
	redis       *redis.Client     // Redis 缓存(可选,nil 时跳过缓存)
}

// NewRAGService 创建 RAG 服务
// mysqlDB: 业务数据库(必须)
// pgDB: PostgreSQL(可选,启用时同时用于 BM25 检索和 PgVectorStore)
// cfg: LLM 配置(决定 Embedding 和 Reranker provider)
// 向量存储默认使用 PgVectorStore(基于 pgDB),可通过 SetVectorStore 替换为其他实现
func NewRAGService(mysqlDB *gorm.DB, pgDB *gorm.DB, cfg config.LLMConfig) *RAGService {
	svc := &RAGService{
		mysqlDB:  mysqlDB,
		pgDB:     pgDB,
		embedder: NewEmbeddingProvider(cfg),
		reranker: NewRerankerProvider(cfg),
		redis:    nil, // 默认不开缓存,由 SetRedis 注入
	}
	// pgDB 可用时默认创建 PgVectorStore
	// 测试或本地开发可通过 SetVectorStore 注入 InMemoryVectorStore
	if pgDB != nil {
		svc.vectorStore = NewPgVectorStore(pgDB)
	}
	return svc
}

// SetVectorStore 注入自定义 VectorStore(用于测试或运行时切换存储后端)
// 传入 nil 等同于禁用向量检索(降级到 TF-IDF)
func (r *RAGService) SetVectorStore(s VectorStore) {
	r.vectorStore = s
}

// VectorStoreName 返回当前向量存储类型标识(用于监控和调试)
func (r *RAGService) VectorStoreName() string {
	if r.vectorStore == nil {
		return "none"
	}
	return r.vectorStore.Name()
}

// SetRedis 注入 Redis 客户端以启用检索结果缓存
func (r *RAGService) SetRedis(client *redis.Client) {
	r.redis = client
}

// SetReranker 注入自定义 Reranker(用于测试或运行时切换)
func (r *RAGService) SetReranker(p RerankerProvider) {
	r.reranker = p
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
// 链路: Redis 缓存 -> 混合检索(向量 top-N + BM25 top-N -> RRF 融合) -> Reranker 重排 -> top-K
// 降级: 无 pgvector -> TF-IDF; 无 Rerank API -> 启发式重排
// 全程埋点 Prometheus 指标:延迟/分数/策略/降级/缓存命中/重排延迟
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

	// 2. 混合检索:向量 + BM25 并行召回,RRF 融合
	// 召回窗口扩大到 topK*4,给 reranker 更多候选
	recallN := topK * 4
	if recallN < 20 {
		recallN = 20
	}

	strategy := "tfidf"
	var finalDocs []RAGDocument

	// 向量检索可用性: VectorStore 已注入且 Available
	vecAvailable := r.vectorStore != nil && r.vectorStore.Available()
	if vecAvailable {
		// 2a. 向量检索 + BM25 检索并行
		var (
			vecDocs []RAGDocument
			bmDocs  []RAGDocument
			vecErr  error
		)
		vecDocs, vecErr = r.vectorSearch(context.Background(), query, knowledgeBaseID, recallN)
		if vecErr != nil {
			reason := "query_failed"
			if strings.Contains(vecErr.Error(), "embed") {
				reason = "embed_failed"
			}
			middleware.RAGFallbackTotal.WithLabelValues(reason).Inc()
			logger.Get().Warnf("vector search failed: %v", vecErr)
		}
		// BM25 检索(基于 PostgreSQL tsvector,失败静默)
		// BM25 仍直接使用 pgDB,因为它依赖 PostgreSQL 的 tsvector 列和 GIN 索引
		bmDocs, _ = r.bm25Search(context.Background(), query, knowledgeBaseID, recallN)

		// 2b. RRF 融合(两路都有结果时融合,单路时直接用)
		if len(vecDocs) > 0 && len(bmDocs) > 0 {
			finalDocs = RRFusion([][]RAGDocument{vecDocs, bmDocs}, DefaultRRFParams(), recallN)
			strategy = "hybrid"
		} else if len(vecDocs) > 0 {
			finalDocs = vecDocs
			strategy = "vector"
		} else if len(bmDocs) > 0 {
			finalDocs = bmDocs
			strategy = "bm25"
		}

		if len(finalDocs) == 0 {
			middleware.RAGFallbackTotal.WithLabelValues("no_results").Inc()
			// 继续走 TF-IDF 降级
		}
	} else {
		middleware.RAGFallbackTotal.WithLabelValues("pg_unavailable").Inc()
	}

	// 3. 降级:TF-IDF 关键词检索(无 pgvector 或混合检索无结果时)
	if len(finalDocs) == 0 {
		docs, err := r.tfidfSearch(query, knowledgeBaseID, topK)
		if err != nil {
			middleware.RAGSearchTotal.WithLabelValues(strategy, "failed").Inc()
			middleware.RAGSearchDuration.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
			return nil, err
		}
		finalDocs = docs
	}

	if len(finalDocs) == 0 {
		middleware.RAGSearchTotal.WithLabelValues(strategy, "empty").Inc()
		middleware.RAGSearchDuration.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
		return finalDocs, nil
	}

	// 4. Reranker 重排序(可选,提升精度)
	// reranker 为 nil 或不可用时跳过,保持原排序
	if r.reranker != nil && r.reranker.Available() {
		rerankStart := time.Now()
		reranked, err := r.reranker.Rerank(context.Background(), query, finalDocs, topK)
		middleware.RAGRerankDuration.Observe(time.Since(rerankStart).Seconds())
		if err != nil {
			middleware.RAGRerankTotal.WithLabelValues("failed").Inc()
			logger.Get().Warnf("rerank failed, use original order: %v", err)
		} else {
			middleware.RAGRerankTotal.WithLabelValues("success").Inc()
			finalDocs = reranked
		}
	} else if r.reranker != nil {
		// HeuristicRerankerProvider 始终可用,走启发式重排
		rerankStart := time.Now()
		reranked, err := r.reranker.Rerank(context.Background(), query, finalDocs, topK)
		middleware.RAGRerankDuration.Observe(time.Since(rerankStart).Seconds())
		if err == nil {
			middleware.RAGRerankTotal.WithLabelValues("heuristic").Inc()
			finalDocs = reranked
		}
	}

	// 5. 截断到 topK(reranker 可能已截断,这里兜底)
	if topK > 0 && len(finalDocs) > topK {
		finalDocs = finalDocs[:topK]
	}

	// 6. 指标埋点 + 缓存
	middleware.RAGSearchTotal.WithLabelValues(strategy, "success").Inc()
	middleware.RAGSearchDuration.WithLabelValues(strategy).Observe(time.Since(start).Seconds())
	if finalDocs[0].Score > 0 {
		middleware.RAGSearchScore.WithLabelValues(strategy).Observe(finalDocs[0].Score)
	}
	r.setCache(query, knowledgeBaseID, topK, finalDocs)

	return finalDocs, nil
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

// ============== 向量检索(VectorStore 抽象) ==============

// vectorSearch 通过 VectorStore 接口进行语义检索
// 内部流程: 生成 query embedding -> 调用 VectorStore.Search -> 转换为 RAGDocument
// 支持 PgVectorStore / InMemoryVectorStore / 未来的 MilvusStore 等实现
func (r *RAGService) vectorSearch(ctx context.Context, query string, kbID uint, topK int) ([]RAGDocument, error) {
	if r.vectorStore == nil || !r.vectorStore.Available() {
		return nil, fmt.Errorf("vector store unavailable")
	}

	// 生成查询向量
	embeddings, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	queryVec := embeddings[0]

	// 调用 VectorStore 检索
	results, err := r.vectorStore.Search(ctx, queryVec, kbID, topK)
	if err != nil {
		return nil, fmt.Errorf("vector store search: %w", err)
	}

	// 转换为 RAGDocument(统一结构,供 RRF 融合和 Reranker 使用)
	docs := make([]RAGDocument, 0, len(results))
	for _, res := range results {
		docs = append(docs, RAGDocument{
			Title:    "", // VectorStore 不存标题,由上层 JOIN documents 补充(此处简化)
			Content:  truncate(res.Content, 500),
			Source:   "",
			Score:    res.Score,
			ChunkIdx: res.ChunkIndex,
		})
	}
	return docs, nil
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

// ============== BM25 全文检索(PostgreSQL tsvector) ==============

// bm25Search 使用 PostgreSQL tsvector/ts_rank 进行 BM25 风格全文检索
// 与向量检索互补:向量擅长语义近似,BM25 擅长精确关键词(型号/SKU/数字)匹配
// 需要 knowledge_chunks 表有 content_tsv 列(tsvector),由迁移脚本创建
func (r *RAGService) bm25Search(ctx context.Context, query string, kbID uint, topK int) ([]RAGDocument, error) {
	if r.pgDB == nil {
		return nil, fmt.Errorf("pgvector not available")
	}

	// 构造 tsquery:支持多词 OR 匹配(任一命中即召回)
	tsquery := buildTSQuery(query)
	if tsquery == "" {
		return nil, nil
	}

	// SQL: 使用 ts_rank_cd 按相关度排名
	// ts_rank_cd 计算覆盖密度,比 ts_rank 更精确
	sql := `
		SELECT kd.title, kd.source, kc.content, kc.chunk_index,
		       ts_rank_cd(kc.content_tsv, to_tsquery(?)) AS score
		FROM knowledge_chunks kc
		JOIN knowledge_documents kd ON kc.knowledge_doc_id = kd.id
		WHERE kd.status = 'ready'
		  AND kc.content_tsv @@ to_tsquery(?)
		  AND kc.content IS NOT NULL`
	args := []interface{}{tsquery, tsquery}
	if kbID > 0 {
		sql += " AND kc.knowledge_base_id = ?"
		args = append(args, kbID)
	}
	sql += " ORDER BY score DESC LIMIT ?"
	args = append(args, topK)

	rows, err := r.pgDB.WithContext(ctx).Raw(sql, args...).Rows()
	if err != nil {
		// content_tsv 列可能不存在(迁移未执行),静默降级
		logger.Get().Warnf("bm25 search failed (column may not exist): %v", err)
		return nil, err
	}
	defer rows.Close()

	results := make([]RAGDocument, 0, topK)
	for rows.Next() {
		var doc RAGDocument
		if err := rows.Scan(&doc.Title, &doc.Source, &doc.Content, &doc.ChunkIdx, &doc.Score); err != nil {
			continue
		}
		// ts_rank_cd 分数范围较大,归一化到 [0,1](经验值,除以 0.1)
		doc.Score = math.Min(doc.Score/0.1, 1.0)
		if doc.Score < 0 {
			doc.Score = 0
		}
		doc.Content = truncate(doc.Content, 500)
		results = append(results, doc)
	}
	return results, rows.Err()
}

// buildTSQuery 将自然语言 query 转为 PostgreSQL tsquery 格式
// "负离子吹风机" -> "负 & 离子 & 吹风机"(中文按字分词,英文按词)
// 使用 & (AND) 提高精确度,无结果时上游会自动降级
func buildTSQuery(query string) string {
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return ""
	}
	// 过滤过短 token 并转义特殊字符
	var parts []string
	for _, tk := range tokens {
		tk = strings.TrimSpace(tk)
		if len(tk) < 1 {
			continue
		}
		parts = append(parts, tk)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " & ")
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
	// 同时清理 VectorStore 中该文档的旧向量(幂等)
	if r.pgDB != nil {
		r.pgDB.Where("knowledge_doc_id = ?", doc.ID).Delete(&models.KnowledgeChunk{})
	}
	if r.vectorStore != nil && r.vectorStore.Available() {
		if err := r.vectorStore.DeleteByDoc(ctx, doc.ID); err != nil {
			logger.Get().Warnf("vector store delete doc %d failed: %v", doc.ID, err)
		}
	}

	// 3. 批量生成向量(一次性调用 Embedding API,降低延迟和成本)
	// 仅当 VectorStore 可用时才生成向量,否则跳过节省 API 调用
	var embeddings [][]float64
	vecAvailable := r.vectorStore != nil && r.vectorStore.Available()
	if vecAvailable {
		emb, err := r.embedder.Embed(ctx, chunks)
		if err != nil {
			logger.Get().Warnf("embed chunks failed, storing without vectors: %v", err)
		} else {
			embeddings = emb
		}
	}

	// 4. 写入分块元数据(优先 PostgreSQL 以支持 BM25,降级 MySQL)
	// VectorStore 可用时写 PostgreSQL(knowledge_chunks 表),否则降级 MySQL
	// chunks 表同时承载 BM25 tsvector 列(PostgreSQL 触发器自动维护)
	createdChunks := make([]models.KnowledgeChunk, 0, len(chunks))
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
		} else {
			// 降级:存 MySQL(无向量、无 BM25)
			if err := r.mysqlDB.Create(&chunk).Error; err != nil {
				logger.Get().Warnf("create chunk %d (mysql) failed: %v", i, err)
				continue
			}
		}
		createdChunks = append(createdChunks, chunk)
	}

	// 5. 批量写入向量到 VectorStore(通过抽象接口,支持 pgvector/memory/milvus)
	if vecAvailable && len(embeddings) > 0 {
		records := make([]VectorRecord, 0, len(createdChunks))
		for i, chunk := range createdChunks {
			if i >= len(embeddings) || len(embeddings[i]) == 0 {
				continue
			}
			records = append(records, VectorRecord{
				ChunkID:    chunk.ID,
				DocID:      doc.ID,
				KBID:       doc.KnowledgeBaseID,
				ChunkIndex: chunk.ChunkIndex,
				Content:    chunk.Content,
				Embedding:  embeddings[i],
			})
		}
		if len(records) > 0 {
			if err := r.vectorStore.UpsertVectors(ctx, records); err != nil {
				logger.Get().Warnf("vector store upsert doc %d failed: %v", doc.ID, err)
			}
		}
	}

	// 6. 更新文档状态
	doc.Status = "ready"
	doc.ChunkCount = len(chunks)
	if err := r.mysqlDB.Save(doc).Error; err != nil {
		middleware.RAGIndexDocsTotal.WithLabelValues("failed").Inc()
		return fmt.Errorf("update document status: %w", err)
	}

	// 指标埋点
	middleware.RAGIndexDocsTotal.WithLabelValues("success").Inc()
	middleware.RAGIndexChunks.Observe(float64(len(chunks)))

	logger.Get().Infof("RAG indexed doc %d: %d chunks, %d embeddings (store=%s)",
		doc.ID, len(chunks), len(embeddings), r.VectorStoreName())
	return nil
}

// BatchIndexResult 单文档批量入库结果
type BatchIndexResult struct {
	DocID      uint   // 文档 ID
	ChunkCount int    // 生成的分块数
	Success    bool   // 是否成功
	Error      string // 失败原因(Success=true 时为空)
}

// BatchIndexDocuments 批量入库多个文档(知识库初始化/批量导入场景)
// 相比循环调用 IndexDocument 的优化点:
//  1. 合并所有 chunks 一次性调用 Embedding API,显著降低 API 调用次数和延迟
//     (例如 10 文档 * 20 chunks = 200 chunks,单次调用 vs 10 次调用)
//  2. 批量写入向量到 VectorStore(单次 UpsertVectors 调用,利用批量 UPDATE)
//  3. 逐文档写入 chunks 元数据(保持事务边界清晰,单文档失败不影响其他)
//
// 注意:单次批量建议不超过 100 文档(避免 Embedding API 单次 token 限制)
// 超大规模导入应分批调用,每批 50-100 文档
func (r *RAGService) BatchIndexDocuments(ctx context.Context, docs []*models.KnowledgeDocument, cfg ChunkConfig) []BatchIndexResult {
	results := make([]BatchIndexResult, 0, len(docs))
	if len(docs) == 0 {
		return results
	}

	vecAvailable := r.vectorStore != nil && r.vectorStore.Available()

	// 1. 收集所有文档的分块(保留 doc 索引映射)
	type docChunks struct {
		docIdx int
		chunks []string
	}
	allChunks := make([]string, 0, len(docs)*4)
	docChunkMap := make([]docChunks, 0, len(docs))
	for i, doc := range docs {
		chunks := ChunkText(doc.Content, cfg)
		docChunkMap = append(docChunkMap, docChunks{i, chunks})
		allChunks = append(allChunks, chunks...)
	}

	// 2. 一次性批量生成所有 chunks 的向量(降低 API 调用次数)
	var allEmbeddings [][]float64
	if vecAvailable && len(allChunks) > 0 {
		emb, err := r.embedder.Embed(ctx, allChunks)
		if err != nil {
			logger.Get().Warnf("batch embed failed, storing without vectors: %v", err)
		} else {
			allEmbeddings = emb
		}
	}

	// 3. 逐文档写入 chunks 元数据 + 收集向量记录
	embeddingOffset := 0
	allRecords := make([]VectorRecord, 0, len(allChunks))
	for _, dc := range docChunkMap {
		doc := docs[dc.docIdx]
		// 清理旧分块
		if r.pgDB != nil {
			r.pgDB.Where("knowledge_doc_id = ?", doc.ID).Delete(&models.KnowledgeChunk{})
		}

		// 写入新分块
		for i, content := range dc.chunks {
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
					logger.Get().Warnf("batch create chunk doc=%d idx=%d failed: %v", doc.ID, i, err)
					continue
				}
			} else if r.mysqlDB != nil {
				if err := r.mysqlDB.Create(&chunk).Error; err != nil {
					logger.Get().Warnf("batch create chunk (mysql) doc=%d idx=%d failed: %v", doc.ID, i, err)
					continue
				}
			}

			// 收集向量记录(对应 allChunks 中的位置)
			if vecAvailable && embeddingOffset < len(allEmbeddings) && len(allEmbeddings[embeddingOffset]) > 0 {
				allRecords = append(allRecords, VectorRecord{
					ChunkID:    chunk.ID,
					DocID:      doc.ID,
					KBID:       doc.KnowledgeBaseID,
					ChunkIndex: i,
					Content:    content,
					Embedding:  allEmbeddings[embeddingOffset],
				})
			}
			embeddingOffset++
		}

		// 更新文档状态
		doc.Status = "ready"
		doc.ChunkCount = len(dc.chunks)
		if r.mysqlDB != nil {
			if err := r.mysqlDB.Save(doc).Error; err != nil {
				logger.Get().Warnf("batch update doc %d status failed: %v", doc.ID, err)
			}
		}

		middleware.RAGIndexDocsTotal.WithLabelValues("success").Inc()
		middleware.RAGIndexChunks.Observe(float64(len(dc.chunks)))

		results = append(results, BatchIndexResult{
			DocID:      doc.ID,
			ChunkCount: len(dc.chunks),
			Success:    true,
		})
	}

	// 4. 批量写入所有向量到 VectorStore(单次调用,利用 PgVectorStore 批量 UPDATE)
	// InMemoryVectorStore 的 UpsertVectors 按 DocID 幂等覆盖,批量写入安全
	if vecAvailable && len(allRecords) > 0 {
		if err := r.vectorStore.UpsertVectors(ctx, allRecords); err != nil {
			logger.Get().Warnf("batch vector upsert failed: %v", err)
		}
	}

	logger.Get().Infof("RAG batch indexed %d docs: %d chunks, %d embeddings (store=%s)",
		len(docs), len(allChunks), len(allRecords), r.VectorStoreName())
	return results
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
