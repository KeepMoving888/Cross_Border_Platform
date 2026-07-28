package handler

import (
	"strconv"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/errors"
	"github.com/cb-platform/internal/pkg/middleware"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserHandler struct {
	db *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// GetCurrentUser 获取当前登录用户信息
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, _ := strconv.Atoi(userID)

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrUserNotFound)
		return
	}
	response.OK(c, user)
}

type updateUserProfileRequest struct {
	RealName   string `json:"real_name" binding:"max=64"`
	Email      string `json:"email" binding:"omitempty,email,max=128"`
	Phone      string `json:"phone" binding:"max=32"`
	Department string `json:"department" binding:"max=64"`
	Avatar     string `json:"avatar" binding:"max=512"`
}

func (h *UserHandler) UpdateCurrentUser(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, _ := strconv.Atoi(userID)

	var req updateUserProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	updates := map[string]interface{}{
		"real_name":  req.RealName,
		"email":      req.Email,
		"phone":      req.Phone,
		"department": req.Department,
		"avatar":     req.Avatar,
	}
	if err := h.db.Model(&models.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}

	var user models.User
	h.db.First(&user, id)
	response.OKWithMsg(c, "更新成功", user)
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=6"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=64"`
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, _ := strconv.Atoi(userID)

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrUserNotFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		response.FailWithCode(c, errors.ErrWrongPassword)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		response.Fail(c, errors.ErrInternal)
		return
	}

	h.db.Model(&user).Update("password", string(hashed))
	response.OKWithMsg(c, "密码修改成功", nil)
}

// ListUsers 用户列表(管理员)
func (h *UserHandler) ListUsers(c *gin.Context) {
	p := models.Pagination{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
	}
	p.Normalize()

	query := h.db.Model(&models.User{})
	if keyword := c.Query("keyword"); keyword != "" {
		query = query.Where("username LIKE ? OR real_name LIKE ? OR email LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if role := c.Query("role"); role != "" {
		query = query.Where("role = ?", role)
	}

	var total int64
	query.Count(&total)

	var users []models.User
	query.Order("id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&users)

	response.OKPage(c, users, total, p.Page, p.PageSize)
}

func (h *UserHandler) GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.FailWithCode(c, errors.ErrUserNotFound)
		return
	}
	response.OK(c, user)
}

type updateUserRequest struct {
	RealName   string              `json:"real_name"`
	Email      string              `json:"email"`
	Phone      string              `json:"phone"`
	Role       string              `json:"role"`
	Department string              `json:"department"`
	Status     models.CommonStatus `json:"status"`
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}
	updates := map[string]interface{}{
		"real_name":  req.RealName,
		"email":      req.Email,
		"phone":      req.Phone,
		"role":       req.Role,
		"department": req.Department,
		"status":     req.Status,
	}
	if err := h.db.Model(&models.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	var user models.User
	h.db.First(&user, id)
	response.OKWithMsg(c, "更新成功", user)
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.FailWithCode(c, errors.ErrInvalidParam)
		return
	}
	if err := h.db.Delete(&models.User{}, id).Error; err != nil {
		response.Fail(c, errors.ErrDBOperation)
		return
	}
	response.OKWithMsg(c, "删除成功", nil)
}

// atoiDefault 字符串转 int 带默认值
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
