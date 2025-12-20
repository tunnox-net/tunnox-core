// Package errors 提供统一的错误处理机制
//
// 设计原则：
// 1. 所有错误都应该可以通过 errors.Is() 和 errors.As() 进行类型检查
// 2. 错误应该包含足够的上下文信息用于调试
// 3. 错误码用于 API 响应和日志分类
// 4. 支持错误链（error wrapping）
package errors

import (
	"errors"
	"fmt"
)

// ErrorCode 错误码类型
type ErrorCode string

// 错误码定义
const (
	// 认证相关 (1xxx)
	CodeAuthFailed       ErrorCode = "AUTH_FAILED"
	CodeInvalidToken     ErrorCode = "INVALID_TOKEN"
	CodeTokenExpired     ErrorCode = "TOKEN_EXPIRED"
	CodeTokenRevoked     ErrorCode = "TOKEN_REVOKED"
	CodeInvalidAuthCode  ErrorCode = "INVALID_AUTH_CODE"
	CodeInvalidSecretKey ErrorCode = "INVALID_SECRET_KEY"

	// 资源不存在 (2xxx)
	CodeNotFound        ErrorCode = "NOT_FOUND"
	CodeClientNotFound  ErrorCode = "CLIENT_NOT_FOUND"
	CodeUserNotFound    ErrorCode = "USER_NOT_FOUND"
	CodeNodeNotFound    ErrorCode = "NODE_NOT_FOUND"
	CodeMappingNotFound ErrorCode = "MAPPING_NOT_FOUND"

	// 资源冲突 (3xxx)
	CodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	CodeConflict      ErrorCode = "CONFLICT"

	// 请求错误 (4xxx)
	CodeInvalidRequest  ErrorCode = "INVALID_REQUEST"
	CodeInvalidParam    ErrorCode = "INVALID_PARAM"
	CodeMissingParam    ErrorCode = "MISSING_PARAM"
	CodeValidationError ErrorCode = "VALIDATION_ERROR"

	// 权限错误 (5xxx)
	CodeForbidden     ErrorCode = "FORBIDDEN"
	CodeClientBlocked ErrorCode = "CLIENT_BLOCKED"
	CodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED"
	CodeRateLimited   ErrorCode = "RATE_LIMITED"

	// 系统错误 (6xxx)
	CodeInternal      ErrorCode = "INTERNAL_ERROR"
	CodeStorageError  ErrorCode = "STORAGE_ERROR"
	CodeNetworkError  ErrorCode = "NETWORK_ERROR"
	CodeTimeout       ErrorCode = "TIMEOUT"
	CodeUnavailable   ErrorCode = "UNAVAILABLE"
	CodeNotConfigured ErrorCode = "NOT_CONFIGURED"

	// 流/连接错误 (7xxx)
	CodeStreamClosed    ErrorCode = "STREAM_CLOSED"
	CodeConnectionError ErrorCode = "CONNECTION_ERROR"
	CodeHandshakeFailed ErrorCode = "HANDSHAKE_FAILED"
	CodeTunnelError     ErrorCode = "TUNNEL_ERROR"

	// 数据包错误 (8xxx)
	CodeInvalidPacket    ErrorCode = "INVALID_PACKET"
	CodePacketTooLarge   ErrorCode = "PACKET_TOO_LARGE"
	CodeCompressionError ErrorCode = "COMPRESSION_ERROR"
	CodeEncryptionError  ErrorCode = "ENCRYPTION_ERROR"
)

// Error 统一错误类型
type Error struct {
	Code    ErrorCode              // 错误码
	Message string                 // 错误消息
	Cause   error                  // 原始错误
	Details map[string]interface{} // 额外详情
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is 支持 errors.Is 进行错误码比较
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.Code == t.Code
	}
	return false
}

// WithDetail 添加详情
func (e *Error) WithDetail(key string, value interface{}) *Error {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// New 创建新错误
func New(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Newf 创建格式化错误
func Newf(code ErrorCode, format string, args ...interface{}) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// Wrap 包装错误
func Wrap(err error, code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// Wrapf 格式化包装错误
func Wrapf(err error, code ErrorCode, format string, args ...interface{}) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   err,
	}
}

// GetCode 从错误中提取错误码
func GetCode(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}

// IsCode 检查错误是否为指定错误码
func IsCode(err error, code ErrorCode) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

// Is 重导出 errors.Is
var Is = errors.Is

// As 重导出 errors.As
var As = errors.As

// ============================================================================
// 特定错误类型构造函数（从 internal/errors 迁移）
// ============================================================================

// NewPacketError 创建数据包错误
func NewPacketError(packetType, message string, cause error) *Error {
	return &Error{
		Code:    CodeInvalidPacket,
		Message: fmt.Sprintf("[%s] %s", packetType, message),
		Cause:   cause,
	}
}

// NewStreamError 创建流错误
func NewStreamError(operation, message string, cause error) *Error {
	return &Error{
		Code:    CodeStreamClosed,
		Message: fmt.Sprintf("[%s] %s", operation, message),
		Cause:   cause,
	}
}

// NewRateLimitError 创建限速错误
func NewRateLimitError(rate int64, message string, cause error) *Error {
	return &Error{
		Code:    CodeRateLimited,
		Message: fmt.Sprintf("[%d bytes/s] %s", rate, message),
		Cause:   cause,
	}
}

// NewCompressionError 创建压缩错误
func NewCompressionError(operation, message string, cause error) *Error {
	return &Error{
		Code:    CodeCompressionError,
		Message: fmt.Sprintf("[%s] %s", operation, message),
		Cause:   cause,
	}
}

// NewEncryptionError 创建加密错误
func NewEncryptionError(operation, message string, cause error) *Error {
	return &Error{
		Code:    CodeEncryptionError,
		Message: fmt.Sprintf("[%s] %s", operation, message),
		Cause:   cause,
	}
}

// WrapError 包装错误（简单版本，兼容旧代码）
func WrapError(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}
