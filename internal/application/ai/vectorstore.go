package ai

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/cb-platform/internal/pkg/logger"
	"gorm.io/gorm"
)

// VectorRecord 向量记录(VectorStore 写入和检索的最小单元)
// ChunkID 关联 knowledge_chunks 表主键,作为向量与元数据的桥梁
type VectorRecord struct {
	ChunkID    uint      // 对应 knowledge_chunks.id
	DocID      uint      // 对应 knowledge_documents.id
	KBID       uint      // 知识库 ID
	ChunkIndex int       // 分块索引
	Content    string    // 分块文本(供 InMemory 实现检索时返回)
	Embedding  []float64 // 向量
}

// VectorSearchResult 向量检索结果
type VectorSearchResult struct {
	ChunkID    uint
	DocID      uint
	ChunkIndex int
	Content    string
	Score      float64 // 余弦相似度 [0,1],越高越相似
}

// VectorStore 向量存储抽象接口
// 实现方负责向量的持久化和检索,与 RAGService 解耦
// 已知实现:
//   - PgVectorStore: PostgreSQL + pgvector(生产推荐)
//   - InMemoryVectorStore: 内存实现(测试和本地开发,无外部依赖)
//
// 扩展点: 未来可实现 MilvusStore / QdrantStore 作为可插拔替代方案
type VectorStore interface {
	// Name 返回存储类型标识(pgvector / memory / milvus ...)
	Name() string

	// Available 是否可用(连接已建立且配置完整)
	Available() bool

	// UpsertVectors 批量写入向量(同一 docID 下先清后写,保证幂等)
	// records 为空时仅执行清理
	UpsertVectors(ctx context.Context, records []VectorRecord) error

	// Search 向量检索,返回 topK 结果(按相似度降序)
	// kbID=0 表示跨知识库检索
	Search(ctx context.Context, queryVec []float64, kbID uint, topK int) ([]VectorSearchResult, error)

	// DeleteByDoc 清除指定文档的所有向量(不删除 chunk 元数据)
	DeleteByDoc(ctx context.Context, docID uint) error

	// DeleteByKB 清除指定知识库的所有向量
	DeleteByKB(ctx context.Context, kbID uint) error
}

// ============== PgVectorStore ==============

// PgVectorStore 基于 PostgreSQL + pgvector 的向量存储
// 直接操作 knowledge_chunks 表的 embedding 列(由 GORM AutoMigrate 创建)
// 使用 pgvector 的 <=> 运算符(余弦距离)进行 ANN 检索
type PgVectorStore struct {
	db *gorm.DB
}

// NewPgVectorStore 创建 pgvector 向量存储
// db 必须是已启用 pgvector 扩展的 PostgreSQL 连接
func NewPgVectorStore(db *gorm.DB) *PgVectorStore {
	return &PgVectorStore{db: db}
}

func (s *PgVectorStore) Name() string      { return "pgvector" }
func (s *PgVectorStore) Available() bool    { return s.db != nil }

// UpsertVectors 批量更新 embedding 列
// chunks 元数据由 RAGService 写入,这里只更新 embedding 列
// 使用 PostgreSQL 的 UPDATE ... FROM (VALUES ...) 语法批量更新,显著降低 RTT
// 对于 N 条记录:逐条 UPDATE 需 N 次 RTT,批量 UPDATE 仅需 1 次
func (s *PgVectorStore) UpsertVectors(ctx context.Context, records []VectorRecord) error {
	if len(records) == 0 {
		return nil
	}
	// 过滤掉无向量的记录
	type pair struct {
		chunkID uint
		vecStr  string
	}
	pairs := make([]pair, 0, len(records))
	for _, r := range records {
		if len(r.Embedding) == 0 {
			continue
		}
		pairs = append(pairs, pair{r.ChunkID, vectorToPgString(r.Embedding)})
	}
	if len(pairs) == 0 {
		return nil
	}

	// 构造批量 UPDATE SQL:
	//   UPDATE knowledge_chunks AS kc SET embedding = t.vec::vector
	//   FROM (VALUES (1::bigint, '[...]'::text), (2, '[...]')) AS t(id, vec)
	//   WHERE kc.id = t.id
	// 使用 text 中转再 cast::vector,避免 GORM 参数绑定对 vector 类型的限制
	var sb strings.Builder
	sb.WriteString("UPDATE knowledge_chunks AS kc SET embedding = t.vec::vector FROM (VALUES ")
	args := make([]interface{}, 0, len(pairs)*2)
	for i, p := range pairs {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(?::bigint, ?::text)")
		args = append(args, p.chunkID, p.vecStr)
	}
	sb.WriteString(") AS t(id, vec) WHERE kc.id = t.id")

	if err := s.db.WithContext(ctx).Exec(sb.String(), args...).Error; err != nil {
		// 批量失败时降级为逐条写入,保证部分成功
		logger.Get().Warnf("pgvector batch upsert failed, fallback to per-row: %v", err)
		for _, p := range pairs {
			if err := s.db.WithContext(ctx).Exec(
				"UPDATE knowledge_chunks SET embedding = ? WHERE id = ?",
				p.vecStr, p.chunkID,
			).Error; err != nil {
				logger.Get().Warnf("pgvector upsert chunk %d failed: %v", p.chunkID, err)
			}
		}
	}
	return nil
}

