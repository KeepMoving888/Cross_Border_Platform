package ai

import (
	"context"
	"testing"

	"github.com/cb-platform/internal/pkg/config"
)

// ============== Embedding Provider 测试 ==============

func TestLocalHashEmbeddingProvider_Name(t *testing.T) {
	p := &LocalHashEmbeddingProvider{dim: 128}
	if p.Name() != "local-hash" {
		t.Errorf("expected local-hash, got %s", p.Name())
	}
}

func TestLocalHashEmbeddingProvider_Dimension(t *testing.T) {
	p := &LocalHashEmbeddingProvider{dim: 256}
	if p.Dimension() != 256 {
		t.Errorf("expected 256, got %d", p.Dimension())
	}
}

func TestLocalHashEmbeddingProvider_EmptyText(t *testing.T) {
	p := &LocalHashEmbeddingProvider{dim: 64}
	vec, err := p.Embed(context.Background(), []string{""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vec))
	}
	if len(vec[0]) != 64 {
		t.Errorf("expected dim 64, got %d", len(vec[0]))
	}
	// 空文本的向量应全为 0
	for i, v := range vec[0] {
		if v != 0 {
			t.Errorf("expected 0 at index %d, got %f", i, v)
		}
	}
}

func TestLocalHashEmbeddingProvider_SameTextSameVector(t *testing.T) {
	p := &LocalHashEmbeddingProvider{dim: 128}
	vec1, _ := p.Embed(context.Background(), []string{"hello world"})
	vec2, _ := p.Embed(context.Background(), []string{"hello world"})
	if len(vec1) != 1 || len(vec2) != 1 {
		t.Fatal("expected 1 vector each")
	}
	for i := range vec1[0] {
		if vec1[0][i] != vec2[0][i] {
			t.Errorf("same text should produce same vector at index %d", i)
		}
	}
}

func TestLocalHashEmbeddingProvider_L2Normalized(t *testing.T) {
	p := &LocalHashEmbeddingProvider{dim: 64}
	vec, _ := p.Embed(context.Background(), []string{"跨境电商运营中台"})
	if len(vec) != 1 || len(vec[0]) != 64 {
		t.Fatal("unexpected vector dimension")
	}
	// L2 范数应接近 1
	sum := 0.0
	for _, v := range vec[0] {
		sum += v * v
	}
	norm := 0.0
	if sum > 0 {
		// sqrt
		x := sum
		for i := 0; i < 20; i++ {
			x = 0.5 * (x + sum/x)
		}
		norm = x
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected L2 norm ~1.0, got %f (sum=%f)", norm, sum)
	}
}

func TestNewEmbeddingProvider_DefaultFallback(t *testing.T) {
	// 无 API Key 时应降级到 LocalHashEmbeddingProvider
	cfg := config.LLMConfig{Provider: "glm"}
	p := NewEmbeddingProvider(cfg)
	if p.Name() != "local-hash" {
		t.Errorf("expected local-hash fallback, got %s", p.Name())
	}
}

func TestNewEmbeddingProvider_WithAPIKey(t *testing.T) {
	// 有 API Key 时应使用 OpenAI 兼容 provider
	cfg := config.LLMConfig{Provider: "openai", APIKey: "test-key"}
	p := NewEmbeddingProvider(cfg)
	if p.Name() != "openai" {
		t.Errorf("expected openai, got %s", p.Name())
	}
}

func TestChooseEmbeddingModel(t *testing.T) {
	tests := []struct {
		provider string
		expected string
	}{
		{"openai", "text-embedding-3-small"},
		{"glm", "embedding-3"},
		{"qwen", "text-embedding-v3"},
		{"deepseek", "text-embedding-3-small"},
		{"claude", "text-embedding-3-small"},
		{"unknown", "text-embedding-3-small"},
		{"", "text-embedding-3-small"},
	}
	for _, tt := range tests {
		cfg := config.LLMConfig{Provider: tt.provider}
		got := chooseEmbeddingModel(cfg)
		if got != tt.expected {
			t.Errorf("provider=%s: expected %s, got %s", tt.provider, tt.expected, got)
		}
	}
}

// ============== ChunkText 测试 ==============

func TestChunkText_Empty(t *testing.T) {
	chunks := ChunkText("", DefaultChunkConfig())
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestChunkText_ShortText(t *testing.T) {
	text := "这是一段简短的文本。"
	chunks := ChunkText(text, DefaultChunkConfig())
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk content mismatch: got %q", chunks[0])
	}
}

