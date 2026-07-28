package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/logger"
)

// RerankerProvider 重排序 Provider 接口
// 用 cross-encoder 对 query-doc 对进行精细化打分,显著提升 RAG 召回质量
// 工业级 RAG 标配:vector recall top-N -> rerank -> top-K
type RerankerProvider interface {
	// Name 返回 provider 名称
	Name() string
	// Rerank 对 (query, docs) 打分并按相关度降序返回
	// topK > 0 时只返回前 topK 条,否则返回全部
	Rerank(ctx context.Context, query string, docs []RAGDocument, topK int) ([]RAGDocument, error)
	// Available 是否可用(无 API Key 的降级 provider 返回 false)
	Available() bool
}

// NewRerankerProvider 根据配置创建 Reranker Provider
// 优先级: Rerank API(OpenAI 兼容/Cohere/Jina) > 本地启发式重排 > Noop 跳过
// 配置 LLM_API_KEY 时启用 API 重排,否则使用本地启发式(基于关键词覆盖度+位置加权)
func NewRerankerProvider(cfg config.LLMConfig) RerankerProvider {
	// 有 API Key 时尝试使用 API 重排
	if cfg.APIKey != "" {
		return &APIRerankerProvider{
			apiKey:  cfg.APIKey,
			baseURL: cfg.BaseURL,
			model:   chooseRerankerModel(cfg),
			client:  &http.Client{Timeout: 30 * time.Second},
		}
	}
	// 无 API Key 时使用本地启发式重排(仍能提升精度,只是不如 cross-encoder)
	return &HeuristicRerankerProvider{}
}

// chooseRerankerModel 根据 provider 选择 reranker 模型
// 各厂商 reranker 接口未统一,这里按常见命名约定选择
func chooseRerankerModel(cfg config.LLMConfig) string {
	switch strings.ToLower(cfg.Provider) {
	case "jina":
		return "jina-reranker-v2-base-multilingual"
	case "cohere":
		return "rerank-multilingual-v3.0"
	case "bge", "baichuan":
		return "bge-reranker-large"
	case "glm", "zhipu":
		return "" // 智谱无独立 rerank 接口,APIRerankerProvider 会自动降级
	case "openai", "":
		return "" // OpenAI 无 rerank 接口,自动降级
	default:
		return ""
	}
}

// ============== API Reranker(Cohere/Jina/BGE 兼容协议) ==============

// APIRerankerProvider 调用外部 rerank API 进行 cross-encoder 重排序
// 兼容 Cohere / Jina / BGE 等 rerank 接口(请求/响应格式统一)
// 不支持的 provider(如 OpenAI/GLM)会自动降级到本地启发式重排
type APIRerankerProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func (p *APIRerankerProvider) Name() string { return "api-reranker" }

// Available 模型为空表示该 provider 无 rerank 能力,降级到启发式
func (p *APIRerankerProvider) Available() bool { return p.model != "" && p.apiKey != "" }

// rerankRequest rerank API 请求体(Cohere/Jina 兼容)
type rerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	Model     string   `json:"model,omitempty"`
	TopK      int      `json:"top_n,omitempty"`
}

// rerankResponse rerank API 响应体
type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Score          float64 `json:"score"` // Jina 使用 score 字段
	} `json:"results"`
}

// Rerank 调用 rerank API 重排序
// API 不可用或返回异常时,自动降级到本地启发式重排,保证可用性
func (p *APIRerankerProvider) Rerank(ctx context.Context, query string, docs []RAGDocument, topK int) ([]RAGDocument, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	// 模型未配置(如 OpenAI/GLM),直接降级到启发式
	if !p.Available() {
		hr := &HeuristicRerankerProvider{}
		return hr.Rerank(ctx, query, docs, topK)
	}

	// 构造请求
	docTexts := make([]string, len(docs))
	for i, d := range docs {
		// 截断避免超长(单个文档片段最多 1000 字符)
		docTexts[i] = truncate(d.Title+" "+d.Content, 1000)
	}

	reqBody := rerankRequest{
		Query:     query,
		Documents: docTexts,
		Model:     p.model,
		TopK:      topK,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	url := p.rerankURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		logger.Get().Warnf("rerank API call failed, fallback to heuristic: %v", err)
		hr := &HeuristicRerankerProvider{}
		return hr.Rerank(ctx, query, docs, topK)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Get().Warnf("rerank API returned %d: %s, fallback to heuristic", resp.StatusCode, string(body))
		hr := &HeuristicRerankerProvider{}
		return hr.Rerank(ctx, query, docs, topK)
	}

	var result rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Get().Warnf("decode rerank response failed: %v, fallback to heuristic", err)
		hr := &HeuristicRerankerProvider{}
		return hr.Rerank(ctx, query, docs, topK)
	}

	// 按重排分数组装结果
	reranked := make([]RAGDocument, 0, len(result.Results))
	for _, r := range result.Results {
		if r.Index < 0 || r.Index >= len(docs) {
			continue
		}
		doc := docs[r.Index]
		// 兼容 Cohere(relevance_score) 和 Jina(score) 字段
		score := r.RelevanceScore
		if score == 0 {
			score = r.Score
		}
		doc.Score = score
		reranked = append(reranked, doc)
	}
	if topK > 0 && len(reranked) > topK {
		reranked = reranked[:topK]
	}
	return reranked, nil
}

// rerankURL 根据 baseURL 和模型推断 rerank API 端点
func (p *APIRerankerProvider) rerankURL() string {
	if p.baseURL != "" {
		return strings.TrimRight(p.baseURL, "/") + "/v1/rerank"
	}
	// 默认 Jina rerank 端点
	return "https://api.jina.ai/v1/rerank"
}

