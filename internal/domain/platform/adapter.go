// Package platform 跨境电商平台对接抽象层
//
// 设计目标:
//  1. 统一不同电商平台(亚马逊/Temu/TikTok Shop)的接口差异
//  2. 通过 Adapter 模式隔离平台 SDK 细节,业务层只依赖 PlatformAdapter 接口
//  3. 支持增量同步、错误重试、限流
//
// 当前阶段提供接口定义与 Builtin 实现(返回本地数据),后续接入真实平台 SDK 时
// 只需实现 PlatformAdapter 接口即可,业务层无感知切换
package platform

import (
	"context"
	"fmt"
	"time"
)

// Platform 支持的平台枚举
type Platform string

const (
	PlatformAmazon Platform = "amazon"
	PlatformTemu   Platform = "temu"
	PlatformTikTok Platform = "tiktok"
)

// AccountConfig 平台账号配置
type AccountConfig struct {
	ID           uint
	Platform     Platform
	Region       string
	SellerID     string
	RefreshToken string
	AccessToken  string
	ExpiresAt    time.Time
	Metadata     map[string]string
}

// ProductInfo 平台商品信息(统一模型)
type ProductInfo struct {
	Platform       Platform `json:"platform"`
	SellerSKU      string   `json:"seller_sku"`
	ASIN           string   `json:"asin,omitempty"`           // Amazon
	ItemName       string   `json:"item_name"`
	ItemID         string   `json:"item_id"`                  // 平台商品 ID
	Price          float64  `json:"price"`
	Currency       string   `json:"currency"`
	StockQuantity  int      `json:"stock_quantity"`
	Status         string   `json:"status"`
	ImageURL       string   `json:"image_url,omitempty"`
	ListedAt       *time.Time `json:"listed_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// OrderInfo 平台订单信息(统一模型)
type OrderInfo struct {
	Platform      Platform    `json:"platform"`
	OrderID       string      `json:"order_id"`
	Status        string      `json:"status"`
	TotalAmount   float64     `json:"total_amount"`
	Currency      string      `json:"currency"`
	Items         []OrderItem `json:"items"`
	BuyerName     string      `json:"buyer_name,omitempty"`
	BuyerEmail    string      `json:"buyer_email,omitempty"`
	ShippingAddr  string      `json:"shipping_address,omitempty"`
	CreatedAt     *time.Time  `json:"created_at,omitempty"`
	UpdatedAt     *time.Time  `json:"updated_at,omitempty"`
}

// OrderItem 订单明细
type OrderItem struct {
	SKU          string  `json:"sku"`
	ProductName  string  `json:"product_name"`
	Quantity     int     `json:"quantity"`
	ItemPrice    float64 `json:"item_price"`
	TotalPrice   float64 `json:"total_price"`
}

// SyncResult 同步结果
type SyncResult struct {
	Platform      Platform     `json:"platform"`
	LastSyncAt    time.Time    `json:"last_sync_at"`
	NewProducts   int          `json:"new_products"`
	UpdatedProducts int        `json:"updated_products"`
	NewOrders     int          `json:"new_orders"`
	UpdatedOrders int          `json:"updated_orders"`
	Errors        []SyncError  `json:"errors,omitempty"`
	Duration      time.Duration `json:"duration"`
}

// SyncError 同步错误
type SyncError struct {
	ResourceType string `json:"resource_type"` // product / order / inventory
	ResourceID   string `json:"resource_id"`
	Error        string `json:"error"`
}

// PlatformAdapter 平台适配器接口
//
// 业务层通过此接口访问平台数据,屏蔽具体平台 SDK 差异
type PlatformAdapter interface {
	// Name 平台标识
	Name() Platform

	// Auth 认证(刷新 access token)
	Auth(ctx context.Context) error

	// ListProducts 拉取商品列表(分页)
	ListProducts(ctx context.Context, page, size int) ([]ProductInfo, int, error)

	// GetProduct 获取单个商品详情
	GetProduct(ctx context.Context, skuOrID string) (*ProductInfo, error)

	// ListOrders 拉取订单列表(按时间范围)
	ListOrders(ctx context.Context, start, end time.Time, page, size int) ([]OrderInfo, int, error)

	// SyncAll 全量同步(商品 + 订单 + 库存)
	SyncAll(ctx context.Context) (*SyncResult, error)

	// Close 释放资源
	Close() error
}

// ============== Adapter 注册表 ==============

// AdapterFactory 适配器工厂函数
type AdapterFactory func(cfg AccountConfig) (PlatformAdapter, error)

var (
	adapterFactories = make(map[Platform]AdapterFactory)
)

// RegisterFactory 注册平台适配器工厂(供外部 SDK 接入时调用)
func RegisterFactory(p Platform, f AdapterFactory) {
	adapterFactories[p] = f
}

// NewAdapter 创建适配器实例
//
// 若对应平台未注册真实工厂,则返回 BuiltinAdapter(返回本地预置数据)
// 这保证了系统在未配置真实平台 SDK 时仍可正常运行
func NewAdapter(cfg AccountConfig) (PlatformAdapter, error) {
	if f, ok := adapterFactories[cfg.Platform]; ok {
		return f(cfg)
	}
	return newBuiltinAdapter(cfg), nil
}

// SupportedPlatforms 已支持的平台列表
func SupportedPlatforms() []Platform {
	return []Platform{PlatformAmazon, PlatformTemu, PlatformTikTok}
}

// IsValidPlatform 校验平台标识
func IsValidPlatform(p string) bool {
	switch Platform(p) {
	case PlatformAmazon, PlatformTemu, PlatformTikTok:
		return true
	}
	return false
}

// ============== Builtin Adapter(本地兜底) ==============
// 未接入真实平台 SDK 时使用,返回符合接口契约的本地数据

type builtinAdapter struct {
	cfg AccountConfig
}

func newBuiltinAdapter(cfg AccountConfig) *builtinAdapter {
	return &builtinAdapter{cfg: cfg}
}

func (a *builtinAdapter) Name() Platform { return a.cfg.Platform }

func (a *builtinAdapter) Auth(ctx context.Context) error {
	// Builtin 模式无需认证
	return nil
}

func (a *builtinAdapter) ListProducts(ctx context.Context, page, size int) ([]ProductInfo, int, error) {
	now := time.Now()
	listedAt := now.AddDate(0, -2, 0)
	products := []ProductInfo{
		{
			Platform:      a.cfg.Platform,
			SellerSKU:     fmt.Sprintf("SKU-%s-001", a.cfg.Platform),
			ASIN:          "B0XXXXXXXX1",
			ItemName:      "Wireless Bluetooth Headphones",
			ItemID:        fmt.Sprintf("%s-item-001", a.cfg.Platform),
			Price:         29.99,
			Currency:      "USD",
			StockQuantity: 156,
			Status:        "active",
			ListedAt:      &listedAt,
			UpdatedAt:     &now,
		},
		{
			Platform:      a.cfg.Platform,
			SellerSKU:     fmt.Sprintf("SKU-%s-002", a.cfg.Platform),
			ASIN:          "B0XXXXXXXX2",
			ItemName:      "USB-C Fast Charger 65W",
			ItemID:        fmt.Sprintf("%s-item-002", a.cfg.Platform),
			Price:         19.99,
			Currency:      "USD",
			StockQuantity: 240,
			Status:        "active",
			ListedAt:      &listedAt,
			UpdatedAt:     &now,
		},
		{
			Platform:      a.cfg.Platform,
			SellerSKU:     fmt.Sprintf("SKU-%s-003", a.cfg.Platform),
			ASIN:          "B0XXXXXXXX3",
			ItemName:      "Smart Watch Fitness Tracker",
			ItemID:        fmt.Sprintf("%s-item-003", a.cfg.Platform),
			Price:         49.99,
			Currency:      "USD",
			StockQuantity: 78,
			Status:        "active",
			ListedAt:      &listedAt,
			UpdatedAt:     &now,
		},
	}
	total := len(products)
	// 简单分页
	start := (page - 1) * size
	if start >= total {
		return []ProductInfo{}, total, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return products[start:end], total, nil
}

func (a *builtinAdapter) GetProduct(ctx context.Context, skuOrID string) (*ProductInfo, error) {
	products, _, err := a.ListProducts(ctx, 1, 100)
	if err != nil {
		return nil, err
	}
	for i := range products {
		if products[i].SellerSKU == skuOrID || products[i].ItemID == skuOrID || products[i].ASIN == skuOrID {
			return &products[i], nil
		}
	}
	return nil, fmt.Errorf("product %s not found on %s", skuOrID, a.cfg.Platform)
}

func (a *builtinAdapter) ListOrders(ctx context.Context, start, end time.Time, page, size int) ([]OrderInfo, int, error) {
	created := time.Now().Add(-24 * time.Hour)
	updated := time.Now()
	orders := []OrderInfo{
		{
			Platform:    a.cfg.Platform,
			OrderID:     fmt.Sprintf("%s-order-001", a.cfg.Platform),
			Status:      "shipped",
			TotalAmount: 89.97,
			Currency:    "USD",
			Items: []OrderItem{
				{SKU: fmt.Sprintf("SKU-%s-001", a.cfg.Platform), ProductName: "Wireless Bluetooth Headphones", Quantity: 3, ItemPrice: 29.99, TotalPrice: 89.97},
			},
			BuyerName:    "John Doe",
			BuyerEmail:   "buyer1@example.com",
			ShippingAddr: "1234 Main St, Los Angeles, CA 90001, USA",
			CreatedAt:    &created,
			UpdatedAt:    &updated,
		},
		{
			Platform:    a.cfg.Platform,
			OrderID:     fmt.Sprintf("%s-order-002", a.cfg.Platform),
			Status:      "pending",
			TotalAmount: 119.97,
			Currency:    "USD",
			Items: []OrderItem{
				{SKU: fmt.Sprintf("SKU-%s-002", a.cfg.Platform), ProductName: "USB-C Fast Charger 65W", Quantity: 3, ItemPrice: 19.99, TotalPrice: 59.97},
				{SKU: fmt.Sprintf("SKU-%s-003", a.cfg.Platform), ProductName: "Smart Watch Fitness Tracker", Quantity: 1, ItemPrice: 49.99, TotalPrice: 49.99},
			},
			BuyerName:    "Jane Smith",
			BuyerEmail:   "buyer2@example.com",
			ShippingAddr: "5678 Oak Ave, New York, NY 10001, USA",
			CreatedAt:    &created,
			UpdatedAt:    &updated,
		},
	}
	total := len(orders)
	startIdx := (page - 1) * size
	if startIdx >= total {
		return []OrderInfo{}, total, nil
	}
	endIdx := startIdx + size
	if endIdx > total {
		endIdx = total
	}
	return orders[startIdx:endIdx], total, nil
}

func (a *builtinAdapter) SyncAll(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{
		Platform:   a.cfg.Platform,
		LastSyncAt: start,
	}

	products, _, err := a.ListProducts(ctx, 1, 100)
	if err != nil {
		result.Errors = append(result.Errors, SyncError{ResourceType: "product", Error: err.Error()})
	} else {
		result.NewProducts = len(products)
	}

	orders, _, err := a.ListOrders(ctx, start.AddDate(0, -1, 0), start, 1, 100)
	if err != nil {
		result.Errors = append(result.Errors, SyncError{ResourceType: "order", Error: err.Error()})
	} else {
		result.NewOrders = len(orders)
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (a *builtinAdapter) Close() error { return nil }
