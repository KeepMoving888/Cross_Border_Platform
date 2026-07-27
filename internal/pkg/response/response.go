package response

import (
	"net/http"

	"github.com/cb-platform/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`    // 0 表示成功,非 0 表示业务错误码
	Message string      `json:"message"` // 提示信息
	Data    interface{} `json:"data,omitempty"`
	TraceID string      `json:"trace_id,omitempty"`
}

// PageResult 分页结果
type PageResult struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
		TraceID: c.GetString("trace_id"),
	})
}

// OKWithMsg 自定义消息的成功响应
func OKWithMsg(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: msg,
		Data:    data,
		TraceID: c.GetString("trace_id"),
	})
}

// OKPage 分页响应
func OKPage(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: PageResult{
			List:     list,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
		TraceID: c.GetString("trace_id"),
	})
}

// Fail 失败响应
func Fail(c *gin.Context, err error) {
	bizErr := errors.FromError(err)
	c.JSON(bizErr.HTTPStatus(), Response{
		Code:    bizErr.Code,
		Message: bizErr.Message,
		TraceID: c.GetString("trace_id"),
	})
}

// FailWithCode 指定错误码的失败响应
func FailWithCode(c *gin.Context, err *errors.Error) {
	c.JSON(err.HTTPStatus(), Response{
		Code:    err.Code,
		Message: err.Message,
		TraceID: c.GetString("trace_id"),
	})
}

// AbortFail 中断并返回失败
func AbortFail(c *gin.Context, err error) {
	bizErr := errors.FromError(err)
	c.AbortWithStatusJSON(bizErr.HTTPStatus(), Response{
		Code:    bizErr.Code,
		Message: bizErr.Message,
		TraceID: c.GetString("trace_id"),
	})
}
