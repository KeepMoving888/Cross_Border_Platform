package handler

import (
	"strconv"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/cb-platform/internal/application/ai"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type ProductHandler struct {
	db *gorm.DB
}

func NewProductHandler(db *gorm.DB) *ProductHandler {
	return &ProductHandler{db: db}
}

type listProductQuery struct {
	Pagination
	Keyword   string `form:"keyword"`
	Category  string `form:"category"`
	Stage     string `form:"stage"`
	Platform  string `form:"platform"`
	MinScore  string `form:"min_score"`
	OwnerID   string `form:"owner_id"`
}

// List 选品列表
func (h *ProductHandler) List(c *gin.Context) {
	var q listProductQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.Product{})
	if q.Keyword != "" {
		query = query.Where("name LIKE ? OR sku LIKE ? OR asin LIKE ?",
			"%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Category != "" {
		query = query.Where("category = ?", q.Category)
	}
	if q.Stage != "" {
		query = query.Where("stage = ?", q.Stage)
	}
	if q.Platform != "" {
		query = query.Where("platform = ?", q.Platform)
	}
	if q.OwnerID != "" {
		query = query.Where("owner_id = ?", q.OwnerID)
	}
	if q.MinScore != "" {
		if minScore, err := decimal.NewFromString(q.MinScore); err == nil {
			query = query.Where("ai_score >= ?", minScore)
		}
	}

	var total int64
	query.Count(&total)

	var list []models.Product
	query.Order("ai_score DESC, id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)

	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *ProductHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var p models.Product
	if err := h.db.First(&p, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrProductNotFound)
		return
	}
	response.OK(c, p)
}

type createProductRequest struct {
	SKU          string          `json:"sku" binding:"required,max=64"`
	ASIN         string          `json:"asin" binding:"max=32"`
	Name         string          `json:"name" binding:"required,max=255"`
	ImageURL     string          `json:"image_url" binding:"max=512"`
	Category     string          `json:"category" binding:"max=128"`
	SubCategory  string          `json:"sub_category" binding:"max=128"`
	Stage        string          `json:"stage" binding:"omitempty,oneof=sourcing testing approved rejected archived"`
	Platform     string          `json:"platform" binding:"max=32"`
	TargetMarket string          `json:"target_market" binding:"max=64"`
	ListPrice    decimal.Decimal `json:"list_price"`
	EstCostPrice decimal.Decimal `json:"est_cost_price"`
	Currency     string          `json:"currency"`
	MonthlySales int             `json:"monthly_sales"`
	ReviewCount  int             `json:"review_count"`
	Rating       decimal.Decimal `json:"rating"`
	Tags         string          `json:"tags" binding:"max=512"`
	Remark       string          `json:"remark"`
	SupplierID   *uint           `json:"supplier_id"`
}

func (h *ProductHandler) Create(c *gin.Context) {
	var req createProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	// SKU 重复检查
	var count int64
	h.db.Model(&models.Product{}).Where("sku = ?", req.SKU).Count(&count)
	if count > 0 {
		response.FailWithCode(c, errors.ErrDuplicateEntry)
		return
	}

	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	p := models.Product{
		SKU:          req.SKU,
		ASIN:         req.ASIN,
		Name:         req.Name,
		ImageURL:     req.ImageURL,
		Category:     req.Category,
		SubCategory:  req.SubCategory,
		Stage:        req.Stage,
		Platform:     req.Platform,
		TargetMarket: req.TargetMarket,
		ListPrice:    req.ListPrice,
		EstCostPrice: req.EstCostPrice,
		Currency:     req.Currency,
		MonthlySales: req.MonthlySales,
		ReviewCount:  req.ReviewCount,
		Rating:       req.Rating,
		Tags:         req.Tags,
		Remark:       req.Remark,
		SupplierID:   req.SupplierID,
		OwnerID:      uint(userID),
		AIScore:      decimal.Zero,
		EstMarginRate: decimal.Zero,
	}
	if p.Stage == "" {
		p.Stage = models.ProductStageSourcing
	}
	if p.Currency == "" {
		p.Currency = "USD"
	}
	// 计算预估毛利率
	if p.ListPrice.GreaterThan(decimal.Zero) && p.EstCostPrice.GreaterThan(decimal.Zero) {
		margin := p.ListPrice.Sub(p.EstCostPrice).Div(p.ListPrice).Mul(decimal.NewFromInt(100))
		p.EstMarginRate = margin.Round(2)
	}

	if err := h.db.Create(&p).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建商品失败"))
		return
	}
	response.OKWithMsg(c, "创建成功", p)
}

