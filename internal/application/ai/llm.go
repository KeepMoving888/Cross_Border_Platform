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
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/shopspring/decimal"
)

// LLMProvider LLM 提供商抽象
type LLMProvider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Message 消息
type Message struct {
	Role       string     `json:"role"` // system / user / assistant / tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool 工具(function calling)
type Tool struct {
	Type     string   `json:"type"` // function
	Function Function `json:"function"`
}

// Function 函数定义
type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice 选项
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage 用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewLLMProvider 根据 config 创建 LLM Provider
// 注意:若 LLM_API_KEY 未配置,所有外部 Provider 均回退至 BuiltinLLMProvider,
// 以保证在离线/演示环境下 AI 工作流仍可返回结构化结果,实现完整业务闭环。
func NewLLMProvider(cfg config.LLMConfig) (LLMProvider, error) {
	// API Key 为空时统一回退至内置 Provider(支持选品/采购/客服/数据/内容多场景)
	if cfg.APIKey == "" {
		logger.Get().Infof("LLM api key not configured, fallback to BuiltinLLMProvider (provider=%s)", cfg.Provider)
		return NewBuiltinLLMProvider(), nil
	}
	switch strings.ToLower(cfg.Provider) {
	case "glm", "zhipu", "bigmodel":
		return NewGLMProvider(cfg), nil
	case "claude", "anthropic":
		return NewClaudeProvider(cfg), nil
	case "deepseek":
		return NewDeepSeekProvider(cfg), nil
	case "qwen", "tongyi":
		return NewQwenProvider(cfg), nil
	case "openai", "gpt":
		return NewOpenAICompatibleProvider(cfg), nil
	case "builtin", "offline", "":
		return NewBuiltinLLMProvider(), nil
	default:
		// 默认走 OpenAI 兼容协议(国内大部分模型都兼容)
		return NewOpenAICompatibleProvider(cfg), nil
	}
}

// ============== GLM(智谱) ==============

type GLMProvider struct {
	cfg    config.LLMConfig
	client *http.Client
}

func NewGLMProvider(cfg config.LLMConfig) *GLMProvider {
	return &GLMProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *GLMProvider) Name() string { return "glm" }

func (p *GLMProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("glm api key not configured")
	}
	if req.Model == "" {
		req.Model = p.cfg.Model
	}
	if req.Model == "" {
		req.Model = "glm-4-plus"
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	return doHTTPChat(ctx, p.client, url, p.cfg.APIKey, req)
}

// ============== Claude(Anthropic) ==============

type ClaudeProvider struct {
	cfg    config.LLMConfig
	client *http.Client
}

func NewClaudeProvider(cfg config.LLMConfig) *ClaudeProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-5-20250929"
	}
	return &ClaudeProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("claude api key not configured")
	}
	if req.Model == "" {
		req.Model = p.cfg.Model
	}

	// Claude 协议与 OpenAI 兼容:很多代理服务(如 one-api)已转为 OpenAI 兼容格式
	// 这里直接走 OpenAI 兼容接口,降低复杂度
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	return doHTTPChat(ctx, p.client, url, p.cfg.APIKey, req)
}

// ============== DeepSeek ==============

type DeepSeekProvider struct {
	cfg    config.LLMConfig
	client *http.Client
}

func NewDeepSeekProvider(cfg config.LLMConfig) *DeepSeekProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	return &DeepSeekProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *DeepSeekProvider) Name() string { return "deepseek" }

func (p *DeepSeekProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("deepseek api key not configured")
	}
	if req.Model == "" {
		req.Model = p.cfg.Model
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	return doHTTPChat(ctx, p.client, url, p.cfg.APIKey, req)
}

// ============== Qwen(通义千问) ==============

type QwenProvider struct {
	cfg    config.LLMConfig
	client *http.Client
}

func NewQwenProvider(cfg config.LLMConfig) *QwenProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen-plus"
	}
	return &QwenProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *QwenProvider) Name() string { return "qwen" }

func (p *QwenProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("qwen api key not configured")
	}
	if req.Model == "" {
		req.Model = p.cfg.Model
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	return doHTTPChat(ctx, p.client, url, p.cfg.APIKey, req)
}

