package handler

import (
	"strconv"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

// overviewResp 工作台总览返回结构(对齐前端 DashboardOverview)
type overviewResp struct {
	// 销售 & 利润
	TodaySales      decimal.Decimal `json:"today_sales"`
	YesterdaySales  decimal.Decimal `json:"yesterday_sales"`
	MonthSales      decimal.Decimal `json:"month_sales"`
	NetProfit       decimal.Decimal `json:"net_profit"`
	TotalRevenue    decimal.Decimal `json:"total_revenue"`
	MonthProfit     decimal.Decimal `json:"month_profit"`
	MarginRate      decimal.Decimal `json:"margin_rate"`
	OrderCount30d   int64           `json:"order_count_30d"`
	RefundAmount    decimal.Decimal `json:"refund_amount"`
	// 选品
	ProductTotal    int64 `json:"product_total"`
	ProductApproved int64 `json:"product_approved"`
	ProductSourcing int64 `json:"product_sourcing"`
	NewProducts7d  int64 `json:"new_products_7d"`
	// 采购
	PurchasePending int64 `json:"pending_purchase_orders"`
	PurchaseTotal   int64 `json:"purchase_total"`
	// 供应商
	SupplierActive int64 `json:"supplier_active"`
	// 库存
	InventorySKUs int64 `json:"inventory_skus"`
	StockAlerts  int64 `json:"inventory_alerts"`
	// 财务对账
	BillsPending int64 `json:"bills_pending"`
	// AI
	AIRunsToday    int64 `json:"ai_runs_today"`
	AIRunsTotal    int64 `json:"ai_runs_total"`
	AIRunningTasks int64           `json:"ai_running_tasks"`
	AISuccessRate  decimal.Decimal `json:"ai_success_rate"`
}

// Overview 总览(首页核心指标,字段对齐前端 DashboardOverview)
func (h *DashboardHandler) Overview(c *gin.Context) {
	var ov overviewResp

	// ---- 销售与利润(基于 profit_reports 表,默认 CNY 口径) ----
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	type salesSum struct {
		Revenue   decimal.Decimal
		NetProfit decimal.Decimal
		Refund    decimal.Decimal
		OrderCnt  int64
	}
	// 今日
	var todaySum salesSum
	h.db.Model(&models.ProfitReport{}).
		Where("stat_date = ?", today).
		Select("COALESCE(SUM(revenue),0) as revenue, COALESCE(SUM(net_profit),0) as net_profit, COALESCE(SUM(refund_cost),0) as refund, COUNT(*) as order_cnt").
		Scan(&todaySum)
	ov.TodaySales = todaySum.Revenue
	// 昨日
	var ySum salesSum
	h.db.Model(&models.ProfitReport{}).
		Where("stat_date = ?", yesterday).
		Select("COALESCE(SUM(revenue),0) as revenue, COALESCE(SUM(net_profit),0) as net_profit, COALESCE(SUM(refund_cost),0) as refund, COUNT(*) as order_cnt").
		Scan(&ySum)
	ov.YesterdaySales = ySum.Revenue
	// 本月
	var mSum salesSum
	h.db.Model(&models.ProfitReport{}).
		Where("stat_date >= ? AND stat_date <= ?", monthStart, today).
		Select("COALESCE(SUM(revenue),0) as revenue, COALESCE(SUM(net_profit),0) as net_profit, COALESCE(SUM(refund_cost),0) as refund, COUNT(*) as order_cnt").
		Scan(&mSum)
	ov.MonthSales = mSum.Revenue
	ov.MonthProfit = mSum.NetProfit
	ov.RefundAmount = mSum.Refund
	ov.OrderCount30d = mSum.OrderCnt
	// 近 30 天累计
	var allSum salesSum
	h.db.Model(&models.ProfitReport{}).
		Where("stat_date >= ?", today.AddDate(0, 0, -29)).
		Select("COALESCE(SUM(revenue),0) as revenue, COALESCE(SUM(net_profit),0) as net_profit, COALESCE(SUM(refund_cost),0) as refund, COUNT(*) as order_cnt").
		Scan(&allSum)
	ov.TotalRevenue = allSum.Revenue
	ov.NetProfit = allSum.NetProfit
	if allSum.Revenue.GreaterThan(decimal.Zero) {
		ov.MarginRate = allSum.NetProfit.Div(allSum.Revenue).Mul(decimal.NewFromInt(100))
	}

	// ---- 选品 ----
	h.db.Model(&models.Product{}).Count(&ov.ProductTotal)
	h.db.Model(&models.Product{}).Where("stage = ?", models.ProductStageApproved).Count(&ov.ProductApproved)
	h.db.Model(&models.Product{}).Where("stage IN ?", []string{models.ProductStageSourcing, models.ProductStageTesting}).Count(&ov.ProductSourcing)
	h.db.Model(&models.Product{}).Where("created_at >= ?", today.AddDate(0, 0, -6)).Count(&ov.NewProducts7d)

	// ---- 采购 ----
	h.db.Model(&models.PurchaseOrder{}).Where("status IN ?", []string{"inquiry", "quoting", "ordered", "tracking"}).Count(&ov.PurchasePending)
	h.db.Model(&models.PurchaseOrder{}).Count(&ov.PurchaseTotal)

	// ---- 供应商 ----
	h.db.Model(&models.Supplier{}).Where("coop_status = ?", "active").Count(&ov.SupplierActive)

	// ---- 库存 ----
	h.db.Model(&models.Inventory{}).Count(&ov.InventorySKUs)
	h.db.Model(&models.StockAlert{}).Where("status = ?", "pending").Count(&ov.StockAlerts)

	// ---- 财务对账 ----
	h.db.Model(&models.Bill{}).Where("status IN ?", []string{"draft", "matching", "disputed"}).Count(&ov.BillsPending)

	// ---- AI 使用统计 ----
	h.db.Model(&models.AIWorkflowRun{}).Where("DATE(created_at) = CURDATE()").Count(&ov.AIRunsToday)
	h.db.Model(&models.AIWorkflowRun{}).Count(&ov.AIRunsTotal)
	h.db.Model(&models.AIWorkflowRun{}).Where("status = ?", "running").Count(&ov.AIRunningTasks)
	// AI 成功率
	var aiRate struct {
		Total   int64
		Success int64
	}
	h.db.Model(&models.AIWorkflowRun{}).
		Select("COUNT(*) as total, SUM(CASE WHEN status='success' THEN 1 ELSE 0 END) as success").
		Where("created_at >= ?", today.AddDate(0, 0, -29)).
		Scan(&aiRate)
	if aiRate.Total > 0 {
		ov.AISuccessRate = decimal.NewFromInt(aiRate.Success).Div(decimal.NewFromInt(aiRate.Total)).Mul(decimal.NewFromInt(100))
	}

	response.OK(c, ov)
}

// SalesTrend 近 N 天销售趋势(基于 profit_reports 表按日聚合)
func (h *DashboardHandler) SalesTrend(c *gin.Context) {
	days := 30
	if d := c.Query("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 180 {
			days = v
		}
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := today.AddDate(0, 0, -(days - 1))

	type row struct {
		Date         string          `json:"date"`
		Sales        decimal.Decimal `json:"sales"`
		Orders       int64           `json:"orders"`
		NetProfit    decimal.Decimal `json:"net_profit"`
		RefundAmount decimal.Decimal `json:"refund_amount"`
	}
	var rows []row
	h.db.Model(&models.ProfitReport{}).
		Select(`DATE_FORMAT(stat_date, '%Y-%m-%d') as date,
			COALESCE(SUM(revenue),0) as sales,
			COALESCE(SUM(qty),0) as orders,
			COALESCE(SUM(net_profit),0) as net_profit,
			COALESCE(SUM(refund_cost),0) as refund_amount`).
		Where("stat_date >= ? AND stat_date <= ?", start, today).
		Group("date").
		Order("date ASC").
		Scan(&rows)

	// 补全空日期(保证折线连续)
	dateMap := make(map[string]row, len(rows))
	for _, r := range rows {
		dateMap[r.Date] = r
	}
	full := make([]row, 0, days)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		if r, ok := dateMap[key]; ok {
			full = append(full, r)
		} else {
			full = append(full, row{Date: key, Sales: decimal.Zero, Orders: 0, NetProfit: decimal.Zero, RefundAmount: decimal.Zero})
		}
	}

	response.OK(c, full)
}

