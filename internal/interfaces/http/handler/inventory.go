package handler

import (
	"strconv"
	"time"

	"github.com/cb-platform/internal/application/inventory"
	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InventoryHandler struct {
	db *gorm.DB
}

func NewInventoryHandler(db *gorm.DB) *InventoryHandler {
	return &InventoryHandler{db: db}
}

type listInventoryQuery struct {
	Pagination
	SKU         string `form:"sku"`
	WarehouseID string `form:"warehouse_id"`
	LowStock    string `form:"low_stock"` // 1=仅看低库存
}

func (h *InventoryHandler) List(c *gin.Context) {
	var q listInventoryQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.Inventory{})
	if q.SKU != "" {
		query = query.Where("sku LIKE ?", "%"+q.SKU+"%")
	}
	if q.WarehouseID != "" {
		query = query.Where("warehouse_id = ?", q.WarehouseID)
	}
	if q.LowStock == "1" {
		query = query.Where("available_qty <= safety_stock")
	}

	var total int64
	query.Count(&total)

	var list []models.Inventory
	query.Order("updated_at DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *InventoryHandler) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var inv models.Inventory
	if err := h.db.First(&inv, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	response.OK(c, inv)
}

type updateInventoryRequest struct {
	SafetyStock int     `json:"safety_stock"`
	UnitCost    float64 `json:"unit_cost"`
	Currency    string  `json:"currency"`
}

func (h *InventoryHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req updateInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"safety_stock": req.SafetyStock,
		"currency":     req.Currency,
	}
	if err := h.db.Model(&models.Inventory{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var inv models.Inventory
	h.db.First(&inv, id)
	response.OKWithMsg(c, "更新成功", inv)
}

type adjustInventoryRequest struct {
	WarehouseID uint   `json:"warehouse_id" binding:"required"`
	SKU         string `json:"sku" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required"` // 正数=增加,负数=减少
	Type        string `json:"type" binding:"required,oneof=inbound outbound adjust return"`
	Remark      string `json:"remark"`
}

// Adjust 库存调整(原子操作 + 流水记录)
func (h *InventoryHandler) Adjust(c *gin.Context) {
	var req adjustInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	userID, _ := strconv.Atoi(middleware.GetUserID(c))

	tx := h.db.Begin()
	// 查找库存(行锁)
	var inv models.Inventory
	result := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("warehouse_id = ? AND sku = ?", req.WarehouseID, req.SKU).First(&inv)
	if result.Error != nil {
		// 不存在则创建(仅入库场景)
		if req.Quantity > 0 && req.Type == "inbound" {
			inv = models.Inventory{
				WarehouseID:  req.WarehouseID,
				SKU:          req.SKU,
				AvailableQty: 0,
			}
			if err := tx.Create(&inv).Error; err != nil {
				tx.Rollback()
				response.Fail(c, errors.ErrDBOperation)
				return
			}
		} else {
			tx.Rollback()
			response.FailWithCode(c, errors.ErrNotFound)
			return
		}
	}

	beforeQty := inv.AvailableQty
	afterQty := beforeQty + req.Quantity
	if afterQty < 0 {
		tx.Rollback()
		response.FailWithCode(c, errors.ErrStockInsufficient)
		return
	}

	inv.AvailableQty = afterQty
	now := time.Now()
	if req.Quantity > 0 {
		inv.LastInboundAt = &now
	} else {
		inv.LastOutboundAt = &now
	}

	if err := tx.Save(&inv).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	// 写入流水
	absQty := req.Quantity
	if absQty < 0 {
		absQty = -absQty
	}
	movement := models.InventoryMovement{
		WarehouseID: req.WarehouseID,
		SKU:         req.SKU,
		Type:        req.Type,
		Quantity:    absQty,
		BeforeQty:   beforeQty,
		AfterQty:    afterQty,
		RefType:     "adjust",
		OperatorID:  uint(userID),
		Remark:      req.Remark,
	}
	if err := tx.Create(&movement).Error; err != nil {
		tx.Rollback()
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	// 检查是否触发低库存预警
	if inv.SafetyStock > 0 && afterQty <= inv.SafetyStock {
		alert := models.StockAlert{
			WarehouseID: req.WarehouseID,
			SKU:         req.SKU,
			Type:        "low_stock",
			CurrentQty:  afterQty,
			Threshold:   inv.SafetyStock,
			Status:      "pending",
		}
		tx.Create(&alert)
	}

	tx.Commit()
	response.OKWithMsg(c, "库存调整成功", gin.H{
		"inventory": inv,
		"movement":  movement,
	})
}

type listMovementQuery struct {
	Pagination
	SKU         string `form:"sku"`
	WarehouseID string `form:"warehouse_id"`
	Type        string `form:"type"`
}

func (h *InventoryHandler) ListMovements(c *gin.Context) {
	var q listMovementQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	q.Normalize()

	query := h.db.Model(&models.InventoryMovement{})
	if q.SKU != "" {
		query = query.Where("sku = ?", q.SKU)
	}
	if q.WarehouseID != "" {
		query = query.Where("warehouse_id = ?", q.WarehouseID)
	}
	if q.Type != "" {
		query = query.Where("type = ?", q.Type)
	}

	var total int64
	query.Count(&total)

	var list []models.InventoryMovement
	query.Order("id DESC").Offset(q.Offset()).Limit(q.PageSize).Find(&list)
	response.OKPage(c, list, total, q.Page, q.PageSize)
}

func (h *InventoryHandler) ListAlerts(c *gin.Context) {
	p := Pagination{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
	}
	p.Normalize()

	query := h.db.Model(&models.StockAlert{})
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		// 默认只看待处理预警，避免历史 resolved 淹没看板
		query = query.Where("status = ?", "pending")
	}
	if alertType := c.Query("type"); alertType != "" {
		query = query.Where("type = ?", alertType)
	}

	var total int64
	query.Count(&total)

	var list []models.StockAlert
	query.Order("id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&list)

	whMap := map[uint]string{}
	var warehouses []models.Warehouse
	h.db.Find(&warehouses)
	for _, w := range warehouses {
		whMap[w.ID] = w.Name
	}
	skuSet := make(map[string]struct{}, len(list))
	for _, a := range list {
		skuSet[a.SKU] = struct{}{}
	}
	skus := make([]string, 0, len(skuSet))
	for s := range skuSet {
		skus = append(skus, s)
	}
	prodMap := map[string]string{}
	if len(skus) > 0 {
		var products []models.Product
		h.db.Select("sku, name").Where("sku IN ?", skus).Find(&products)
		for _, p := range products {
			prodMap[p.SKU] = p.Name
		}
	}

	type alertVO struct {
		models.StockAlert
		WarehouseName string `json:"warehouse_name"`
		ProductName   string `json:"product_name"`
		AvailableQty  int    `json:"available_qty"`
		SafetyStock   int    `json:"safety_stock"`
		Level         string `json:"level"`
	}
	out := make([]alertVO, 0, len(list))
	for _, a := range list {
		level := "warning"
		if a.Type == "out_of_stock" || a.CurrentQty <= 0 {
			level = "critical"
		}
		out = append(out, alertVO{
			StockAlert:    a,
			WarehouseName: whMap[a.WarehouseID],
			ProductName:   prodMap[a.SKU],
			AvailableQty:  a.CurrentQty,
			SafetyStock:   a.Threshold,
			Level:         level,
		})
	}
	response.OKPage(c, out, total, p.Page, p.PageSize)
}

func (h *InventoryHandler) ResolveAlert(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	now := time.Now()
	uid := uint(userID)

	if err := h.db.Model(&models.StockAlert{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     "resolved",
		"handled_by": &uid,
		"handled_at": &now,
	}).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "预警已处理", nil)
}

// TriggerAlerts 手动触发库存预警检查(创建采购询价 + 消息)
func (h *InventoryHandler) TriggerAlerts(c *gin.Context) {
	svc := inventory.NewAlertService(h.db)
	if err := svc.CheckAndTrigger(); err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "库存预警触发失败"))
		return
	}
	response.OKWithMsg(c, "库存预警检查已执行", nil)
}