// ============== OpenAI 兼容协议(通用) ==============

type OpenAICompatibleProvider struct {
	cfg    config.LLMConfig
	client *http.Client
}

func NewOpenAICompatibleProvider(cfg config.LLMConfig) *OpenAICompatibleProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	return &OpenAICompatibleProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OpenAICompatibleProvider) Name() string { return "openai" }

func (p *OpenAICompatibleProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("api key not configured")
	}
	if req.Model == "" {
		req.Model = p.cfg.Model
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	return doHTTPChat(ctx, p.client, url, p.cfg.APIKey, req)
}

// ============== Builtin(无 key 时兜底,保证流程可跑) ==============
// 内置离线 Provider,仅在未配置 LLM API Key 时使用,返回结构固定结果以便本地开发与流程联调

type BuiltinLLMProvider struct{}

func NewBuiltinLLMProvider() *BuiltinLLMProvider { return &BuiltinLLMProvider{} }
func (p *BuiltinLLMProvider) Name() string       { return "builtin" }

func (p *BuiltinLLMProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	logger.Get().Warn("using builtin llm provider, please configure LLM_API_KEY for production")

	// 根据最后一条 user message 生成响应
	var lastUserMsg string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserMsg = req.Messages[i].Content
			break
		}
	}

	// 根据输入内容智能生成结构化结果(基于关键词匹配)
	builtinOutput := generateBuiltinResponse(lastUserMsg, req.Messages)

	return &ChatResponse{
		ID:    "builtin-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Model: req.Model,
		Choices: []Choice{
			{
				Index:        0,
				Message:      Message{Role: "assistant", Content: builtinOutput},
				FinishReason: "stop",
			},
		},
		Usage: Usage{PromptTokens: 100, CompletionTokens: 80, TotalTokens: 180},
	}, nil
}

// generateBuiltinResponse 基于输入内容生成结构化响应
// 按场景关键词匹配,返回符合前端 StructuredResult 解析的 JSON 格式
func generateBuiltinResponse(userMsg string, messages []Message) string {
	systemPrompt := ""
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		}
	}

	lowerMsg := strings.ToLower(userMsg)
	combined := systemPrompt + " " + userMsg

	// === Text2SQL 场景(优先级最高,系统提示中包含 "SQL 专家" 或 "只输出一条 SELECT 语句") ===
	// Text2SQLNode 要求 LLM 返回纯 SQL 文本(非 JSON),否则 SQLExecuteNode 的 SELECT 检查会失败
	if strings.Contains(systemPrompt, "SQL 专家") ||
		strings.Contains(systemPrompt, "只输出一条 SELECT 语句") ||
		strings.Contains(systemPrompt, "将用户问题翻译为") {
		return generateSQLByQuestion(userMsg)
	}

	// === 选品分析场景 ===
	if strings.Contains(combined, "选品") || strings.Contains(combined, "分析以下选品") ||
		strings.Contains(lowerMsg, "product") || strings.Contains(combined, "类目") {
		return generateProductAnalysisJSON(userMsg)
	}

	// === 采购助手场景 ===
	if strings.Contains(combined, "采购") || strings.Contains(combined, "供应商") ||
		strings.Contains(combined, "报价") || strings.Contains(lowerMsg, "purchase") {
		return generatePurchaseAssistantJSON(userMsg)
	}

	// === 数据分析场景(必须在客服场景之前,因为 user 消息可能含 "question" 关键字) ===
	// 注意:两种子场景
	// 1) Text2SQL 节点(系统提示含"SQL 专家"):已在前面拦截,返回纯 SQL
	// 2) 数据分析 LLM 节点(系统提示含"数据分析专家" 或 "基于 SQL 查询结果"):返回含 insight 的 JSON
	if strings.Contains(systemPrompt, "数据分析专家") ||
		strings.Contains(systemPrompt, "基于 SQL 查询结果") {
		return generateDataInsightJSON(userMsg)
	}
	if strings.Contains(combined, "数据分析") || strings.Contains(combined, "SQL") ||
		strings.Contains(lowerMsg, "data") || strings.Contains(combined, "近30天") {
		return generateDataAnalysisJSON(userMsg)
	}

	// === 客服回复场景 ===
	if strings.Contains(combined, "客服") || strings.Contains(combined, "客户") ||
		strings.Contains(lowerMsg, "customer") || strings.Contains(combined, "question") {
		return generateCustomerServiceJSON(userMsg)
	}

	// === 内容生成场景 ===
	if strings.Contains(combined, "Listing") || strings.Contains(combined, "内容生成") ||
		strings.Contains(lowerMsg, "content") || strings.Contains(combined, "描述") {
		return generateContentJSON(userMsg)
	}

	// === 默认兜底 ===
	return fmt.Sprintf(`{
  "score": 78.5,
  "reason": "基于市场数据分析,该选品具有中等偏上的市场潜力。",
  "risks": ["季节性波动", "头部卖家垄断", "广告成本上升"],
  "suggestion": "建议小批量测试,关注转化率与广告 ACOS",
  "input_received": %q
}`, truncate(userMsg, 200))
}

