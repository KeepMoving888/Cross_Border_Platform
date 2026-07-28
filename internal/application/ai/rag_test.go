package ai

import (
	"context"
	"strings"
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

// ============== Redis 缓存 ==============

func TestCacheKey_Deterministic(t *testing.T) {
	k1 := cacheKey("hello world", 1, 5)
	k2 := cacheKey("hello world", 1, 5)
	if k1 != k2 {
		t.Errorf("cache key not deterministic: %s vs %s", k1, k2)
	}
}

func TestCacheKey_DifferentArgs(t *testing.T) {
	k1 := cacheKey("hello", 1, 5)
	k2 := cacheKey("world", 1, 5)
	k3 := cacheKey("hello", 2, 5)
	k4 := cacheKey("hello", 1, 10)
	if k1 == k2 || k1 == k3 || k1 == k4 {
		t.Error("cache key collision for different args")
	}
}

func TestCacheKey_Format(t *testing.T) {
	k := cacheKey("query", 7, 5)
	if !strings.HasPrefix(k, "rag:search:7:5:") {
		t.Errorf("cache key format unexpected: %s", k)
	}
}

func TestFnv1a64_EmptyString(t *testing.T) {
	// FNV-1a 64 of empty string is the offset basis
	if fnv1a64("") != 14695981039346656037 {
		t.Error("fnv1a64 empty string mismatch")
	}
}

func TestFnv1a64_Deterministic(t *testing.T) {
	if fnv1a64("test") != fnv1a64("test") {
		t.Error("fnv1a64 not deterministic")
	}
	if fnv1a64("test") == fnv1a64("tset") {
		t.Error("fnv1a64 collision for different input")
	}
}

func TestRAGService_GetCache_NoRedis(t *testing.T) {
	// 无 Redis 时 getCache 应返回 hit=false 且不 panic
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	docs, hit := svc.getCache("query", 1, 5)
	if hit {
		t.Error("expected cache miss when redis is nil")
	}
	if docs != nil {
		t.Error("expected nil docs when redis is nil")
	}
}

func TestRAGService_SetCache_NoRedis(t *testing.T) {
	// 无 Redis 时 setCache 应不 panic
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	svc.setCache("query", 1, 5, []RAGDocument{{Title: "test"}})
}

func TestRAGService_Search_EmptyQuery(t *testing.T) {
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	docs, err := svc.Search("", 1, 5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(docs) != 0 {
		t.Error("expected empty result for empty query")
	}
}

func TestRAGService_Search_NoDB(t *testing.T) {
	// 无任何 DB 时 Search 应返回 error 且不 panic
	// Search 内部降级到 tfidfSearch 时检测到 mysqlDB==nil 应提前返回
	svc := NewRAGService(nil, nil, config.LLMConfig{})
	_, err := svc.Search("test query", 1, 5)
	if err == nil {
		t.Error("expected error when all DBs are nil")
	}
}

// ============== Reranker 测试 ==============

func TestNewRerankerProvider_NoAPIKey(t *testing.T) {
	// 无 API Key 时应降级到启发式 reranker
	p := NewRerankerProvider(config.LLMConfig{})
	if p.Name() != "heuristic-reranker" {
		t.Errorf("expected heuristic-reranker, got %s", p.Name())
	}
	if !p.Available() {
		t.Error("heuristic reranker should always be available")
	}
}

func TestNewRerankerProvider_WithAPIKey(t *testing.T) {
	// 有 API Key 时应使用 API reranker
	p := NewRerankerProvider(config.LLMConfig{Provider: "jina", APIKey: "test-key"})
	if p.Name() != "api-reranker" {
		t.Errorf("expected api-reranker, got %s", p.Name())
	}
}

func TestNewRerankerProvider_OpenAI_NoRerankModel(t *testing.T) {
	// OpenAI 无 rerank 接口,Available 应返回 false(运行时降级到启发式)
	p := NewRerankerProvider(config.LLMConfig{Provider: "openai", APIKey: "test-key"})
	if p.Available() {
		t.Error("openai reranker should not be available")
	}
}

func TestChooseRerankerModel(t *testing.T) {
	tests := []struct {
		provider string
		expectNE bool // 是否期望非空
	}{
		{"jina", true},
		{"cohere", true},
		{"bge", true},
		{"openai", false},
		{"glm", false},
		{"", false},
	}
	for _, tt := range tests {
		cfg := config.LLMConfig{Provider: tt.provider}
		model := chooseRerankerModel(cfg)
		if tt.expectNE && model == "" {
			t.Errorf("provider=%s: expected non-empty model", tt.provider)
		}
		if !tt.expectNE && model != "" {
			t.Errorf("provider=%s: expected empty model, got %s", tt.provider, model)
		}
	}
}

func TestHeuristicReranker_EmptyDocs(t *testing.T) {
	p := &HeuristicRerankerProvider{}
	result, err := p.Rerank(context.Background(), "query", nil, 5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Error("expected empty result for empty docs")
	}
}

func TestHeuristicReranker_ReorderByRelevance(t *testing.T) {
	// 文档 B 包含 query 关键词,文档 A 不包含
	// 初始 A 排在前面(向量分高),重排后 B 应排在前面(关键词命中)
	docs := []RAGDocument{
		{Title: "doc-a", Content: "完全无关的内容", Score: 0.9},
		{Title: "doc-b", Content: "负离子吹风机的使用方法", Score: 0.7},
	}
	p := &HeuristicRerankerProvider{}
	result, err := p.Rerank(context.Background(), "负离子吹风机", docs, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(result))
	}
	// doc-b 应排在前面(关键词覆盖率高)
	if result[0].Title != "doc-b" {
		t.Errorf("expected doc-b first after rerank, got %s", result[0].Title)
	}
}

func TestHeuristicReranker_TopK(t *testing.T) {
	docs := []RAGDocument{
		{Title: "a", Content: "test content one", Score: 0.8},
		{Title: "b", Content: "test content two", Score: 0.7},
		{Title: "c", Content: "test content three", Score: 0.6},
	}
	p := &HeuristicRerankerProvider{}
	result, _ := p.Rerank(context.Background(), "test", docs, 2)
	if len(result) != 2 {
		t.Errorf("expected 2 docs after topK=2, got %d", len(result))
	}
}

func TestHeuristicReranker_TitleBonus(t *testing.T) {
	// 标题命中应比仅正文命中得分更高
	docs := []RAGDocument{
		{Title: "generic title", Content: "负离子吹风机", Score: 0.5},
		{Title: "负离子吹风机指南", Content: "generic content", Score: 0.5},
	}
	p := &HeuristicRerankerProvider{}
	result, _ := p.Rerank(context.Background(), "负离子吹风机", docs, 2)
	if result[0].Title != "负离子吹风机指南" {
		t.Errorf("expected title-hit doc first, got %s", result[0].Title)
	}
}

func TestHeuristicScore_NoKeywords(t *testing.T) {
	p := &HeuristicRerankerProvider{}
	score := p.heuristicScore("query", nil, RAGDocument{Title: "t", Content: "c"})
	if score != 0 {
		t.Error("expected 0 score for empty keyword set")
	}
}

func TestAPIReranker_UnavailableFallbackToHeuristic(t *testing.T) {
	// APIRerankerProvider.Available()=false 时应降级到启发式
	p := &APIRerankerProvider{apiKey: "key", model: ""} // model 空 -> unavailable
	if p.Available() {
		t.Error("expected unavailable when model is empty")
	}
	docs := []RAGDocument{
		{Title: "a", Content: "test doc", Score: 0.5},
	}
	result, err := p.Rerank(context.Background(), "test", docs, 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 doc, got %d", len(result))
	}
}

// ============== RRF 融合测试 ==============

func TestRRFusion_EmptyInput(t *testing.T) {
	result := RRFusion(nil, DefaultRRFParams(), 5)
	if len(result) != 0 {
		t.Error("expected empty result for empty input")
	}
}

func TestRRFusion_SingleRanking(t *testing.T) {
	ranking := []RAGDocument{
		{Title: "a", Content: "content-a", Score: 0.9},
		{Title: "b", Content: "content-b", Score: 0.7},
	}
	result := RRFusion([][]RAGDocument{ranking}, DefaultRRFParams(), 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(result))
	}
	// 单路输入应保持原顺序
	if result[0].Title != "a" {
		t.Error("expected doc-a first for single ranking")
	}
}