// CategoryShare 各品类销售占比(关联 products 表 category 聚合近 30 天)
func (h *DashboardHandler) CategoryShare(c *gin.Context) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := today.AddDate(0, 0, -29)

	type row struct {
		Category     string          `json:"category"`
		CategoryName string          `json:"category_name"`
		Sales        decimal.Decimal `json:"sales"`
		Share        decimal.Decimal `json:"share"`
	}
	// 中英文映射(后端 category 存中文,前端需 ProductCategory 枚举,统一返回枚举值)
	categoryMap := map[string]string{
		"个护电器": "personal_care",
		"美容仪器": "beauty_device",
		"美体仪器": "body_shaping",
		"配件耗材": "accessories",
	}
	categoryCN := map[string]string{
		"personal_care": "个护电器",
		"beauty_device": "美容仪器",
		"body_shaping":  "美体仪器",
		"accessories":   "配件耗材",
	}

	type aggRow struct {
		Category string
		Sales    decimal.Decimal
	}
	var agg []aggRow
	h.db.Table("profit_reports").
		Select("products.category as category, COALESCE(SUM(profit_reports.revenue),0) as sales").
		Joins("LEFT JOIN products ON products.sku = profit_reports.sku").
		Where("profit_reports.stat_date >= ? AND profit_reports.stat_date <= ?", start, today).
		Group("products.category").
		Scan(&agg)

	// 计算总销售额用于占比
	totalSales := decimal.Zero
	for _, a := range agg {
		totalSales = totalSales.Add(a.Sales)
	}

	rows := make([]row, 0, len(agg))
	for _, a := range agg {
		enumCode := categoryMap[a.Category]
		if enumCode == "" {
			enumCode = "accessories"
		}
		share := decimal.Zero
		if totalSales.GreaterThan(decimal.Zero) {
			share = a.Sales.Div(totalSales)
		}
		rows = append(rows, row{
			Category:     enumCode,
			CategoryName: categoryCN[enumCode],
			Sales:        a.Sales,
			Share:        share,
		})
	}

	response.OK(c, rows)
}

