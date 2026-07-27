package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestTraceID_GeneratesNewID(t *testing.T) {
	r := setupTestRouter()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		traceID := c.GetString("trace_id")
		if traceID == "" {
			t.Error("expected non-empty trace_id")
		}
		c.JSON(200, gin.H{"trace_id": traceID})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	// 响应头应包含 X-Trace-Id
	if got := w.Header().Get("X-Trace-Id"); got == "" {
		t.Error("expected X-Trace-Id header to be set")
	}
}

func TestTraceID_UsesClientHeader(t *testing.T) {
	r := setupTestRouter()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"trace_id": c.GetString("trace_id")})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Trace-Id", "client-trace-id-123")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Trace-Id"); got != "client-trace-id-123" {
		t.Errorf("expected 'client-trace-id-123', got %s", got)
	}
}

func TestCORS_Headers(t *testing.T) {
	r := setupTestRouter()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Errorf("expected origin echo, got %s", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected non-empty Allow-Methods")
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected 'true', got %s", got)
	}
}

func TestCORS_OptionsRequest(t *testing.T) {
	r := setupTestRouter()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}
}

func TestCORS_NoOrigin(t *testing.T) {
	r := setupTestRouter()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	// 不设置 Origin
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected '*' for no origin, got %s", got)
	}
}

func TestRecovery_RecoversFromPanic(t *testing.T) {
	r := setupTestRouter()
	r.Use(Recovery())
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRecovery_NoPanic(t *testing.T) {
	r := setupTestRouter()
	r.Use(Recovery())
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ok", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	r := setupTestRouter()
	r.Use(RateLimiter(100)) // 高限流,确保不被限
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
