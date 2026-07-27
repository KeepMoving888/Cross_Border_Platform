package errors

import (
	"fmt"
	"net/http"
)

// Error 统一业务错误
type Error struct {
	Code    int    `json:"code"`    // 业务错误码
	Message string `json:"message"` // 错误信息
	Err     error  `json:"-"`       // 原始错误
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// New 构造业务错误
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap 包装原始错误
func Wrap(err error, code int, message string) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

// HTTPStatus 业务错误码到 HTTP 状态码的映射
func (e *Error) HTTPStatus() int {
	switch {
	case e.Code >= 1000 && e.Code < 2000:
		return http.StatusBadRequest
	case e.Code >= 2000 && e.Code < 3000:
		return http.StatusUnauthorized
	case e.Code >= 3000 && e.Code < 4000:
		return http.StatusForbidden
	case e.Code >= 4000 && e.Code < 5000:
		return http.StatusNotFound
	case e.Code >= 5000 && e.Code < 6000:
		return http.StatusConflict
	case e.Code >= 9000:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// 预定义错误码段:
// 1xxx: 参数错误
// 2xxx: 认证错误
// 3xxx: 权限错误
// 4xxx: 资源不存在
// 5xxx: 状态冲突
// 9xxx: 系统错误

var (
	ErrInvalidParam    = New(1001, "参数错误")
	ErrInvalidJSON     = New(1002, "JSON 解析失败")
	ErrValidation      = New(1003, "数据校验失败")

	ErrUnauthorized    = New(2001, "未登录或登录已过期")
	ErrInvalidToken    = New(2002, "无效的 Token")
	ErrAccountDisabled = New(2003, "账号已被禁用")
	ErrWrongPassword   = New(2004, "账号或密码错误")

	ErrForbidden       = New(3001, "无操作权限")

	ErrNotFound        = New(4001, "资源不存在")
	ErrUserNotFound    = New(4002, "用户不存在")
	ErrProductNotFound = New(4003, "商品不存在")
	ErrOrderNotFound   = New(4004, "订单不存在")

	ErrStateConflict   = New(5001, "状态冲突,当前状态不允许该操作")
	ErrDuplicateEntry  = New(5002, "数据已存在")
	ErrStockInsufficient = New(5003, "库存不足")

	ErrInternal        = New(9001, "系统内部错误")
	ErrDBOperation     = New(9002, "数据库操作失败")
	ErrExternalAPI     = New(9003, "外部 API 调用失败")
	ErrLLMUnavailable  = New(9004, "LLM 服务不可用")
)

// FromError 从普通 error 转换为业务错误
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return Wrap(err, 9001, "系统内部错误")
}
