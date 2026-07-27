package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Warehouse 仓库
type Warehouse struct {
	BaseModel
	Code      string `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Type      string `gorm:"size:32;index" json:"type"` // domestic / overseas / fba / third_party
	Country   string `gorm:"size:64" json:"country"`
	Address   string `gorm:"size:255" json:"address"`
	Manager   string `gorm:"size:64" json:"manager"`
	Status    CommonStatus `gorm:"default:1" json:"status"`
}

func (Warehouse) TableName() string { return "warehouses" }

// Inventory 库存表(SKU + 仓库 维度)
type Inventory struct {
	BaseModel
	WarehouseID uint   `gorm:"index;not null" json:"warehouse_id"`
	SKU         string `gorm:"size:64;index;not null" json:"sku"`
	// 实际库存
	AvailableQty int   `gorm:"default:0" json:"available_qty"`
	// 锁定库存(已下单未发货)
	LockedQty   int   `gorm:"default:0" json:"locked_qty"`
	// 在途库存(采购在途)
	InTransitQty int  `gorm:"default:0" json:"in_transit_qty"`
	// 安全库存
	SafetyStock int   `gorm:"default:0" json:"safety_stock"`
	// 单位成本
	UnitCost    decimal.Decimal `gorm:"type:decimal(18,4)" json:"unit_cost"`
	Currency    string `gorm:"size:8;default:CNY" json:"currency"`
	// 最近入库时间
	LastInboundAt *time.Time `json:"last_inbound_at,omitempty"`
	// 最近出库时间
	LastOutboundAt *time.Time `json:"last_outbound_at,omitempty"`
}

func (Inventory) TableName() string { return "inventories" }

// InventoryMovement 库存流水
type InventoryMovement struct {
	BaseModel
	WarehouseID uint   `gorm:"index;not null" json:"warehouse_id"`
	SKU         string `gorm:"size:64;index;not null" json:"sku"`
	// 变动类型:inbound(入库) / outbound(出库) / lock(锁定) / unlock(解锁) / adjust(调整) / return(退货)
	Type        string `gorm:"size:32;index;not null" json:"type"`
	// 变动数量(正数)
	Quantity    int    `gorm:"not null" json:"quantity"`
	// 变动前后库存
	BeforeQty   int    `json:"before_qty"`
	AfterQty    int    `json:"after_qty"`
	// 关联单据
	RefType     string `gorm:"size:32" json:"ref_type"` // purchase / sale / transfer / adjust
	RefID       string `gorm:"size:64;index" json:"ref_id"`
	// 操作人
	OperatorID  uint   `gorm:"index" json:"operator_id"`
	Remark      string `gorm:"size:255" json:"remark,omitempty"`
}

func (InventoryMovement) TableName() string { return "inventory_movements" }

// StockAlert 库存预警
type StockAlert struct {
	BaseModel
	WarehouseID uint   `gorm:"index;not null" json:"warehouse_id"`
	SKU         string `gorm:"size:64;index;not null" json:"sku"`
	// 预警类型:low_stock(低于安全库存) / overstock(滞销库存) / out_of_stock(断货)
	Type        string `gorm:"size:32;index" json:"type"`
	// 当前库存
	CurrentQty  int    `json:"current_qty"`
	// 阈值
	Threshold   int    `json:"threshold"`
	// 状态:pending / resolved / ignored
	Status      string `gorm:"size:32;index;default:pending" json:"status"`
	HandledBy   *uint  `gorm:"index" json:"handled_by,omitempty"`
	HandledAt   *time.Time `json:"handled_at,omitempty"`
}

func (StockAlert) TableName() string { return "stock_alerts" }
