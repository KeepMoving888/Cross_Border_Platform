package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Bill 账单(对账单)
type Bill struct {
	BaseModel
	BillNo      string `gorm:"size:64;uniqueIndex;not null" json:"bill_no"`
	// 关联采购单
	OrderID     *uint  `gorm:"index" json:"order_id,omitempty"`
	OrderNo     string `gorm:"size:64;index" json:"order_no"`
	// 供应商
	SupplierID  uint   `gorm:"index;not null" json:"supplier_id"`
	// 账单类型:purchase(采购) / logistics(物流) / other(其他)
	Type        string `gorm:"size:32;index;default:purchase" json:"type"`
	// 账单期间
	PeriodStart *time.Time `json:"period_start,omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty"`
	// 应付金额
	PayableAmount decimal.Decimal `gorm:"type:decimal(18,2);not null" json:"payable_amount"`
	// 实付金额
	PaidAmount   decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"paid_amount"`
	// 差异金额(对账差异)
	DiffAmount   decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"diff_amount"`
	Currency     string `gorm:"size:8;default:CNY" json:"currency"`
	// 状态:draft / matching(对账中) / matched(已对平) / disputed(有差异) / paid(已付款) / closed
	Status       string `gorm:"size:32;index;default:draft" json:"status"`
	// 结算方式
	SettlementMethod string `gorm:"size:64" json:"settlement_method"`
	// 收款方信息
	PayeeName    string `gorm:"size:128" json:"payee_name"`
	PayeeAccount string `gorm:"size:128" json:"payee_account"`
	// 创建人
	CreatorID    uint   `gorm:"index" json:"creator_id"`
	// 对账完成时间
	MatchedAt    *time.Time `json:"matched_at,omitempty"`
	PaidAt       *time.Time `json:"paid_at,omitempty"`
	Remark       string `gorm:"type:text" json:"remark,omitempty"`
}

func (Bill) TableName() string { return "bills" }

// BillItem 账单明细
type BillItem struct {
	BaseModel
	BillID      uint   `gorm:"index;not null" json:"bill_id"`
	// 明细项类型:goods(货款) / freight(运费) / tax(税费) / discount(折扣) / other
	Type        string `gorm:"size:32" json:"type"`
	Description string `gorm:"size:255" json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   decimal.Decimal `gorm:"type:decimal(18,4)" json:"unit_price"`
	Amount      decimal.Decimal `gorm:"type:decimal(18,2);not null" json:"amount"`
	// 系统记录金额(用于对账比对)
	SystemAmount decimal.Decimal `gorm:"type:decimal(18,2)" json:"system_amount"`
	// 差异
	DiffAmount  decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"diff_amount"`
}

func (BillItem) TableName() string { return "bill_items" }

// ProfitReport 利润核算报表
type ProfitReport struct {
	BaseModel
	// 统计周期:day / week / month
	Period      string `gorm:"size:16;index" json:"period"`
	StatDate    time.Time `gorm:"type:date;index" json:"stat_date"`
	// 维度
	SKU         string `gorm:"size:64;index" json:"sku"`
	Platform    string `gorm:"size:32;index" json:"platform"`
	Market      string `gorm:"size:64" json:"market"`
	OrderID     string `gorm:"size:64;index" json:"order_id,omitempty"`
	// 收入
	Revenue     decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"revenue"`
	Qty         int64           `gorm:"default:0" json:"qty"`
	// 成本项拆解
	GoodsCost   decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"goods_cost"`     // 货物成本
	FreightCost decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"freight_cost"`   // 头程运费
	PlatformFee decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"platform_fee"`   // 平台佣金
	AdCost      decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"ad_cost"`        // 广告费
	TaxCost     decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"tax_cost"`       // 税费
	RefundCost  decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"refund_cost"`    // 退款损失
	OtherCost   decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"other_cost"`     // 其他
	// 汇率
	ExchangeRate decimal.Decimal `gorm:"type:decimal(10,4);default:1" json:"exchange_rate"`
	Currency     string `gorm:"size:8;default:USD" json:"currency"`
	// 利润
	GrossProfit decimal.Decimal `gorm:"type:decimal(18,2)" json:"gross_profit"`
	NetProfit   decimal.Decimal `gorm:"type:decimal(18,2)" json:"net_profit"`
	MarginRate  decimal.Decimal `gorm:"type:decimal(5,2)" json:"margin_rate"`
	ROI         decimal.Decimal `gorm:"type:decimal(5,2)" json:"roi"`
}

func (ProfitReport) TableName() string { return "profit_reports" }
