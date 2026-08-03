package ai

import (
	"context"
	"os"
	"testing"
	"time"
)

// ============== Milvus 集成测试(依赖真实 Milvus 实例) ==============
//
// 运行条件: MILVUS_TEST_ADDRESS 环境变量指定 Milvus REST API 地址
// 例如: MILVUS_TEST_ADDRESS=http://localhost:9091 go test -v -run TestMilvusIntegration ./internal/application/ai/
//
// 未设置环境变量时跳过,不影响常规测试

func milvusTestAddress() string {
	return os.Getenv("MILVUS_TEST_ADDRESS")
}

func TestMilvusIntegration_Available(t *testing.T) {
	addr := milvusTestAddress()
	if addr == "" {
		t.Skip("MILVUS_TEST_ADDRESS not set, skipping integration test")
	}
	s := NewMilvusStore(MilvusConfig{
		Address: addr,
		Timeout: 5 * time.Second,
	})
	if !s.Available() {
		t.Fatal("expected milvus to be available at", addr)
	}
	t.Logf("milvus at %s is available", addr)
}

func TestMilvusIntegration_Search_EmptyCollection(t *testing.T) {
	addr := milvusTestAddress()
	if addr == "" {
		t.Skip("MILVUS_TEST_ADDRESS not set, skipping integration test")
	}
	s := NewMilvusStore(MilvusConfig{
		Address:    addr,
		Collection: "cb_test_integration",
		Timeout:    5 * time.Second,
	})
	if !s.Available() {
		t.Skip("milvus not available")
	}
	// 搜索不存在的 collection,应返回空结果或错误(不 panic)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := s.Search(ctx, []float64{0.1, 0.2, 0.3}, 999, 5)
	if err != nil {
		t.Logf("search on non-existent collection returned error (expected): %v", err)
	}
	if len(results) > 0 {
		t.Logf("search returned %d results on non-existent collection", len(results))
	}
	t.Log("milvus integration search test completed without panic")
}
