package handler

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cb-platform/internal/application/purchase"
	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type PurchaseHandler struct {
	db *gorm.DB
}

func NewPurchaseHandler(db *gorm.DB) *PurchaseHandler {
	return &PurchaseHandler{db: db}
}

// ============== 询价单 ==============

type listInquiryQuery struct {
	Pagination
	Keyword string `form:"keyword"`
	Status  string `form:"status"`
	SKU     string `form:"sku"`
}

func (h *PurchaseHandler) ListInquiries(c *gin.Context) {
	var q listInquiryQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.InquirySheet{})
	if q.Keyword != "" {
		query = query.Where("title LIKE ? OR inquiry_no LIKE ? OR product_name LIKE ?",
			"%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}
	if q.SKU != "" {
		query = query.Where("sku = ?", q.SKU)
	}

	var total int64
	query.Count(&total)

	var list []models.InquirySheet
	query.Order("id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *PurchaseHandler) GetInquiry(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var inq models.InquirySheet
	if err := h.db.First(&inq, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	// 附带报价列表
	var quotes []models.Quote
	h.db.Where("inquiry_id = ?", id).Find(&quotes)
	response.OK(c, gin.H{"inquiry": inq, "quotes": quotes})
}

type createInquiryRequest struct {
	Title        string          `json:"title" binding:"max=255"`
	ProductID    *uint           `json:"product_id"`
	SKU          string          `json:"sku" binding:"max=64"`
	ProductName  string          `json:"product_name" binding:"required,max=255"`
	Quantity     int             `json:"quantity" binding:"required,min=1"`
	Spec         string          `json:"spec"`
	ExpectedDate *time.Time      `json:"expected_date"`
	MaxPrice     decimal.Decimal `json:"max_price"`
	Currency     string          `json:"currency"`
	SupplierIDs  string          `json:"supplier_ids"`
	Remark       string          `json:"remark"`
}

func (h *PurchaseHandler) CreateInquiry(c *gin.Context) {
	var req createInquiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	userID, _ := strconv.Atoi(middleware.GetUserID(c))

	inq := models.InquirySheet{
		InquiryNo:    generateBizNo("INQ"),
		Title:        req.Title,
		ProductID:    req.ProductID,
		SKU:          req.SKU,
		ProductName:  req.ProductName,
		Quantity:     req.Quantity,
		Spec:         req.Spec,
		ExpectedDate: req.ExpectedDate,
		MaxPrice:     req.MaxPrice,
		Currency:     req.Currency,
		SupplierIDs:  req.SupplierIDs,
		CreatorID:    uint(userID),
		Remark:       req.Remark,
		Status:       "draft",
	}
	if inq.Currency == "" {
		inq.Currency = "CNY"
	}
	if inq.Title == "" {
		inq.Title = req.ProductName + " 询价单"
	}

	if err := h.db.Create(&inq).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建询价单失败"))
		return
	}
	response.OKWithMsg(c, "询价单创建成功", inq)
}

func (h *PurchaseHandler) UpdateInquiry(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req createInquiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"title":         req.Title,
		"sku":           req.SKU,
		"product_name":  req.ProductName,
		"quantity":      req.Quantity,
		"spec":          req.Spec,
		"expected_date": req.ExpectedDate,
		"max_price":     req.MaxPrice,
		"supplier_ids":  req.SupplierIDs,
		"remark":        req.Remark,
	}
	if err := h.db.Model(&models.InquirySheet{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var inq models.InquirySheet
	h.db.First(&inq, id)
	response.OKWithMsg(c, "更新成功", inq)
}

func (h *PurchaseHandler) DeleteInquiry(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	// 仅 draft 状态可删除
	var inq models.InquirySheet
	if err := h.db.First(&inq, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	if inq.Status != "draft" {
		response.FailWithCode(c, errors.ErrStateConflict)
		return
	}
	h.db.Delete(&inq)
	response.OKWithMsg(c, "删除成功", nil)
}

func (h *PurchaseHandler) CloseInquiry(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	now := time.Now()
	if err := h.db.Model(&models.InquirySheet{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":    "closed",
		"closed_at": &now,
	}).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "询价单已关闭", nil)
}

// ============== 报价 ==============

func (h *PurchaseHandler) ListQuotes(c *gin.Context) {
	inquiryID, _ := strconv.Atoi(c.Param("id"))
	var quotes []models.Quote
	h.db.Where("inquiry_id = ?", inquiryID).Order("unit_price ASC").Find(&quotes)
	response.OK(c, quotes)
}

type createQuoteRequest struct {
	InquiryID   uint            `json:"inquiry_id" binding:"required"`
	SupplierID  uint            `json:"supplier_id" binding:"required"`
	UnitPrice   decimal.Decimal `json:"unit_price" binding:"required"`
	Currency    string          `json:"currency"`
	LeadTime    int             `json:"lead_time"`
	MOQ         int             `json:"moq"`
	TaxIncluded bool            `json:"tax_included"`
	ValidUntil  *time.Time      `json:"valid_until"`
	Remark      string          `json:"remark"`
}

func (h *PurchaseHandler) CreateQuote(c *gin.Context) {
	var req createQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q := models.Quote{
		InquiryID:   req.InquiryID,
		SupplierID:  req.SupplierID,
		UnitPrice:   req.UnitPrice,
		Currency:    req.Currency,
		LeadTime:    req.LeadTime,
		MOQ:         req.MOQ,
		TaxIncluded: req.TaxIncluded,
		ValidUntil:  req.ValidUntil,
		Remark:      req.Remark,
		Selected:    0,
	}
	if q.Currency == "" {
		q.Currency = "CNY"
	}

	tx := h.db.Begin()
	if err := tx.Create(&q).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.Wrap(err, 9002, "创建报价失败"))
		return
	}
	// 询价单状态变为 quoting
	if err := tx.Model(&models.InquirySheet{}).Where("id = ?", req.InquiryID).
		Update("status", "sent").Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	tx.Commit()
	response.OKWithMsg(c, "报价提交成功", q)
}

func (h *PurchaseHandler) SelectQuote(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var q models.Quote
	if err := h.db.First(&q, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	tx := h.db.Begin()
	// 选中当前报价
	if err := tx.Model(&q).Update("selected", 1).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	// 其他报价标记为未选中
	if err := tx.Model(&models.Quote{}).
		Where("inquiry_id = ? AND id != ?", q.InquiryID, id).
		Update("selected", 2).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	// 关闭询价单
	now := time.Now()
	if err := tx.Model(&models.InquirySheet{}).Where("id = ?", q.InquiryID).
		Updates(map[string]interface{}{"status": "closed", "closed_at": &now}).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	tx.Commit()
	response.OKWithMsg(c, "已选定报价", q)
}

// ============== 采购单(状态机驱动) ==============

type listOrderQuery struct {
	Pagination
	Keyword    string `form:"keyword"`
	Status     string `form:"status"`
	SupplierID string `form:"supplier_id"`
	SKU        string `form:"sku"`
}

func (h *PurchaseHandler) ListOrders(c *gin.Context) {
	var q listOrderQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.PurchaseOrder{})
	if q.Keyword != "" {
		query = query.Where("order_no LIKE ? OR title LIKE ? OR product_name LIKE ?",
			"%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}
	if q.SupplierID != "" {
		query = query.Where("supplier_id = ?", q.SupplierID)
	}
	if q.SKU != "" {
		query = query.Where("sku = ?", q.SKU)
	}

	var total int64
	query.Count(&total)

	var list []models.PurchaseOrder
	query.Order("id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *PurchaseHandler) GetOrder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var order models.PurchaseOrder
	if err := h.db.First(&order, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrOrderNotFound)
		return
	}
	// 附带状态日志与入库记录
	var logs []models.PurchaseStatusLog
	h.db.Where("order_id = ?", id).Order("id DESC").Find(&logs)
	var receives []models.ReceiveRecord
	h.db.Where("order_id = ?", id).Find(&receives)
	response.OK(c, gin.H{
		"order":                order,
		"logs":                 logs,
		"receives":             receives,
		"current_status_label": purchase.StatusLabel(order.Status),
		"allowed_events":       h.allowedEvents(order.Status),
	})
}

type createOrderRequest struct {
	Title        string          `json:"title"`
	InquiryID    *uint           `json:"inquiry_id"`
	ProductID    *uint           `json:"product_id"`
	SKU          string          `json:"sku" binding:"max=64"`
	ProductName  string          `json:"product_name" binding:"required,max=255"`
	Spec         string          `json:"spec"`
	SupplierID   uint            `json:"supplier_id" binding:"required"`
	Quantity     int             `json:"quantity" binding:"required,min=1"`
	UnitPrice    decimal.Decimal `json:"unit_price" binding:"required"`
	Currency     string          `json:"currency"`
	PaymentTerms string          `json:"payment_terms"`
	ExpectedDate *time.Time      `json:"expected_date"`
	Remark       string          `json:"remark"`
}

func (h *PurchaseHandler) CreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	userID, _ := strconv.Atoi(middleware.GetUserID(c))

	totalAmount := req.UnitPrice.Mul(decimal.NewFromInt(int64(req.Quantity)))

	order := models.PurchaseOrder{
		OrderNo:      generateBizNo("PO"),
		Title:        req.Title,
		InquiryID:    req.InquiryID,
		ProductID:    req.ProductID,
		SKU:          req.SKU,
		ProductName:  req.ProductName,
		Spec:         req.Spec,
		SupplierID:   req.SupplierID,
		Quantity:     req.Quantity,
		UnitPrice:    req.UnitPrice,
		Currency:     req.Currency,
		TotalAmount:  totalAmount,
		PaymentTerms: req.PaymentTerms,
		ExpectedDate: req.ExpectedDate,
		Status:       models.PurchaseStatusOrdered,
		CreatorID:    uint(userID),
		Remark:       req.Remark,
	}
	if order.Currency == "" {
		order.Currency = "CNY"
	}
	if order.Title == "" {
		order.Title = req.ProductName + " 采购单"
	}

	tx := h.db.Begin()
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.Wrap(err, 9002, "创建采购单失败"))
		return
	}
	// 写入状态变更日志
	log := models.PurchaseStatusLog{
		OrderID:      order.ID,
		FromStatus:   "",
		ToStatus:     models.PurchaseStatusOrdered,
		OperatorID:   uint(userID),
		OperatorName: middleware.GetUsername(c),
		Remark:       "采购单创建",
	}
	if err := tx.Create(&log).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	tx.Commit()
	response.OKWithMsg(c, "采购单创建成功", order)
}

