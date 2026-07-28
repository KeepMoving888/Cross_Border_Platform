package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cb-platform/internal/pkg/logger"
	"github.com/google/uuid"
)

// RemoteRAGClient 远程 RAG 检索客户端(微服务模式)
// AI Service 通过 HTTP 调用 RAG Service 的 /api/v1/ai/rag/search 接口
// 替代进程内调用 RAGService.Search(),实现服务解耦
//
// 链路追踪:每次调用生成 X-Trace-Id header,RAG Service 的 TraceID 中间件会复用该 ID
// 实现 Gateway → AI Service → RAG Service 全链路 TraceID 透传
type RemoteRAGClient struct {
	baseURL    string       // RAG Service 地址,如 http://cb-rag-svc:8082
	httpClient *http.Client // HTTP 客户端(带超时)
	authToken  string       // 内部调用 JWT(可选,微服务内部信任可留空)
}

// NewRemoteRAGClient 创建远程 RAG 客户端
// baseURL: RAG Service 地址(如 http://cb-rag-svc:8082)
func NewRemoteRAGClient(baseURL string) *RemoteRAGClient {
	return &RemoteRAGClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second, // RAG 检索超时 10s
		},
	}
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
// 链路追踪:生成唯一 traceID 写入 X-Trace-Id header,RAG Service 复用该 ID
// 日志中记录 traceID 便于跨服务排障
func (c *RemoteRAGClient) Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error) {
	traceID := uuid.New().String()

	reqBody := ragSearchRequest{
		Query:           query,
		KnowledgeBaseID: knowledgeBaseID,
		TopK:            topK,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal rag search request: %w", err)
	}

	url := c.baseURL + "/api/v1/ai/rag/search"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create rag search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", traceID) // 链路追踪:透传 traceID 到 RAG Service
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		logger.Get().Warnf("remote rag search failed, trace_id=%s, url=%s, duration=%v, err=%v",
			traceID, url, duration, err)
		return nil, fmt.Errorf("remote rag search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rag search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Get().Warnf("remote rag search http error, trace_id=%s, status=%d, body=%s",
			traceID, resp.StatusCode, string(body))
		return nil, fmt.Errorf("rag search http %d: %s", resp.StatusCode, string(body))
	}

	var ragResp ragSearchResponse
	if err := json.Unmarshal(body, &ragResp); err != nil {
		return nil, fmt.Errorf("unmarshal rag search response: %w", err)
	}

	if ragResp.Code != 0 {
		return nil, fmt.Errorf("rag search error: %s", ragResp.Msg)
	}

	logger.Get().Infof("remote rag search success, trace_id=%s, kb_id=%d, topk=%d, docs=%d, duration=%v",
		traceID, knowledgeBaseID, topK, len(ragResp.Data.Documents), duration)

	return ragResp.Data.Documents, nil
}
