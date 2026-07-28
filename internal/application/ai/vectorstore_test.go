package ai

import (
	"context"
	"math"
	"testing"

	"github.com/cb-platform/internal/pkg/config"
)

// ============== InMemoryVectorStore 测试 ==============

func TestInMemoryVectorStore_Name(t *testing.T) {
	s := NewInMemoryVectorStore()
	if s.Name() != "memory" {
		t.Errorf("expected memory, got %s", s.Name())
	}
}

func TestInMemoryVectorStore_Available(t *testing.T) {
	s := NewInMemoryVectorStore()
	if !s.Available() {
		t.Error("InMemoryVectorStore should always be available")
	}
}

func TestInMemoryVectorStore_EmptySearch(t *testing.T) {
	s := NewInMemoryVectorStore()
	results, err := s.Search(context.Background(), []float64{0.1, 0.2}, 0, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty store, got %d", len(results))
	}
}

func TestInMemoryVectorStore_Search_EmptyQuery(t *testing.T) {
	s := NewInMemoryVectorStore()
	_, err := s.Search(context.Background(), nil, 0, 5)
	if err == nil {
		t.Error("expected error for empty query vector")
	}
}

func TestInMemoryVectorStore_UpsertAndSearch(t *testing.T) {
	s := NewInMemoryVectorStore()
	// 写入 3 条向量(维度 2)
	records := []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, ChunkIndex: 0, Content: "doc-a", Embedding: []float64{1.0, 0.0}},
		{ChunkID: 2, DocID: 10, KBID: 1, ChunkIndex: 1, Content: "doc-b", Embedding: []float64{0.0, 1.0}},
		{ChunkID: 3, DocID: 11, KBID: 2, ChunkIndex: 0, Content: "doc-c", Embedding: []float64{0.7, 0.7}},
	}
	if err := s.UpsertVectors(context.Background(), records); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	// 查询向量 [1,0] 应最匹配 chunk 1
	results, err := s.Search(context.Background(), []float64{1.0, 0.0}, 0, 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].ChunkID != 1 {
		t.Errorf("expected chunk 1 first, got %d", results[0].ChunkID)
	}
	if results[0].Score < 0.99 {
		t.Errorf("expected score ~1.0, got %f", results[0].Score)
	}
}

func TestInMemoryVectorStore_Search_TopK(t *testing.T) {
	s := NewInMemoryVectorStore()
	records := []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, Embedding: []float64{1.0, 0.0}},
		{ChunkID: 2, DocID: 10, KBID: 1, Embedding: []float64{0.9, 0.1}},
		{ChunkID: 3, DocID: 10, KBID: 1, Embedding: []float64{0.5, 0.5}},
	}
	s.UpsertVectors(context.Background(), records)

	results, _ := s.Search(context.Background(), []float64{1.0, 0.0}, 0, 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results for topK=2, got %d", len(results))
	}
}

func TestInMemoryVectorStore_Search_FilterByKB(t *testing.T) {
	s := NewInMemoryVectorStore()
	records := []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, Embedding: []float64{1.0, 0.0}},
		{ChunkID: 2, DocID: 11, KBID: 2, Embedding: []float64{1.0, 0.0}},
		{ChunkID: 3, DocID: 12, KBID: 2, Embedding: []float64{0.9, 0.1}},
	}
	s.UpsertVectors(context.Background(), records)

	// kbID=2 应只返回 KB 2 的 2 条
	results, _ := s.Search(context.Background(), []float64{1.0, 0.0}, 2, 5)
	if len(results) != 2 {
		t.Errorf("expected 2 results for kbID=2, got %d", len(results))
	}
	for _, r := range results {
		if r.DocID != 11 && r.DocID != 12 {
			t.Errorf("unexpected docID %d for kbID=2 filter", r.DocID)
		}
	}
}

