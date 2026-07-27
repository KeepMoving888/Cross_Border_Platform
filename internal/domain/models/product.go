package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// 选品阶段
const (
	ProductStageSourcing  = "sourcing"  // 寻源中
	ProductStageTesting   = "testing"   // 测试中
	ProductStageApproved  = "approved"  // 已通过
	ProductStageRejected  = "rejected"  // 已否决
	ProductStageArchived  = "archived"  // 已归档
)

// Product 选品池商品
type Product struct {
	BaseModel
	// 选品标识
	SKU          string `gorm:"size:64;uniqueIndex;not null" json:"sku"`
	ASIN         string `gorm:"size:32;index" json:"asin,omitempty"`
	Name         string `gorm:"size:255;not null" json:"name"`
	ImageURL     string `gorm:"size:512" json:"image_url,omitempty"`
	Category     string `gorm:"size:128;index" json:"category"`
	SubCategory  string `gorm:"size:128;index" json:"sub_category,omitempty"`
	// 选品阶段:sourcing / testing / approved / rejected / archived
	Stage        string `gorm:"size:32;index;default:sourcing" json:"stage"`
	// 目标平台
	Platform     string `gorm:"size:32;index" json:"platform"`
	TargetMarket string `gorm:"size:64" json:"target_market,omitempty"`
	// 价格(目标售价,本币)
	ListPrice    decimal.Decimal `gorm:"type:decimal(18,2)" json:"list_price"`
	// 采购成本估算
	EstCostPrice decimal.Decimal `gorm:"type:decimal(18,4)" json:"est_cost_price"`
	// 预估毛利率
	EstMarginRate decimal.Decimal `gorm:"type:decimal(5,2)" json:"est_margin_rate"`
	Currency     string `gorm:"size:8;default:USD" json:"currency"`
	// AI 选品评分(0-100)
	AIScore      decimal.Decimal `gorm:"type:decimal(5,2);index" json:"ai_score"`
	AIInsight    string `gorm:"type:text" json:"ai_insight,omitempty"`
	// 业务字段
	MonthlySales int   `gorm:"default:0" json:"monthly_sales"` // 预估月销
	ReviewCount  int   `gorm:"default:0" json:"review_count"`
	Rating       decimal.Decimal `gorm:"type:decimal(3,2)" json:"rating"`
	// 选品负责人
	OwnerID      uint  `gorm:"index" json:"owner_id"`
	// 关联供应商(已选定)
	SupplierID   *uint `gorm:"index" json:"supplier_id,omitempty"`
	// 关键标签(逗号分隔)
	Tags         string `gorm:"size:512" json:"tags,omitempty"`
	Remark       string `gorm:"type:text" json:"remark,omitempty"`
	// 决策时间
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
}

func (Product) TableName() string { return "products" }

// ProductTrend 商品市场趋势数据
type ProductTrend struct {
	BaseModel
	ProductID    uint   `gorm:"index;not null" json:"product_id"`
	SKU          string `gorm:"size:64;index" json:"sku"`
	// 统计日期
	StatDate     time.Time `gorm:"type:date;index" json:"stat_date"`
	// 平台维度
	Platform     string `gorm:"size:32;index" json:"platform"`
	Market       string `gorm:"size:64" json:"market"`
	// 市场数据
	SearchVolume int   `gorm:"default:0" json:"search_volume"` // 搜索量
	SalesVolume  int   `gorm:"default:0" json:"sales_volume"`  // 销量
	CompetitorCount int `gorm:"default:0" json:"competitor_count"` // 竞品数
	AvgPrice     decimal.Decimal `gorm:"type:decimal(18,2)" json:"avg_price"`
	ReviewGrowth int   `gorm:"default:0" json:"review_growth"`
}

func (ProductTrend) TableName() string { return "product_trends" }

// ProductCompetitor 竞品监控
type ProductCompetitor struct {
	BaseModel
	ProductID    uint   `gorm:"index;not null" json:"product_id"`
	CompetitorASIN string `gorm:"size:32;index" json:"competitor_asin"`
	CompetitorSKU string `gorm:"size:64" json:"competitor_sku"`
	Brand        string `gorm:"size:128" json:"brand"`
	Price        decimal.Decimal `gorm:"type:decimal(18,2)" json:"price"`
	SalesEst     int   `gorm:"default:0" json:"sales_est"`
	ReviewCount  int   `gorm:"default:0" json:"review_count"`
	Rating       decimal.Decimal `gorm:"type:decimal(3,2)" json:"rating"`
	ListingURL   string `gorm:"size:512" json:"listing_url,omitempty"`
}

func (ProductCompetitor) TableName() string { return "product_competitors" }