func TestRRFusion_MergeDuplicates(t *testing.T) {
	// 同一文档被两路召回,应合并并累加 RRF 分数
	ranking1 := []RAGDocument{
		{Title: "shared", Content: "shared-content", Score: 0.9},
		{Title: "only-a", Content: "only-a-content", Score: 0.7},
	}
	ranking2 := []RAGDocument{
		{Title: "shared", Content: "shared-content", Score: 0.8},
		{Title: "only-b", Content: "only-b-content", Score: 0.6},
	}
	result := RRFusion([][]RAGDocument{ranking1, ranking2}, DefaultRRFParams(), 5)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique docs, got %d", len(result))
	}
	// shared 在两路都排第一,RRF 分数最高,应排首位
	if result[0].Title != "shared" {
		t.Errorf("expected shared doc first, got %s", result[0].Title)
	}
}

func TestRRFusion_TopK(t *testing.T) {
	ranking := []RAGDocument{
		{Title: "a", Content: "c-a", Score: 0.9},
		{Title: "b", Content: "c-b", Score: 0.8},
		{Title: "c", Content: "c-c", Score: 0.7},
	}
	result := RRFusion([][]RAGDocument{ranking}, DefaultRRFParams(), 2)
	if len(result) != 2 {
		t.Errorf("expected 2 docs after topK=2, got %d", len(result))
	}
}

func TestRRFusion_DefaultParamsWhenZero(t *testing.T) {
	// K=0 应自动使用默认值 60
	ranking := []RAGDocument{
		{Title: "a", Content: "c-a", Score: 0.9},
	}
	result := RRFusion([][]RAGDocument{ranking}, RRFParams{K: 0}, 5)
	if len(result) != 1 {
		t.Error("expected 1 doc with default params")
	}
}

func TestBuildTSQuery_Empty(t *testing.T) {
	if buildTSQuery("") != "" {
		t.Error("expected empty tsquery for empty query")
	}
}

func TestBuildTSQuery_Chinese(t *testing.T) {
	q := buildTSQuery("负离子吹风机")
	if q == "" {
		t.Error("expected non-empty tsquery for Chinese query")
	}
	// 应包含 & 连接符
	if !strings.Contains(q, "&") {
		t.Error("expected & separator in tsquery")
	}
}

func TestBuildTSQuery_English(t *testing.T) {
	q := buildTSQuery("hair dryer product")
	if q == "" {
		t.Error("expected non-empty tsquery for English query")
	}
}
