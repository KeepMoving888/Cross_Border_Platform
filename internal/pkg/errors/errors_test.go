package errors

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestError_Error(t *testing.T) {
	// 不带原始 error
	e := New(1001, "参数错误")
	if got := e.Error(); got != "[1001] 参数错误" {
		t.Errorf("expected '[1001] 参数错误', got %q", got)
	}

	// 带原始 error
	e = Wrap(errors.New("db conn timeout"), 9002, "数据库操作失败")
	got := e.Error()
	if got == "" {
		t.Error("expected non-empty error string")
	}
	if !strings.Contains(got, "9002") || !strings.Contains(got, "数据库操作失败") || !strings.Contains(got, "db conn timeout") {
		t.Errorf("unexpected error string: %q", got)
	}
}

func TestError_Unwrap(t *testing.T) {
	original := errors.New("original error")
	e := Wrap(original, 1001, "wrapped")
	if unwrapped := e.Unwrap(); unwrapped != original {
		t.Error("Unwrap should return original error")
	}

	// 无原始错误
	e2 := New(1001, "no original")
	if unwrapped := e2.Unwrap(); unwrapped != nil {
		t.Error("expected nil for unwrapped error")
	}
}

func TestError_HTTPStatus(t *testing.T) {
	tests := []struct {
		code   int
		expect int
	}{
		{1001, http.StatusBadRequest},
		{1999, http.StatusBadRequest},
		{2001, http.StatusUnauthorized},
		{2999, http.StatusUnauthorized},
		{3001, http.StatusForbidden},
		{3999, http.StatusForbidden},
		{4001, http.StatusNotFound},
		{4999, http.StatusNotFound},
		{5001, http.StatusConflict},
		{5999, http.StatusConflict},
		{9001, http.StatusInternalServerError},
		{9999, http.StatusInternalServerError},
		{500, http.StatusInternalServerError}, // 未定义区间
	}
	for _, tt := range tests {
		e := New(tt.code, "test")
		if got := e.HTTPStatus(); got != tt.expect {
			t.Errorf("code %d: expected %d, got %d", tt.code, tt.expect, got)
		}
	}
}

func TestFromError(t *testing.T) {
	// nil 应返回 nil
	if e := FromError(nil); e != nil {
		t.Errorf("expected nil for nil input, got %v", e)
	}

	// 业务错误应保持不变
	bizErr := New(1001, "业务错误")
	if e := FromError(bizErr); e != bizErr {
		t.Error("expected same instance for business error")
	}

	// 普通 error 应被包装为系统错误
	plainErr := errors.New("plain error")
	e := FromError(plainErr)
	if e == nil {
		t.Fatal("expected non-nil error")
	}
	if e.Code != 9001 {
		t.Errorf("expected code 9001, got %d", e.Code)
	}
	if e.Err != plainErr {
		t.Error("expected wrapped original error")
	}
}

func TestPredefinedErrors(t *testing.T) {
	// 验证预定义错误码段
	tests := []struct {
		err    *Error
		expect int
	}{
		{ErrInvalidParam, 1001},
		{ErrInvalidJSON, 1002},
		{ErrValidation, 1003},
		{ErrUnauthorized, 2001},
		{ErrInvalidToken, 2002},
		{ErrAccountDisabled, 2003},
		{ErrWrongPassword, 2004},
		{ErrForbidden, 3001},
		{ErrNotFound, 4001},
		{ErrUserNotFound, 4002},
		{ErrProductNotFound, 4003},
		{ErrOrderNotFound, 4004},
		{ErrStateConflict, 5001},
		{ErrDuplicateEntry, 5002},
		{ErrStockInsufficient, 5003},
		{ErrInternal, 9001},
		{ErrDBOperation, 9002},
		{ErrExternalAPI, 9003},
		{ErrLLMUnavailable, 9004},
	}
	for _, tt := range tests {
		if tt.err.Code != tt.expect {
			t.Errorf("%s: expected code %d, got %d", tt.err.Message, tt.expect, tt.err.Code)
		}
		if tt.err.Message == "" {
			t.Errorf("code %d: expected non-empty message", tt.expect)
		}
	}
}