func TestChunkText_ParagraphSplit(t *testing.T) {
	text := "第一段落内容。\n\n第二段落内容。\n\n第三段落内容。"
	chunks := ChunkText(text, DefaultChunkConfig())
	if len(chunks) != 1 {
		// 三个短段落应合并为 1 个块
		t.Errorf("expected 1 chunk for short paragraphs, got %d", len(chunks))
	}
}

func TestChunkText_LongParagraph(t *testing.T) {
	// 生成超过 chunkSize 的长段落
	longPara := ""
	for i := 0; i < 100; i++ {
		longPara += "这是第" + string(rune('0'+i%10)) + "段测试内容。"
	}
	cfg := ChunkConfig{ChunkSize: 100, Overlap: 10}
	chunks := ChunkText(longPara, cfg)
	if len(chunks) < 2 {
		t.Errorf("expected >=2 chunks for long text, got %d", len(chunks))
	}
}

func TestChunkText_DefaultConfig(t *testing.T) {
	cfg := DefaultChunkConfig()
	if cfg.ChunkSize != 500 {
		t.Errorf("expected chunk size 500, got %d", cfg.ChunkSize)
	}
	if cfg.Overlap != 50 {
		t.Errorf("expected overlap 50, got %d", cfg.Overlap)
	}
}

func TestChunkText_InvalidConfig(t *testing.T) {
	// ChunkSize <= 0 时应使用默认配置
	chunks := ChunkText("测试文本", ChunkConfig{ChunkSize: 0, Overlap: 0})
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk with default config, got %d", len(chunks))
	}
}

// ============== 工具函数测试 ==============

func TestEstimateTokens(t *testing.T) {
	// 纯中文(1 字 = 1 token)
	if got := estimateTokens("你好世界"); got != 4 {
		t.Errorf("expected 4 tokens for 4 Chinese chars, got %d", got)
	}
	// 纯英文(4 字符 = 1 token)
	if got := estimateTokens("hello"); got != 1 {
		t.Errorf("expected 1 token for 5 English chars, got %d", got)
	}
	// 混合
	if got := estimateTokens("你好hello"); got != 2+1 {
		t.Errorf("expected 3 tokens for mixed text, got %d", got)
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("跨境电商 platform")
	if len(tokens) == 0 {
		t.Error("expected non-empty tokens")
	}
	// 应包含中文字符
	hasChinese := false
	for _, tk := range tokens {
		for _, r := range tk {
			if r > 0x4e00 {
				hasChinese = true
			}
		}
	}
	if !hasChinese {
		t.Error("expected Chinese tokens")
	}
}

func TestVectorToPgString(t *testing.T) {
	vec := []float64{0.1, 0.2, 0.3}
	got := vectorToPgString(vec)
	expected := "[0.100000,0.200000,0.300000]"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestHashString(t *testing.T) {
	// 相同输入应产生相同 hash
	h1 := hashString("test")
	h2 := hashString("test")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	// 不同输入应产生不同 hash
	h3 := hashString("other")
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestExtractNGrams(t *testing.T) {
	grams := extractNGrams("hello", 2)
	if len(grams) != 4 {
		t.Errorf("expected 4 2-grams for 'hello', got %d", len(grams))
	}
	expected := []string{"he", "el", "ll", "lo"}
	for i, g := range grams {
		if g != expected[i] {
			t.Errorf("gram %d: expected %s, got %s", i, expected[i], g)
		}
	}
}

func TestCalculateScore(t *testing.T) {
	score := calculateScore("hello world", []string{"hello"})
	if score <= 0 {
		t.Error("expected positive score for matching keyword")
	}
	score = calculateScore("hello world", []string{"xyz"})
	if score != 0 {
		t.Error("expected 0 score for non-matching keyword")
	}
}

func TestSortDocumentsByScore(t *testing.T) {
	docs := []RAGDocument{
		{Title: "a", Score: 0.3},
		{Title: "b", Score: 0.8},
		{Title: "c", Score: 0.5},
	}
	sortDocumentsByScore(docs)
	if docs[0].Score != 0.8 || docs[1].Score != 0.5 || docs[2].Score != 0.3 {
		t.Error("documents not sorted by score descending")
	}
}