// 解析输入参数(从 user message 中提取 key=value 格式或 JSON)
func parseInputParams(userMsg string) map[string]string {
	params := make(map[string]string)
	// 尝试 JSON 解析
	if strings.HasPrefix(strings.TrimSpace(userMsg), "{") {
		return params // JSON 格式交给上游处理
	}
	// 解析 key:value 或 key=value 格式
	for _, line := range strings.Split(userMsg, "\n") {
		for _, sep := range []string{":", "="} {
			if idx := strings.Index(line, sep); idx > 0 {
				k := strings.TrimSpace(line[:idx])
				v := strings.TrimSpace(line[idx+1:])
				if k != "" {
					params[k] = v
				}
			}
		}
	}
	return params
}

func generateProductAnalysisJSON(userMsg string) string {
	// 尝试从输入中提取参数
	params := parseInputParams(userMsg)
	sku := getParam(params, "sku", "PC-HD-001")
	name := getParam(params, "name", "负离子恒温高速吹风机")
	category := getParam(params, "category", "beauty_device")
	priceStr := getParam(params, "list_price", "39.99")
	costStr := getParam(params, "est_cost_price", "14.50")
	salesStr := getParam(params, "monthly_sales", "1200")

	price := parseFloat(priceStr, 39.99)
	cost := parseFloat(costStr, 14.50)
	sales := parseInt(salesStr, 1200)
	margin := 0.0
	if price > 0 {
		margin = (price - cost) / price * 100
	}

	// 基于参数计算评分(模拟 LLM 推理)
	score := 62.0 + margin*0.35 + math.Min(float64(sales)/80, 18)
	if score > 96 {
		score = 96
	}
	if score < 55 {
		score = 55
	}
	scoreVal := math.Round(score*10) / 10

	recommendation := "不建议"
	suggestion := "建议暂缓投入,优先优化成本或切换更有空间的细分场景。"
	if scoreVal >= 80 {
		recommendation = "推荐进入"
		suggestion = "建议进入测试阶段:先做小批量打样 + Listing 预热,观察 14 天广告 ROAS。"
	} else if scoreVal >= 65 {
		recommendation = "谨慎进入"
		suggestion = "建议补充竞品评论痛点与成本结构后再决策,优先验证差异化卖点。"
	}

	marginStr := fmt.Sprintf("%.1f", margin)
	salesStr2 := fmt.Sprintf("%d", sales)

	return fmt.Sprintf(`{
  "score": %.1f,
  "recommendation": %q,
  "reasons": [
    "%s(%s)预估毛利率约 %s%%,具备基础利润空间",
    "月销参考 %s 单,需求侧信号%s",
    "%s 赛道在美欧市场仍有搜索增长,可做差异化卖点包装"
  ],
  "risks": [
    "头部品牌价格战可能导致广告成本上升",
    "需关注目标市场认证与售后配件供应",
    "若与现有 SKU 高度同质,可能出现内部竞争"
  ],
  "suggestion": %q,
  "metrics": {
    "est_margin_rate": %s,
    "list_price": %.2f,
    "est_cost_price": %.2f,
    "monthly_sales": %d
  }
}`, scoreVal, recommendation, name, sku, marginStr, salesStr2,
		map[bool]string{true: "较强", false: "中等"}[sales >= 800],
		category, suggestion, marginStr, price, cost, sales)
}

