package http

import (
	"testing"

	"github.com/cb-platform/internal/pkg/config"
)

func TestProxyMiddleware_NotEnabled_WhenEmptyConfig(t *testing.T) {
	pm := NewProxyMiddleware(config.ServiceConfig{})
	if pm.Enabled() {
		t.Error("expected disabled when config is empty")
	}
	if len(pm.routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(pm.routes))
	}
}

func TestProxyMiddleware_Enabled_WhenAIServiceConfigured(t *testing.T) {
	pm := NewProxyMiddleware(config.ServiceConfig{
		AIServiceURL: "http://cb-ai-svc:8081",
	})
	if !pm.Enabled() {
		t.Error("expected enabled when AI Service URL configured")
	}
	// 应注册 6 条 AI 路由
	if len(pm.routes) != 6 {
		t.Errorf("expected 6 AI routes, got %d", len(pm.routes))
	}
}

func TestProxyMiddleware_Enabled_WhenRAGServiceConfigured(t *testing.T) {
	pm := NewProxyMiddleware(config.ServiceConfig{
		RAGServiceURL: "http://cb-rag-svc:8082",
	})
	if !pm.Enabled() {
		t.Error("expected enabled when RAG Service URL configured")
	}
	// 应注册 2 条 RAG 路由
	if len(pm.routes) != 2 {
		t.Errorf("expected 2 RAG routes, got %d", len(pm.routes))
	}
}

func TestProxyMiddleware_Enabled_WhenBothConfigured(t *testing.T) {
	pm := NewProxyMiddleware(config.ServiceConfig{
		AIServiceURL:  "http://cb-ai-svc:8081",
		RAGServiceURL: "http://cb-rag-svc:8082",
	})
	if !pm.Enabled() {
		t.Error("expected enabled when both URLs configured")
	}
	// 6 AI + 2 RAG = 8 条路由
	if len(pm.routes) != 8 {
		t.Errorf("expected 8 routes, got %d", len(pm.routes))
	}
}

func TestProxyMiddleware_MatchProxyRoute(t *testing.T) {
	pm := NewProxyMiddleware(config.ServiceConfig{
		AIServiceURL:  "http://cb-ai-svc:8081",
		RAGServiceURL: "http://cb-rag-svc:8082",
	})

	tests := []struct {
		path   string
		expect bool
	}{
		{"/api/v1/ai/workflows", true},
		{"/api/v1/ai/workflows/1", true},
		{"/api/v1/ai/runs", true},
		{"/api/v1/ai/knowledge-bases", true},
		{"/api/v1/ai/knowledge-bases/1/documents", true},
		{"/api/v1/ai/rag/search", true},
		{"/api/v1/ai/analyze/product", true},
		{"/api/v1/products", false},
		{"/api/v1/auth/login", false},
		{"/api/v1/finance/bills", false},
		{"/health", false},
	}

	for _, tt := range tests {
		got := pm.matchProxyRoute(tt.path)
		if got != tt.expect {
			t.Errorf("matchProxyRoute(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestProxyMiddleware_RouteTargets(t *testing.T) {
	aiURL := "http://cb-ai-svc:8081"
	ragURL := "http://cb-rag-svc:8082"
	pm := NewProxyMiddleware(config.ServiceConfig{
		AIServiceURL:  aiURL,
		RAGServiceURL: ragURL,
	})

	for _, route := range pm.routes {
		switch route.prefix {
		case "/api/v1/ai/workflows", "/api/v1/ai/runs", "/api/v1/ai/prompts",
			"/api/v1/ai/analyze", "/api/v1/ai/generate", "/api/v1/ai/reply":
			if route.targetURL != aiURL {
				t.Errorf("route %s target = %s, want %s", route.prefix, route.targetURL, aiURL)
			}
		case "/api/v1/ai/knowledge-bases", "/api/v1/ai/rag":
			if route.targetURL != ragURL {
				t.Errorf("route %s target = %s, want %s", route.prefix, route.targetURL, ragURL)
			}
		default:
			t.Errorf("unexpected route prefix: %s", route.prefix)
		}
	}
}