// ProductStats 选品统计
func (h *DashboardHandler) ProductStats(c *gin.Context) {
	type stageStat struct {
		Stage string `json:"stage"`
		Count int64  `json:"count"`
	}
	var byStage []stageStat
	h.db.Model(&models.Product{}).
		Select("stage, COUNT(*) as count").
		Group("stage").
		Scan(&byStage)

	type categoryStat struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
		AvgScore float64 `json:"avg_score"`
	}
	var byCategory []categoryStat
	h.db.Model(&models.Product{}).
		Select("COALESCE(NULLIF(category, ''), '未分类') as category, COUNT(*) as count, COALESCE(AVG(ai_score), 0) as avg_score").
		Group("category").
		Order("count DESC").
		Limit(10).
		Scan(&byCategory)

	response.OK(c, gin.H{
		"by_stage":    byStage,
		"by_category": byCategory,
	})
}

// PurchaseStats 采购统计
func (h *DashboardHandler) PurchaseStats(c *gin.Context) {
	type statusStat struct {
		Status string          `json:"status"`
		Count  int64           `json:"count"`
		Amount decimal.Decimal `json:"amount"`
	}
	var byStatus []statusStat
	h.db.Model(&models.PurchaseOrder{}).
		Select("status, COUNT(*) as count, COALESCE(SUM(total_amount), 0) as amount").
		Group("status").
		Scan(&byStatus)

	type supplierStat struct {
		SupplierID   uint            `json:"supplier_id"`
		SupplierName string          `json:"supplier_name"`
		OrderCount   int64           `json:"order_count"`
		TotalAmount  decimal.Decimal `json:"total_amount"`
	}
	var bySupplier []supplierStat
	h.db.Table("purchase_orders").
		Select("purchase_orders.supplier_id, suppliers.name as supplier_name, COUNT(*) as order_count, SUM(purchase_orders.total_amount) as total_amount").
		Joins("LEFT JOIN suppliers ON suppliers.id = purchase_orders.supplier_id").
		Group("purchase_orders.supplier_id, suppliers.name").
		Order("total_amount DESC").
		Limit(10).
		Scan(&bySupplier)

	response.OK(c, gin.H{
		"by_status":   byStatus,
		"by_supplier": bySupplier,
	})
}