func generatePurchaseAssistantJSON(userMsg string) string {
	params := parseInputParams(userMsg)
	orderNo := getParam(params, "order_no", "PO-0001")
	unitStr := getParam(params, "unit_price", "18.50")
	qtyStr := getParam(params, "quantity", "500")

	unit := parseFloat(unitStr, 18.50)
	qty := parseInt(qtyStr, 500)
	marketAvg := unit * 1.04
	reasonable := unit <= marketAvg*1.02
	deliveryRisk := "low"
	if qty >= 1000 {
		deliveryRisk = "medium"
	}

	negotiation := fmt.Sprintf("报价接近市场均价 $%.2f,建议锁定 2 个月长单换取 2%%-3%% 折扣,并约定分批发货。", marketAvg)
	if !reasonable {
		negotiation = fmt.Sprintf("报价偏高,建议以市场均价 $%.2f 为锚点压价,或要求供应商承担部分头程/质检费用。", marketAvg)
	}

	return fmt.Sprintf(`{
  "price_reasonable": %v,
  "market_avg": %.2f,
  "quote_price": %.2f,
  "quantity": %d,
  "delivery_risk": %q,
  "negotiation": %q,
  "checklist": [
    "核验样品一致性(订单 %s)",
    "确认交期与违约条款",
    "预留 5%% 质检损耗"
  ]
}`, reasonable, marketAvg, unit, qty, deliveryRisk, negotiation, orderNo)
}

func generateCustomerServiceJSON(userMsg string) string {
	params := parseInputParams(userMsg)
	question := getParam(params, "question", "Does this hair dryer support dual voltage?")
	lang := getParam(params, "language", "en")
	if lang == "" {
		lang = "en"
	}

	reply := "Hi, thanks for your question. This hair dryer supports dual voltage 110V/220V and is suitable for global travel. If you need accessories or order support, please share your order ID."
	if lang == "zh" || lang == "cn" {
		reply = "您好,这款高速吹风机支持 110V/220V 双电压,适合全球旅行使用。如需更多配件信息,请告知您的订单号,我们会优先协助。"
	}

	intent := "general_inquiry"
	qLower := strings.ToLower(question)
	if strings.Contains(qLower, "voltage") || strings.Contains(question, "电压") {
		intent = "product_spec"
	} else if strings.Contains(qLower, "return") || strings.Contains(question, "退货") {
		intent = "after_sales"
	} else if strings.Contains(qLower, "shipping") || strings.Contains(question, "发货") {
		intent = "logistics"
	}

	return fmt.Sprintf(`{
  "reply": %q,
  "language": %q,
  "confidence": 0.91,
  "intent": %q,
  "suggested_actions": [
    "发送说明书 PDF",
    "推荐兼容配件",
    "创建售后工单"
  ]
}`, reply, lang, intent)
}

func generateDataAnalysisJSON(userMsg string) string {
	params := parseInputParams(userMsg)
	question := getParam(params, "question", "近30天哪些SKU利润最高?")

	return fmt.Sprintf(`{
  "sql": "SELECT sku, SUM(revenue) AS revenue, SUM(net_profit) AS net_profit FROM profit_reports WHERE stat_date >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY sku ORDER BY net_profit DESC LIMIT 5",
  "result": [
    {"sku": "PC-HD-001", "revenue": 286400, "net_profit": 81200},
    {"sku": "IPL-HAIR-PRO", "revenue": 241800, "net_profit": 73450},
    {"sku": "BS-EMS-02", "revenue": 168200, "net_profit": 42100}
  ],
  "insight": "针对问题「%s」:近 30 天利润贡献最高的是个护与脱毛仪 SKU,建议加大高毛利 SKU 广告预算,并复盘低毛利 SKU 的广告结构。"
}`, question)
}