// ============== 仓库 ==============

func (h *InventoryHandler) ListWarehouses(c *gin.Context) {
	var list []models.Warehouse
	h.db.Find(&list)
	response.OK(c, list)
}

type createWarehouseRequest struct {
	Code     string `json:"code" binding:"required,max=64"`
	Name     string `json:"name" binding:"required,max=128"`
	Type     string `json:"type" binding:"oneof=domestic overseas fba third_party"`
	Country  string `json:"country" binding:"max=64"`
	Address  string `json:"address" binding:"max=255"`
	Manager  string `json:"manager" binding:"max=64"`
}

func (h *InventoryHandler) CreateWarehouse(c *gin.Context) {
	var req createWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	wh := models.Warehouse{
		Code:    req.Code,
		Name:    req.Name,
		Type:    req.Type,
		Country: req.Country,
		Address: req.Address,
		Manager: req.Manager,
		Status:  models.StatusEnabled,
	}
	if err := h.db.Create(&wh).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建仓库失败"))
		return
	}
	response.OKWithMsg(c, "创建成功", wh)
}

func (h *InventoryHandler) UpdateWarehouse(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req createWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	if err := h.db.Model(&models.Warehouse{}).Where("id = ?", id).Updates(req).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var wh models.Warehouse
	h.db.First(&wh, id)
	response.OKWithMsg(c, "更新成功", wh)
}
