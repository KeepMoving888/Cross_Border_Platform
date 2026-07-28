package handler

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	// 固定时间测试
	ts := time.Date(2026, 1, 15, 14, 30, 45, 0, time.UTC)
	got := formatTime(ts)
	expected := "2026-01-15 14:30:45"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		year    int
	}{
		{"完整日期时间", "2026-01-15 14:30:45", false, 2026},
		{"仅日期", "2026-01-15", false, 2026},
		{"RFC3339", "2026-01-15T14:30:45Z", false, 2026},
		{"空字符串", "", false, 0},        // 空字符串返回零值
		{"无效格式", "invalid", false, 0}, // 无效格式返回零值(不报错)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := parseTime(tt.input)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if tt.wantErr {
				t.Error("expected error, got nil")
				return
			}
			if tt.year > 0 && ts.Year() != tt.year {
				t.Errorf("expected year %d, got %d", tt.year, ts.Year())
			}
		})
	}
}

func TestParseTime_Empty(t *testing.T) {
	ts, err := parseTime("")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !ts.IsZero() {
		t.Error("expected zero time for empty string")
	}
}

func TestPtrString(t *testing.T) {
	// 非空字符串
	p := ptrString("hello")
	if p == nil || *p != "hello" {
		t.Errorf("expected pointer to 'hello'")
	}

	// 空字符串应返回 nil
	p = ptrString("")
	if p != nil {
		t.Error("expected nil for empty string")
	}
}

func TestStrValue(t *testing.T) {
	// 非 nil 指针
	s := "hello"
	if got := strValue(&s); got != "hello" {
		t.Errorf("expected 'hello', got %s", got)
	}

	// nil 指针
	if got := strValue(nil); got != "" {
		t.Errorf("expected empty string for nil, got %s", got)
	}
}

func TestTimeNow(t *testing.T) {
	before := time.Now()
	ts := timeNow()
	after := time.Now()

	if ts.Before(before) || ts.After(after) {
		t.Error("timeNow should return current time")
	}
}

func TestDecimalZero(t *testing.T) {
	if !decimalZero.IsZero() {
		t.Error("decimalZero should be zero")
	}
}