// ============== 本地启发式 Reranker(降级方案) ==============

// HeuristicRerankerProvider 无 rerank API 时的降级方案
// 基于 query-doc 关键词覆盖度、命中位置、命中密度进行启发式打分
// 虽不如 cross-encoder 精确,但相比纯向量召回仍能提升精度:
//   - 命中 query 全部关键词的文档加分
//   - 命中位置靠前(标题/首段)的文档加分
//   - 命中关键词密集的文档加分
type HeuristicRerankerProvider struct{}

func (p *HeuristicRerankerProvider) Name() string { return "heuristic-reranker" }

// Available 始终可用(降级方案)
func (p *HeuristicRerankerProvider) Available() bool { return true }

// Rerank 启发式重排序
// 将向量召回分数(余弦相似度)与关键词匹配分数加权融合,再按融合分排序
// 融合公式: final_score = 0.6 * vector_score + 0.4 * keyword_score
func (p *HeuristicRerankerProvider) Rerank(ctx context.Context, query string, docs []RAGDocument, topK int) ([]RAGDocument, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	queryKeywords := tokenize(query)
	keywordSet := make(map[string]bool, len(queryKeywords))
	for _, kw := range queryKeywords {
		keywordSet[kw] = true
	}

	for i := range docs {
		keywordScore := p.heuristicScore(query, keywordSet, docs[i])
		// 融合:向量分(0.6) + 关键词分(0.4)
		vectorScore := docs[i].Score
		if vectorScore < 0 {
			vectorScore = 0
		}
		docs[i].Score = 0.6*vectorScore + 0.4*keywordScore
	}

	// 按融合分降序
	sort.SliceStable(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})

	if topK > 0 && len(docs) > topK {
		docs = docs[:topK]
	}
	return docs, nil
}

// heuristicScore 计算启发式相关度分数(归一化到 [0,1])
// 因子:
//  1. 关键词覆盖率(title/content 任一命中即算覆盖) - 主因子
//  2. 位置加权(标题命中额外加分)
//  3. 命中密度(命中次数 / 文档长度)
func (p *HeuristicRerankerProvider) heuristicScore(query string, keywordSet map[string]bool, doc RAGDocument) float64 {
	if len(keywordSet) == 0 {
		return 0
	}

	titleLower := strings.ToLower(doc.Title)
	contentLower := strings.ToLower(doc.Content)

	// 1. 关键词覆盖率(title 或 content 任一命中即算)
	hitCount := 0
	titleHitCount := 0
	for kw := range keywordSet {
		kwLower := strings.ToLower(kw)
		inContent := strings.Contains(contentLower, kwLower)
		inTitle := strings.Contains(titleLower, kwLower)
		if inContent || inTitle {
			hitCount++
		}
		if inTitle {
			titleHitCount++
		}
	}
	coverage := float64(hitCount) / float64(len(keywordSet))

	// 2. 位置加权(标题命中额外加分)
	titleBonus := float64(titleHitCount) / float64(len(keywordSet)) * 0.3

	// 3. 命中密度(标题命中权重 2x,正文 1x)
	totalHits := 0
	for kw := range keywordSet {
		kwLower := strings.ToLower(kw)
		totalHits += strings.Count(contentLower, kwLower)
		totalHits += strings.Count(titleLower, kwLower) * 2
	}
	docLen := len(doc.Content) + len(doc.Title) + 1
	density := float64(totalHits) / float64(docLen) * 100
	densityScore := math.Min(density/5.0, 1.0) * 0.2

	// 融合(coverage 为主,titleBonus 和 density 为辅)
	score := coverage*0.5 + titleBonus + densityScore
	if score > 1 {
		score = 1
	}
	return score
}

// ============== RRF 融合(用于混合检索) ==============

// RRFParams Reciprocal Rank Fusion 参数
type RRFParams struct {
	K int64 // RRF 平滑常数(标准值 60,Cormack et al. 2009)
}

// DefaultRRFParams 默认 RRF 参数(k=60)
func DefaultRRFParams() RRFParams {
	return RRFParams{K: 60}
}

// RRFusion 使用 Reciprocal Rank Fusion 融合多路检索结果
// 公式: score(d) = sum( 1 / (k + rank_i(d)) )
// 优势:无需归一化不同检索器的分数,仅依赖排名,鲁棒性强
// 输入:多路检索结果(每路已按相关度降序)
// 输出:融合后按 RRF 分数降序的结果
func RRFusion(rankings [][]RAGDocument, params RRFParams, topK int) []RAGDocument {
	if len(rankings) == 0 {
		return nil
	}
	if params.K <= 0 {
		params = DefaultRRFParams()
	}

	// 用 content 作为文档唯一标识(同一文档可能被多路召回)
	type docEntry struct {
		doc   RAGDocument
		score float64
	}
	merged := make(map[string]*docEntry)

	for _, ranking := range rankings {
		for rank, doc := range ranking {
			key := doc.Content
			entry, exists := merged[key]
			if !exists {
				entry = &docEntry{doc: doc}
				merged[key] = entry
			}
			// RRF 分数累加
			entry.score += 1.0 / float64(params.K+int64(rank+1))
			// 保留各路中最高分(用于展示)
			if doc.Score > entry.doc.Score {
				entry.doc.Score = doc.Score
			}
		}
	}

	// 转为切片并按 RRF 分数降序
	result := make([]RAGDocument, 0, len(merged))
	for _, entry := range merged {
		entry.doc.Score = entry.score
		result = append(result, entry.doc)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	if topK > 0 && len(result) > topK {
		result = result[:topK]
	}
	return result
}