func (h *PurchaseHandler) UpdateOrder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var order models.PurchaseOrder
	if err := h.db.First(&order, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrOrderNotFound)
		return
	}
	// 仅 inquiry/quoting 状态可修改
	if order.Status != models.PurchaseStatusInquiry && order.Status != models.PurchaseStatusQuoting {
		response.FailWithCode(c, errors.ErrStateConflict)
		return
	}
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	totalAmount := req.UnitPrice.Mul(decimal.NewFromInt(int64(req.Quantity)))
	updates := map[string]interface{}{
		"title":         req.Title,
		"sku":           req.SKU,
		"product_name":  req.ProductName,
		"spec":          req.Spec,
		"supplier_id":   req.SupplierID,
		"quantity":      req.Quantity,
		"unit_price":    req.UnitPrice,
		"total_amount":  totalAmount,
		"payment_terms": req.PaymentTerms,
		"expected_date": req.ExpectedDate,
		"remark":        req.Remark,
	}
	if err := h.db.Model(&order).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	h.db.First(&order, id)
	response.OKWithMsg(c, "更新成功", order)
}

type transitionRequest struct {
	Event            string     `json:"event" binding:"required"`
	Remark           string     `json:"remark"`
	LogisticsNo      string     `json:"logistics_no"`
	LogisticsCompany string     `json:"logistics_company"`
	ActualDate       *time.Time `json:"actual_date"`
}

