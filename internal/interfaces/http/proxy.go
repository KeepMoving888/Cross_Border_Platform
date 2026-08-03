package http

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// ============== API Gateway 反向代理中间件 ==============
//
// 设计目标:
//   1. 单体模式(默认): AIServiceURL/RAGServiceURL 为空,所有请求本地处理
//   2. 微服务模式: 配置了 Service URL 后,自动转发对应路径到下游服务
//   3. 透明转发: JWT、TraceID 等 header 原样透传,下游服务无需重复鉴权
//
// 转发规则(基于 server.go 实际路由):
//   /api/v1/ai/workflows  → AI Service  (工作流管理+执行)
//   /api/v1/ai/runs       → AI Service  (执行历史)
//   /api/v1/ai/prompts    → AI Service  (Prompt 模板)
//   /api/v1/ai/analyze    → AI Service  (选品分析)
//   /api/v1/ai/generate   → AI Service  (Listing 生成)
//   /api/v1/ai/reply      → AI Service  (客服回复)
//   /api/v1/ai/knowledge-bases → RAG Service (知识库+文档)
//   /api/v1/ai/rag        → RAG Service (向量检索)
//
// 注意: AI 和 RAG 路由前缀均为 /api/v1/ai/*,需按子路径精确匹配

// proxyRoute 定义一条转发规则
type proxyRoute struct {
	prefix    string // 路径前缀,如 /api/v1/ai/workflows
	targetURL string // 目标服务地址,如 http://cb-ai-svc:8081
}

// ProxyMiddleware 反向代理中间件
type ProxyMiddleware struct {
	cfg    config.ServiceConfig
	routes []proxyRoute
}

// NewProxyMiddleware 创建反向代理中间件
// 根据 ServiceConfig 自动构建转发规则,未配置 URL 时不启用转发
func NewProxyMiddleware(cfg config.ServiceConfig) *ProxyMiddleware {
	pm := &ProxyMiddleware{cfg: cfg}

	// 构建 AI Service 转发规则(仅当配置了 AIServiceURL)
	if cfg.AIServiceURL != "" {
		aiPrefixes := []string{
			"/api/v1/ai/workflows",
			"/api/v1/ai/runs",
			"/api/v1/ai/prompts",
			"/api/v1/ai/analyze",
			"/api/v1/ai/generate",
			"/api/v1/ai/reply",
		}
		for _, p := range aiPrefixes {
			pm.routes = append(pm.routes, proxyRoute{prefix: p, targetURL: cfg.AIServiceURL})
		}
		logger.Get().Infof("proxy: AI Service routes registered, target=%s, routes=%d",
			cfg.AIServiceURL, len(aiPrefixes))
	}

	// 构建 RAG Service 转发规则(仅当配置了 RAGServiceURL)
	if cfg.RAGServiceURL != "" {
		ragPrefixes := []string{
			"/api/v1/ai/knowledge-bases",
			"/api/v1/ai/rag",
		}
		for _, p := range ragPrefixes {
			pm.routes = append(pm.routes, proxyRoute{prefix: p, targetURL: cfg.RAGServiceURL})
		}
		logger.Get().Infof("proxy: RAG Service routes registered, target=%s, routes=%d",
			cfg.RAGServiceURL, len(ragPrefixes))
	}

	return pm
}

// Enabled 是否启用转发(配置了任一 Service URL 即启用)
func (pm *ProxyMiddleware) Enabled() bool {
	return len(pm.routes) > 0
}

// Register 注册反向代理路由到 Gin Engine
// 在 registerRoutes 之前调用,优先匹配转发规则,未命中则走本地路由
func (pm *ProxyMiddleware) Register(r *gin.Engine) {
	if !pm.Enabled() {
		return
	}

	// 为每条规则注册 NoRoute 兜底 + 精确路由
	// 使用 gin.Any 匹配所有 HTTP 方法,路径通配 /*path
	for _, route := range pm.routes {
		pm.registerProxyRoute(r, route)
	}
}

// registerProxyRoute 注册单条转发规则
// 使用 httputil.ReverseProxy 实现透明转发,header 原样透传
func (pm *ProxyMiddleware) registerProxyRoute(r *gin.Engine, route proxyRoute) {
	target, err := url.Parse(route.targetURL)
	if err != nil {
		logger.Get().Errorf("proxy: parse target url %s failed: %v", route.targetURL, err)
		return
	}

	// 创建 ReverseProxy 实例
	// 超时由 Transport 控制,避免下游服务不可用时 Gateway 长时间阻塞
	timeout := time.Duration(pm.cfg.ProxyTimeout) * time.Second
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// 保留原始路径和 query,下游服务按原路径处理
			// X-Forwarded-For 由 Go 标准库自动追加
			// OTel traceparent header 由 TraceID 中间件注入,ReverseProxy 原样透传
			req.Host = target.Host
		},
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			logger.Get().Warnf("proxy: forward %s to %s failed: %v",
				req.URL.Path, route.targetURL, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":502,"message":"下游服务暂不可用,请稍后重试"}`))
		},
	}

	// 注册精确前缀匹配 + 通配符
	// 路径模式: {prefix} 和 {prefix}/*path
	r.Any(route.prefix, gin.WrapH(proxy))
	r.Any(route.prefix+"/*path", gin.WrapH(proxy))
	logger.Get().Infof("proxy: route %s → %s registered", route.prefix, route.targetURL)
}

// matchProxyRoute 检查路径是否匹配某条转发规则
// 用于在本地路由注册前判断是否需要跳过(避免路由冲突)
func (pm *ProxyMiddleware) matchProxyRoute(path string) bool {
	for _, route := range pm.routes {
		if strings.HasPrefix(path, route.prefix) {
			return true
		}
	}
	return false
}
