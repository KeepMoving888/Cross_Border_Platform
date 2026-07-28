package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// AIWorkflow AI 工作流定义
type AIWorkflow struct {
	BaseModel
	Code        string `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name        string `gorm:"size:128;not null" json:"name"`
	Description string `gorm:"size:512" json:"description"`
	// 类型:agent / rag / automation / text2sql
	Type string `gorm:"size:32;index" json:"type"`
	// 业务场景:product_analysis / purchase_assistant / customer_service / content_generation / data_analysis
	Scene string `gorm:"size:64;index" json:"scene"`
	// 工作流定义 JSON(节点+边)
	Definition string `gorm:"type:longtext" json:"definition"`
	// Prompt 模板
	PromptTemplate string `gorm:"type:text" json:"prompt_template"`
	// 输入参数 schema(JSON Schema)
	InputSchema string `gorm:"type:text" json:"input_schema"`
	// 输出参数 schema
	OutputSchema string `gorm:"type:text" json:"output_schema"`
	// 关联模型
	Provider    string          `gorm:"size:32;default:glm" json:"provider"` // glm / claude / deepseek / qwen
	Model       string          `gorm:"size:64" json:"model"`
	Temperature decimal.Decimal `gorm:"type:decimal(3,2);default:0.70" json:"temperature"`
	MaxTokens   int             `gorm:"default:2000" json:"max_tokens"`
	// 状态:enabled / disabled
	Status  string `gorm:"size:16;index;default:enabled" json:"status"`
	Version int    `gorm:"default:1" json:"version"`
}

func (AIWorkflow) TableName() string { return "ai_workflows" }

// AIWorkflowRun 工作流执行记录
type AIWorkflowRun struct {
	BaseModel
	WorkflowID   uint   `gorm:"index;not null" json:"workflow_id"`
	WorkflowCode string `gorm:"size:64;index" json:"workflow_code"`
	// 触发类型:manual / scheduled / event
	TriggerType string `gorm:"size:32" json:"trigger_type"`
	// 输入参数 JSON
	Input string `gorm:"type:text" json:"input"`
	// 输出结果 JSON
	Output string `gorm:"type:longtext" json:"output"`
	// 执行状态:running / success / failed / timeout
	Status string `gorm:"size:32;index;default:running" json:"status"`
	// 错误信息
	Error string `gorm:"type:text" json:"error,omitempty"`
	// 执行耗时(毫秒)
	Duration int64 `gorm:"column:duration_ms" json:"duration_ms"`
	// Token 用量
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// 调用成本(美元)
	Cost decimal.Decimal `gorm:"type:decimal(10,6);default:0" json:"cost"`
	// 操作人
	OperatorID *uint `gorm:"index" json:"operator_id,omitempty"`
	// 关联业务对象
	RefType     string     `gorm:"size:32" json:"ref_type,omitempty"`
	RefID       string     `gorm:"size:64;index" json:"ref_id,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (AIWorkflowRun) TableName() string { return "ai_workflow_runs" }

// KnowledgeBase 知识库(RAG)
type KnowledgeBase struct {
	BaseModel
	Name        string `gorm:"size:128;not null" json:"name"`
	Code        string `gorm:"size:64;uniqueIndex" json:"code"`
	Description string `gorm:"size:512" json:"description"`
	// 类型:product_manual / purchase_contract / faq / operation_guide
	Type string `gorm:"size:32;index" json:"type"`
	// 嵌入模型
	EmbeddingModel string `gorm:"size:64;default:text-embedding-ada-002" json:"embedding_model"`
	// 向量维度
	Dimension int `gorm:"default:1536" json:"dimension"`
	// 文档数量
	DocumentCount int    `gorm:"default:0" json:"document_count"`
	Status        string `gorm:"size:16;default:enabled" json:"status"`
}

func (KnowledgeBase) TableName() string { return "knowledge_bases" }

// KnowledgeDocument 知识文档
type KnowledgeDocument struct {
	BaseModel
	KnowledgeBaseID uint   `gorm:"index;not null" json:"knowledge_base_id"`
	Title           string `gorm:"size:255;not null" json:"title"`
	Source          string `gorm:"size:255" json:"source"` // 文件路径或 URL
	// 内容
	Content string `gorm:"type:longtext" json:"content"`
	// 分块数量
	ChunkCount int `gorm:"default:0" json:"chunk_count"`
	// 状态:processing / ready / failed
	Status string `gorm:"size:16;default:processing" json:"status"`
	Error  string `gorm:"type:text" json:"error,omitempty"`
}

func (KnowledgeDocument) TableName() string { return "knowledge_documents" }

// PromptTemplate Prompt 模板
type PromptTemplate struct {
	BaseModel
	Code  string `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name  string `gorm:"size:128" json:"name"`
	Scene string `gorm:"size:64;index" json:"scene"`
	// 系统提示词
	SystemPrompt string `gorm:"type:text" json:"system_prompt"`
	// 用户提示词模板(支持变量 {{.Var}})
	UserPrompt string `gorm:"type:text" json:"user_prompt"`
	// 输入变量定义 JSON
	Variables string `gorm:"type:text" json:"variables"`
	// 输出格式:text / json
	OutputFormat string `gorm:"size:16;default:text" json:"output_format"`
	Version      int    `gorm:"default:1" json:"version"`
	Status       string `gorm:"size:16;default:enabled" json:"status"`
}

func (PromptTemplate) TableName() string { return "prompt_templates" }
