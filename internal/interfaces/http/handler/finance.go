package handler

import (
	"strconv"
	"time"

	"github.com/cb-platform/internal/application/finance"
	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type FinanceHandler struct {
	db *gorm.DB
}

func NewFinanceHandler(db *gorm.DB) *FinanceHandler {
	return &FinanceHandler{db: db}
}

// ============== 账单 ==============

type listBillQuery struct {
	Pagination
	Keyword    string `form:"keyword"`
	Status     string `form:"status"`
	SupplierID string `form:"supplier_id"`
	Type       string `form:"type"`
}

func (h *FinanceHandler) ListBills(c *gin.Context) {
	var q listBillQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.Bill{})
	if q.Keyword != "" {
		query = query.Where("bill_no LIKE ? OR order_no LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}
	if q.SupplierID != "" {
		query = query.Where("supplier_id = ?", q.SupplierID)
	}
	if q.Type != "" {
		query = query.Where("type = ?", q.Type)
	}

	var total int64
	query.Count(&total)

	var list []models.Bill
	query.Order("id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *FinanceHandler) GetBill(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var bill models.Bill
	if err := h.db.First(&bill, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	var items []models.BillItem
	h.db.Where("bill_id = ?", id).Find(&items)
	response.OK(c, gin.H{"bill": bill, "items": items})
}

type createBillRequest struct {
	OrderID          *uint           `json:"order_id"`
	OrderNo          string          `json:"order_no"`
	SupplierID       uint            `json:"supplier_id" binding:"required"`
	Type             string          `json:"type"`
	PeriodStart      *time.Time      `json:"period_start"`
	PeriodEnd        *time.Time      `json:"period_end"`
	PayableAmount    decimal.Decimal `json:"payable_amount" binding:"required"`
	Currency         string          `json:"currency"`
	SettlementMethod string          `json:"settlement_method"`
	PayeeName        string          `json:"payee_name"`
	PayeeAccount     string          `json:"payee_account"`
	Remark           string          `json:"remark"`
}

func (h *FinanceHandler) CreateBill(c *gin.Context) {
	var req createBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	bill := models.Bill{
		BillNo:           generateBizNo("BILL"),
		OrderID:          req.OrderID,
		OrderNo:          req.OrderNo,
		SupplierID:       req.SupplierID,
		Type:             req.Type,
		PeriodStart:      req.PeriodStart,
		PeriodEnd:        req.PeriodEnd,
		PayableAmount:    req.PayableAmount,
		Currency:         req.Currency,
		SettlementMethod: req.SettlementMethod,
		PayeeName:        req.PayeeName,
		PayeeAccount:     req.PayeeAccount,
		CreatorID:        uint(userID),
		Remark:           req.Remark,
		Status:           "draft",
	}
	if bill.Type == "" {
		bill.Type = "purchase"
	}
	if bill.Currency == "" {
		bill.Currency = "CNY"
	}

	if err := h.db.Create(&bill).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建账单失败"))
		return
	}
	response.OKWithMsg(c, "账单创建成功", bill)
}

func (h *FinanceHandler) UpdateBill(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req createBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"order_no":          req.OrderNo,
		"supplier_id":       req.SupplierID,
		"type":              req.Type,
		"period_start":      req.PeriodStart,
		"period_end":        req.PeriodEnd,
		"payable_amount":    req.PayableAmount,
		"currency":          req.Currency,
		"settlement_method": req.SettlementMethod,
		"payee_name":        req.PayeeName,
		"payee_account":     req.PayeeAccount,
		"remark":            req.Remark,
	}
	if err := h.db.Model(&models.Bill{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var bill models.Bill
	h.db.First(&bill, id)
	response.OKWithMsg(c, "更新成功", bill)
}

// MatchBill 对账:比对账单金额与系统金额,标记差异
func (h *FinanceHandler) MatchBill(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var bill models.Bill
	if err := h.db.First(&bill, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}

	// 计算系统应付金额(从采购单汇总)
	var systemAmount decimal.Decimal
	if bill.OrderID != nil {
		var order models.PurchaseOrder
		if err := h.db.First(&order, *bill.OrderID).Error; err == nil {
			systemAmount = order.TotalAmount
		}
	}

	diff := bill.PayableAmount.Sub(systemAmount)
	now := time.Now()

	updates := map[string]interface{}{
		"status":      "matched",
		"diff_amount": diff,
		"matched_at":  &now,
	}
	if !diff.IsZero() {
		updates["status"] = "disputed"
	}

	if err := h.db.Model(&bill).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	response.OKWithMsg(c, "对账完成", gin.H{
		"bill":           bill,
		"system_amount":  systemAmount,
		"diff_amount":    diff,
		"matched":        diff.IsZero(),
	})
}

// AutoMatch 自动对账匹配:批量匹配待对账账单与采购单,检测差异并标记状态
func (h *FinanceHandler) AutoMatch(c *gin.Context) {
	svc := finance.NewReconciliationService(h.db)
	matched, disputed, err := svc.AutoMatch()
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "自动对账失败"))
		return
	}
	response.OKWithMsg(c, "自动对账完成", gin.H{
		"matched":  matched,
		"disputed": disputed,
	})
}

// PayBill 付款
func (h *FinanceHandler) PayBill(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		PaidAmount decimal.Decimal `json:"paid_amount" binding:"required"`
		Remark     string          `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	var bill models.Bill
	if err := h.db.First(&bill, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"paid_amount": req.PaidAmount,
		"status":      "paid",
		"paid_at":     &now,
	}
	if err := h.db.Model(&bill).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "付款成功", bill)
}

func (h *FinanceHandler) ListBillItems(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var items []models.BillItem
	h.db.Where("bill_id = ?", id).Find(&items)
	response.OK(c, items)
}

// ============== 利润报表 ==============

type profitSummaryQuery struct {
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Platform  string `form:"platform"`
}

func (h *FinanceHandler) ProfitSummary(c *gin.Context) {
	var q profitSummaryQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	query := h.db.Model(&models.ProfitReport{})
	if q.StartDate != "" {
		query = query.Where("stat_date >= ?", q.StartDate)
	}
	if q.EndDate != "" {
		query = query.Where("stat_date <= ?", q.EndDate)
	}
	if q.Platform != "" {
		query = query.Where("platform = ?", q.Platform)
	}

	var result struct {
		TotalRevenue   decimal.Decimal `json:"total_revenue"`
		TotalGoodsCost decimal.Decimal `json:"total_goods_cost"`
		TotalFreight   decimal.Decimal `json:"total_freight_cost"`
		TotalPlatform  decimal.Decimal `json:"total_platform_fee"`
		TotalAd        decimal.Decimal `json:"total_ad_cost"`
		TotalTax       decimal.Decimal `json:"total_tax_cost"`
		TotalRefund    decimal.Decimal `json:"total_refund_cost"`
		TotalOther     decimal.Decimal `json:"total_other_cost"`
		TotalNet       decimal.Decimal `json:"total_net_profit"`
		OrderCount     int64           `json:"order_count"`
		AvgMargin      decimal.Decimal `json:"avg_margin_rate"`
	}
	query.Select(`
		COALESCE(SUM(revenue), 0) as total_revenue,
		COALESCE(SUM(goods_cost), 0) as total_goods_cost,
		COALESCE(SUM(freight_cost), 0) as total_freight_cost,
		COALESCE(SUM(platform_fee), 0) as total_platform_fee,
		COALESCE(SUM(ad_cost), 0) as total_ad_cost,
		COALESCE(SUM(tax_cost), 0) as total_tax_cost,
		COALESCE(SUM(refund_cost), 0) as total_refund_cost,
		COALESCE(SUM(other_cost), 0) as total_other_cost,
		COALESCE(SUM(net_profit), 0) as total_net_profit,
		COUNT(*) as order_count,
		COALESCE(AVG(margin_rate), 0) as avg_margin_rate
	`).Scan(&result)

	response.OK(c, result)
}

func (h *FinanceHandler) ProfitBySKU(c *gin.Context) {
	p := Pagination{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
	}
	p.Normalize()

	query := h.db.Table("profit_reports").
		Select(`sku,
			SUM(revenue) as revenue,
			SUM(goods_cost) as goods_cost,
			SUM(freight_cost) as freight_cost,
			SUM(platform_fee) as platform_fee,
			SUM(ad_cost) as ad_cost,
			SUM(net_profit) as net_profit,
			AVG(margin_rate) as margin_rate,
			COUNT(*) as order_count`).
		Group("sku")

	if platform := c.Query("platform"); platform != "" {
		query = query.Where("platform = ?", platform)
	}

	var total int64
	h.db.Table("(SELECT DISTINCT sku FROM profit_reports) t").Count(&total)

	type skuProfit struct {
		SKU         string          `json:"sku"`
		Revenue     decimal.Decimal `json:"revenue"`
		GoodsCost   decimal.Decimal `json:"goods_cost"`
		FreightCost decimal.Decimal `json:"freight_cost"`
		PlatformFee decimal.Decimal `json:"platform_fee"`
		AdCost      decimal.Decimal `json:"ad_cost"`
		NetProfit   decimal.Decimal `json:"net_profit"`
		MarginRate  decimal.Decimal `json:"margin_rate"`
		OrderCount  int64           `json:"order_count"`
	}
	var list []skuProfit
	query.Order("net_profit DESC").Offset(p.Offset()).Limit(p.PageSize).Scan(&list)

	response.OKPage(c, list, total, p.Page, p.PageSize)
}

func (h *FinanceHandler) ProfitByPlatform(c *gin.Context) {
	type platformProfit struct {
		Platform    string          `json:"platform"`
		Revenue     decimal.Decimal `json:"revenue"`
		NetProfit   decimal.Decimal `json:"net_profit"`
		MarginRate  decimal.Decimal `json:"margin_rate"`
		OrderCount  int64           `json:"order_count"`
	}
	var list []platformProfit
	h.db.Table("profit_reports").
		Select(`platform,
			SUM(revenue) as revenue,
			SUM(net_profit) as net_profit,
			AVG(margin_rate) as margin_rate,
			COUNT(*) as order_count`).
		Group("platform").
		Order("revenue DESC").
		Scan(&list)

	response.OK(c, list)
}

func (h *FinanceHandler) ProfitTrend(c *gin.Context) {
	days := atoiDefault(c.Query("days"), 30)
	if days > 365 {
		days = 365
	}

	type trendItem struct {
		Date       string          `json:"date"`
		Revenue    decimal.Decimal `json:"revenue"`
		NetProfit  decimal.Decimal `json:"net_profit"`
		MarginRate decimal.Decimal `json:"margin_rate"`
	}
	var list []trendItem
	h.db.Table("profit_reports").
		Select(`DATE(stat_date) as date,
			SUM(revenue) as revenue,
			SUM(net_profit) as net_profit,
			AVG(margin_rate) as margin_rate`).
		Where("stat_date >= DATE_SUB(CURDATE(), INTERVAL ? DAY)", days).
		Group("DATE(stat_date)").
		Order("date ASC").
		Scan(&list)

	response.OK(c, list)
}
