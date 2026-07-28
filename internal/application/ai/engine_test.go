package ai

import (
	"context"
	"testing"

	"github.com/cb-platform/internal/pkg/config"
)

func TestBuiltinLLMProvider_Chat(t *testing.T) {
	p := NewBuiltinLLMProvider()
	if p.Name() != "builtin" {
		t.Errorf("expected builtin, got %s", p.Name())
	}

	req := ChatRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: "分析这个商品"},
		},
	}

	resp, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("builtin chat failed: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least 1 choice")
	}
	if resp.Choices[0].Message.Content == "" {
		t.Error("expected non-empty content")
	}
	if resp.Usage.TotalTokens == 0 {
		t.Error("expected non-zero tokens")
	}
	if resp.ID == "" {
		t.Error("expected non-empty response id")
	}
}

func TestNewLLMProvider_Builtin(t *testing.T) {
	cfg := config.LLMConfig{Provider: "builtin"}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "builtin" {
		t.Errorf("expected builtin, got %s", p.Name())
	}
}

func TestNewLLMProvider_OfflineAlias(t *testing.T) {
	cfg := config.LLMConfig{Provider: "offline"}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "builtin" {
		t.Errorf("expected builtin for offline alias, got %s", p.Name())
	}
}

func TestNewLLMProvider_EmptyDefaultsBuiltin(t *testing.T) {
	cfg := config.LLMConfig{Provider: ""}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "builtin" {
		t.Errorf("expected builtin for empty provider, got %s", p.Name())
	}
}

func TestNewLLMProvider_GLM(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "glm",
		APIKey:   "test-key",
		BaseURL:  "https://open.bigmodel.cn/api/paas/v4",
		Model:    "glm-4-plus",
	}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "glm" {
		t.Errorf("expected glm, got %s", p.Name())
	}
}

func TestNewLLMProvider_Claude(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "claude",
		APIKey:   "test-key",
	}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("expected claude, got %s", p.Name())
	}
}

func TestNewLLMProvider_DeepSeek(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "deepseek",
		APIKey:   "test-key",
	}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "deepseek" {
		t.Errorf("expected deepseek, got %s", p.Name())
	}
}

func TestNewLLMProvider_Qwen(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: "qwen",
		APIKey:   "test-key",
	}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "qwen" {
		t.Errorf("expected qwen, got %s", p.Name())
	}
}

func TestNewLLMProvider_DefaultFallback(t *testing.T) {
	// 未知 provider(配置了 API Key)应该回退到 OpenAI 兼容
	cfg := config.LLMConfig{Provider: "unknown_provider", APIKey: "test-key"}
	p, err := NewLLMProvider(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected openai fallback, got %s", p.Name())
	}
}

func TestEstimateCost(t *testing.T) {
	usage := Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500}

	// GLM 成本
	cost := EstimateCost("glm", usage)
	if cost.IsZero() {
		t.Error("expected non-zero cost for glm")
	}

	// Builtin 成本应为 0
	cost = EstimateCost("builtin", usage)
	if !cost.IsZero() {
		t.Errorf("expected zero cost for builtin, got %s", cost.String())
	}
}

func TestEstimateCost_UnknownProviderFallback(t *testing.T) {
	usage := Usage{PromptTokens: 1000, CompletionTokens: 500}
	cost := EstimateCost("unknown_provider", usage)
	if cost.IsZero() {
		t.Error("expected non-zero cost for unknown provider (openai pricing)")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"纯 JSON", `{"key":"value"}`, `{"key":"value"}`},
		{"markdown 包裹", "```json\n{\"key\":\"value\"}\n```", `{"key":"value"}`},
		{"普通代码块", "```\n{\"key\":\"value\"}\n```", `{"key":"value"}`},
		{"非 JSON", `hello world`, `hello world`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseWorkflowDefinition_Empty(t *testing.T) {
	def, err := parseWorkflowDefinition("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Nodes) != 3 {
		t.Errorf("expected 3 default nodes, got %d", len(def.Nodes))
	}
	if len(def.Edges) != 2 {
		t.Errorf("expected 2 default edges, got %d", len(def.Edges))
	}
}

func TestParseWorkflowDefinition_Valid(t *testing.T) {
	input := `{"nodes":[{"id":"start","type":"input"},{"id":"end","type":"output"}],"edges":[{"from":"start","to":"end"}]}`
	def, err := parseWorkflowDefinition(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(def.Nodes))
	}
}

func TestParseWorkflowDefinition_Invalid(t *testing.T) {
	_, err := parseWorkflowDefinition("{invalid json}")
	if err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestEvalCondition(t *testing.T) {
	ctx := map[string]interface{}{
		"stage": "approved",
		"score": 80,
	}
	tests := []struct {
		cond   string
		expect bool
	}{
		{"stage=approved", true},
		{"stage=rejected", false},
		{"score=80", true},
		{"unknown=key", false},
		{"invalid_condition", true}, // 无 = 默认 true
	}
	for _, tt := range tests {
		if got := evalCondition(tt.cond, ctx); got != tt.expect {
			t.Errorf("evalCondition(%q) = %v, want %v", tt.cond, got, tt.expect)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	// 有模板
	out, err := renderTemplate("Hello {{.name}}", map[string]interface{}{"name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Hello World" {
		t.Errorf("expected 'Hello World', got %s", out)
	}

	// 空模板(应返回 JSON)
	out, _ = renderTemplate("", map[string]interface{}{"key": "value"})
	if out == "" {
		t.Error("expected non-empty output for empty template")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("expected 'hello', got %s", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("expected 'hello...', got %s", got)
	}
}

func TestBuiltinLLMProvider_NoUserMessage(t *testing.T) {
	p := NewBuiltinLLMProvider()
	req := ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "system", Content: "你是助手"}},
	}
	resp, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("builtin chat failed: %v", err)
	}
	if resp.Choices[0].Message.Content == "" {
		t.Error("expected non-empty content even without user message")
	}
}