type updateProductRequest struct {
	Name         string          `json:"name"`
	ImageURL     string          `json:"image_url"`
	Category     string          `json:"category"`
	SubCategory  string          `json:"sub_category"`
	Platform     string          `json:"platform"`
	TargetMarket string          `json:"target_market"`
	ListPrice    decimal.Decimal `json:"list_price"`
	EstCostPrice decimal.Decimal `json:"est_cost_price"`
	Currency     string          `json:"currency"`
	MonthlySales int             `json:"monthly_sales"`
	ReviewCount  int             `json:"review_count"`
	Rating       decimal.Decimal `json:"rating"`
	Tags         string          `json:"tags"`
	Remark       string          `json:"remark"`
	SupplierID   *uint           `json:"supplier_id"`
}

func (h *ProductHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var req updateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	updates := map[string]interface{}{
		"name":          req.Name,
		"image_url":     req.ImageURL,
		"category":      req.Category,
		"sub_category":  req.SubCategory,
		"platform":      req.Platform,
		"target_market": req.TargetMarket,
		"list_price":    req.ListPrice,
		"est_cost_price": req.EstCostPrice,
		"currency":      req.Currency,
		"monthly_sales": req.MonthlySales,
		"review_count":  req.ReviewCount,
		"rating":        req.Rating,
		"tags":          req.Tags,
		"remark":        req.Remark,
		"supplier_id":   req.SupplierID,
	}

	// 重算毛利率
	if req.ListPrice.GreaterThan(decimal.Zero) && req.EstCostPrice.GreaterThan(decimal.Zero) {
		margin := req.ListPrice.Sub(req.EstCostPrice).Div(req.ListPrice).Mul(decimal.NewFromInt(100))
		updates["est_margin_rate"] = margin.Round(2)
	}

	if err := h.db.Model(&models.Product{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var p models.Product
	h.db.First(&p, id)
	response.OKWithMsg(c, "更新成功", p)
}

func (h *ProductHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	if err := h.db.Delete(&models.Product{}, id).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "删除成功", nil)
}

type changeStageRequest struct {
	Stage   string `json:"stage" binding:"required,oneof=sourcing testing approved rejected archived"`
	Remark  string `json:"remark"`
}

// ChangeStage 变更选品阶段
func (h *ProductHandler) ChangeStage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var req changeStageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	updates := map[string]interface{}{
		"stage": req.Stage,
	}
	if req.Stage == models.ProductStageApproved || req.Stage == models.ProductStageRejected {
		now := timeNow()
		updates["decided_at"] = &now
	}

	if err := h.db.Model(&models.Product{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var p models.Product
	h.db.First(&p, id)
	response.OKWithMsg(c, "阶段变更成功", p)
}

// Analyze AI 选品分析
func (h *ProductHandler) Analyze(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}

	var p models.Product
	if err := h.db.First(&p, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrProductNotFound)
		return
	}

	// 调用 AI 工作流
	engine := ai.GetEngine(h.db)
	result, err := engine.RunWorkflowByCode(c, "wf_product_analysis", map[string]interface{}{
		"category":   p.Category,
		"market":     p.TargetMarket,
		"product":    p.Name,
		"price":      p.ListPrice.String(),
		"sku":        p.SKU,
	}, uint(0))
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "AI 分析失败"))
		return
	}

	// 解析评分并更新
	if score, ok := result.Extra["score"].(float64); ok {
		p.AIScore = decimal.NewFromFloat(score)
	}
	if insight, ok := result.Extra["reason"].(string); ok {
		p.AIInsight = insight
	}
	h.db.Model(&p).Updates(map[string]interface{}{
		"ai_score":   p.AIScore,
		"ai_insight": p.AIInsight,
	})

	response.OKWithMsg(c, "AI 分析完成", gin.H{
		"product": p,
		"analysis": result,
	})
}

// ListTrends 市场趋势数据
func (h *ProductHandler) ListTrends(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var trends []models.ProductTrend
	h.db.Where("product_id = ?", id).Order("stat_date DESC").Limit(90).Find(&trends)
	response.OK(c, trends)
}

func (h *ProductHandler) AddCompetitor(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var comp models.ProductCompetitor
	if err := c.ShouldBindJSON(&comp); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	comp.ProductID = uint(id)
	if err := h.db.Create(&comp).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "添加竞品成功", comp)
}

func (h *ProductHandler) ListCompetitors(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var list []models.ProductCompetitor
	h.db.Where("product_id = ?", id).Find(&list)
	response.OK(c, list)
}
