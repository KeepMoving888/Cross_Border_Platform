package platform

import (
	"context"
	"testing"
	"time"
)

func TestIsValidPlatform(t *testing.T) {
	tests := []struct {
		platform string
		expect   bool
	}{
		{"amazon", true},
		{"temu", true},
		{"tiktok", true},
		{"ebay", false},
		{"", false},
		{"AMAZON", false}, // 大小写敏感
	}
	for _, tt := range tests {
		if got := IsValidPlatform(tt.platform); got != tt.expect {
			t.Errorf("IsValidPlatform(%q) = %v, want %v", tt.platform, got, tt.expect)
		}
	}
}

func TestSupportedPlatforms(t *testing.T) {
	list := SupportedPlatforms()
	if len(list) != 3 {
		t.Errorf("expected 3 platforms, got %d", len(list))
	}
	seen := map[Platform]bool{}
	for _, p := range list {
		seen[p] = true
	}
	if !seen[PlatformAmazon] || !seen[PlatformTemu] || !seen[PlatformTikTok] {
		t.Error("missing required platform")
	}
}

func TestNewAdapter_BuiltinFallback(t *testing.T) {
	cfg := AccountConfig{
		ID:       1,
		Platform: PlatformAmazon,
		Region:   "US",
		SellerID: "SELLER001",
	}
	adapter, err := NewAdapter(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.Name() != PlatformAmazon {
		t.Errorf("expected amazon, got %s", adapter.Name())
	}
	defer adapter.Close()
}

func TestBuiltinAdapter_Auth(t *testing.T) {
	cfg := AccountConfig{Platform: PlatformTemu}
	adapter := newBuiltinAdapter(cfg)
	if err := adapter.Auth(context.Background()); err != nil {
		t.Errorf("builtin auth should not fail, got %v", err)
	}
}

func TestBuiltinAdapter_ListProducts(t *testing.T) {
	cfg := AccountConfig{Platform: PlatformAmazon, Region: "US"}
	adapter := newBuiltinAdapter(cfg)

	// 第一页,3 个商品
	products, total, err := adapter.ListProducts(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
	if len(products) != 3 {
		t.Errorf("expected 3 products, got %d", len(products))
	}

	// 验证字段
	for _, p := range products {
		if p.Platform != PlatformAmazon {
			t.Errorf("expected platform=amazon, got %s", p.Platform)
		}
		if p.SellerSKU == "" {
			t.Error("expected non-empty SKU")
		}
		if p.Price <= 0 {
			t.Error("expected positive price")
		}
	}

	// 超出页数应返回空
	products, _, err = adapter.ListProducts(context.Background(), 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(products) != 0 {
		t.Errorf("expected 0 products on page 2, got %d", len(products))
	}

	// 分页测试:page=1, size=2 应返回 2 个
	products, _, err = adapter.ListProducts(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(products) != 2 {
		t.Errorf("expected 2 products with size=2, got %d", len(products))
	}
}

func TestBuiltinAdapter_GetProduct(t *testing.T) {
	cfg := AccountConfig{Platform: PlatformAmazon}
	adapter := newBuiltinAdapter(cfg)

	// 先拿一个 SKU
	products, _, _ := adapter.ListProducts(context.Background(), 1, 10)
	if len(products) == 0 {
		t.Fatal("no products for setup")
	}
	sku := products[0].SellerSKU

	// 通过 SKU 查询
	p, err := adapter.GetProduct(context.Background(), sku)
	if err != nil {
		t.Fatalf("get product by SKU failed: %v", err)
	}
	if p.SellerSKU != sku {
		t.Errorf("returned wrong product: expected SKU %s, got %s", sku, p.SellerSKU)
	}

	// 通过 ItemID 查询
	p, err = adapter.GetProduct(context.Background(), products[0].ItemID)
	if err != nil {
		t.Fatalf("get product by ItemID failed: %v", err)
	}

	// 通过 ASIN 查询
	p, err = adapter.GetProduct(context.Background(), products[0].ASIN)
	if err != nil {
		t.Fatalf("get product by ASIN failed: %v", err)
	}

	// 不存在的商品
	_, err = adapter.GetProduct(context.Background(), "NOT_EXIST")
	if err == nil {
		t.Error("expected error for non-existent product")
	}
}

func TestBuiltinAdapter_ListOrders(t *testing.T) {
	cfg := AccountConfig{Platform: PlatformTikTok}
	adapter := newBuiltinAdapter(cfg)

	now := time.Now()
	orders, total, err := adapter.ListOrders(context.Background(), now.AddDate(0, -1, 0), now, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if len(orders) != 2 {
		t.Errorf("expected 2 orders, got %d", len(orders))
	}

	// 验证字段
	for _, o := range orders {
		if o.Platform != PlatformTikTok {
			t.Errorf("expected platform=tiktok, got %s", o.Platform)
		}
		if o.OrderID == "" {
			t.Error("expected non-empty order ID")
		}
		if len(o.Items) == 0 {
			t.Error("expected non-empty items")
		}
	}
}

func TestBuiltinAdapter_SyncAll(t *testing.T) {
	cfg := AccountConfig{Platform: PlatformTemu}
	adapter := newBuiltinAdapter(cfg)

	result, err := adapter.SyncAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Platform != PlatformTemu {
		t.Errorf("expected platform=temu, got %s", result.Platform)
	}
	if result.NewProducts == 0 {
		t.Error("expected non-zero new products")
	}
	if result.NewOrders == 0 {
		t.Error("expected non-zero new orders")
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestRegisterFactory(t *testing.T) {
	// 注册一个测试用工厂
	customAdapter := &customTestAdapter{platform: PlatformAmazon}
	RegisterFactory(PlatformAmazon, func(cfg AccountConfig) (PlatformAdapter, error) {
		return customAdapter, nil
	})

	cfg := AccountConfig{Platform: PlatformAmazon}
	adapter, err := NewAdapter(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter != customAdapter {
		t.Error("expected custom adapter to be returned")
	}

	// 测试未注册的平台仍走 Builtin
	cfg2 := AccountConfig{Platform: PlatformTikTok}
	adapter2, err := NewAdapter(cfg2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter2.Name() != PlatformTikTok {
		t.Errorf("expected tiktok, got %s", adapter2.Name())
	}

	// 清理:重新注册为 nil 以恢复默认行为(实际中不需要,因为 map 是全局的)
	// 这里通过注册一个返回 builtin 的工厂来"清理"
	RegisterFactory(PlatformAmazon, func(cfg AccountConfig) (PlatformAdapter, error) {
		return newBuiltinAdapter(cfg), nil
	})
}

// 自定义测试 adapter
type customTestAdapter struct {
	platform Platform
}

func (a *customTestAdapter) Name() Platform { return a.platform }
func (a *customTestAdapter) Auth(ctx context.Context) error { return nil }
func (a *customTestAdapter) ListProducts(ctx context.Context, page, size int) ([]ProductInfo, int, error) {
	return nil, 0, nil
}
func (a *customTestAdapter) GetProduct(ctx context.Context, skuOrID string) (*ProductInfo, error) {
	return nil, nil
}
func (a *customTestAdapter) ListOrders(ctx context.Context, start, end time.Time, page, size int) ([]OrderInfo, int, error) {
	return nil, 0, nil
}
func (a *customTestAdapter) SyncAll(ctx context.Context) (*SyncResult, error) {
	return &SyncResult{Platform: a.platform}, nil
}
func (a *customTestAdapter) Close() error { return nil }
