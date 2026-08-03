package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============== RAG Service 多副本水平扩展测试 ==============
//
// 本测试模拟多副本 RAG Service 场景,验证:
//   1. 并发请求下 RemoteRAGClient 的线程安全性
//   2. 多副本负载均衡(请求分布到不同副本)
//   3. 单副本故障时熔断器降级到本地 TF-IDF
//   4. 高并发下熔断器状态机的正确性

// TestRemoteRAGClient_HighConcurrencySuccess 高并发成功场景
// 模拟多副本 RAG Service 接收 100 个并发请求,验证全部成功
func TestRemoteRAGClient_HighConcurrencySuccess(t *testing.T) {
	var requestCount int64

	// 模拟多副本 RAG Service(通过单 server 模拟,实际多副本由负载均衡器分发)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		// 模拟处理延迟(5-20ms)
		time.Sleep(time.Duration(5+requestCount%15) * time.Millisecond)
		resp := ragSearchResponse{}
		resp.Code = 0
		resp.Data.Documents = []RAGDocument{
			{Title: "doc-1", Content: "vector search result", Score: 0.92},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRemoteRAGClient(server.URL)
	// 高阈值熔断器,确保测试期间不触发熔断
	client.breaker = newCircuitBreaker(1000, 1*time.Second)

	const concurrency = 100
	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			docs, err := client.Search(fmt.Sprintf("query-%d", idx), 1, 5)
			if err != nil || len(docs) == 0 {
				atomic.AddInt64(&errorCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if errorCount > 0 {
		t.Errorf("expected 0 errors in high concurrency, got %d", errorCount)
	}
	if successCount != concurrency {
		t.Errorf("expected %d successes, got %d", concurrency, successCount)
	}
	t.Logf("并发 %d 请求:成功=%d,失败=%d,服务端接收=%d",
		concurrency, successCount, errorCount, atomic.LoadInt64(&requestCount))
}

// TestRemoteRAGClient_MultiReplicaLoadBalancing 多副本负载均衡验证
// 模拟 3 个 RAG Service 副本,验证请求是否均匀分布
func TestRemoteRAGClient_MultiReplicaLoadBalancing(t *testing.T) {
	// 启动 3 个模拟副本,各自统计请求数
	var replicaCounts [3]int64
	var replicas []*httptest.Server

	for i := 0; i < 3; i++ {
		idx := i
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&replicaCounts[idx], 1)
			resp := ragSearchResponse{}
			resp.Code = 0
			resp.Data.Documents = []RAGDocument{
				{Title: fmt.Sprintf("replica-%d-doc", idx), Score: 0.9},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		replicas = append(replicas, server)
	}
	defer func() {
		for _, s := range replicas {
			s.Close()
		}
	}()

	// 轮询策略:手动实现简单的负载均衡器
	var replicaIdx int64
	totalRequests := 90
	var wg sync.WaitGroup

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx := atomic.AddInt64(&replicaIdx, 1) % 3
			client := NewRemoteRAGClient(replicas[idx].URL)
			client.breaker = newCircuitBreaker(100, 1*time.Second)
			_, _ = client.Search("test", 1, 5)
		}()
	}
	wg.Wait()

	// 验证请求分布(允许 ±20% 偏差)
	expected := totalRequests / 3
	tolerance := expected / 5 // 20%
	for i := range replicaCounts {
		count := atomic.LoadInt64(&replicaCounts[i])
		deviation := count - int64(expected)
		if deviation < 0 {
			deviation = -deviation
		}
		t.Logf("副本 %d: 处理 %d 请求(期望 %d,偏差 %d)", i, count, expected, deviation)
		if deviation > int64(tolerance) {
			t.Errorf("副本 %d 请求分布偏差过大: %d (容忍 %d)", i, deviation, tolerance)
		}
	}
}

// TestRemoteRAGClient_ReplicaFailureTriggersFallback 单副本故障降级测试
// 模拟部分副本故障,验证熔断器正确触发降级
func TestRemoteRAGClient_ReplicaFailureTriggersFallback(t *testing.T) {
	// 副本 0:正常
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ragSearchResponse{}
		resp.Code = 0
		resp.Data.Documents = []RAGDocument{{Title: "healthy-replica", Score: 0.95}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer healthyServer.Close()

	// 副本 1:故障(始终返回 503)
	failedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failedServer.Close()

	// 测试故障副本的熔断器降级
	failedClient := NewRemoteRAGClient(failedServer.URL)
	failedClient.breaker = newCircuitBreaker(3, 1*time.Second) // 3 次失败后熔断

	fallback := &fakeRAGSearcher{docs: []RAGDocument{{Title: "tfidf-fallback"}}}
	failedClient.SetFallback(fallback)

	// 连续请求故障副本,验证降级
	for i := 0; i < 5; i++ {
		docs, err := failedClient.Search("test", 1, 5)
		if err != nil {
			t.Fatalf("请求 %d 应降级而非返回错误: %v", i, err)
		}
		if len(docs) != 1 || docs[0].Title != "tfidf-fallback" {
			t.Fatalf("请求 %d 应返回降级结果,got %+v", i, docs)
		}
	}

	// 验证健康副本正常工作
	healthyClient := NewRemoteRAGClient(healthyServer.URL)
	docs, err := healthyClient.Search("test", 1, 5)
	if err != nil {
		t.Fatalf("健康副本不应返回错误: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "healthy-replica" {
		t.Fatalf("健康副本应返回正常结果,got %+v", docs)
	}

	t.Logf("故障副本降级验证通过:5 次请求全部降级到 TF-IDF")
	t.Logf("健康副本验证通过:返回正常向量检索结果")
}

// TestRemoteRAGClient_CircuitBreakerRecovery 多副本熔断恢复测试
// 验证熔断器在副本恢复后能自动恢复
func TestRemoteRAGClient_CircuitBreakerRecovery(t *testing.T) {
	// 模拟可恢复的副本:先失败后恢复
	var shouldFail int32 = 1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&shouldFail) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resp := ragSearchResponse{}
		resp.Code = 0
		resp.Data.Documents = []RAGDocument{{Title: "recovered", Score: 0.88}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRemoteRAGClient(server.URL)
	client.breaker = newCircuitBreaker(2, 200*time.Millisecond) // 2 次失败熔断,200ms 后半开

	fallback := &fakeRAGSearcher{docs: []RAGDocument{{Title: "fallback"}}}
	client.SetFallback(fallback)

	// 阶段 1:副本故障,触发熔断
	for i := 0; i < 3; i++ {
		_, _ = client.Search("test", 1, 5)
	}
	if fallback.callCount() < 3 {
		t.Fatalf("阶段 1:应全部降级,期望 fallback 调用 >=3,got %d", fallback.callCount())
	}
	t.Logf("阶段 1:副本故障,熔断器开启,3 次请求降级到 TF-IDF")

	// 阶段 2:等待熔断器进入半开状态
	time.Sleep(250 * time.Millisecond)

	// 阶段 3:副本恢复,熔断器半开探测成功后恢复
	atomic.StoreInt32(&shouldFail, 0)

	// 半开探测请求应成功
	docs, err := client.Search("test", 1, 5)
	if err != nil {
		t.Fatalf("阶段 3:半开探测应成功,got error: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "recovered" {
		t.Fatalf("阶段 3:应返回恢复后的结果,got %+v", docs)
	}
	t.Logf("阶段 3:副本恢复,熔断器半开探测成功,恢复正常调用")

	// 阶段 4:后续请求应直接命中副本(不再降级)
	prevFallbackCount := fallback.callCount()
	for i := 0; i < 5; i++ {
		_, _ = client.Search("test", 1, 5)
	}
	if fallback.callCount() != prevFallbackCount {
		t.Fatalf("阶段 4:恢复后不应再降级,fallback 调用数不应增加: %d -> %d",
			prevFallbackCount, fallback.callCount())
	}
	t.Logf("阶段 4:熔断器完全恢复,5 次请求直接命中副本")
}
