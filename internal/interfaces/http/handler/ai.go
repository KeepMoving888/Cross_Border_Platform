package handler

import (
	"encoding/json"
	"strconv"

	"github.com/cb-platform/internal/application/ai"
	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type AIHandler struct {
	db *gorm.DB
}

func NewAIHandler(db *gorm.DB) *AIHandler {
	return &AIHandler{db: db}
}

// ============== 工作流管理 ==============

type listWorkflowQuery struct {
	Pagination
	Keyword string `form:"keyword"`
	Scene   string `form:"scene"`
	Type    string `form:"type"`
	Status  string `form:"status"`
}

func (h *AIHandler) ListWorkflows(c *gin.Context) {
	var q listWorkflowQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.AIWorkflow{})
	if q.Keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Scene != "" {
		query = query.Where("scene = ?", q.Scene)
	}
	if q.Type != "" {
		query = query.Where("type = ?", q.Type)
	}
	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}

	var total int64
	query.Count(&total)

	var list []models.AIWorkflow
	query.Order("id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *AIHandler) GetWorkflow(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var wf models.AIWorkflow
	if err := h.db.First(&wf, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	// 最近执行记录
	var recentRuns []models.AIWorkflowRun
	h.db.Where("workflow_id = ?", id).Order("id DESC").Limit(10).Find(&recentRuns)
	response.OK(c, gin.H{"workflow": wf, "recent_runs": recentRuns})
}

type createWorkflowRequest struct {
	Code           string          `json:"code" binding:"required,max=64"`
	Name           string          `json:"name" binding:"required,max=128"`
	Description    string          `json:"description"`
	Type           string          `json:"type" binding:"oneof=agent rag automation text2sql"`
	Scene          string          `json:"scene"`
	Definition     string          `json:"definition"`
	PromptTemplate string          `json:"prompt_template"`
	InputSchema    string          `json:"input_schema"`
	OutputSchema   string          `json:"output_schema"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	Temperature    decimal.Decimal `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	Status         string          `json:"status"`
}

func (h *AIHandler) CreateWorkflow(c *gin.Context) {
	var req createWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	// 检查 code 重复
	var count int64
	h.db.Model(&models.AIWorkflow{}).Where("code = ?", req.Code).Count(&count)
	if count > 0 {
		response.FailWithCode(c, errors.ErrDuplicateEntry)
		return
	}

	wf := models.AIWorkflow{
		Code:           req.Code,
		Name:           req.Name,
		Description:    req.Description,
		Type:           req.Type,
		Scene:          req.Scene,
		Definition:     req.Definition,
		PromptTemplate: req.PromptTemplate,
		InputSchema:    req.InputSchema,
		OutputSchema:   req.OutputSchema,
		Provider:       req.Provider,
		Model:          req.Model,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		Status:         "enabled",
		Version:        1,
	}
	if wf.Provider == "" {
		wf.Provider = "glm"
	}
	if wf.Temperature.IsZero() {
		wf.Temperature = decimal.NewFromFloat(0.7)
	}
	if wf.MaxTokens == 0 {
		wf.MaxTokens = 2000
	}
	if req.Status != "" {
		wf.Status = req.Status
	}

	if err := h.db.Create(&wf).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建工作流失败"))
		return
	}
	response.OKWithMsg(c, "工作流创建成功", wf)
}

func (h *AIHandler) UpdateWorkflow(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req createWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"name":            req.Name,
		"description":     req.Description,
		"type":            req.Type,
		"scene":           req.Scene,
		"definition":      req.Definition,
		"prompt_template": req.PromptTemplate,
		"input_schema":    req.InputSchema,
		"output_schema":   req.OutputSchema,
		"provider":        req.Provider,
		"model":           req.Model,
		"temperature":     req.Temperature,
		"max_tokens":      req.MaxTokens,
		"status":          req.Status,
	}
	if err := h.db.Model(&models.AIWorkflow{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	// 版本递增
	h.db.Model(&models.AIWorkflow{}).Where("id = ?", id).
		UpdateColumn("version", gorm.Expr("version + 1"))
	var wf models.AIWorkflow
	h.db.First(&wf, id)
	response.OKWithMsg(c, "工作流更新成功", wf)
}

type runWorkflowRequest struct {
	Input    map[string]interface{} `json:"input"`
	Workflow string                 `json:"workflow"` // 可选,通过 code 执行
}

// RunWorkflow 执行工作流
func (h *AIHandler) RunWorkflow(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req runWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	if req.Input == nil {
		req.Input = map[string]interface{}{}
	}

	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	engine := ai.GetEngine(h.db)

	result, err := engine.RunWorkflowByID(c.Request.Context(), uint(id), req.Input, uint(userID))
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "工作流执行失败"))
		return
	}

	// 记录 Prometheus 指标
	middleware.AIWorkflowRunsTotal.WithLabelValues(result.WorkflowCode, result.Status).Inc()
	if result.Duration > 0 {
		middleware.AIWorkflowDuration.WithLabelValues(result.WorkflowCode).Observe(float64(result.Duration) / 1000.0)
	}
	if result.Tokens > 0 {
		middleware.AIWorkflowTokens.WithLabelValues(result.WorkflowCode).Observe(float64(result.Tokens))
	}
	if result.Cost.IsPositive() {
		middleware.AIWorkflowCost.WithLabelValues(result.WorkflowCode).Observe(result.Cost.InexactFloat64())
	}

	response.OKWithMsg(c, "工作流执行完成", result)
}

