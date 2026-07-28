package models

import "github.com/shopspring/decimal"

// User 用户表(内部系统用户)
type User struct {
	BaseModel
	Username    string       `gorm:"size:64;uniqueIndex;not null" json:"username"`
	Password    string       `gorm:"size:128;not null" json:"-"`
	RealName    string       `gorm:"size:64" json:"real_name"`
	Email       string       `gorm:"size:128;index" json:"email"`
	Phone       string       `gorm:"size:32" json:"phone"`
	Avatar      string       `gorm:"size:255" json:"avatar"`
	Role        string       `gorm:"size:32;index;default:staff" json:"role"` // admin / manager / staff
	Department  string       `gorm:"size:64;index" json:"department"`
	Status      CommonStatus `gorm:"default:1" json:"status"`
	LastLoginAt *string      `gorm:"size:32" json:"last_login_at,omitempty"`
}

func (User) TableName() string { return "users" }

// PlatformAccount 跨境平台账号
type PlatformAccount struct {
	BaseModel
	Name          string       `gorm:"size:128;not null" json:"name"`
	Platform      string       `gorm:"size:32;index;not null" json:"platform"` // amazon / temu / tiktok
	Region        string       `gorm:"size:32" json:"region"`                  // US / UK / DE / JP
	SellerID      string       `gorm:"size:128" json:"seller_id"`
	RefreshToken  string       `gorm:"type:text" json:"-"`
	AccessToken   string       `gorm:"type:text" json:"-"`
	TokenExpireAt *string      `gorm:"size:64" json:"token_expire_at,omitempty"`
	Status        CommonStatus `gorm:"default:1" json:"status"`
	// 后续接入时填充: AWSAccessKey / AWSSecretKey / MarketplaceID 等
	Metadata string `gorm:"type:text" json:"metadata,omitempty"`
}

func (PlatformAccount) TableName() string { return "platform_accounts" }

// Supplier 供应商
type Supplier struct {
	BaseModel
	Name            string `gorm:"size:128;not null" json:"name"`
	Code            string `gorm:"size:64;uniqueIndex" json:"code"`
	ContactName     string `gorm:"size:64" json:"contact_name"`
	Phone           string `gorm:"size:32" json:"phone"`
	Email           string `gorm:"size:128" json:"email"`
	Address         string `gorm:"size:255" json:"address"`
	Region          string `gorm:"size:64" json:"region"`
	PaymentTerms    string `gorm:"size:128" json:"payment_terms"`   // 结算方式
	SettlementCycle string `gorm:"size:64" json:"settlement_cycle"` // 结算周期
	// 评级:A / B / C
	Rating string `gorm:"size:8;default:B" json:"rating"`
	// 合作状态:active / suspended / terminated
	CoopStatus string `gorm:"size:32;index;default:active" json:"coop_status"`
	// 累计合作金额
	TotalAmount decimal.Decimal `gorm:"type:decimal(18,2);default:0" json:"total_amount"`
	// 交付准时率
	OnTimeRate decimal.Decimal `gorm:"type:decimal(5,2);default:0" json:"on_time_rate"`
	// 质量合格率
	QualityRate decimal.Decimal `gorm:"type:decimal(5,2);default:0" json:"quality_rate"`
	Remark      string          `gorm:"type:text" json:"remark,omitempty"`
}

func (Supplier) TableName() string { return "suppliers" }

// SupplierProduct 供应商可供货商品
type SupplierProduct struct {
	BaseModel
	SupplierID  uint            `gorm:"index;not null" json:"supplier_id"`
	SKU         string          `gorm:"size:64;index" json:"sku"`
	ProductName string          `gorm:"size:255" json:"product_name"`
	Spec        string          `gorm:"size:255" json:"spec"`
	Category    string          `gorm:"size:128;index" json:"category"`
	Unit        string          `gorm:"size:32" json:"unit"`
	MOQ         int             `gorm:"default:1" json:"moq"`       // 最小起订量
	LeadTime    int             `gorm:"default:7" json:"lead_time"` // 交货周期(天)
	CostPrice   decimal.Decimal `gorm:"type:decimal(18,4)" json:"cost_price"`
	Currency    string          `gorm:"size:8;default:CNY" json:"currency"`
}

func (SupplierProduct) TableName() string { return "supplier_products" }
