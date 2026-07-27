package inventory

import (
	"fmt"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// AlertService 库存预警服务：自动触发采购询价 + 写入消息中心
type AlertService struct {
	db *gorm.DB
}

// NewAlertService 创建预警服务实例
func NewAlertService(db *gorm.DB) *AlertService {
	return &AlertService{db: db}
}

// CheckAndTrigger 检查所有低库存/无货库存，自动创建采购询价 + 消息
// 去重规则：同一 SKU + 仓库已有未读(is_read=false)的 stock_alert 消息时不重复创建
func (s *AlertService) CheckAndTrigger() error {
	// 1. 查询所有 available_qty <= safety_stock 的库存记录
	//    safety_stock > 0 避免未设阈值的记录被误判
	var invs []models.Inventory
	if err := s.db.Where("safety_stock > 0 AND available_qty <= safety_stock").
		Order("warehouse_id, sku").Find(&invs).Error; err != nil {
		return fmt.Errorf("查询低库存记录失败: %w", err)
	}
	if len(invs) == 0 {
		logger.Get().Info("库存预警检查：无低库存记录")
		return nil
	}

	// 2. 预加载依赖数据，减少 N+1 查询
	whMap := s.loadWarehouseMap()
	prodMap := s.loadProductMap()
	defaultSupplierID, systemUserID := s.loadSystemDefaults()
	notifyUserIDs := s.loadNotifyUserIDs()
	if len(notifyUserIDs) == 0 {
		logger.Get().Warn("库存预警：无 admin/manager 用户可通知，跳过消息创建")
	}

	// 3. 预加载已有未读预警消息的 ref_id 集合(去重用)
	existingRefIDs := s.loadExistingAlertRefIDs()

	triggered, skipped, failed := 0, 0, 0
	for _, inv := range invs {
		refID := fmt.Sprintf("%s-%d", inv.SKU, inv.WarehouseID)
		// 已有未读预警消息则跳过，避免重复触发
		if _, exists := existingRefIDs[refID]; exists {
			skipped++
			continue
		}
		if err := s.triggerOne(inv, refID, whMap, prodMap, defaultSupplierID, systemUserID, notifyUserIDs); err != nil {
			logger.Get().Warnf("触发预警失败 %s@仓库%d: %v", inv.SKU, inv.WarehouseID, err)
			failed++
			continue
		}
		triggered++
	}

	logger.Get().Infof("库存预警检查完成: 触发=%d, 跳过=%d, 失败=%d, 低库存总数=%d",
		triggered, skipped, failed, len(invs))
	return nil
}

// triggerOne 对单条低库存记录创建采购询价单 + 预警消息(事务内原子执行)
func (s *AlertService) triggerOne(
	inv models.Inventory,
	refID string,
	whMap map[uint]string,
	prodMap map[string]models.Product,
	defaultSupplierID uint,
	systemUserID uint,
	notifyUserIDs []uint,
) error {
	// 判断预警等级：断货=critical，低库存=warning
	level := "warning"
	alertType := "low_stock"
	titlePrefix := "库存偏低预警"
	if inv.AvailableQty == 0 {
		level = "critical"
		alertType = "out_of_stock"
		titlePrefix = "断货预警"
	}

	// 查找产品信息(名称、供应商、产品 ID)
	productName := inv.SKU
	var productID *uint
	supplierID := defaultSupplierID
	if p, ok := prodMap[inv.SKU]; ok {
		productName = p.Name
		productID = &p.ID
		if p.SupplierID != nil && *p.SupplierID > 0 {
			supplierID = *p.SupplierID
		}
	}

	// 无可用供应商则跳过(下次运行会重试)
	if supplierID == 0 {
		return fmt.Errorf("SKU %s 无可用供应商，跳过创建采购询价单", inv.SKU)
	}

	// 采购数量 = 安全库存 × 2
	quantity := inv.SafetyStock * 2
	if quantity <= 0 {
		quantity = 100
	}

	whName := whMap[inv.WarehouseID]
	if whName == "" {
		whName = fmt.Sprintf("仓库#%d", inv.WarehouseID)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1) 创建采购询价单(status=inquiry)
	orderNo := generateBizNo("PO")
	unitPrice := inv.UnitCost
	totalAmount := unitPrice.Mul(decimal.NewFromInt(int64(quantity)))
	order := models.PurchaseOrder{
		OrderNo:      orderNo,
		Title:        fmt.Sprintf("自动询价 · %s", productName),
		ProductID:    productID,
		SKU:          inv.SKU,
		ProductName:  productName,
		SupplierID:   supplierID,
		Quantity:     quantity,
		UnitPrice:    unitPrice,
		Currency:     inv.Currency,
		TotalAmount:  totalAmount,
		PaymentTerms: "deposit_balance",
		Status:       models.PurchaseStatusInquiry,
		CreatorID:    systemUserID,
		Remark: fmt.Sprintf("库存预警自动触发: 仓库 %s 当前库存 %d, 安全库存 %d",
			whName, inv.AvailableQty, inv.SafetyStock),
	}
	if order.Currency == "" {
		order.Currency = "CNY"
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("创建采购询价单失败: %w", err)
	}

	// 2) 写入采购状态变更日志
	statusLog := models.PurchaseStatusLog{
		OrderID:      order.ID,
		FromStatus:   "",
		ToStatus:     models.PurchaseStatusInquiry,
		OperatorID:   systemUserID,
		OperatorName: "system",
		Remark:       "库存预警自动触发询价",
	}
	if err := tx.Create(&statusLog).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("写入采购状态日志失败: %w", err)
	}

	// 3) 同步创建 StockAlert(若该 SKU+仓库无 pending 预警)
	var existingAlert int64
	tx.Model(&models.StockAlert{}).
		Where("warehouse_id = ? AND sku = ? AND status = ?", inv.WarehouseID, inv.SKU, "pending").
		Count(&existingAlert)
	if existingAlert == 0 {
		alert := models.StockAlert{
			WarehouseID: inv.WarehouseID,
			SKU:         inv.SKU,
			Type:        alertType,
			CurrentQty:  inv.AvailableQty,
			Threshold:   inv.SafetyStock,
			Status:      "pending",
		}
		if err := tx.Create(&alert).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("创建库存预警记录失败: %w", err)
		}
	}

	// 4) 消息中心：为每个 admin/manager 用户创建一条预警消息
	if len(notifyUserIDs) > 0 {
		title := fmt.Sprintf("%s · %s", titlePrefix, inv.SKU)
		content := fmt.Sprintf("%s · SKU %s 当前库存 %d，安全库存 %d，已自动创建采购询价单 %s。",
			whName, inv.SKU, inv.AvailableQty, inv.SafetyStock, orderNo)
		messages := make([]models.Message, 0, len(notifyUserIDs))
		for _, uid := range notifyUserIDs {
			messages = append(messages, models.Message{
				UserID:  uid,
				Type:    "stock_alert",
				Level:   level,
				Title:   title,
				Content: content,
				RefType: "inventory",
				RefID:   refID,
				Link:    "/inventory",
				IsRead:  false,
			})
		}
		if err := tx.Create(&messages).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("批量创建预警消息失败: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// loadWarehouseMap 加载仓库 ID -> 名称映射
func (s *AlertService) loadWarehouseMap() map[uint]string {
	m := make(map[uint]string)
	var ws []models.Warehouse
	s.db.Find(&ws)
	for _, w := range ws {
		m[w.ID] = w.Name
	}
	return m
}

// loadProductMap 加载 SKU -> Product 映射(名称、供应商、产品 ID)
func (s *AlertService) loadProductMap() map[string]models.Product {
	m := make(map[string]models.Product)
	var products []models.Product
	s.db.Find(&products)
	for _, p := range products {
		m[p.SKU] = p
	}
	return m
}

// loadSystemDefaults 加载系统默认值：默认供应商 ID(优先 A 级 active) + 系统管理员用户 ID
func (s *AlertService) loadSystemDefaults() (supplierID uint, userID uint) {
	// 默认供应商：优先 A 级，其次任意 active
	var sup models.Supplier
	if err := s.db.Where("coop_status = ? AND rating = ?", "active", "A").
		Order("id ASC").First(&sup).Error; err != nil {
		if err = s.db.Where("coop_status = ?", "active").Order("id ASC").First(&sup).Error; err != nil {
			logger.Get().Warnf("未找到可用供应商: %v", err)
		}
	}
	supplierID = sup.ID

	// 系统管理员用户(admin 角色)
	var user models.User
	if err := s.db.Where("role = ? AND status = ?", "admin", models.StatusEnabled).
		Order("id ASC").First(&user).Error; err != nil {
		logger.Get().Warnf("未找到 admin 用户，回退 creator_id=1: %v", err)
		userID = 1
	} else {
		userID = user.ID
	}
	return
}

// loadNotifyUserIDs 加载需要通知的用户 ID(admin/manager)
func (s *AlertService) loadNotifyUserIDs() []uint {
	var ids []uint
	s.db.Model(&models.User{}).
		Where("role IN ? AND status = ?", []string{"admin", "manager"}, models.StatusEnabled).
		Pluck("id", &ids)
	return ids
}

// loadExistingAlertRefIDs 加载已有未读预警消息的 ref_id 集合(去重用)
func (s *AlertService) loadExistingAlertRefIDs() map[string]struct{} {
	m := make(map[string]struct{})
	var refIDs []string
	s.db.Model(&models.Message{}).
		Where("type = ? AND ref_type = ? AND is_read = ?", "stock_alert", "inventory", false).
		Pluck("ref_id", &refIDs)
	for _, r := range refIDs {
		if r != "" {
			m[r] = struct{}{}
		}
	}
	return m
}

// generateBizNo 生成业务单号(格式: PREFIX-YYYYMMDDHHmmssNNNN)
func generateBizNo(prefix string) string {
	now := time.Now()
	return fmt.Sprintf("%s-%s%04d", prefix, now.Format("20060102150405"), now.UnixNano()%10000)
}
