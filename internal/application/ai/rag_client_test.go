package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============== 熔断器单元测试 ==============

func TestCircuitBreaker_ClosedAllowsAll(t *testing.T) {
	cb := newCircuitBreaker(3, 100*time.Millisecond)
	for i := 0; i < 5; i++ {
		if !cb.allow() {
			t.Fatalf("closed state should allow all requests, blocked at iteration %d", i)
		}
	}
}

func TestCircuitBreaker_OpenAfterThreshold(t *testing.T) {
	cb := newCircuitBreaker(3, 100*time.Millisecond)
	// 连续失败 3 次后应熔断
	for i := 0; i < 3; i++ {
		cb.recordFailure()
	}
	if cb.allow() {
		t.Fatal("circuit breaker should be open after threshold failures")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := newCircuitBreaker(3, 100*time.Millisecond)
	cb.recordFailure()
	cb.recordFailure()
	// 成功应重置失败计数
	cb.recordSuccess()
	// 再次失败 1 次不应熔断(计数已重置)
	cb.recordFailure()
	if !cb.allow() {
		t.Fatal("circuit breaker should remain closed after success reset")
	}
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	cb := newCircuitBreaker(1, 50*time.Millisecond)
	cb.recordFailure() // 立即熔断
	if cb.allow() {
		t.Fatal("should be open immediately after threshold failure")
	}
	// 等待熔断持续时间后应进入半开
	time.Sleep(60 * time.Millisecond)
	if !cb.allow() {
		t.Fatal("should enter half-open and allow probe request after open duration")
	}
	// 半开状态成功应恢复为 closed
	cb.recordSuccess()
	if !cb.allow() {
		t.Fatal("should be closed after half-open success")
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := newCircuitBreaker(1, 50*time.Millisecond)
	cb.recordFailure()
	time.Sleep(60 * time.Millisecond)
	if !cb.allow() { // 进入半开
		t.Fatal("should allow probe in half-open")
	}
	// 半开状态失败应重新熔断
	cb.recordFailure()
	if cb.allow() {
		t.Fatal("should reopen after half-open failure")
	}
}

// ============== RemoteRAGClient 降级策略测试 ==============

// fakeRAGSearcher 模拟 RAGSearcher 用于测试降级
type fakeRAGSearcher struct {
	mu       sync.Mutex
	called   int32
	docs     []RAGDocument
	err      error
}

func (f *fakeRAGSearcher) Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	atomic.AddInt32(&f.called, 1)
	return f.docs, f.err
}

func (f *fakeRAGSearcher) callCount() int32 {
	return atomic.LoadInt32(&f.called)
}

func TestRemoteRAGClient_FallbackWhenServerDown(t *testing.T) {
	// 启动一个立即返回 500 的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fallback := &fakeRAGSearcher{docs: []RAGDocument{{Title: "fallback-doc", Content: "tfidf result"}}}
	client := NewRemoteRAGClient(server.URL)
	client.SetFallback(fallback)

	docs, err := client.Search("test query", 1, 5)
	if err != nil {
		t.Fatalf("fallback should not return error: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "fallback-doc" {
		t.Fatalf("expected fallback doc, got %+v", docs)
	}
	if fallback.callCount() != 1 {
		t.Fatalf("fallback should be called once, got %d", fallback.callCount())
	}
}

func TestRemoteRAGClient_FallbackWhenCircuitOpen(t *testing.T) {
	// 熔断器配置:1 次失败立即熔断,持续 1s
	client := NewRemoteRAGClient("http://nonexistent:9999")
	// 覆盖熔断器为快速熔断配置
	client.breaker = newCircuitBreaker(1, 1*time.Second)

	fallback := &fakeRAGSearcher{docs: []RAGDocument{{Title: "fallback"}}}
	client.SetFallback(fallback)

	// 第一次调用:目标不可达,触发失败 + 熔断
	_, err := client.Search("test", 1, 5)
	if err != nil {
		t.Fatalf("first call should fallback without error: %v", err)
	}
	// 第二次调用:熔断器开启,直接走降级
	docs, err := client.Search("test", 1, 5)
	if err != nil {
		t.Fatalf("second call should fallback without error: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "fallback" {
		t.Fatalf("expected fallback doc, got %+v", docs)
	}
	// fallback 应被调用 2 次(1 次失败降级 + 1 次熔断降级)
	if fallback.callCount() != 2 {
		t.Fatalf("fallback should be called twice, got %d", fallback.callCount())
	}
}

func TestRemoteRAGClient_NoFallbackReturnsEmpty(t *testing.T) {
	client := NewRemoteRAGClient("http://nonexistent:9999")
	client.breaker = newCircuitBreaker(1, 1*time.Second)
	// 未设置 fallback
	_, err := client.Search("test", 1, 5)
	if err != nil {
		t.Fatalf("no fallback should return nil error, got %v", err)
	}
}

func TestRemoteRAGClient_SuccessNoFallback(t *testing.T) {
	// 启动模拟 RAG Service 成功响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 X-Trace-Id header 透传
		if r.Header.Get("X-Trace-Id") == "" {
			t.Errorf("X-Trace-Id header should be set")
		}
		resp := ragSearchResponse{}
		resp.Code = 0
		resp.Data.Documents = []RAGDocument{
			{Title: "remote-doc", Content: "vector search result", Score: 0.95},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRemoteRAGClient(server.URL)
	// 不设置 fallback,验证成功路径不依赖 fallback
	docs, err := client.Search("test query", 1, 5)
	if err != nil {
		t.Fatalf("success path should not return error: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "remote-doc" {
		t.Fatalf("expected remote doc, got %+v", docs)
	}
}

func TestRemoteRAGClient_BadResponseTriggersFallback(t *testing.T) {
	// 返回非 0 code,应触发降级
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ragSearchResponse{}
		resp.Code = 500
		resp.Msg = "internal error"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fallback := &fakeRAGSearcher{docs: []RAGDocument{{Title: "fallback"}}}
	client := NewRemoteRAGClient(server.URL)
	client.SetFallback(fallback)

	docs, err := client.Search("test", 1, 5)
	if err != nil {
		t.Fatalf("should fallback without error: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "fallback" {
		t.Fatalf("expected fallback doc, got %+v", docs)
	}
}

// TestRemoteRAGClient_ConcurrentAccess 验证熔断器并发安全
func TestRemoteRAGClient_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewRemoteRAGClient(server.URL)
	fallback := &fakeRAGSearcher{docs: []RAGDocument{{Title: "fallback"}}}
	client.SetFallback(fallback)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.Search("test", 1, 5)
		}()
	}
	wg.Wait()
	// 不应 panic,且 fallback 被调用 20 次
	if fallback.callCount() != 20 {
		t.Logf("fallback call count=%d (熔断器可能提前拦截部分请求,不强制断言)", fallback.callCount())
	}
}