func TestInMemoryVectorStore_Upsert_Idempotent(t *testing.T) {
	s := NewInMemoryVectorStore()
	// 首次写入 doc 10 的 2 条向量
	s.UpsertVectors(context.Background(), []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, Embedding: []float64{1.0, 0.0}},
		{ChunkID: 2, DocID: 10, KBID: 1, Embedding: []float64{0.0, 1.0}},
	})
	// 再次写入 doc 10 的 1 条向量(应覆盖,先删后插)
	s.UpsertVectors(context.Background(), []VectorRecord{
		{ChunkID: 3, DocID: 10, KBID: 1, Embedding: []float64{0.5, 0.5}},
	})
	results, _ := s.Search(context.Background(), []float64{1.0, 0.0}, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result after idempotent upsert, got %d", len(results))
	}
	if results[0].ChunkID != 3 {
		t.Errorf("expected chunk 3, got %d", results[0].ChunkID)
	}
}

func TestInMemoryVectorStore_DeleteByDoc(t *testing.T) {
	s := NewInMemoryVectorStore()
	s.UpsertVectors(context.Background(), []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, Embedding: []float64{1.0, 0.0}},
		{ChunkID: 2, DocID: 11, KBID: 1, Embedding: []float64{1.0, 0.0}},
	})
	if err := s.DeleteByDoc(context.Background(), 10); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	results, _ := s.Search(context.Background(), []float64{1.0, 0.0}, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result after delete, got %d", len(results))
	}
	if results[0].DocID != 11 {
		t.Errorf("expected doc 11 remaining, got %d", results[0].DocID)
	}
}

func TestInMemoryVectorStore_DeleteByKB(t *testing.T) {
	s := NewInMemoryVectorStore()
	s.UpsertVectors(context.Background(), []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, Embedding: []float64{1.0, 0.0}},
		{ChunkID: 2, DocID: 11, KBID: 2, Embedding: []float64{1.0, 0.0}},
	})
	if err := s.DeleteByKB(context.Background(), 1); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	results, _ := s.Search(context.Background(), []float64{1.0, 0.0}, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result after delete, got %d", len(results))
	}
	// kb=1 已删除,剩余的应来自 doc 11(属于 kb=2)
	if results[0].DocID != 11 {
		t.Errorf("expected doc 11 remaining, got %d", results[0].DocID)
	}
}

func TestInMemoryVectorStore_DimensionMismatch(t *testing.T) {
	s := NewInMemoryVectorStore()
	s.UpsertVectors(context.Background(), []VectorRecord{
		{ChunkID: 1, DocID: 10, KBID: 1, Embedding: []float64{1.0, 0.0, 0.0}}, // 3 维
	})
	// 用 2 维查询向量,应跳过维度不匹配的记录
	results, _ := s.Search(context.Background(), []float64{1.0, 0.0}, 0, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results for dimension mismatch, got %d", len(results))
	}
}

func TestInMemoryVectorStore_UpsertEmpty(t *testing.T) {
	s := NewInMemoryVectorStore()
	// 空记录不应 panic
	if err := s.UpsertVectors(context.Background(), nil); err != nil {
		t.Errorf("unexpected error for empty upsert: %v", err)
	}
}

// ============== PgVectorStore 基本测试(不依赖真实 DB) ==============

func TestPgVectorStore_Name(t *testing.T) {
	s := NewPgVectorStore(nil)
	if s.Name() != "pgvector" {
		t.Errorf("expected pgvector, got %s", s.Name())
	}
}

func TestPgVectorStore_Available_NilDB(t *testing.T) {
	s := NewPgVectorStore(nil)
	if s.Available() {
		t.Error("expected unavailable when db is nil")
	}
}

func TestPgVectorStore_UpsertEmpty(t *testing.T) {
	s := NewPgVectorStore(nil)
	// 空记录在 nil db 下也不应 error(直接返回 nil)
	if err := s.UpsertVectors(context.Background(), nil); err != nil {
		t.Errorf("unexpected error for empty upsert: %v", err)
	}
}

// ============== cosineSimilarity 测试 ==============

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	a := []float64{1.0, 0.5, 0.3}
	score := cosineSimilarity(a, a)
	if math.Abs(score-1.0) > 0.0001 {
		t.Errorf("expected 1.0 for identical vectors, got %f", score)
	}
}