// ============== 执行历史 ==============

type listRunQuery struct {
	Pagination
	WorkflowCode string `form:"workflow_code"`
	Status       string `form:"status"`
	OperatorID   string `form:"operator_id"`
}

func (h *AIHandler) ListRuns(c *gin.Context) {
	var q listRunQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.AIWorkflowRun{})
	if q.WorkflowCode != "" {
		query = query.Where("workflow_code = ?", q.WorkflowCode)
	}
	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}
	if q.OperatorID != "" {
		query = query.Where("operator_id = ?", q.OperatorID)
	}

	var total int64
	query.Count(&total)

	var list []models.AIWorkflowRun
	query.Order("id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *AIHandler) GetRun(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var run models.AIWorkflowRun
	if err := h.db.First(&run, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	response.OK(c, run)
}

// ============== Prompt 模板 ==============

func (h *AIHandler) ListPrompts(c *gin.Context) {
	var list []models.PromptTemplate
	query := h.db.Model(&models.PromptTemplate{})
	if scene := c.Query("scene"); scene != "" {
		query = query.Where("scene = ?", scene)
	}
	query.Order("id DESC").Find(&list)
	response.OK(c, list)
}

type createPromptRequest struct {
	Code         string `json:"code" binding:"required,max=64"`
	Name         string `json:"name" binding:"max=128"`
	Scene        string `json:"scene"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
	Variables    string `json:"variables"`
	OutputFormat string `json:"output_format"`
}

func (h *AIHandler) CreatePrompt(c *gin.Context) {
	var req createPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	pt := models.PromptTemplate{
		Code:         req.Code,
		Name:         req.Name,
		Scene:        req.Scene,
		SystemPrompt: req.SystemPrompt,
		UserPrompt:   req.UserPrompt,
		Variables:    req.Variables,
		OutputFormat: req.OutputFormat,
		Version:      1,
		Status:       "enabled",
	}
	if pt.OutputFormat == "" {
		pt.OutputFormat = "text"
	}
	if err := h.db.Create(&pt).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建 Prompt 模板失败"))
		return
	}
	response.OKWithMsg(c, "创建成功", pt)
}

func (h *AIHandler) UpdatePrompt(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req createPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"name":          req.Name,
		"scene":         req.Scene,
		"system_prompt": req.SystemPrompt,
		"user_prompt":   req.UserPrompt,
		"variables":     req.Variables,
		"output_format": req.OutputFormat,
	}
	if err := h.db.Model(&models.PromptTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	h.db.Model(&models.PromptTemplate{}).Where("id = ?", id).
		UpdateColumn("version", gorm.Expr("version + 1"))
	var pt models.PromptTemplate
	h.db.First(&pt, id)
	response.OKWithMsg(c, "更新成功", pt)
}

// ============== 知识库 ==============

func (h *AIHandler) ListKnowledgeBases(c *gin.Context) {
	var list []models.KnowledgeBase
	h.db.Find(&list)
	response.OK(c, list)
}

type createKBRequest struct {
	Name        string `json:"name" binding:"required,max=128"`
	Code        string `json:"code" binding:"max=64"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

func (h *AIHandler) CreateKnowledgeBase(c *gin.Context) {
	var req createKBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	kb := models.KnowledgeBase{
		Name:           req.Name,
		Code:           req.Code,
		Description:    req.Description,
		Type:           req.Type,
		EmbeddingModel: "text-embedding-ada-002",
		Dimension:      1536,
		Status:         "enabled",
	}
	if err := h.db.Create(&kb).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建知识库失败"))
		return
	}
	response.OKWithMsg(c, "知识库创建成功", kb)
}

type uploadDocRequest struct {
	Title   string `json:"title" binding:"required,max=255"`
	Source  string `json:"source"`
	Content string `json:"content" binding:"required"`
}

func (h *AIHandler) UploadDocument(c *gin.Context) {
	kbID, _ := strconv.Atoi(c.Param("id"))
	var req uploadDocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	doc := models.KnowledgeDocument{
		KnowledgeBaseID: uint(kbID),
		Title:           req.Title,
		Source:          req.Source,
		Content:         req.Content,
		ChunkCount:      1,
		Status:          "ready",
	}
	if err := h.db.Create(&doc).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	// 更新知识库文档计数
	h.db.Model(&models.KnowledgeBase{}).Where("id = ?", kbID).
		UpdateColumn("document_count", gorm.Expr("document_count + 1"))
	response.OKWithMsg(c, "文档上传成功", doc)
}

func (h *AIHandler) ListDocuments(c *gin.Context) {
	kbID, _ := strconv.Atoi(c.Param("id"))
	var list []models.KnowledgeDocument
	h.db.Where("knowledge_base_id = ?", kbID).Order("id DESC").Find(&list)
	response.OK(c, list)
}

// ============== 业务场景直调 ==============

type analyzeProductRequest struct {
	ProductID uint `json:"product_id" binding:"required"`
}

func (h *AIHandler) AnalyzeProduct(c *gin.Context) {
	var req analyzeProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	var p models.Product
	if err := h.db.First(&p, req.ProductID).Error; err != nil {
		response.FailWithCode(c, errors.ErrProductNotFound)
		return
	}

	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	engine := ai.GetEngine(h.db)
	result, err := engine.RunWorkflowByCode(c.Request.Context(), "wf_product_analysis",
		map[string]interface{}{
			"category": p.Category,
			"market":   p.TargetMarket,
			"product":  p.Name,
			"price":    p.ListPrice.String(),
		}, uint(userID))
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "AI 分析失败"))
		return
	}

	// 更新商品 AI 评分
	if score, ok := result.Extra["score"].(float64); ok {
		p.AIScore = decimal.NewFromFloat(score)
		h.db.Model(&p).Update("ai_score", p.AIScore)
	}
	if insight, ok := result.Extra["reason"].(string); ok {
		p.AIInsight = insight
		h.db.Model(&p).Update("ai_insight", p.AIInsight)
	}
	response.OKWithMsg(c, "AI 选品分析完成", gin.H{"product": p, "result": result})
}

