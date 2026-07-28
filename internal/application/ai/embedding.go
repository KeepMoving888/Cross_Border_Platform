package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/cb-platform/internal/pkg/config"
)

// EmbeddingProvider Embedding 向量生成接口
type EmbeddingProvider interface {
	// Name 返回 provider 名称
	Name() string
	// Embed 生成文本的向量表示
	Embed(ctx context.Context, texts []string) ([][]float64, error)
	// Dimension 返回向量维度
	Dimension() int
}

const defaultEmbeddingDimension = 1536

// NewEmbeddingProvider 根据配置创建 Embedding Provider
// 优先级: OpenAI 兼容 API > 本地 Hash 向量
func NewEmbeddingProvider(cfg config.LLMConfig) EmbeddingProvider {
	// 有 API Key 时使用 OpenAI 兼容协议
	if cfg.APIKey != "" {
		return &OpenAIEmbeddingProvider{
			apiKey:  cfg.APIKey,
			baseURL: cfg.BaseURL,
			model:   chooseEmbeddingModel(cfg),
			client:  &http.Client{Timeout: 30 * time.Second},
			dim:     defaultEmbeddingDimension,
		}
	}
	// 无 API Key 时降级到本地 Hash 向量
	return &LocalHashEmbeddingProvider{dim: defaultEmbeddingDimension}
}

// chooseEmbeddingModel 根据主 provider 选择 embedding 模型
func chooseEmbeddingModel(cfg config.LLMConfig) string {
	switch strings.ToLower(cfg.Provider) {
	case "openai", "":
		return "text-embedding-3-small"
	case "glm", "zhipu":
		return "embedding-3"
	case "qwen":
		return "text-embedding-v3"
	case "deepseek":
		return "text-embedding-3-small" // DeepSeek 兼容 OpenAI 协议
	case "claude":
		return "text-embedding-3-small" // Claude 无 embedding,回退 OpenAI
	default:
		return "text-embedding-3-small"
	}
}

// ============== OpenAI 兼容 Embedding Provider ==============

// OpenAIEmbeddingProvider 调用 OpenAI 兼容协议生成向量
// 支持 OpenAI / GLM / Qwen 等提供 embedding 接口的厂商
type OpenAIEmbeddingProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
	dim     int
}

func (p *OpenAIEmbeddingProvider) Name() string { return "openai" }

func (p *OpenAIEmbeddingProvider) Dimension() int { return p.dim }

// Embed 调用 /v1/embeddings 接口批量生成向量
func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	url := p.baseURL + "/v1/embeddings"
	if p.baseURL == "" {
		url = "https://api.openai.com/v1/embeddings"
	}

	reqBody := struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}{
		Input: texts,
		Model: p.model,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call embedding API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	// 按 index 排序(确保与输入顺序一致)
	embeddings := make([][]float64, len(texts))
	for _, item := range result.Data {
		if item.Index >= 0 && item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}
	if len(result.Data) > 0 && len(result.Data[0].Embedding) > 0 {
		p.dim = len(result.Data[0].Embedding)
	}
	return embeddings, nil
}

// ============== 本地 Hash Embedding Provider(降级方案) ==============

// LocalHashEmbeddingProvider 无 LLM API Key 时的降级方案
// 使用字符 n-gram + hash 生成固定维度向量(语义检索能力弱,但保证可用)
type LocalHashEmbeddingProvider struct {
	dim int
}

func (p *LocalHashEmbeddingProvider) Name() string { return "local-hash" }

func (p *LocalHashEmbeddingProvider) Dimension() int { return p.dim }

// Embed 使用字符 2-gram + hash 函数生成稀疏向量
// 相同文本产生相同向量,相似文本产生相近向量(局部敏感)
func (p *LocalHashEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, 0, len(texts))
	for _, text := range texts {
		result = append(result, p.embedOne(text))
	}
	return result, nil
}

// embedOne 单文本生成向量
func (p *LocalHashEmbeddingProvider) embedOne(text string) []float64 {
	text = strings.ToLower(text)
	dim := p.dim
	if dim <= 0 {
		dim = defaultEmbeddingDimension
	}
	vec := make([]float64, dim)
	if text == "" {
		return vec
	}

	// 提取字符 2-gram 并 hash 到向量维度
	grams := extractNGrams(text, 2)
	for _, g := range grams {
		idx := hashString(g) % uint32(dim)
		vec[idx] += 1.0
	}
	// 中文字符按 1-gram 补充
	for _, r := range text {
		if r > 0x4e00 { // CJK 统一汉字
			idx := hashString(string(r)) % uint32(dim)
			vec[idx] += 1.0
		}
	}

	// L2 归一化,便于余弦相似度计算
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

// extractNGrams 提取字符 n-gram
func extractNGrams(text string, n int) []string {
	runes := []rune(text)
	if len(runes) < n {
		return []string{text}
	}
	grams := make([]string, 0, len(runes)-n+1)
	for i := 0; i+n <= len(runes); i++ {
		grams = append(grams, string(runes[i:i+n]))
	}
	return grams
}

// hashString FNV-1a hash
func hashString(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
