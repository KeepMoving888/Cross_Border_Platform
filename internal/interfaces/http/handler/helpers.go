package handler

import (
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/shopspring/decimal"
)

// Pagination 分页参数别名(简化 handler 引用)
type Pagination = models.Pagination

// timeNow 当前时间(便于测试时替换)
func timeNow() time.Time {
	return time.Now()
}

// formatTime 格式化时间
func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// parseTime 解析时间字符串
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

// decimalZero 零值 decimal
var decimalZero = decimal.Zero

// ptrString 字符串指针
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// strValue 安全获取字符串值
func strValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