type generateListingRequest struct {
	ProductName string `json:"product_name" binding:"required"`
	Features    string `json:"features"`
	Keywords    string `json:"keywords"`
	Market      string `json:"market"`
}

func (h *AIHandler) GenerateListing(c *gin.Context) {
	var req generateListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	engine := ai.GetEngine(h.db)
	result, err := engine.RunWorkflowByCode(c.Request.Context(), "wf_content_generation",
		map[string]interface{}{
			"product_name": req.ProductName,
			"features":     req.Features,
			"keywords":     req.Keywords,
			"market":       req.Market,
		}, uint(userID))
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "Listing 生成失败"))
		return
	}
	response.OKWithMsg(c, "Listing 生成完成", result)
}

type replyCustomerRequest struct {
	Product  string `json:"product" binding:"required"`
	Question string `json:"question" binding:"required"`
	Language string `json:"language"`
}

func (h *AIHandler) ReplyCustomer(c *gin.Context) {
	var req replyCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	if req.Language == "" {
		req.Language = "English"
	}
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	engine := ai.GetEngine(h.db)
	result, err := engine.RunWorkflowByCode(c.Request.Context(), "wf_customer_service",
		map[string]interface{}{
			"product":  req.Product,
			"question": req.Question,
			"language": req.Language,
		}, uint(userID))
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "客服回复生成失败"))
		return
	}
	response.OKWithMsg(c, "客服回复生成完成", result)
}

// ============== RAG 知识库检索 ==============

// RAGSearch RAG 知识库检索
func (h *AIHandler) RAGSearch(c *gin.Context) {
	var req struct {
		Query           string `json:"query"`
		KnowledgeBaseID uint   `json:"knowledge_base_id"`
		TopK            int    `json:"top_k"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	ragService := ai.NewRAGService(h.db)
	docs, err := ragService.Search(req.Query, req.KnowledgeBaseID, req.TopK)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9003, "RAG 检索失败"))
		return
	}
	response.OK(c, gin.H{"documents": docs, "total": len(docs)})
}

// 确保 json 包被使用
var _ = json.Marshal
