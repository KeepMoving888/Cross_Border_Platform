package middleware

import (
	"strings"
	"time"

	"github.com/cb-platform/internal/pkg/logger"
	"github.com/cb-platform/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TraceID 链路追踪中间件,为每个请求生成唯一 trace_id
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.New().String()
		}
		c.Set("trace_id", traceID)
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

// Logger 请求日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		traceID := c.GetString("trace_id")

		fields := []interface{}{
			"trace_id", traceID,
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}

		if userID := c.GetString("user_id"); userID != "" {
			fields = append(fields, "user_id", userID)
		}

		if len(c.Errors) > 0 {
			logger.Get().With(fields...).Errorw("request completed with errors", "errors", c.Errors.String())
		} else if status >= 500 {
			logger.Get().With(fields...).Errorw("request failed")
		} else if status >= 400 {
			logger.Get().With(fields...).Warnw("request client error")
		} else {
			logger.Get().With(fields...).Infow("request completed")
		}
	}
}

// CORS 跨域中间件配置
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Trace-Id, X-Request-ID")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// Recovery panic 恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Get().Errorw("panic recovered",
					"trace_id", c.GetString("trace_id"),
					"error", err,
					"path", c.Request.URL.Path,
				)
				c.AbortWithStatusJSON(500, response.Response{
					Code:    9001,
					Message: "系统内部错误",
					TraceID: c.GetString("trace_id"),
				})
			}
		}()
		c.Next()
	}
}

// RateLimiter 简易令牌桶限流(每秒 N 次,基于 IP)
func RateLimiter(rps int) gin.HandlerFunc {
	// 简化实现:生产环境应使用 Redis 分布式限流
	// 这里使用 token bucket,每请求消耗一个令牌
	bucket := make(chan struct{}, rps)
	// 预填充令牌,确保首个请求不被拒绝
	for i := 0; i < rps; i++ {
		bucket <- struct{}{}
	}
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rps))
		defer ticker.Stop()
		for range ticker.C {
			select {
			case bucket <- struct{}{}:
			default:
			}
		}
	}()

	return func(c *gin.Context) {
		select {
		case <-bucket:
			c.Next()
		default:
			c.AbortWithStatusJSON(429, response.Response{
				Code:    1005,
				Message: "请求过于频繁,请稍后再试",
				TraceID: c.GetString("trace_id"),
			})
		}
	}
}

// SkipPaths 不记录日志的路径
func SkipPaths(paths []string) gin.HandlerFunc {
	skip := make(map[string]bool, len(paths))
	for _, p := range paths {
		skip[strings.TrimSpace(p)] = true
	}
	return func(c *gin.Context) {
		if skip[c.Request.URL.Path] {
			return
		}
		c.Next()
	}
}