// Transition 状态机驱动状态变更
func (h *PurchaseHandler) Transition(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req transitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	var order models.PurchaseOrder
	if err := h.db.First(&order, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrOrderNotFound)
		return
	}

	// 使用状态机校验
	sm := purchase.NewStateMachine(order.Status)
	if err := sm.Transition(req.Event); err != nil {
		response.Fail(c, errors.Wrap(err, 5001, err.Error()))
		return
	}

	newStatus := sm.Current()
	userID, _ := strconv.Atoi(middleware.GetUserID(c))

	tx := h.db.Begin()
	// 更新订单状态
	updates := map[string]interface{}{
		"status": newStatus,
	}
	if req.LogisticsNo != "" {
		updates["logistics_no"] = req.LogisticsNo
	}
	if req.LogisticsCompany != "" {
		updates["logistics_company"] = req.LogisticsCompany
	}
	if req.ActualDate != nil {
		updates["actual_date"] = req.ActualDate
	}
	// 状态历史追加
	history := appendStatusHistory(order.StatusHistory, statusHistoryItem{
		From: order.Status, To: newStatus, Event: req.Event,
		Time: time.Now().Format("2006-01-02 15:04:05"), Remark: req.Remark,
	})
	updates["status_history"] = history

	if err := tx.Model(&order).Updates(updates).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	// 写入状态变更日志
	log := models.PurchaseStatusLog{
		OrderID:      order.ID,
		FromStatus:   order.Status,
		ToStatus:     newStatus,
		OperatorID:   uint(userID),
		OperatorName: middleware.GetUsername(c),
		Remark:       req.Remark,
	}
	if err := tx.Create(&log).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	// 如果是入库事件,触发库存增加
	if req.Event == purchase.EventReceive {
		if err := h.handleReceive(tx, &order, uint(userID)); err != nil {
			tx.Rollback()
			response.Fail(c, errors.Wrap(err, 9002, "入库处理失败"))
			return
		}
	}
	tx.Commit()

	h.db.First(&order, id)
	response.OKWithMsg(c, fmt.Sprintf("状态变更成功:%s -> %s",
		purchase.StatusLabel(order.Status), purchase.StatusLabel(newStatus)), order)
}

