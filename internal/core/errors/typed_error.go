package errors

import (
	"fmt"
)

// ErrorType 错误类型
type ErrorType string

const (
	ErrorTypeTemporary ErrorType = "temporary" // 可重试
	ErrorTypePermanent ErrorType = "permanent" // 永久错误
	ErrorTypeProtocol  ErrorType = "protocol"  // 协议错误
	ErrorTypeNetwork   ErrorType = "network"   // 网络错误
	ErrorTypeStorage   ErrorType = "storage"    // 存储错误
	ErrorTypeAuth       ErrorType = "auth"       // 认证错误
	ErrorTypeFatal      ErrorType = "fatal"      // 致命错误
)

// TypedError 带类型的错误
type TypedError struct {
	Type      ErrorType
	Message   string
	Err       error
	Retryable bool
	Alertable bool
}

// Error 实现 error 接口
func (e *TypedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap 返回原始错误
func (e *TypedError) Unwrap() error {
	return e.Err
}

// isRetryable 判断错误类型是否可重试
func isRetryable(errType ErrorType) bool {
	switch errType {
	case ErrorTypeTemporary, ErrorTypeNetwork, ErrorTypeStorage:
		return true
	default:
		return false
	}
}

// isAlertable 判断错误类型是否需要告警
func isAlertable(errType ErrorType) bool {
	switch errType {
	case ErrorTypeProtocol, ErrorTypeStorage, ErrorTypeAuth, ErrorTypeFatal:
		return true
	default:
		return false
	}
}

// Wrap 包装错误
func Wrap(err error, errType ErrorType, message string) error {
	if err == nil {
		return nil
	}
	return &TypedError{
		Type:      errType,
		Message:   message,
		Err:       err,
		Retryable: isRetryable(errType),
		Alertable: isAlertable(errType),
	}
}

// Wrapf 格式化包装错误
func Wrapf(err error, errType ErrorType, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return Wrap(err, errType, fmt.Sprintf(format, args...))
}

// New 创建新的 TypedError
func New(errType ErrorType, message string) *TypedError {
	return &TypedError{
		Type:      errType,
		Message:   message,
		Retryable: isRetryable(errType),
		Alertable: isAlertable(errType),
	}
}

// Newf 格式化创建新的 TypedError
func Newf(errType ErrorType, format string, args ...interface{}) *TypedError {
	return New(errType, fmt.Sprintf(format, args...))
}

// IsRetryable 判断是否可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if typedErr, ok := err.(*TypedError); ok {
		return typedErr.Retryable
	}
	// 递归检查包装的错误
	if unwrapped := unwrapError(err); unwrapped != nil {
		return IsRetryable(unwrapped)
	}
	return false
}

// IsAlertable 判断是否需要告警
func IsAlertable(err error) bool {
	if err == nil {
		return false
	}
	if typedErr, ok := err.(*TypedError); ok {
		return typedErr.Alertable
	}
	// 递归检查包装的错误
	if unwrapped := unwrapError(err); unwrapped != nil {
		return IsAlertable(unwrapped)
	}
	return false
}

// GetErrorType 获取错误类型
func GetErrorType(err error) ErrorType {
	if err == nil {
		return ErrorTypePermanent
	}
	if typedErr, ok := err.(*TypedError); ok {
		return typedErr.Type
	}
	// 递归检查包装的错误
	if unwrapped := unwrapError(err); unwrapped != nil {
		return GetErrorType(unwrapped)
	}
	return ErrorTypePermanent
}

// unwrapError 解包错误（支持标准库 errors.Unwrap）
func unwrapError(err error) error {
	type unwrapper interface {
		Unwrap() error
	}
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

// Sentinel errors - 预定义的错误实例
var (
	// ErrTemporary 临时错误（可重试）
	ErrTemporary = New(ErrorTypeTemporary, "temporary error")
	// ErrPermanent 永久错误（不可重试）
	ErrPermanent = New(ErrorTypePermanent, "permanent error")
	// ErrProtocol 协议错误（需告警）
	ErrProtocol = New(ErrorTypeProtocol, "protocol error")
	// ErrNetwork 网络错误（可重试）
	ErrNetwork = New(ErrorTypeNetwork, "network error")
	// ErrStorage 存储错误（可重试，需告警）
	ErrStorage = New(ErrorTypeStorage, "storage error")
	// ErrAuth 认证错误（不可重试，需告警）
	ErrAuth = New(ErrorTypeAuth, "authentication error")
	// ErrFatal 致命错误（不可重试，需告警）
	ErrFatal = New(ErrorTypeFatal, "fatal error")
)