// generateDataInsightJSON 数据分析 LLM 节点输出:基于 SQL 查询结果生成业务洞察
// userMsg 通常包含 "用户问题:xxx\nSQL 结果:[...]" 格式的上下文
func generateDataInsightJSON(userMsg string) string {
	// 尝试从 userMsg 中提取问题
	question := "用户提问"
	if idx := strings.Index(userMsg, "问题:"); idx >= 0 {
		rest := userMsg[idx+len("问题:"):]
		if endIdx := strings.Index(rest, "\n"); endIdx >= 0 {
			question = strings.TrimSpace(rest[:endIdx])
		} else {
			question = strings.TrimSpace(rest)
		}
	}

	// 根据 userMsg 中的关键字推断分析维度
	lower := strings.ToLower(userMsg)
	insight := ""
	switch {
	case strings.Contains(lower, "利润") || strings.Contains(lower, "profit"):
		insight = fmt.Sprintf("针对问题「%s」:近 30 天利润贡献 Top SKU 集中在个护电器与美容仪器赛道,毛利率维持在 35%%-55%% 区间。建议:1)对 Top 3 SKU 加大广告预算,扩量优先;2)对长尾 SKU 复盘广告结构,压降 ACOS;3)结合库存周转避免断货损失。", question)
	case strings.Contains(lower, "库存") || strings.Contains(lower, "inventory"):
		insight = fmt.Sprintf("针对问题「%s」:当前存在多仓低库存预警,深圳主仓与美西海外仓缺口较大。建议:1)对断货 SKU 紧急补货,优先发 FBA 闪电补货;2)对低周转 SKU 调整安全库存阈值;3)同步供应商锁价长单,降低采购成本。", question)
	case strings.Contains(lower, "采购") || strings.Contains(lower, "purchase"):
		insight = fmt.Sprintf("针对问题「%s」:近 30 天采购单集中在头部 A 级供应商,履约率 94%%-98%%。建议:1)对 delay 状态订单加催,触发违约条款;2)对 B 级供应商引入备选,降低单点依赖;3)对账单差异 >2%% 的供应商启动对账复核。", question)
	case strings.Contains(lower, "供应商") || strings.Contains(lower, "supplier"):
		insight = fmt.Sprintf("针对问题「%s」:A 级供应商贡献 78%% 采购额,质量率均 >97%%。建议:1)续签 A 级供应商年度框架,锁定价格;2)对 C 级供应商启动淘汰或整改;3)引入 2-3 家备选供应商,提升议价能力。", question)
	default:
		insight = fmt.Sprintf("针对问题「%s」:基于查询结果,建议聚焦 Top 表现 SKU,加大资源投入;同时对底部表现复盘归因,优化选品与运营策略。", question)
	}

	return fmt.Sprintf(`{
  "insight": %q,
  "summary": "已完成 SQL 查询并基于结果生成业务洞察",
  "confidence": 0.88,
  "recommended_actions": [
    "加大 Top SKU 广告预算",
    "复盘长尾 SKU 广告结构",
    "锁定 A 级供应商长单价格"
  ]
}`, insight)
}