// handleReceive 入库处理:增加库存 + 写流水
func (h *PurchaseHandler) handleReceive(tx *gorm.DB, order *models.PurchaseOrder, operatorID uint) error {
	// 查找默认仓库(国内主仓)
	var wh models.Warehouse
	if err := tx.Where("code = ?", "WH-CN-01").First(&wh).Error; err != nil {
		return fmt.Errorf("默认仓库未配置: %w", err)
	}

	// 查找或创建库存记录
	var inv models.Inventory
	result := tx.Where("warehouse_id = ? AND sku = ?", wh.ID, order.SKU).First(&inv)
	if result.Error != nil {
		inv = models.Inventory{
			WarehouseID:  wh.ID,
			SKU:          order.SKU,
			AvailableQty: 0,
			UnitCost:     order.UnitPrice,
			Currency:     order.Currency,
		}
		if err := tx.Create(&inv).Error; err != nil {
			return err
		}
	}

	beforeQty := inv.AvailableQty
	inv.AvailableQty += order.Quantity
	inv.LastInboundAt = &[]time.Time{time.Now()}[0]

	if err := tx.Save(&inv).Error; err != nil {
		return err
	}

	// 写入库存流水
	movement := models.InventoryMovement{
		WarehouseID: wh.ID,
		SKU:         order.SKU,
		Type:        "inbound",
		Quantity:    order.Quantity,
		BeforeQty:   beforeQty,
		AfterQty:    inv.AvailableQty,
		RefType:     "purchase",
		RefID:       order.OrderNo,
		OperatorID:  operatorID,
		Remark:      fmt.Sprintf("采购单 %s 入库", order.OrderNo),
	}
	if err := tx.Create(&movement).Error; err != nil {
		return err
	}

	// 创建入库记录
	rcv := models.ReceiveRecord{
		OrderID:     order.ID,
		OrderNo:     order.OrderNo,
		ReceiveNo:   generateBizNo("RCV"),
		ReceivedQty: order.Quantity,
		WarehouseID: wh.ID,
		OperatorID:  operatorID,
		Status:      "received",
	}
	if err := tx.Create(&rcv).Error; err != nil {
		return err
	}

	return nil
}

