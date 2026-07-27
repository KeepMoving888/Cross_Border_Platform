package middleware

import (
	"strings"
	"time"

	"github.com/cb-platform/internal/pkg/config"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims 自定义 Claims
type JWTClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT
func GenerateToken(userID, username, role string) (string, error) {
	cfg := config.Get()
	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(cfg.JWT.ExpireHours) * time.Hour)),
			Issuer:    cfg.App.Name,
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWT.Secret))
}

// ParseToken 解析 JWT
func ParseToken(tokenStr string) (*JWTClaims, error) {
	cfg := config.Get()
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return []byte(cfg.JWT.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}

// Auth JWT 鉴权中间件
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(401, response.Response{Code: 2001, Message: "未登录或登录已过期", TraceID: c.GetString("trace_id")})
			return
		}

		claims, err := ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(401, response.Response{Code: 2002, Message: "无效的 Token", TraceID: c.GetString("trace_id")})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// OptionalAuth 可选鉴权(登录可获取用户信息,未登录也放行)
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.Next()
			return
		}
		if claims, err := ParseToken(token); err == nil {
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("role", claims.Role)
		}
		c.Next()
	}
}

// RequireRole 角色校验
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		role := c.GetString("role")
		if !allowed[role] {
			c.AbortWithStatusJSON(403, response.Response{Code: 3001, Message: "无操作权限", TraceID: c.GetString("trace_id")})
			return
		}
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	// 1. Header: Authorization: Bearer <token>
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if auth != "" {
		return auth
	}
	// 2. Query: ?token=<token>
	if q := c.Query("token"); q != "" {
		return q
	}
	return ""
}

// GetUserID 从上下文获取用户 ID
func GetUserID(c *gin.Context) string {
	return c.GetString("user_id")
}

// GetUsername 从上下文获取用户名
func GetUsername(c *gin.Context) string {
	return c.GetString("username")
}

// GetRole 从上下文获取角色
func GetRole(c *gin.Context) string {
	return c.GetString("role")
}