func TestCosineSimilarity_OrthogonalVectors(t *testing.T) {
	a := []float64{1.0, 0.0}
	b := []float64{0.0, 1.0}
	score := cosineSimilarity(a, b)
	if math.Abs(score) > 0.0001 {
		t.Errorf("expected 0 for orthogonal vectors, got %f", score)
	}
}

func TestCosineSimilarity_DimensionMismatch(t *testing.T) {
	a := []float64{1.0, 0.0}
	b := []float64{1.0, 0.0, 0.0}
	if cosineSimilarity(a, b) != 0 {
		t.Error("expected 0 for dimension mismatch")
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	if cosineSimilarity(nil, nil) != 0 {
		t.Error("expected 0 for empty vectors")
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	// 零向量应返回 0(避免除以 0)
	a := []float64{0.0, 0.0}
	b := []float64{1.0, 0.0}
	if cosineSimilarity(a, b) != 0 {
		t.Error("expected 0 for zero vector")
	}
}

// ============== RAGService + VectorStore 集成测试 ==============

func TestRAGService_SetVectorStore(t *testing.T) {
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	// 默认无 VectorStore
	if svc.VectorStoreName() != "none" {
		t.Errorf("expected none, got %s", svc.VectorStoreName())
	}
	// 注入 InMemoryVectorStore
	svc.SetVectorStore(NewInMemoryVectorStore())
	if svc.VectorStoreName() != "memory" {
		t.Errorf("expected memory, got %s", svc.VectorStoreName())
	}
}

func TestRAGService_SetVectorStore_Nil(t *testing.T) {
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	svc.SetVectorStore(nil)
	if svc.VectorStoreName() != "none" {
		t.Errorf("expected none after nil set, got %s", svc.VectorStoreName())
	}
}

func TestRAGService_VectorSearch_NoStore(t *testing.T) {
	// 无 VectorStore 时 vectorSearch 应返回 error
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	_, err := svc.vectorSearch(context.Background(), "test", 1, 5)
	if err == nil {
		t.Error("expected error when vector store is nil")
	}
}

// ============== VectorStore 接口契约测试 ==============

// TestVectorStoreContract 验证所有 VectorStore 实现都满足接口契约
// 新增实现(如 MilvusStore)时,将其加入 testCases 即可复用测试
func TestVectorStoreContract(t *testing.T) {
	testCases := []struct {
		name string
		store VectorStore
	}{
		{"InMemory", NewInMemoryVectorStore()},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.store

			// 1. 初始状态 Available
			if !store.Available() {
				t.Fatal("store should be available")
			}

			// 2. 空检索返回空结果
			results, err := store.Search(context.Background(), []float64{1.0, 0.0}, 0, 5)
			if err != nil {
				t.Fatalf("empty search failed: %v", err)
			}
			if len(results) != 0 {
				t.Errorf("expected 0 results, got %d", len(results))
			}

			// 3. 写入向量后可检索
			records := []VectorRecord{
				{ChunkID: 1, DocID: 10, KBID: 1, ChunkIndex: 0, Content: "hello", Embedding: []float64{1.0, 0.0}},
				{ChunkID: 2, DocID: 10, KBID: 1, ChunkIndex: 1, Content: "world", Embedding: []float64{0.0, 1.0}},
			}
			if err := store.UpsertVectors(context.Background(), records); err != nil {
				t.Fatalf("upsert failed: %v", err)
			}
			results, err = store.Search(context.Background(), []float64{1.0, 0.0}, 0, 5)
			if err != nil {
				t.Fatalf("search after upsert failed: %v", err)
			}
			if len(results) != 2 {
				t.Errorf("expected 2 results, got %d", len(results))
			}

			// 4. DeleteByDoc 后该文档向量消失
			if err := store.DeleteByDoc(context.Background(), 10); err != nil {
				t.Fatalf("delete failed: %v", err)
			}
			results, _ = store.Search(context.Background(), []float64{1.0, 0.0}, 0, 5)
			if len(results) != 0 {
				t.Errorf("expected 0 results after delete, got %d", len(results))
			}
		})
	}
}
