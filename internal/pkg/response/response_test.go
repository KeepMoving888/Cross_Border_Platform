package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cb-platform/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	return r
}

func TestOK(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		c.Set("trace_id", "trace-123")
		OK(c, map[string]interface{}{"foo": "bar"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("expected 'success', got %s", resp.Message)
	}
	if resp.TraceID != "trace-123" {
		t.Errorf("expected trace_id 'trace-123', got %s", resp.TraceID)
	}
}

func TestOKWithMsg(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		OKWithMsg(c, "操作成功", "data")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "操作成功" {
		t.Errorf("expected '操作成功', got %s", resp.Message)
	}
}

func TestOKPage(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		OKPage(c, []string{"a", "b"}, 100, 1, 20)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	// data 是 PageResult
	dataBytes, _ := json.Marshal(resp.Data)
	var page PageResult
	if err := json.Unmarshal(dataBytes, &page); err != nil {
		t.Fatalf("unmarshal page failed: %v", err)
	}
	if page.Total != 100 {
		t.Errorf("expected total 100, got %d", page.Total)
	}
	if page.Page != 1 {
		t.Errorf("expected page 1, got %d", page.Page)
	}
	if page.PageSize != 20 {
		t.Errorf("expected page_size 20, got %d", page.PageSize)
	}
}

func TestFail(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		Fail(c, errors.New(1001, "参数错误"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 1001 {
		t.Errorf("expected code 1001, got %d", resp.Code)
	}
	if resp.Message != "参数错误" {
		t.Errorf("expected '参数错误', got %s", resp.Message)
	}
}

func TestFailWithCode(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		FailWithCode(c, errors.ErrNotFound)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 4001 {
		t.Errorf("expected code 4001, got %d", resp.Code)
	}
}

func TestAbortFail(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		AbortFail(c, errors.ErrForbidden)
	}, func(c *gin.Context) {
		// 中间件,不应执行(被 Abort 阻止)
		c.JSON(http.StatusOK, Response{Code: 0, Message: "should not see"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 3001 {
		t.Errorf("expected code 3001, got %d", resp.Code)
	}
}

func TestFail_WithPlainError(t *testing.T) {
	r := setupRouter()
	r.GET("/test", func(c *gin.Context) {
		// 传入非业务 error 应自动包装为系统错误
		Fail(c, errors.FromError(http.ErrAbortHandler))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	// http.ErrAbortHandler 是普通 error,会被 FromError 包装为 9001 -> 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var resp Response
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 9001 {
		t.Errorf("expected code 9001, got %d", resp.Code)
	}
}
