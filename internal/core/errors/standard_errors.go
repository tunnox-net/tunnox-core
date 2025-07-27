package errors

import (
	"fmt"
	"time"
)

// ErrorCode 错误码类型
type ErrorCode int

const (
	// 通用错误码 (1000-1999)
	ErrCodeSuccess          ErrorCode = 1000
	ErrCodeUnknown          ErrorCode = 1001
	ErrCodeInvalidParameter ErrorCode = 1002
	ErrCodeNotFound         ErrorCode = 1003
	ErrCodeAlreadyExists    ErrorCode = 1004
	ErrCodeTimeout          ErrorCode = 1005
	ErrCodeUnauthorized     ErrorCode = 1006
	ErrCodeForbidden        ErrorCode = 1007
	ErrCodeInternal         ErrorCode = 1008

	// 网络错误码 (2000-2999)
	ErrCodeNetworkError     ErrorCode = 2000
	ErrCodeConnectionFailed ErrorCode = 2001
	ErrCodeConnectionClosed ErrorCode = 2002
	ErrCodeProtocolError    ErrorCode = 2003

	// 存储错误码 (3000-3999)
	ErrCodeStorageError     ErrorCode = 3000
	ErrCodeStorageFull      ErrorCode = 3001
	ErrCodeStorageCorrupted ErrorCode = 3002
	ErrCodeStorageTimeout   ErrorCode = 3003

	// 业务错误码 (4000-4999)
	ErrCodeBusinessError     ErrorCode = 4000
	ErrCodeResourceExhausted ErrorCode = 4001
	ErrCodeRateLimit         ErrorCode = 4002
	ErrCodeQuotaExceeded     ErrorCode = 4003
)

// StandardError 标准错误类型
type StandardError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Cause     error     `json:"-"`
}