// generateSQLByQuestion 根据自然语言问题生成纯 SQL 文本(供 Text2SQLNode 使用)
// 注意:返回值必须是纯 SQL(以 SELECT 开头),不能包含 JSON 包装或 Markdown 标记
func generateSQLByQuestion(userMsg string) string {
	params := parseInputParams(userMsg)
	question := strings.ToLower(getParam(params, "question", userMsg))

	// 按问题关键词路由到不同 SQL 模板(均为 SELECT 语句,严格遵循 schema)
	switch {
	case strings.Contains(question, "利润") || strings.Contains(question, "profit") || strings.Contains(question, "sku"):
		// 利润相关:近30天 Top SKU 利润
		return "SELECT sku, SUM(revenue) AS revenue, SUM(net_profit) AS net_profit, ROUND(SUM(net_profit)/SUM(revenue)*100, 2) AS margin_rate FROM profit_reports WHERE stat_date >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY sku ORDER BY net_profit DESC LIMIT 10"

	case strings.Contains(question, "库存") || strings.Contains(question, "inventory") || strings.Contains(question, "断货"):
		// 库存相关:低库存预警
		return "SELECT p.sku, p.name, w.name AS warehouse, i.available_qty, i.safety_stock, (i.safety_stock - i.available_qty) AS shortfall FROM inventories i JOIN products p ON p.sku = i.sku JOIN warehouses w ON w.id = i.warehouse_id WHERE i.available_qty < i.safety_stock ORDER BY shortfall DESC LIMIT 20"

	case strings.Contains(question, "采购") || strings.Contains(question, "purchase") || strings.Contains(question, "订单"):
		// 采购相关:近30天采购单状态
		return "SELECT order_no, sku, supplier_id, quantity, unit_price, total_amount, status, created_at FROM purchase_orders WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) ORDER BY created_at DESC LIMIT 50"

	case strings.Contains(question, "供应商") || strings.Contains(question, "supplier"):
		// 供应商相关:供应商评级与履约
		return "SELECT name, code, rating, on_time_rate, quality_rate, total_amount, coop_status FROM suppliers WHERE deleted_at IS NULL ORDER BY total_amount DESC LIMIT 20"

	case strings.Contains(question, "账单") || strings.Contains(question, "bill") || strings.Contains(question, "对账"):
		// 财务对账
		return "SELECT bill_no, supplier_id, payable_amount, paid_amount, diff_amount, status, created_at FROM bills WHERE created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) ORDER BY created_at DESC LIMIT 50"

	case strings.Contains(question, "选品") || strings.Contains(question, "product") || strings.Contains(question, "ai_score"):
		// 选品 AI 评分排行
		return "SELECT sku, name, category, ai_score, monthly_sales, list_price, est_cost_price, stage FROM products WHERE deleted_at IS NULL ORDER BY ai_score DESC LIMIT 20"

	default:
		// 默认:近30天利润 Top 10
		return "SELECT sku, SUM(revenue) AS revenue, SUM(net_profit) AS net_profit, ROUND(SUM(net_profit)/SUM(revenue)*100, 2) AS margin_rate FROM profit_reports WHERE stat_date >= DATE_SUB(CURDATE(), INTERVAL 30 DAY) GROUP BY sku ORDER BY net_profit DESC LIMIT 10"
	}
}

func generateContentJSON(userMsg string) string {
	params := parseInputParams(userMsg)
	name := getParam(params, "name", "负离子恒温高速吹风机")
	sku := getParam(params, "sku", "PC-HD-001")

	return fmt.Sprintf(`{
  "title": "%s - Ionic High-Speed Hair Dryer, Fast Dry & Heat Protect (%s)",
  "bullets": [
    "1800W high-speed motor dries hair in minutes with less heat damage",
    "Negative ion technology for smoother, shinier hair finish",
    "Constant temperature control helps protect scalp and hair cuticle",
    "Dual voltage 110V/220V design ideal for home and travel",
    "Includes concentrator nozzle and storage bag for daily convenience"
  ],
  "description": "Upgrade your daily routine with a salon-level high-speed dryer. Engineered for fast drying, heat protection, and travel-ready dual voltage, it balances performance and comfort for modern beauty lifestyles.",
  "keywords": ["ionic hair dryer", "high speed dryer", "heat protect", "dual voltage", "travel hair dryer"]
}`, name, sku)
}

func getParam(params map[string]string, key, defaultVal string) string {
	if v, ok := params[key]; ok && v != "" {
		return v
	}
	return defaultVal
}

func parseFloat(s string, defaultVal float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return defaultVal
	}
	return f
}

func parseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err != nil {
		return defaultVal
	}
	return i
}

// ============== 通用 HTTP 调用 ==============

func doHTTPChat(ctx context.Context, client *http.Client, url, apiKey string, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w, body: %s", err, string(respBody))
	}

	return &chatResp, nil
}

// EstimateCost 估算调用成本(美元)
func EstimateCost(provider string, usage Usage) decimal.Decimal {
	// 粗略单价(每 1K tokens 美元)
	pricing := map[string]struct{ in, out float64 }{
		"glm":      {0.005, 0.005},
		"claude":   {0.003, 0.015},
		"deepseek": {0.001, 0.002},
		"qwen":     {0.004, 0.012},
		"openai":   {0.005, 0.015},
		"builtin":  {0, 0},
	}
	p, ok := pricing[provider]
	if !ok {
		p = pricing["openai"]
	}
	cost := float64(usage.PromptTokens)/1000*p.in + float64(usage.CompletionTokens)/1000*p.out
	return decimal.NewFromFloat(cost).Round(6)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
