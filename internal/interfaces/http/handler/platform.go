package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/domain/platform"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PlatformHandler 平台对接 handler
//
// 设计原则:
//   - 对外提供统一 RESTful 接口
//   - 内部通过 PlatformAdapter 接口对接不同平台,业务层不感知平台 SDK 差异
//   - 真实平台 SDK 接入时,只需在 platform.RegisterFactory 中注册即可
type PlatformHandler struct {
	db *gorm.DB
}

func NewPlatformHandler(db *gorm.DB) *PlatformHandler {
	return &PlatformHandler{db: db}
}

// ============== 平台账号管理 ==============

func (h *PlatformHandler) ListAccounts(c *gin.Context) {
	var list []models.PlatformAccount
	query := h.db.Model(&models.PlatformAccount{})
	if p := c.Query("platform"); p != "" {
		query = query.Where("platform = ?", p)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	query.Order("id DESC").Find(&list)
	response.OK(c, list)
}

type createAccountRequest struct {
	Name     string `json:"name" binding:"required,max=128"`
	Platform string `json:"platform" binding:"required,oneof=amazon temu tiktok"`
	Region   string `json:"region"`
	SellerID string `json:"seller_id"`
	// 授权信息(后续接入时使用)
	RefreshToken string `json:"refresh_token"`
	Metadata     string `json:"metadata"`
}

func (h *PlatformHandler) CreateAccount(c *gin.Context) {
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	acc := models.PlatformAccount{
		Name:         req.Name,
		Platform:     req.Platform,
		Region:       req.Region,
		SellerID:     req.SellerID,
		RefreshToken: req.RefreshToken,
		Metadata:     req.Metadata,
		Status:       models.StatusEnabled,
	}
	if err := h.db.Create(&acc).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建平台账号失败"))
		return
	}
	response.OKWithMsg(c, "平台账号添加成功", acc)
}

func (h *PlatformHandler) UpdateAccount(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"name":          req.Name,
		"region":        req.Region,
		"seller_id":     req.SellerID,
		"refresh_token": req.RefreshToken,
		"metadata":      req.Metadata,
	}
	if err := h.db.Model(&models.PlatformAccount{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var acc models.PlatformAccount
	h.db.First(&acc, id)
	response.OKWithMsg(c, "更新成功", acc)
}

// SyncAccount 触发数据同步
//
// 通过 PlatformAdapter 接口调用对应平台的同步逻辑:
//   - 已注册真实平台 SDK 时,执行真实同步
//   - 未注册时,使用 Builtin Adapter 返回本地数据,保证流程可联调
func (h *PlatformHandler) SyncAccount(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var acc models.PlatformAccount
	if err := h.db.First(&acc, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}

	cfg := platform.AccountConfig{
		ID:           acc.ID,
		Platform:     platform.Platform(acc.Platform),
		Region:       acc.Region,
		SellerID:     acc.SellerID,
		RefreshToken: acc.RefreshToken,
	}

	adapter, err := platform.NewAdapter(cfg)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9003, "创建平台适配器失败"))
		return
	}
	defer adapter.Close()

	// 设置同步超时(平台 API 可能较慢)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	result, err := adapter.SyncAll(ctx)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "同步失败"))
		return
	}

	response.OKWithMsg(c, "数据同步完成", result)
}

// ListPlatformProducts 平台商品列表
func (h *PlatformHandler) ListPlatformProducts(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	var acc models.PlatformAccount
	if err := h.db.First(&acc, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}

	cfg := platform.AccountConfig{
		ID:       acc.ID,
		Platform: platform.Platform(acc.Platform),
		Region:   acc.Region,
		SellerID: acc.SellerID,
	}
	adapter, err := platform.NewAdapter(cfg)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9003, "创建平台适配器失败"))
		return
	}
	defer adapter.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	products, total, err := adapter.ListProducts(ctx, page, size)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "获取平台商品失败"))
		return
	}

	response.OK(c, gin.H{
		"account_id": acc.ID,
		"platform":   acc.Platform,
		"page":       page,
		"size":       size,
		"total":      total,
		"products":   products,
	})
}

// ListPlatformOrders 平台订单列表
func (h *PlatformHandler) ListPlatformOrders(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	// 默认查询最近 30 天
	end := time.Now()
	start := end.AddDate(0, -1, 0)

	var acc models.PlatformAccount
	if err := h.db.First(&acc, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}

	cfg := platform.AccountConfig{
		ID:       acc.ID,
		Platform: platform.Platform(acc.Platform),
		Region:   acc.Region,
		SellerID: acc.SellerID,
	}
	adapter, err := platform.NewAdapter(cfg)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9003, "创建平台适配器失败"))
		return
	}
	defer adapter.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	orders, total, err := adapter.ListOrders(ctx, start, end, page, size)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9004, "获取平台订单失败"))
		return
	}

	response.OK(c, gin.H{
		"account_id": acc.ID,
		"platform":   acc.Platform,
		"page":       page,
		"size":       size,
		"total":      total,
		"start_date": start.Format("2006-01-02"),
		"end_date":   end.Format("2006-01-02"),
		"orders":     orders,
	})
}
