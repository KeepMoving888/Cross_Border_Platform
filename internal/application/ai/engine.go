package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// WorkflowResult 工作流执行结果(对齐前端 AIWorkflowRunResult)
type WorkflowResult struct {
	WorkflowID   uint                   `json:"workflow_id"`
	WorkflowCode string                 `json:"workflow_code,omitempty"`
	RunID        string                 `json:"run_id"`
	Status       string                 `json:"status"` // success / failed / partial
	Output       string                 `json:"output"`
	Metrics      map[string]interface{} `json:"metrics,omitempty"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
	Tokens       int                    `json:"tokens"`
	Cost         decimal.Decimal        `json:"cost"`
	Duration     int64                  `json:"duration_ms"`
	FinishedAt   string                 `json:"finished_at"`
}

// Engine 工作流执行引擎
type Engine struct {
	db          *gorm.DB
	llmProvider LLMProvider
	registry    map[string]NodeFactory
	mu          sync.RWMutex
}

var (
	globalEngine *Engine
	once         sync.Once
)

// GetEngine 获取全局引擎实例
func GetEngine(db *gorm.DB) *Engine {
	once.Do(func() {
		globalEngine = NewEngine(db)
	})
	if globalEngine.db == nil {
		globalEngine.db = db
	}
	return globalEngine
}

// NewEngine 创建引擎
func NewEngine(db *gorm.DB) *Engine {
	e := &Engine{
		db:       db,
		registry: make(map[string]NodeFactory),
	}
	e.registerDefaults()
	return e
}

// NodeFactory 节点工厂函数
type NodeFactory func(def NodeDefinition) (Node, error)

// NodeDefinition 节点定义
type NodeDefinition struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Name   string                 `json:"name"`
	Config map[string]interface{} `json:"config"`
}

// Node 节点接口
type Node interface {
	Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
	Type() string
}

// WorkflowDefinition 工作流定义
type WorkflowDefinition struct {
	Nodes []NodeDefinition `json:"nodes"`
	Edges []Edge           `json:"edges"`
}

// Edge 边定义
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}

// registerDefaults 注册默认节点类型
func (e *Engine) registerDefaults() {
	e.registry["input"] = func(def NodeDefinition) (Node, error) {
		return &InputNode{def: def}, nil
	}
	e.registry["output"] = func(def NodeDefinition) (Node, error) {
		return &OutputNode{def: def}, nil
	}
	e.registry["llm"] = func(def NodeDefinition) (Node, error) {
		return &LLMNode{def: def, provider: e.llmProvider}, nil
	}
	e.registry["rag"] = func(def NodeDefinition) (Node, error) {
		return &RAGNode{def: def, db: e.db}, nil
	}
	e.registry["condition"] = func(def NodeDefinition) (Node, error) {
		return &ConditionNode{def: def}, nil
	}
	e.registry["text2sql"] = func(def NodeDefinition) (Node, error) {
		return &Text2SQLNode{def: def, provider: e.llmProvider, db: e.db}, nil
	}
	e.registry["sql_execute"] = func(def NodeDefinition) (Node, error) {
		return &SQLExecuteNode{def: def, db: e.db}, nil
	}
	e.registry["tool"] = func(def NodeDefinition) (Node, error) {
		return &ToolNode{def: def, db: e.db}, nil
	}
}

// SetLLMProvider 设置 LLM Provider
func (e *Engine) SetLLMProvider(p LLMProvider) {
	e.llmProvider = p
}

// EnsureProvider 确保有 LLM provider(惰性初始化)
func (e *Engine) EnsureProvider() LLMProvider {
	if e.llmProvider != nil {
		return e.llmProvider
	}
	cfg := config.Get()
	p, err := NewLLMProvider(cfg.LLM)
	if err != nil {
		logger.Get().Warnf("init llm provider failed, fallback to builtin: %v", err)
		p = NewBuiltinLLMProvider()
	}
	e.llmProvider = p
	return p
}

// RunWorkflowByCode 通过 code 执行工作流
func (e *Engine) RunWorkflowByCode(ctx context.Context, code string, input map[string]interface{}, operatorID uint) (*WorkflowResult, error) {
	var wf models.AIWorkflow
	if err := e.db.Where("code = ?", code).First(&wf).Error; err != nil {
		return nil, fmt.Errorf("workflow %s not found: %w", code, err)
	}
	if wf.Status != "enabled" {
		return nil, fmt.Errorf("workflow %s is disabled", code)
	}
	return e.executeWorkflow(ctx, &wf, input, operatorID)
}

// RunWorkflowByID 通过 ID 执行工作流
func (e *Engine) RunWorkflowByID(ctx context.Context, id uint, input map[string]interface{}, operatorID uint) (*WorkflowResult, error) {
	var wf models.AIWorkflow
	if err := e.db.First(&wf, id).Error; err != nil {
		return nil, fmt.Errorf("workflow %d not found: %w", id, err)
	}
	return e.executeWorkflow(ctx, &wf, input, operatorID)
}

// Execute 执行工作流(对外暴露的执行入口,供调度器等外部调用)
func (e *Engine) Execute(ctx context.Context, wf *models.AIWorkflow, input map[string]interface{}, operatorID uint) (*WorkflowResult, error) {
	if wf.Status != "enabled" {
		return nil, fmt.Errorf("workflow %s is disabled", wf.Code)
	}
	return e.executeWorkflow(ctx, wf, input, operatorID)
}

// executeWorkflow 执行工作流核心逻辑
func (e *Engine) executeWorkflow(ctx context.Context, wf *models.AIWorkflow, input map[string]interface{}, operatorID uint) (*WorkflowResult, error) {
	start := time.Now()

	// 创建执行记录
	run := &models.AIWorkflowRun{
		WorkflowID:   wf.ID,
		WorkflowCode: wf.Code,
		TriggerType:  "manual",
		Input:        toJSON(input),
		Status:       "running",
		OperatorID:   &operatorID,
		StartedAt:    &start,
	}
	if err := e.db.Create(run).Error; err != nil {
		return nil, fmt.Errorf("create run record failed: %w", err)
	}

	// 解析工作流定义
	def, err := parseWorkflowDefinition(wf.Definition)
	if err != nil {
		e.failRun(run, fmt.Errorf("parse definition failed: %w", err), start)
		return nil, err
	}

	// 构建节点图
	nodes, err := e.buildNodes(def)
	if err != nil {
		e.failRun(run, err, start)
		return nil, err
	}

	// 拓扑执行
	output, extra, tokens, cost, execErr := e.walkAndExecute(ctx, def, nodes, input, wf)

	// 更新执行记录
	end := time.Now()
	run.Duration = end.Sub(start).Milliseconds()
	run.Output = toJSON(output)
	run.TotalTokens = tokens
	run.PromptTokens = tokens * 3 / 4
	run.CompletionTokens = tokens / 4
	run.Cost = cost
	run.CompletedAt = &end

	if execErr != nil {
		e.failRun(run, execErr, start)
		return &WorkflowResult{
			WorkflowID:   wf.ID,
			WorkflowCode: wf.Code,
			RunID:        fmt.Sprintf("%d", run.ID),
			Status:       "failed",
			Output:       fmt.Sprintf("error: %v", execErr),
			Metrics:      buildMetrics(extra, tokens, cost),
			Extra:        extra,
			Tokens:       tokens,
			Cost:         cost,
			Duration:     run.Duration,
			FinishedAt:   end.Format(time.RFC3339),
		}, execErr
	}

	run.Status = "success"
	e.db.Model(run).Updates(map[string]interface{}{
		"status":            run.Status,
		"output":            run.Output,
		"duration_ms":       run.Duration,
		"total_tokens":      run.TotalTokens,
		"prompt_tokens":     run.PromptTokens,
		"completion_tokens": run.CompletionTokens,
		"cost":              run.Cost,
		"completed_at":      run.CompletedAt,
		"error":             "",
	})

	return &WorkflowResult{
		WorkflowID:   wf.ID,
		WorkflowCode: wf.Code,
		RunID:        fmt.Sprintf("%d", run.ID),
		Status:       "success",
		Output:       toJSONString(output),
		Metrics:      buildMetrics(extra, tokens, cost),
		Extra:        extra,
		Tokens:       tokens,
		Cost:         cost,
		Duration:     run.Duration,
		FinishedAt:   end.Format(time.RFC3339),
	}, nil
}

// buildMetrics 构造前端展示用的 metrics 字段
func buildMetrics(extra map[string]interface{}, tokens int, cost decimal.Decimal) map[string]interface{} {
	m := map[string]interface{}{
		"tokens": tokens,
		"cost":   cost.String(),
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// buildNodes 构建节点实例
func (e *Engine) buildNodes(def *WorkflowDefinition) (map[string]Node, error) {
	nodes := make(map[string]Node, len(def.Nodes))
	for _, nd := range def.Nodes {
		factory, ok := e.registry[nd.Type]
		if !ok {
			return nil, fmt.Errorf("unknown node type: %s", nd.Type)
		}
		node, err := factory(nd)
		if err != nil {
			return nil, fmt.Errorf("build node %s failed: %w", nd.ID, err)
		}
		nodes[nd.ID] = node
	}
	return nodes, nil
}

// walkAndExecute 拓扑遍历执行
func (e *Engine) walkAndExecute(ctx context.Context, def *WorkflowDefinition, nodes map[string]Node, input map[string]interface{}, wf *models.AIWorkflow) (output map[string]interface{}, extra map[string]interface{}, tokens int, cost decimal.Decimal, err error) {
	// 找到入口节点(input 类型)
	var startID string
	for _, n := range def.Nodes {
		if n.Type == "input" {
			startID = n.ID
			break
		}
	}
	if startID == "" && len(def.Nodes) > 0 {
		startID = def.Nodes[0].ID
	}

	// 构建邻接表
	adj := make(map[string][]Edge)
	for _, e := range def.Edges {
		adj[e.From] = append(adj[e.From], e)
	}

	// 顺序执行(简化版,不做并行)
	currentID := startID
	context := map[string]interface{}{}
	for k, v := range input {
		context[k] = v
	}
	extra = map[string]interface{}{}

	executed := make(map[string]bool)
	steps := 0
	maxSteps := 50

	for currentID != "" && steps < maxSteps {
		steps++
		if executed[currentID] {
			break
		}
		executed[currentID] = true

		node, ok := nodes[currentID]
		if !ok {
			err = fmt.Errorf("node %s not found", currentID)
			return
		}

		logger.Get().Debugf("executing node %s (type=%s)", currentID, node.Type())

		// 执行节点
		result, execErr := node.Execute(ctx, context)
		if execErr != nil {
			err = fmt.Errorf("node %s execute failed: %w", currentID, execErr)
			return
		}

		// 合并结果到 context
		for k, v := range result {
			context[k] = v
		}

		// 统计 token 和 cost
		if r, ok := node.(*LLMNode); ok {
			tokens += r.lastTokens
			cost = cost.Add(r.lastCost)
		}
		if r, ok := node.(*Text2SQLNode); ok {
			tokens += r.lastTokens
			cost = cost.Add(r.lastCost)
		}

		// 如果是 output 节点,记录输出
		if node.Type() == "output" {
			output = result
		}

		// 找下一个节点
		edges := adj[currentID]
		if len(edges) == 0 {
			break
		}
		nextID := ""
		for _, edge := range edges {
			if edge.Condition != "" {
				// 简单条件判断
				if evalCondition(edge.Condition, context) {
					nextID = edge.To
					break
				}
			} else {
				nextID = edge.To
				break
			}
		}
		currentID = nextID
	}

	// 如果没有显式 output,使用最后 context
	if output == nil {
		output = context
	}

	// 尝试从输出中提取结构化字段到 extra
	if s, ok := output["output"].(string); ok {
		extra["output"] = s
		// 尝试解析 JSON
		if strings.HasPrefix(strings.TrimSpace(s), "{") {
			var m map[string]interface{}
			if json.Unmarshal([]byte(s), &m) == nil {
				for k, v := range m {
					extra[k] = v
				}
			}
		}
	}

	return
}

// failRun 标记执行失败
func (e *Engine) failRun(run *models.AIWorkflowRun, err error, start time.Time) {
	end := time.Now()
	run.Status = "failed"
	run.Error = err.Error()
	run.Duration = end.Sub(start).Milliseconds()
	run.CompletedAt = &end
	e.db.Model(run).Updates(map[string]interface{}{
		"status":       run.Status,
		"error":        run.Error,
		"duration_ms":  run.Duration,
		"completed_at": run.CompletedAt,
	})
}

// parseWorkflowDefinition 解析工作流定义
func parseWorkflowDefinition(s string) (*WorkflowDefinition, error) {
	if s == "" {
		// 默认简单工作流:input -> llm -> output
		return &WorkflowDefinition{
			Nodes: []NodeDefinition{
				{ID: "start", Type: "input"},
				{ID: "llm", Type: "llm"},
				{ID: "end", Type: "output"},
			},
			Edges: []Edge{
				{From: "start", To: "llm"},
				{From: "llm", To: "end"},
			},
		}, nil
	}
	var def WorkflowDefinition
	if err := json.Unmarshal([]byte(s), &def); err != nil {
		return nil, fmt.Errorf("invalid workflow definition: %w", err)
	}
	return &def, nil
}

// evalCondition 简单条件求值(支持 key=value 格式)
func evalCondition(cond string, ctx map[string]interface{}) bool {
	parts := strings.SplitN(cond, "=", 2)
	if len(parts) != 2 {
		return true
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if v, ok := ctx[key]; ok {
		return fmt.Sprintf("%v", v) == val
	}
	return false
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func toJSONString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
