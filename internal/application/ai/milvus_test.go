package ai

import (
	"context"
	"testing"
)

// ============== MilvusStore 测试(不依赖真实 Milvus 实例) ==============

func TestMilvusStore_Name(t *testing.T) {
	s := NewMilvusStore(MilvusConfig{Address: "http://localhost:19530"})
	if s.Name() != "milvus" {
		t.Errorf("expected milvus, got %s", s.Name())
	}
}

func TestMilvusStore_Available_EmptyAddress(t *testing.T) {
	s := NewMilvusStore(MilvusConfig{})
	if s.Available() {
		t.Error("expected unavailable when address is empty")
	}
}

func TestMilvusStore_Available_Unreachable(t *testing.T) {
	// 使用不可达地址,Available 应返回 false 且不 panic
	s := NewMilvusStore(MilvusConfig{Address: "http://127.0.0.1:1"})
	if s.Available() {
		t.Error("expected unavailable for unreachable address")
	}
}

func TestMilvusStore_CollectionName(t *testing.T) {
	s := NewMilvusStore(MilvusConfig{Address: "http://localhost:19530", Collection: "cb_kb"})
	// kbID=0 使用默认 Collection
	if name := s.collectionName(0); name != "cb_kb" {
		t.Errorf("expected cb_kb, got %s", name)
	}
	// kbID>0 拼接 ID
	if name := s.collectionName(5); name != "cb_kb_5" {
		t.Errorf("expected cb_kb_5, got %s", name)
	}
}

func TestMilvusStore_CollectionName_DefaultPrefix(t *testing.T) {
	s := NewMilvusStore(MilvusConfig{Address: "http://localhost:19530"})
	if name := s.collectionName(3); name != "cb_knowledge_3" {
		t.Errorf("expected cb_knowledge_3, got %s", name)
	}
}

func TestMilvusStore_UpsertEmpty(t *testing.T) {
	s := NewMilvusStore(MilvusConfig{Address: "http://127.0.0.1:1"})
	// 空记录不应 error
	if err := s.UpsertVectors(context.Background(), nil); err != nil {
		t.Errorf("unexpected error for empty upsert: %v", err)
	}
}

func TestMilvusStore_Search_EmptyQuery(t *testing.T) {
	s := NewMilvusStore(MilvusConfig{Address: "http://127.0.0.1:1"})
	_, err := s.Search(context.Background(), nil, 0, 5)
	if err == nil {
		t.Error("expected error for empty query vector")
	}
}

// ============== VectorStore 工厂函数测试 ==============

func TestNewVectorStore_InMemory(t *testing.T) {
	s := NewVectorStore(VectorStoreInMemory, nil, MilvusConfig{})
	if s == nil {
		t.Fatal("expected non-nil InMemoryVectorStore")
	}
	if s.Name() != "memory" {
		t.Errorf("expected memory, got %s", s.Name())
	}
}

func TestNewVectorStore_Milvus_EmptyAddress(t *testing.T) {
	// 空地址应返回 nil
	s := NewVectorStore(VectorStoreMilvus, nil, MilvusConfig{})
	if s != nil {
		t.Error("expected nil for milvus with empty address")
	}
}

func TestNewVectorStore_Milvus_ValidAddress(t *testing.T) {
	s := NewVectorStore(VectorStoreMilvus, nil, MilvusConfig{Address: "http://localhost:19530"})
	if s == nil {
		t.Fatal("expected non-nil MilvusStore")
	}
	if s.Name() != "milvus" {
		t.Errorf("expected milvus, got %s", s.Name())
	}
}

func TestNewVectorStore_UnknownType(t *testing.T) {
	s := NewVectorStore("unknown", nil, MilvusConfig{})
	if s != nil {
		t.Error("expected nil for unknown type")
	}
}

// ============== VectorStore 全实现契约测试 ==============

// TestAllVectorStoresContract 验证所有可在无外部依赖下运行的 VectorStore 实现
// PgVectorStore 需要 PostgreSQL,不在此测试;MilvusStore 需要 Milvus,不在此测试
// InMemoryVectorStore 无外部依赖,完整覆盖
func TestAllVectorStoresContract(t *testing.T) {
	stores := []struct {
		name  string
		store VectorStore
	}{
		{"InMemory", NewInMemoryVectorStore()},
	}
	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.store
			if !store.Available() {
				t.Fatal("store should be available")
			}
			// 写入向量
			records := []VectorRecord{
				{ChunkID: 1, DocID: 10, KBID: 1, ChunkIndex: 0, Content: "test", Embedding: []float64{1.0, 0.0}},
			}
			if err := store.UpsertVectors(context.Background(), records); err != nil {
				t.Fatalf("upsert failed: %v", err)
			}
			// 检索
			results, err := store.Search(context.Background(), []float64{1.0, 0.0}, 0, 5)
			if err != nil {
				t.Fatalf("search failed: %v", err)
			}
			if len(results) != 1 {
				t.Errorf("expected 1 result, got %d", len(results))
			}
		})
	}
}
