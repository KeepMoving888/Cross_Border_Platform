package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// 采购单状态
const (
	PurchaseStatusInquiry    = "inquiry"    // 询价中
	PurchaseStatusQuoting     = "quoting"    // 比价中
	PurchaseStatusOrdered     = "ordered"    // 已下单
	PurchaseStatusTracking    = "tracking"   // 跟单中
	PurchaseStatusShipped     = "shipped"    // 已发货
	PurchaseStatusReceived    = "received"   // 已入库
	PurchaseStatusQC          = "qc"         // 质检中
	PurchaseStatusReconciling = "reconciling" // 对账中
	PurchaseStatusSettled     = "settled"    // 已结算
	PurchaseStatusCancelled   = "cancelled"  // 已取消
)

// InquirySheet 询价单
type InquirySheet struct {
	BaseModel
	InquiryNo    string `gorm:"size:64;uniqueIndex;not null" json:"inquiry_no"`
	Title        string `gorm:"size:255" json:"title"`
	// 询价商品
	ProductID    *uint  `gorm:"index" json:"product_id,omitempty"`
	SKU          string `gorm:"size:64;index" json:"sku"`
	ProductName  string `gorm:"size:255" json:"product_name"`
	Quantity     int    `gorm:"not null" json:"quantity"`
	Spec         string `gorm:"size:255" json:"spec,omitempty"`
	// 期望交期
	ExpectedDate *time.Time `json:"expected_date,omitempty"`
	// 询价状态:draft / sent / closed
	Status       string `gorm:"size:32;index;default:draft" json:"status"`
	// 期望价格上限
	MaxPrice     decimal.Decimal `gorm:"type:decimal(18,4)" json:"max_price"`
	Currency     string `gorm:"size:8;default:CNY" json:"currency"`
	// 发给哪些供应商(逗号分隔 supplier_id)
	SupplierIDs  string `gorm:"size:512" json:"supplier_ids,omitempty"`
	// 询价发起人
	CreatorID    uint   `gorm:"index;not null" json:"creator_id"`
	Remark       string `gorm:"type:text" json:"remark,omitempty"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

func (InquirySheet) TableName() string { return "inquiry_sheets" }

// Quote 供应商报价
type Quote struct {
	BaseModel
	InquiryID    uint   `gorm:"index;not null" json:"inquiry_id"`
	SupplierID   uint   `gorm:"index;not null" json:"supplier_id"`
	// 报价金额
	UnitPrice    decimal.Decimal `gorm:"type:decimal(18,4);not null" json:"unit_price"`
	Currency     string `gorm:"size:8;default:CNY" json:"currency"`
	// 交期(天)
	LeadTime     int    `json:"lead_time"`
	// 最小起订量
	MOQ          int    `json:"moq"`
	// 是否含税
	TaxIncluded  bool   `gorm:"default:false" json:"tax_included"`
	// 报价有效期
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	// 报价备注
	Remark       string `gorm:"type:text" json:"remark,omitempty"`
	// 选中状态:0=未选 1=选中 2=未选中
	Selected     uint8  `gorm:"default:0" json:"selected"`
}

func (Quote) TableName() string { return "quotes" }

// PurchaseOrder 采购单(核心)
type PurchaseOrder struct {
	BaseModel
	OrderNo      string `gorm:"size:64;uniqueIndex;not null" json:"order_no"`
	Title        string `gorm:"size:255" json:"title"`
	// 关联询价单
	InquiryID    *uint  `gorm:"index" json:"inquiry_id,omitempty"`
	// 关联选品
	ProductID    *uint  `gorm:"index" json:"product_id,omitempty"`
	SKU          string `gorm:"size:64;index" json:"sku"`
	ProductName  string `gorm:"size:255" json:"product_name"`
	Spec         string `gorm:"size:255" json:"spec,omitempty"`
	// 供应商
	SupplierID   uint   `gorm:"index;not null" json:"supplier_id"`
	// 数量/价格
	Quantity     int    `gorm:"not null" json:"quantity"`
	UnitPrice    decimal.Decimal `gorm:"type:decimal(18,4);not null" json:"unit_price"`
	Currency     string `gorm:"size:8;default:CNY" json:"currency"`
	// 总金额(冗余,用于查询)
	TotalAmount  decimal.Decimal `gorm:"type:decimal(18,2);not null" json:"total_amount"`
	// 结算方式:prepay / deposit_balance / net_30 / net_60
	PaymentTerms string `gorm:"size:64" json:"payment_terms"`
	// 期望交期 / 实际交期
	ExpectedDate *time.Time `json:"expected_date,omitempty"`
	ActualDate   *time.Time `json:"actual_date,omitempty"`
	// 状态(状态机驱动)
	Status       string `gorm:"size:32;index;default:inquiry" json:"status"`
	// 创建人
	CreatorID    uint   `gorm:"index;not null" json:"creator_id"`
	// 物流单号
	LogisticsNo  string `gorm:"size:128" json:"logistics_no,omitempty"`
	LogisticsCompany string `gorm:"size:64" json:"logistics_company,omitempty"`
	// 备注
	Remark       string `gorm:"type:text" json:"remark,omitempty"`
	// 状态变更历史(冗余 JSON)
	StatusHistory string `gorm:"type:text" json:"status_history,omitempty"`
}

func (PurchaseOrder) TableName() string { return "purchase_orders" }

// PurchaseStatusLog 状态变更日志
type PurchaseStatusLog struct {
	BaseModel
	OrderID      uint   `gorm:"index;not null" json:"order_id"`
	FromStatus   string `gorm:"size:32" json:"from_status"`
	ToStatus     string `gorm:"size:32;not null" json:"to_status"`
	OperatorID   uint   `gorm:"index" json:"operator_id"`
	OperatorName string `gorm:"size:64" json:"operator_name"`
	Remark       string `gorm:"type:text" json:"remark,omitempty"`
}

func (PurchaseStatusLog) TableName() string { return "purchase_status_logs" }

// ReceiveRecord 入库记录
type ReceiveRecord struct {
	BaseModel
	OrderID      uint   `gorm:"index;not null" json:"order_id"`
	OrderNo      string `gorm:"size:64;index" json:"order_no"`
	// 入库单号
	ReceiveNo    string `gorm:"size:64;uniqueIndex" json:"receive_no"`
	// 入库数量
	ReceivedQty  int    `gorm:"not null" json:"received_qty"`
	// 质检合格数
	QCPassQty    int    `json:"qc_pass_qty"`
	// 不合格数
	QCFailQty    int    `json:"qc_fail_qty"`
	// 入库仓库
	WarehouseID  uint   `gorm:"index" json:"warehouse_id"`
	// 操作人
	OperatorID   uint   `gorm:"index" json:"operator_id"`
	// 质检备注
	QCRemark     string `gorm:"type:text" json:"qc_remark,omitempty"`
	// 状态:pending / received / qc_passed / qc_failed
	Status       string `gorm:"size:32;index;default:pending" json:"status"`
}

func (ReceiveRecord) TableName() string { return "receive_records" }
