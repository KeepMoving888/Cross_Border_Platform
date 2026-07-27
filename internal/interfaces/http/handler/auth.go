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

type AuthHandler struct {
	db *gorm.DB
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

type loginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

type loginUserInfo struct {
	ID         uint     `json:"id"`
	Username   string   `json:"username"`
	Nickname   string   `json:"nickname"`
	Avatar     string   `json:"avatar,omitempty"`
	Email      string   `json:"email,omitempty"`
	Role       string   `json:"role"`
	Roles      []string `json:"roles"`
	Department string   `json:"department,omitempty"`
}

type loginResponse struct {
	Token    string        `json:"token"`
	User     loginUserInfo `json:"user"`
	UserInfo loginUserInfo `json:"user_info"`
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		response.Fail(c, errors.ErrWrongPassword)
		return
	}

	if user.Status != models.StatusEnabled {
		response.FailWithCode(c, errors.ErrAccountDisabled)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		response.FailWithCode(c, errors.ErrWrongPassword)
		return
	}

	token, err := middleware.GenerateToken(strconv.Itoa(int(user.ID)), user.Username, user.Role)
	if err != nil {
		response.Fail(c, errors.Wrap(err, 9001, "生成 Token 失败"))
		return
	}

	// 更新最近登录时间
	h.db.Model(&user).Update("last_login_at", gorm.Expr("NOW()"))

	info := loginUserInfo{
		ID:         user.ID,
		Username:   user.Username,
		Nickname:   user.RealName,
		Avatar:     user.Avatar,
		Email:      user.Email,
		Role:       user.Role,
		Roles:      []string{user.Role},
		Department: user.Department,
	}
	if info.Nickname == "" {
		info.Nickname = user.Username
	}
	response.OK(c, loginResponse{Token: token, User: info, UserInfo: info})
}

type registerRequest struct {
	Username   string `json:"username" binding:"required,min=2,max=64"`
	Password   string `json:"password" binding:"required,min=6,max=64"`
	RealName   string `json:"real_name" binding:"max=64"`
	Email      string `json:"email" binding:"omitempty,email,max=128"`
	Phone      string `json:"phone" binding:"max=32"`
	Department string `json:"department" binding:"max=64"`
}

// Register 注册(仅管理员可调用,这里简化为开发环境开放)
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errors.Wrap(err, 1001, "参数错误"))
		return
	}

	// 检查用户名是否已存在
	var count int64
	h.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		response.FailWithCode(c, errors.ErrDuplicateEntry)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.Fail(c, errors.ErrInternal)
		return
	}

	user := models.User{
		Username:   req.Username,
		Password:   string(hashed),
		RealName:   req.RealName,
		Email:      req.Email,
		Phone:      req.Phone,
		Department: req.Department,
		Role:       "staff",
		Status:     models.StatusEnabled,
	}

	if err := h.db.Create(&user).Error; err != nil {
		response.Fail(c, errors.Wrap(err, 9002, "创建用户失败"))
		return
	}

	response.OKWithMsg(c, "注册成功", user)
}

// RefreshToken 刷新 Token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	username := middleware.GetUsername(c)
	role := middleware.GetRole(c)

	if userID == "" {
		response.FailWithCode(c, errors.ErrUnauthorized)
		return
	}

	token, err := middleware.GenerateToken(userID, username, role)
	if err != nil {
		response.Fail(c, errors.ErrInternal)
		return
	}

	response.OK(c, gin.H{"token": token})
}
