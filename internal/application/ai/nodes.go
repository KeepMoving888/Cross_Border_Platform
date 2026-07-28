package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/config"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ============== Input / Output 节点 ==============

// InputNode 输入节点
type InputNode struct {
	def NodeDefinition
}

func (n *InputNode) Type() string { return "input" }

func (n *InputNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// 直接透传输入
	return input, nil
}

// OutputNode 输出节点
type OutputNode struct {
	def NodeDefinition
}

func (n *OutputNode) Type() string { return "output" }

func (n *OutputNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	output := map[string]interface{}{}
	// 如果配置了字段映射,按映射输出
	if mapping, ok := n.def.Config["mapping"].(map[string]interface{}); ok {
		for outKey, inKey := range mapping {
			if k, ok := inKey.(string); ok {
				output[outKey] = input[k]
			}
		}
	} else {
		// 默认透传
		for k, v := range input {
			output[k] = v
		}
	}
	// 确保 output 字段存在
	if _, ok := output["output"]; !ok {
		if v, ok := input["llm_output"]; ok {
			output["output"] = v
		} else if v, ok := input["output"]; ok {
			output["output"] = v
		}
	}
	return output, nil
}

// ============== LLM 节点 ==============

// LLMNode 大模型节点
type LLMNode struct {
	def        NodeDefinition
	provider   LLMProvider
	lastTokens int
	lastCost   decimal.Decimal
}

func (n *LLMNode) Type() string { return "llm" }

func (n *LLMNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	provider := n.provider
	if provider == nil {
		// 通过全局引擎获取
		provider = GetEngine(getDB()).EnsureProvider()
	}

	// 构建 prompt
	systemPrompt, _ := n.def.Config["system_prompt"].(string)
	userPromptTpl, _ := n.def.Config["user_prompt"].(string)
	if userPromptTpl == "" {
		userPromptTpl, _ = n.def.Config["prompt"].(string)
	}

	// 渲染用户 prompt 模板
	userPrompt, err := renderTemplate(userPromptTpl, input)
	if err != nil {
		return nil, fmt.Errorf("render prompt failed: %w", err)
	}

	// 模型参数
	model, _ := n.def.Config["model"].(string)
	temperature := 0.7
	if t, ok := n.def.Config["temperature"].(float64); ok {
		temperature = t
	}
	maxTokens := 2000
	if t, ok := n.def.Config["max_tokens"].(float64); ok {
		maxTokens = int(t)
	}
	if t, ok := n.def.Config["max_tokens"].(int); ok {
		maxTokens = t
	}

	messages := []Message{}
	if systemPrompt != "" {
		messages = append(messages, Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, Message{Role: "user", Content: userPrompt})

	req := ChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	resp, err := provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	content := resp.Choices[0].Message.Content
	n.lastTokens = resp.Usage.TotalTokens
	// 修复 cost 计算
	n.lastCost = EstimateCost(provider.Name(), resp.Usage)

	// 返回结果
	result := map[string]interface{}{
		"llm_output": content,
		"output":     content,
		"tokens":     resp.Usage.TotalTokens,
		"model":      resp.Model,
	}

	// 尝试解析 JSON
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// 提取 JSON 块(可能被 ```json ... ``` 包裹)
		jsonStr := extractJSON(content)
		if jsonStr != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
				result["parsed"] = parsed
				if m, ok := parsed.(map[string]interface{}); ok {
					for k, v := range m {
						result[k] = v
					}
				}
			}
		}
	}

	return result, nil
}

// ============== RAG 节点 ==============

// RAGSearcher RAG 检索接口(解耦 RAGNode 与具体实现)
// 进程内调用时使用 RAGService,微服务模式下使用 RemoteRAGClient(HTTP)
type RAGSearcher interface {
	Search(query string, knowledgeBaseID uint, topK int) ([]RAGDocument, error)
}

// RAGNode 检索增强生成节点
type RAGNode struct {
	def        NodeDefinition
	ragService RAGSearcher
}

func (n *RAGNode) Type() string { return "rag" }

