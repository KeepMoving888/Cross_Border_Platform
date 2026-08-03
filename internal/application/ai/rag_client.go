package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cb-platform/internal/pkg/logger"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// ============== 熔断器(轻量级实现,避免引入外部依赖) ==============

// circuitBreaker 轻量级熔断器
// 连续失败达到阈值后进入"熔断"状态,拒绝请求一段时间
// 恢复后进入"半开"状态,放行一次试探请求;成功则关闭熔断,失败则重新熔断
type circuitBreaker struct {
	failureThreshold uint32        // 连续失败阈值(达到后熔断)
	openDuration     time.Duration // 熔断持续时间
	failures         uint32        // 当前连续失败次数
	state            uint32        // 0=closed, 1=open, 2=half-open
	openedAt         int64         // 熔断开始时间(Unix 纳秒)
	mu               sync.Mutex
}

const (
	cbClosed   uint32 = 0
	cbOpen     uint32 = 1
	cbHalfOpen uint32 = 2
)

// newCircuitBreaker 创建熔断器
func newCircuitBreaker(failureThreshold uint32, openDuration time.Duration) *circuitBreaker {
	return &circuitBreaker{
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
	}
}

// allow 判断是否放行请求
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch atomic.LoadUint32(&cb.state) {
	case cbClosed:
		return true
	case cbOpen:
		// 检查是否到了恢复时间
		openedAt := time.Unix(0, atomic.LoadInt64(&cb.openedAt))
		if time.Since(openedAt) >= cb.openDuration {
			atomic.StoreUint32(&cb.state, cbHalfOpen)
			logger.Get().Info("circuit breaker: open → half-open, allowing probe request")
			return true
		}
		return false
	case cbHalfOpen:
		return true // 半开状态放行试探请求
	default:
		return true
	}
}

// recordSuccess 记录成功
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	atomic.StoreUint32(&cb.failures, 0)
	if atomic.LoadUint32(&cb.state) != cbClosed {
		logger.Get().Info("circuit breaker: half-open → closed, recovered")
		atomic.StoreUint32(&cb.state, cbClosed)
	}
}

// recordFailure 记录失败
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	failures := atomic.AddUint32(&cb.failures, 1)
	state := atomic.LoadUint32(&cb.state)

	if state == cbHalfOpen {
		// 半开状态失败,重新熔断
		atomic.StoreUint32(&cb.state, cbOpen)
		atomic.StoreInt64(&cb.openedAt, time.Now().UnixNano())
		logger.Get().Warnf("circuit breaker: half-open → open (probe failed)")
		return
	}

	if failures >= cb.failureThreshold {
		atomic.StoreUint32(&cb.state, cbOpen)
		atomic.StoreInt64(&cb.openedAt, time.Now().UnixNano())
		logger.Get().Warnf("circuit breaker: closed → open (failures=%d, threshold=%d)",
			failures, cb.failureThreshold)
	}
}

// ============== RemoteRAGClient ==============

// RemoteRAGClient 远程 RAG 检索客户端(微服务模式)
// AI Service 通过 HTTP 调用 RAG Service 的 /api/v1/ai/rag/search 接口
// 替代进程内调用 RAGService.Search(),实现服务解耦
//
// 容错策略:
//  1. 熔断器:连续失败 5 次后熔断 30s,期间直接降级不走网络
//  2. 降级:RAG Service 不可用时,降级到本地 RAGService(TF-IDF 检索)
//
// 链路追踪:每次调用生成 X-Trace-Id header,RAG Service 的 TraceID 中间件会复用该 ID
// 实现 Gateway → AI Service → RAG Service 全链路 TraceID 透传
type RemoteRAGClient struct {
	baseURL    string          // RAG Service 地址,如 http://cb-rag-svc:8082
	httpClient *http.Client    // HTTP 客户端(带超时)
	authToken  string          // 内部调用 JWT(可选,微服务内部信任可留空)
	breaker    *circuitBreaker // 熔断器
	fallback   RAGSearcher     // 降级检索器(本地 RAGService,可选)
}