// InventoryStats 库存统计
func (h *DashboardHandler) InventoryStats(c *gin.Context) {
	type warehouseStat struct {
		WarehouseID   uint   `json:"warehouse_id"`
		WarehouseName string `json:"warehouse_name"`
		SKUCount      int64  `json:"sku_count"`
		TotalQty      int64  `json:"total_qty"`
	}
	var byWarehouse []warehouseStat
	h.db.Table("inventories").
		Select("inventories.warehouse_id, warehouses.name as warehouse_name, COUNT(DISTINCT inventories.sku) as sku_count, SUM(inventories.available_qty) as total_qty").
		Joins("LEFT JOIN warehouses ON warehouses.id = inventories.warehouse_id").
		Group("inventories.warehouse_id, warehouses.name").
		Scan(&byWarehouse)

	type alertStat struct {
		Type   string `json:"type"`
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var byAlert []alertStat
	h.db.Model(&models.StockAlert{}).
		Select("type, status, COUNT(*) as count").
		Group("type, status").
		Scan(&byAlert)

	response.OK(c, gin.H{
		"by_warehouse": byWarehouse,
		"by_alert":     byAlert,
	})
}

// ProfitStats 利润统计
func (h *DashboardHandler) ProfitStats(c *gin.Context) {
	type costBreakdown struct {
		GoodsCost   decimal.Decimal `json:"goods_cost"`
		FreightCost decimal.Decimal `json:"freight_cost"`
		PlatformFee decimal.Decimal `json:"platform_fee"`
		AdCost      decimal.Decimal `json:"ad_cost"`
		TaxCost     decimal.Decimal `json:"tax_cost"`
		RefundCost  decimal.Decimal `json:"refund_cost"`
		OtherCost   decimal.Decimal `json:"other_cost"`
	}
	var breakdown costBreakdown
	h.db.Model(&models.ProfitReport{}).
		Select("COALESCE(SUM(goods_cost),0) as goods_cost, COALESCE(SUM(freight_cost),0) as freight_cost, COALESCE(SUM(platform_fee),0) as platform_fee, COALESCE(SUM(ad_cost),0) as ad_cost, COALESCE(SUM(tax_cost),0) as tax_cost, COALESCE(SUM(refund_cost),0) as refund_cost, COALESCE(SUM(other_cost),0) as other_cost").
		Scan(&breakdown)

	type monthlyProfit struct {
		Month      string          `json:"month"`
		Revenue    decimal.Decimal `json:"revenue"`
		NetProfit  decimal.Decimal `json:"net_profit"`
		MarginRate decimal.Decimal `json:"margin_rate"`
	}
	var byMonth []monthlyProfit
	h.db.Model(&models.ProfitReport{}).
		Select("DATE_FORMAT(stat_date, '%Y-%m') as month, SUM(revenue) as revenue, SUM(net_profit) as net_profit, AVG(margin_rate) as margin_rate").
		Group("month").
		Order("month DESC").
		Limit(12).
		Scan(&byMonth)

	response.OK(c, gin.H{
		"cost_breakdown": breakdown,
		"by_month":       byMonth,
	})
}

// AIStats AI 使用统计
func (h *DashboardHandler) AIStats(c *gin.Context) {
	type sceneStat struct {
		WorkflowCode string          `json:"workflow_code"`
		Count        int64           `json:"count"`
		SuccessCount int64           `json:"success_count"`
		Tokens       int64           `json:"tokens"`
		Cost         decimal.Decimal `json:"cost"`
		AvgDuration  decimal.Decimal `json:"avg_duration_ms"`
	}
	var byScene []sceneStat
	h.db.Model(&models.AIWorkflowRun{}).
		Select("workflow_code, COUNT(*) as count, SUM(CASE WHEN status='success' THEN 1 ELSE 0 END) as success_count, COALESCE(SUM(total_tokens),0) as tokens, COALESCE(SUM(cost),0) as cost, COALESCE(AVG(duration),0) as avg_duration").
		Group("workflow_code").
		Scan(&byScene)

	type dailyStat struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}
	var byDay []dailyStat
	h.db.Model(&models.AIWorkflowRun{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') as date, COUNT(*) as count").
		Where("created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY)").
		Group("DATE_FORMAT(created_at, '%Y-%m-%d')").
		Order("date ASC").
		Scan(&byDay)

	response.OK(c, gin.H{
		"by_scene": byScene,
		"by_day":   byDay,
	})
}