func (n *RAGNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// 查询关键词
	query, _ := input["question"].(string)
	if query == "" {
		if q, ok := input["query"].(string); ok {
			query = q
		}
	}
	if query == "" {
		// 没有查询词,直接透传
		return map[string]interface{}{"rag_context": ""}, nil
	}

	// 从节点配置读取知识库 ID 和 topK
	kbID := uint(0)
	if id, ok := n.def.Config["knowledge_base_id"].(float64); ok {
		kbID = uint(id)
	}
	topK := 5
	if k, ok := n.def.Config["top_k"].(float64); ok {
		topK = int(k)
	}

	// 调用 RAGService 检索(优先向量检索,降级 TF-IDF)
	docs, err := n.ragService.Search(query, kbID, topK)
	if err != nil {
		return nil, fmt.Errorf("rag search failed: %w", err)
	}

	// 拼接上下文
	var sb strings.Builder
	for i, d := range docs {
		sb.WriteString(fmt.Sprintf("[文档%d] %s\n%s\n\n", i+1, d.Title, d.Content))
	}
	contextStr := sb.String()

	return map[string]interface{}{
		"rag_context":   contextStr,
		"rag_documents": docs,
		"query":         query,
		"doc_count":     len(docs),
	}, nil
}

// ============== Condition 节点 ==============

// ConditionNode 条件分支节点
type ConditionNode struct {
	def NodeDefinition
}

func (n *ConditionNode) Type() string { return "condition" }

func (n *ConditionNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// 条件求值结果由 walkAndExecute 处理,这里只透传
	return input, nil
}

// ============== Text2SQL 节点 ==============

// Text2SQLNode 自然语言转 SQL
type Text2SQLNode struct {
	def        NodeDefinition
	provider   LLMProvider
	db         *gorm.DB
	lastTokens int
	lastCost   decimal.Decimal
}

func (n *Text2SQLNode) Type() string { return "text2sql" }

func (n *Text2SQLNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	question, _ := input["question"].(string)
	if question == "" {
		return nil, fmt.Errorf("question is required for text2sql")
	}

	provider := n.provider
	if provider == nil {
		provider = GetEngine(n.db).EnsureProvider()
	}

	// 数据库 schema 描述(简化,实际应动态获取)
	schemaDesc := `可查询的表:
- products(id, sku, name, category, stage, platform, list_price, est_cost_price, ai_score, monthly_sales, created_at)
- purchase_orders(id, order_no, sku, supplier_id, quantity, unit_price, total_amount, status, created_at)
- inventories(id, sku, warehouse_id, available_qty, locked_qty, in_transit_qty, safety_stock)
- profit_reports(id, sku, platform, revenue, goods_cost, freight_cost, platform_fee, ad_cost, net_profit, margin_rate, stat_date)
- bills(id, bill_no, supplier_id, payable_amount, paid_amount, diff_amount, status, created_at)`

	systemPrompt := fmt.Sprintf(`你是一位 SQL 专家。基于以下数据库 Schema,将用户问题翻译为 MySQL 查询 SQL。

%s

规则:
1. 只输出一条 SELECT 语句,不要修改数据
2. 不要输出任何解释,只输出 SQL
3. 使用合理的别名和 ORDER BY
4. 限制结果最多 100 行(LIMIT 100)`, schemaDesc)

	req := ChatRequest{
		Model: config.Get().LLM.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: question},
		},
		Temperature: 0.1,
		MaxTokens:   500,
	}

	resp, err := provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	sql := strings.TrimSpace(resp.Choices[0].Message.Content)
	// 去除可能的 markdown 标记
	sql = strings.TrimPrefix(sql, "```sql")
	sql = strings.TrimPrefix(sql, "```")
	sql = strings.TrimSuffix(sql, "```")
	sql = strings.TrimSpace(sql)

	n.lastTokens = resp.Usage.TotalTokens
	n.lastCost = EstimateCost(provider.Name(), resp.Usage)

	return map[string]interface{}{
		"sql":    sql,
		"tokens": resp.Usage.TotalTokens,
	}, nil
}

// ============== SQL Execute 节点 ==============

// SQLExecuteNode SQL 执行节点(仅 SELECT)
type SQLExecuteNode struct {
	def NodeDefinition
	db  *gorm.DB
}

func (n *SQLExecuteNode) Type() string { return "sql_execute" }

