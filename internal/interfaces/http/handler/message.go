package handler

import (
	"strconv"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MessageHandler struct {
	db *gorm.DB
}

func NewMessageHandler(db *gorm.DB) *MessageHandler {
	return &MessageHandler{db: db}
}

// List 当前用户消息列表
func (h *MessageHandler) List(c *gin.Context) {
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	p := Pagination{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
	}
	p.Normalize()

	query := h.db.Model(&models.Message{}).Where("user_id = ?", userID)
	if isRead := c.Query("is_read"); isRead == "true" {
		query = query.Where("is_read = ?", true)
	} else if isRead == "false" {
		query = query.Where("is_read = ?", false)
	}
	if msgType := c.Query("type"); msgType != "" {
		query = query.Where("type = ?", msgType)
	}

	var total int64
	query.Count(&total)

	var list []models.Message
	query.Order("id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&list)
	response.OKPage(c, list, total, p.Page, p.PageSize)
}

// UnreadCount 未读消息数
func (h *MessageHandler) UnreadCount(c *gin.Context) {
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	var count int64
	h.db.Model(&models.Message{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&count)
	response.OK(c, gin.H{"count": count})
}

// MarkRead 标记单条已读
func (h *MessageHandler) MarkRead(c *gin.Context) {
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	result := h.db.Model(&models.Message{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true)
	if result.Error != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	if result.RowsAffected == 0 {
		response.FailWithCode(c, errors.ErrNotFound)
		return
	}
	response.OKWithMsg(c, "已标记已读", nil)
}

// MarkAllRead 全部标记已读
func (h *MessageHandler) MarkAllRead(c *gin.Context) {
	userID, _ := strconv.Atoi(middleware.GetUserID(c))
	if err := h.db.Model(&models.Message{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "全部已读", nil)
}
