package handler

import (
	"strconv"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type SupplierHandler struct {
	db *gorm.DB
}

func NewSupplierHandler(db *gorm.DB) *SupplierHandler {
	return &SupplierHandler{db: db}
}

type listSupplierQuery struct {
	Pagination
	Keyword    string `form:"keyword"`
	Rating     string `form:"rating"`
	CoopStatus string `form:"coop_status"`
	Region     string `form:"region"`
}

// List 供应商列表
func (h *SupplierHandler) List(c *gin.Context) {
	var q listSupplierQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.Supplier{})
	if q.Keyword != "" {
		query = query.Where("name LIKE ? OR code LIKE ? OR contact_name LIKE ?",
			"%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Rating != "" {
		query = query.Where("rating = ?", q.Rating)
	}
	if q.CoopStatus != "" {
		query = query.Where("coop_status = ?", q.CoopStatus)
	}
	if q.Region != "" {
		query = query.Where("region LIKE ?", "%"+q.Region+"%")
	}

	var total int64
	query.Count(&total)

	var list []models.Supplier
	query.Order("total_amount DESC, id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)

	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *SupplierHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var s models.Supplier
	if err := h.db.First(&s, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	response.OK(c, s)
}

type createSupplierRequest struct {
	Name            string `json:"name" binding:"required,max=128"`
	Code            string `json:"code" binding:"max=64"`
	ContactName     string `json:"contact_name" binding:"max=64"`
	Phone           string `json:"phone" binding:"max=32"`
	Email           string `json:"email" binding:"omitempty,email,max=128"`
	Address         string `json:"address" binding:"max=255"`
	Region          string `json:"region" binding:"max=64"`
	PaymentTerms    string `json:"payment_terms" binding:"max=128"`
	SettlementCycle string `json:"settlement_cycle" binding:"max=64"`
	Rating          string `json:"rating" binding:"omitempty,oneof=A B C"`
	CoopStatus      string `json:"coop_status" binding:"omitempty,oneof=active suspended terminated"`
	Remark          string `json:"remark"`
}

func (h *SupplierHandler) Create(c *gin.Context) {
	var req createSupplierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	// 自动生成 code
	if req.Code == "" {
		req.Code = "SUP-" + strconv.Itoa(int(timeNow().UnixNano()%1000000))
	}

	s := models.Supplier{
		Name:            req.Name,
		Code:            req.Code,
		ContactName:     req.ContactName,
		Phone:           req.Phone,
		Email:           req.Email,
		Address:         req.Address,
		Region:          req.Region,
		PaymentTerms:    req.PaymentTerms,
		SettlementCycle: req.SettlementCycle,
		Rating:          req.Rating,
		CoopStatus:      req.CoopStatus,
		Remark:          req.Remark,
		TotalAmount:     decimal.Zero,
		OnTimeRate:      decimal.Zero,
		QualityRate:     decimal.Zero,
	}
	if s.Rating == "" {
		s.Rating = "B"
	}
	if s.CoopStatus == "" {
		s.CoopStatus = "active"
	}

	if err := h.db.Create(&s).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建供应商失败"))
		return
	}
	response.OKWithMsg(c, "创建成功", s)
}

type updateSupplierRequest struct {
	Name            string `json:"name"`
	ContactName     string `json:"contact_name"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
	Address         string `json:"address"`
	Region          string `json:"region"`
	PaymentTerms    string `json:"payment_terms"`
	SettlementCycle string `json:"settlement_cycle"`
	Rating          string `json:"rating" binding:"omitempty,oneof=A B C"`
	CoopStatus      string `json:"coop_status" binding:"omitempty,oneof=active suspended terminated"`
	Remark          string `json:"remark"`
}

func (h *SupplierHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var req updateSupplierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	if err := h.db.Model(&models.Supplier{}).Where("id = ?", id).Updates(req).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var s models.Supplier
	h.db.First(&s, id)
	response.OKWithMsg(c, "更新成功", s)
}

func (h *SupplierHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	if err := h.db.Delete(&models.Supplier{}, id).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "删除成功", nil)
}

// 供应商可供货商品
func (h *SupplierHandler) ListProducts(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var products []models.SupplierProduct
	h.db.Where("supplier_id = ?", id).Find(&products)
	response.OK(c, products)
}

type addSupplierProductRequest struct {
	SKU         string          `json:"sku" binding:"max=64"`
	ProductName string          `json:"product_name" binding:"required,max=255"`
	Spec        string          `json:"spec"`
	Category    string          `json:"category"`
	Unit        string          `json:"unit"`
	MOQ         int             `json:"moq"`
	LeadTime    int             `json:"lead_time"`
	CostPrice   decimal.Decimal `json:"cost_price"`
	Currency    string          `json:"currency"`
}

func (h *SupplierHandler) AddProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var req addSupplierProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	sp := models.SupplierProduct{
		SupplierID:  uint(id),
		SKU:         req.SKU,
		ProductName: req.ProductName,
		Spec:        req.Spec,
		Category:    req.Category,
		Unit:        req.Unit,
		MOQ:         req.MOQ,
		LeadTime:    req.LeadTime,
		CostPrice:   req.CostPrice,
		Currency:    req.Currency,
	}
	if sp.Currency == "" {
		sp.Currency = "CNY"
	}
	if sp.MOQ == 0 {
		sp.MOQ = 1
	}
	if sp.LeadTime == 0 {
		sp.LeadTime = 7
	}
	if err := h.db.Create(&sp).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "添加成功", sp)
}