func (n *SQLExecuteNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	sql, _ := input["sql"].(string)
	if sql == "" {
		return nil, fmt.Errorf("sql is required")
	}

	// 安全检查:只允许 SELECT
	trimmed := strings.TrimSpace(strings.ToUpper(sql))
	if !strings.HasPrefix(trimmed, "SELECT") {
		return nil, fmt.Errorf("only SELECT statements are allowed")
	}
	if strings.Contains(trimmed, "INSERT") || strings.Contains(trimmed, "UPDATE") ||
		strings.Contains(trimmed, "DELETE") || strings.Contains(trimmed, "DROP") ||
		strings.Contains(trimmed, "ALTER") || strings.Contains(trimmed, "TRUNCATE") {
		return nil, fmt.Errorf("dangerous sql detected")
	}

	var results []map[string]interface{}
	rows, err := n.db.Raw(sql).Rows()
	if err != nil {
		return nil, fmt.Errorf("sql execute failed: %w", err)
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	for rows.Next() {
		row := make(map[string]interface{}, len(cols))
		values := make([]interface{}, len(cols))
		pointers := make([]interface{}, len(cols))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			continue
		}
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return map[string]interface{}{
		"result": results,
		"count":  len(results),
		"sql":    sql,
	}, nil
}

// ============== Tool 节点(可扩展) ==============

// ToolNode 工具节点(调用内部函数)
type ToolNode struct {
	def NodeDefinition
	db  *gorm.DB
}

func (n *ToolNode) Type() string { return "tool" }

func (n *ToolNode) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	toolName, _ := n.def.Config["tool_name"].(string)
	switch toolName {
	case "get_product":
		return n.getProduct(input)
	case "get_supplier":
		return n.getSupplier(input)
	case "calculate_margin":
		return n.calculateMargin(input)
	default:
		return input, nil
	}
}

func (n *ToolNode) getProduct(input map[string]interface{}) (map[string]interface{}, error) {
	idVal, ok := input["product_id"]
	if !ok {
		return nil, fmt.Errorf("product_id is required")
	}
	var p models.Product
	if err := n.db.First(&p, idVal).Error; err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"product": p,
	}, nil
}

func (n *ToolNode) getSupplier(input map[string]interface{}) (map[string]interface{}, error) {
	idVal, ok := input["supplier_id"]
	if !ok {
		return nil, fmt.Errorf("supplier_id is required")
	}
	var s models.Supplier
	if err := n.db.First(&s, idVal).Error; err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"supplier": s,
	}, nil
}

func (n *ToolNode) calculateMargin(input map[string]interface{}) (map[string]interface{}, error) {
	priceStr, _ := input["price"].(string)
	costStr, _ := input["cost"].(string)
	if priceStr == "" || costStr == "" {
		return nil, fmt.Errorf("price and cost are required")
	}
	price, err := decimalFromString(priceStr)
	if err != nil {
		return nil, err
	}
	cost, err := decimalFromString(costStr)
	if err != nil {
		return nil, err
	}
	if price.IsZero() {
		return nil, fmt.Errorf("price cannot be zero")
	}
	margin := price.Sub(cost).Div(price).Mul(decimalFromInt(100))
	return map[string]interface{}{
		"margin_rate":     margin.String(),
		"profit_per_unit": price.Sub(cost).String(),
	}, nil
}

// ============== 辅助函数 ==============

func renderTemplate(tpl string, data map[string]interface{}) (string, error) {
	if tpl == "" {
		// 没有模板,把输入 JSON 序列化
		b, _ := json.Marshal(data)
		return string(b), nil
	}
	t, err := template.New("prompt").Parse(tpl)
	if err != nil {
		return tpl, nil
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return tpl, nil
	}
	return sb.String(), nil
}

func extractJSON(s string) string {
	// 去除 markdown 代码块
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		var sb strings.Builder
		started := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "```") {
				if started {
					break
				}
				started = true
				continue
			}
			if started {
				sb.WriteString(line + "\n")
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return s
}

// getDB 获取 DB(用于节点回查)
func getDB() *gorm.DB {
	// 通过全局引擎获取
	if globalEngine != nil {
		return globalEngine.db
	}
	return nil
}