// NewRemoteRAGClient 创建远程 RAG 客户端
// baseURL: RAG Service 地址(如 http://cb-rag-svc:8082)
func NewRemoteRAGClient(baseURL string) *RemoteRAGClient {
	return &RemoteRAGClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second, // RAG 检索超时 10s
		},
		breaker: newCircuitBreaker(5, 30*time.Second), // 连续失败 5 次熔断 30s
	}
}

// SetFallback 设置降级检索器(RAG Service 不可用时使用本地 TF-IDF 检索)
func (c *RemoteRAGClient) SetFallback(fallback RAGSearcher) {
	c.fallback = fallback
}

// ragSearchRequest RAG 检索请求体
type ragSearchRequest struct {
	Query           string `json:"query"`
	KnowledgeBaseID uint   `json:"knowledge_base_id"`
	TopK            int    `json:"top_k"`
}

// ragSearchResponse RAG 检索响应体(对齐 handler.RAGSearch 返回结构)
type ragSearchResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
	Data struct {
		Documents []RAGDocument `json:"documents"`
	} `json:"data"`
}

// Search 通过 HTTP 调用 RAG Service 进行向量检索
// 实现 RAGSearcher 接口,替代进程内 RAGService.Search()
//
// 容错流程:
//  1. 熔断器开启时,直接走降级
//  2. HTTP 调用失败时,记录失败并走降级
//  3. 降级不可用时,返回空结果(不阻塞工作流执行)
func (c *RemoteRAGClient) Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	// 熔断器检查
	if !c.breaker.allow() {
		logger.Get().Warn("rag client: circuit breaker open, falling back to local search")
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}

	traceID := uuid.New().String()

	reqBody := ragSearchRequest{
		Query:           query,
		KnowledgeBaseID: knowledgeBaseID,
		TopK:            topK,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		c.breaker.recordFailure()
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}

	url := c.baseURL + "/api/v1/ai/rag/search"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		c.breaker.recordFailure()
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", traceID) // 兼容:简化 traceID 用于日志关联

	// OTel: 注入 W3C traceparent header,实现跨服务 span 上下文传播
	// Gateway → AI Service → RAG Service 全链路在 Jaeger 中可视化为一个 trace
	otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		c.breaker.recordFailure()
		logger.Get().Warnf("remote rag search failed, trace_id=%s, url=%s, duration=%v, err=%v",
			traceID, url, duration, err)
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.breaker.recordFailure()
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}

	if resp.StatusCode != http.StatusOK {
		c.breaker.recordFailure()
		logger.Get().Warnf("remote rag search http error, trace_id=%s, status=%d, body=%s",
			traceID, resp.StatusCode, string(body))
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}

	var ragResp ragSearchResponse
	if err := json.Unmarshal(body, &ragResp); err != nil {
		c.breaker.recordFailure()
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}

	if ragResp.Code != 0 {
		c.breaker.recordFailure()
		return c.fallbackSearch(query, knowledgeBaseID, topK)
	}

	// 成功:重置熔断器
	c.breaker.recordSuccess()
	logger.Get().Infof("remote rag search success, trace_id=%s, kb_id=%d, topk=%d, docs=%d, duration=%v",
		traceID, knowledgeBaseID, topK, len(ragResp.Data.Documents), duration)

	return ragResp.Data.Documents, nil
}

// fallbackSearch 降级检索(本地 TF-IDF)
// RAG Service 不可用时,使用本地 RAGService 进行 TF-IDF 检索
// 降级不可用(未配置 fallback)时返回空结果,不阻塞工作流执行
func (c *RemoteRAGClient) fallbackSearch(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	if c.fallback == nil {
		logger.Get().Warn("rag client: no fallback available, returning empty results")
		return []RAGDocument{}, nil
	}
	logger.Get().Info("rag client: falling back to local TF-IDF search")
	return c.fallback.Search(query, knowledgeBaseID, topK)
}

// 确保 RemoteRAGClient 实现 RAGSearcher 接口(编译期检查)
var _ RAGSearcher = (*RemoteRAGClient)(nil)

// suppress unused import warning for fmt (used in error formatting)
var _ = fmt.Sprintf