// NewStandardError 创建新的标准错误
func NewStandardError(code ErrorCode, message string) *StandardError {
	return &StandardError{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// NewStandardErrorWithDetails 创建带详细信息的标准错误
func NewStandardErrorWithDetails(code ErrorCode, message, details string) *StandardError {
	return &StandardError{
		Code:      code,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}
}

// NewStandardErrorWithCause 创建带原因的标准错误
func NewStandardErrorWithCause(code ErrorCode, message string, cause error) *StandardError {
	return &StandardError{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// Error 实现error接口
func (e *StandardError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 返回原因错误
func (e *StandardError) Unwrap() error {
	return e.Cause
}

// Is 检查错误类型
func (e *StandardError) Is(target error) bool {
	if target == nil {
		return false
	}

	if t, ok := target.(*StandardError); ok {
		return e.Code == t.Code
	}

	return false
}

// GetCode 获取错误码
func (e *StandardError) GetCode() ErrorCode {
	return e.Code
}

// GetMessage 获取错误消息
func (e *StandardError) GetMessage() string {
	return e.Message
}

// GetDetails 获取详细信息
func (e *StandardError) GetDetails() string {
	return e.Details
}

// GetTimestamp 获取时间戳
func (e *StandardError) GetTimestamp() time.Time {
	return e.Timestamp
}

// 预定义错误
var (
	ErrSuccess          = NewStandardError(ErrCodeSuccess, "success")
	ErrUnknown          = NewStandardError(ErrCodeUnknown, "unknown error")
	ErrInvalidParameter = NewStandardError(ErrCodeInvalidParameter, "invalid parameter")
	ErrNotFound         = NewStandardError(ErrCodeNotFound, "resource not found")
	ErrAlreadyExists    = NewStandardError(ErrCodeAlreadyExists, "resource already exists")
	ErrTimeout          = NewStandardError(ErrCodeTimeout, "operation timeout")
	ErrUnauthorized     = NewStandardError(ErrCodeUnauthorized, "unauthorized")
	ErrForbidden        = NewStandardError(ErrCodeForbidden, "forbidden")
	ErrInternal         = NewStandardError(ErrCodeInternal, "internal error")

	ErrNetworkError     = NewStandardError(ErrCodeNetworkError, "network error")
	ErrConnectionFailed = NewStandardError(ErrCodeConnectionFailed, "connection failed")
	ErrConnectionClosed = NewStandardError(ErrCodeConnectionClosed, "connection closed")
	ErrProtocolError    = NewStandardError(ErrCodeProtocolError, "protocol error")

	ErrStorageError     = NewStandardError(ErrCodeStorageError, "storage error")
	ErrStorageFull      = NewStandardError(ErrCodeStorageFull, "storage full")
	ErrStorageCorrupted = NewStandardError(ErrCodeStorageCorrupted, "storage corrupted")
	ErrStorageTimeout   = NewStandardError(ErrCodeStorageTimeout, "storage timeout")

	ErrBusinessError     = NewStandardError(ErrCodeBusinessError, "business error")
	ErrResourceExhausted = NewStandardError(ErrCodeResourceExhausted, "resource exhausted")
	ErrRateLimit         = NewStandardError(ErrCodeRateLimit, "rate limit exceeded")
	ErrQuotaExceeded     = NewStandardError(ErrCodeQuotaExceeded, "quota exceeded")
)

// ErrorHelper 错误辅助函数
type ErrorHelper struct{}

// NewErrorHelper 创建错误辅助函数实例
func NewErrorHelper() *ErrorHelper {
	return &ErrorHelper{}
}

// IsStandardError 检查是否为标准错误
func (h *ErrorHelper) IsStandardError(err error) bool {
	_, ok := err.(*StandardError)
	return ok
}

// GetErrorCode 获取错误码
func (h *ErrorHelper) GetErrorCode(err error) ErrorCode {
	if se, ok := err.(*StandardError); ok {
		return se.GetCode()
	}
	return ErrCodeUnknown
}

// IsTimeoutError 检查是否为超时错误
func (h *ErrorHelper) IsTimeoutError(err error) bool {
	return h.GetErrorCode(err) == ErrCodeTimeout
}

// IsNetworkError 检查是否为网络错误
func (h *ErrorHelper) IsNetworkError(err error) bool {
	code := h.GetErrorCode(err)
	return code >= ErrCodeNetworkError && code < ErrCodeStorageError
}

// IsStorageError 检查是否为存储错误
func (h *ErrorHelper) IsStorageError(err error) bool {
	code := h.GetErrorCode(err)
	return code >= ErrCodeStorageError && code < ErrCodeBusinessError
}

// IsBusinessError 检查是否为业务错误
func (h *ErrorHelper) IsBusinessError(err error) bool {
	code := h.GetErrorCode(err)
	return code >= ErrCodeBusinessError
}

// WrapError 包装错误
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}

	if se, ok := err.(*StandardError); ok {
		return NewStandardErrorWithCause(se.GetCode(), message, se)
	}

	return NewStandardErrorWithCause(ErrCodeUnknown, message, err)
}

// WrapErrorWithCode 使用指定错误码包装错误
func WrapErrorWithCode(err error, code ErrorCode, message string) error {
	if err == nil {
		return nil
	}

	return NewStandardErrorWithCause(code, message, err)
}

// ProtocolTimeoutError 协议超时错误类型
// 用于协议适配器的超时错误，包含具体的协议信息
type ProtocolTimeoutError struct {
	Protocol string
}

func (e *ProtocolTimeoutError) Error() string {
	return fmt.Sprintf("timeout waiting for %s", e.Protocol)
}

// IsProtocolTimeoutError 检查是否为协议超时错误
func IsProtocolTimeoutError(err error) bool {
	_, ok := err.(*ProtocolTimeoutError)
	return ok
}

// NewProtocolTimeoutError 创建协议超时错误
func NewProtocolTimeoutError(protocol string) *ProtocolTimeoutError {
	return &ProtocolTimeoutError{Protocol: protocol}
}

// ErrorWithCleanup 带清理操作的错误处理
// 用于在操作失败时自动执行清理操作
type ErrorWithCleanup struct {
	Err     error
	Cleanup func() error
}

// NewErrorWithCleanup 创建带清理操作的错误
func NewErrorWithCleanup(err error, cleanup func() error) *ErrorWithCleanup {
	return &ErrorWithCleanup{
		Err:     err,
		Cleanup: cleanup,
	}
}

// Error 实现error接口
func (e *ErrorWithCleanup) Error() string {
	return e.Err.Error()
}

// Unwrap 返回原始错误
func (e *ErrorWithCleanup) Unwrap() error {
	return e.Err
}

// ExecuteCleanup 执行清理操作
func (e *ErrorWithCleanup) ExecuteCleanup() error {
	if e.Cleanup != nil {
		return e.Cleanup()
	}
	return nil
}

// HandleErrorWithCleanup 处理带清理操作的错误
// 这是一个通用的错误处理模式，用于在操作失败时自动释放已分配的资源
func HandleErrorWithCleanup(err error, cleanup func() error, message string) error {
	if err == nil {
		return nil
	}

	// 执行清理操作
	if cleanup != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			// 如果清理也失败了，记录但不影响主错误
			// 这里可以添加日志记录
		}
	}

	// 返回包装后的错误
	return fmt.Errorf("%s: %w", message, err)
}

// HandleErrorWithCleanupFunc 返回一个带清理操作的错误处理函数
// 用于简化重复的错误处理代码
func HandleErrorWithCleanupFunc(cleanup func() error, message string) func(error) error {
	return func(err error) error {
		return HandleErrorWithCleanup(err, cleanup, message)
	}
}