// Search 使用 pgvector 的 <=> 运算符进行余弦距离检索
// score = 1 - cosine_distance,归一化到 [0,1]
func (s *PgVectorStore) Search(ctx context.Context, queryVec []float64, kbID uint, topK int) ([]VectorSearchResult, error) {
	if len(queryVec) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}
	vecStr := vectorToPgString(queryVec)

	// JOIN knowledge_documents 过滤 status='ready' 的文档
	// (索引过程中 status=processing,不应被检索到)
	sql := `
		SELECT kc.id, kc.knowledge_doc_id, kc.chunk_index, kc.content,
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

	rows, err := s.db.WithContext(ctx).Raw(sql, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("pgvector search: %w", err)
	}
	defer rows.Close()

	results := make([]VectorSearchResult, 0, topK)
	for rows.Next() {
		var r VectorSearchResult
		if err := rows.Scan(&r.ChunkID, &r.DocID, &r.ChunkIndex, &r.Content, &r.Score); err != nil {
			continue
		}
		if r.Score < 0 {
			r.Score = 0
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// DeleteByDoc 清除指定文档所有分块的向量(embedding 置 NULL)
// 不删除 chunk 行,保留元数据供 TF-IDF 降级检索
func (s *PgVectorStore) DeleteByDoc(ctx context.Context, docID uint) error {
	return s.db.WithContext(ctx).Exec(
		"UPDATE knowledge_chunks SET embedding = NULL WHERE knowledge_doc_id = ?",
		docID,
	).Error
}

// DeleteByKB 清除指定知识库所有分块的向量
func (s *PgVectorStore) DeleteByKB(ctx context.Context, kbID uint) error {
	return s.db.WithContext(ctx).Exec(
		"UPDATE knowledge_chunks SET embedding = NULL WHERE knowledge_base_id = ?",
		kbID,
	).Error
}

// ============== InMemoryVectorStore ==============

// InMemoryVectorStore 内存向量存储(测试和本地开发用)
// 使用互斥锁保护切片,暴力计算余弦相似度(O(n*d)),适合小规模数据验证
// 不持久化,进程退出即丢失;适合单元测试和本地无 pgvector 环境的功能验证
type InMemoryVectorStore struct {
	mu      sync.RWMutex
	records []VectorRecord
}

// NewInMemoryVectorStore 创建内存向量存储
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{}
}

func (s *InMemoryVectorStore) Name() string   { return "memory" }
func (s *InMemoryVectorStore) Available() bool { return true }

// UpsertVectors 按 DocID 幂等写入(先删后插)
func (s *InMemoryVectorStore) UpsertVectors(ctx context.Context, records []VectorRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 收集需要删除的 docID 集合
	docIDs := make(map[uint]bool)
	for _, r := range records {
		docIDs[r.DocID] = true
	}
	// 过滤掉同 docID 的旧记录
	filtered := s.records[:0]
	for _, r := range s.records {
		if !docIDs[r.DocID] {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
	// 追加新记录
	s.records = append(s.records, records...)
	return nil
}

// Search 暴力遍历计算余弦相似度,按分数降序取 topK
func (s *InMemoryVectorStore) Search(ctx context.Context, queryVec []float64, kbID uint, topK int) ([]VectorSearchResult, error) {
	if len(queryVec) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		idx   int
		score float64
	}
	scoredList := make([]scored, 0, len(s.records))
	for i, r := range s.records {
		if kbID > 0 && r.KBID != kbID {
			continue
		}
		if len(r.Embedding) != len(queryVec) {
			continue
		}
		scoredList = append(scoredList, scored{i, cosineSimilarity(queryVec, r.Embedding)})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})
	if len(scoredList) > topK {
		scoredList = scoredList[:topK]
	}
	results := make([]VectorSearchResult, 0, len(scoredList))
	for _, sc := range scoredList {
		r := s.records[sc.idx]
		results = append(results, VectorSearchResult{
			ChunkID:    r.ChunkID,
			DocID:      r.DocID,
			ChunkIndex: r.ChunkIndex,
			Content:    r.Content,
			Score:      sc.score,
		})
	}
	return results, nil
}

// DeleteByDoc 删除指定文档的所有向量记录
func (s *InMemoryVectorStore) DeleteByDoc(ctx context.Context, docID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.records[:0]
	for _, r := range s.records {
		if r.DocID != docID {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
	return nil
}

// DeleteByKB 删除指定知识库的所有向量记录
func (s *InMemoryVectorStore) DeleteByKB(ctx context.Context, kbID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.records[:0]
	for _, r := range s.records {
		if r.KBID != kbID {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
	return nil
}

// ============== 工具函数 ==============

// cosineSimilarity 计算两个向量的余弦相似度
// 返回值范围 [-1, 1],RAG 场景下通常 [0, 1]
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