// Receive 直接入库(简化版,不走状态机)
func (h *PurchaseHandler) Receive(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Quantity    int    `json:"quantity" binding:"required,min=1"`
		WarehouseID uint   `json:"warehouse_id"`
		LogisticsNo string `json:"logistics_no"`
		QCRemark    string `json:"qc_remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	var order models.PurchaseOrder
	if err := h.db.First(&order, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrOrderNotFound)
		return
	}

	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	tx := h.db.Begin()

	// 创建入库记录
	rcv := models.ReceiveRecord{
		OrderID:     order.ID,
		OrderNo:     order.OrderNo,
		ReceiveNo:   generateBizNo("RCV"),
		ReceivedQty: req.Quantity,
		WarehouseID: req.WarehouseID,
		OperatorID:  uint(userID),
		QCRemark:    req.QCRemark,
		Status:      "received",
	}
	if err := tx.Create(&rcv).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	// 自动状态转换
	sm := purchase.NewStateMachine(order.Status)
	if sm.CanTransition(purchase.EventReceive) {
		_ = sm.Transition(purchase.EventReceive)
		tx.Model(&order).Update("status", sm.Current())
	}

	tx.Commit()
	response.OKWithMsg(c, "入库成功", rcv)
}

func (h *PurchaseHandler) ListStatusLogs(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var logs []models.PurchaseStatusLog
	h.db.Where("order_id = ?", id).Order("id DESC").Find(&logs)
	response.OK(c, logs)
}

// ============== 入库记录 ==============

func (h *PurchaseHandler) ListReceives(c *gin.Context) {
	p := Pagination{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
	}
	p.Normalize()

	query := h.db.Model(&models.ReceiveRecord{})
	if orderNo := c.Query("order_no"); orderNo != "" {
		query = query.Where("order_no = ?", orderNo)
	}

	var total int64
	query.Count(&total)

	var list []models.ReceiveRecord
	query.Order("id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&list)
	response.OKPage(c, list, total, p.Page, p.PageSize)
}

func (h *PurchaseHandler) GetReceive(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var rcv models.ReceiveRecord
	if err := h.db.First(&rcv, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	response.OK(c, rcv)
}

// ============== 辅助函数 ==============

type statusHistoryItem struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Event  string `json:"event"`
	Time   string `json:"time"`
	Remark string `json:"remark"`
}

func appendStatusHistory(existing string, item statusHistoryItem) string {
	var history []statusHistoryItem
	if existing != "" {
		_ = json.Unmarshal([]byte(existing), &history)
	}
	history = append(history, item)
	b, _ := json.Marshal(history)
	return string(b)
}

func (h *PurchaseHandler) allowedEvents(currentStatus string) []string {
	sm := purchase.NewStateMachine(currentStatus)
	events := []string{}
	for _, e := range purchase.AllEvents() {
		if sm.CanTransition(e) {
			events = append(events, e)
		}
	}
	return events
}

// generateBizNo 生成业务单号
func generateBizNo(prefix string) string {
	now := time.Now()
	return fmt.Sprintf("%s-%s%04d", prefix, now.Format("20060102150405"), now.UnixNano()%10000)
}
