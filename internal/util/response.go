package util

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 统一错误码定义
const (
	CodeSuccess       = 1    // 成功
	CodeError         = -1   // 通用错误
	CodeUnauthorized  = -401 // 未授权
	CodeForbidden     = -403 // 禁止访问
	CodeNotFound      = -404 // 资源不存在
	CodeValidation    = -422 // 参数验证失败
	CodeRateLimit     = -429 // 请求过于频繁
	CodeServerError   = -500 // 服务器内部错误
	CodePaymentFailed = -600 // 支付失败
	CodeOrderExpired  = -601 // 订单过期
	CodeInvalidSign   = -602 // 签名错误
)

// Response 统一响应结构
type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// PageResponse 分页响应结构
type PageResponse struct {
	Code  int         `json:"code"`
	Msg   string      `json:"msg"`
	Data  interface{} `json:"data,omitempty"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Msg:  "success",
		Data: data,
	})
}

// SuccessWithMsg 成功响应（自定义消息）
func SuccessWithMsg(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Msg:  msg,
		Data: data,
	})
}

// SuccessPage 分页成功响应
func SuccessPage(c *gin.Context, data interface{}, total int64, page int) {
	c.JSON(http.StatusOK, PageResponse{
		Code:  CodeSuccess,
		Msg:   "success",
		Data:  data,
		Total: total,
		Page:  page,
	})
}

// Error 错误响应
func Error(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: CodeError,
		Msg:  msg,
	})
}

// ErrorWithCode 带错误码的错误响应
func ErrorWithCode(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Msg:  msg,
	})
}

// ErrorWithData 带数据的错误响应
func ErrorWithData(c *gin.Context, code int, msg string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Msg:  msg,
		Data: data,
	})
}

// Unauthorized 未授权响应
func Unauthorized(c *gin.Context, msg string) {
	if msg == "" {
		msg = "未登录或登录已过期"
	}
	c.JSON(http.StatusUnauthorized, Response{
		Code: CodeUnauthorized,
		Msg:  msg,
	})
}

// Forbidden 禁止访问响应
func Forbidden(c *gin.Context, msg string) {
	if msg == "" {
		msg = "没有访问权限"
	}
	c.JSON(http.StatusForbidden, Response{
		Code: CodeForbidden,
		Msg:  msg,
	})
}

// NotFound 资源不存在响应
func NotFound(c *gin.Context, msg string) {
	if msg == "" {
		msg = "资源不存在"
	}
	c.JSON(http.StatusOK, Response{
		Code: CodeNotFound,
		Msg:  msg,
	})
}

// ValidationError 参数验证失败响应
func ValidationError(c *gin.Context, msg string) {
	if msg == "" {
		msg = "参数错误"
	}
	c.JSON(http.StatusOK, Response{
		Code: CodeValidation,
		Msg:  msg,
	})
}

// RateLimitError 请求过于频繁响应
func RateLimitError(c *gin.Context) {
	c.JSON(http.StatusTooManyRequests, Response{
		Code: CodeRateLimit,
		Msg:  "请求过于频繁，请稍后再试",
	})
}

// ServerError 服务器内部错误响应
func ServerError(c *gin.Context, msg string) {
	if msg == "" {
		msg = "服务器内部错误"
	}
	c.JSON(http.StatusOK, Response{
		Code: CodeServerError,
		Msg:  msg,
	})
}

// PaymentError 支付错误响应
func PaymentError(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Msg:  msg,
	})
}

// AbortWithError 中止请求并返回错误
func AbortWithError(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{
		Code: code,
		Msg:  msg,
	})
	c.Abort()
}
