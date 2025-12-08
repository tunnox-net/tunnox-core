package errors

import (
	"errors"
	"fmt"
)

// ErrorType 错误类型
type ErrorType string

const (
	ErrorTypeTemporary        ErrorType = "temporary"         // 可重试
	ErrorTypePermanent        ErrorType = "permanent"         // 永久错误
	ErrorTypeProtocol         ErrorType = "protocol"          // 协议错误
	ErrorTypeNetwork          ErrorType = "network"           // 网络错误
	ErrorTypeStorage          ErrorType = "storage"           // 存储错误
	ErrorTypeAuth             ErrorType = "auth"               // 认证错误
	ErrorTypeFatal            ErrorType = "fatal"             // 致命错误
	ErrorTypeStreamModeSwitch ErrorType = "stream_mode_switch" // 流模式切换（特殊控制流错误）
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

// Is 实现 errors.Is 接口，支持 Sentinel Errors 检查
// 如果 target 是 *TypedError 且 Type 和 Message 都匹配，则返回 true
// 这样可以支持 errors.Is(err, ErrStreamModeSwitch) 这样的检查
func (e *TypedError) Is(target error) bool {
	if target == nil {
		return false
	}
	
	// 如果 target 也是 TypedError，比较 Type 和 Message
	if t, ok := target.(*TypedError); ok {
		// 如果 target 的 Err 为 nil，只比较 Type 和 Message
		if t.Err == nil {
			return e.Type == t.Type && e.Message == t.Message
		}
		// 如果 target 的 Err 不为 nil，需要递归比较
		if e.Err != nil {
			return e.Type == t.Type && e.Message == t.Message && errors.Is(e.Err, t.Err)
		}
	}
	
	// 递归检查包装的错误
	if e.Err != nil {
		return errors.Is(e.Err, target)
	}
	
	return false
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
	
	// 流模式切换 Sentinel Errors
	// 用于标识隧道连接切换到流模式，这是正常的控制流，不是真正的错误
	ErrStreamModeSwitch              = New(ErrorTypeStreamModeSwitch, "stream mode switch")
	ErrStreamModeSwitchSource        = New(ErrorTypeStreamModeSwitch, "stream mode switch: source")
	ErrStreamModeSwitchTarget        = New(ErrorTypeStreamModeSwitch, "stream mode switch: target")
	ErrStreamModeSwitchCrossServer   = New(ErrorTypeStreamModeSwitch, "stream mode switch: cross-server")
	ErrStreamModeSwitchExistingBridge = New(ErrorTypeStreamModeSwitch, "stream mode switch: existing bridge")
	
	// 连接码服务 Sentinel Errors
	ErrConnectionCodeQuotaExceeded = New(ErrorTypePermanent, "quota exceeded")
	ErrConnectionCodeNotFound      = New(ErrorTypePermanent, "connection code not found or expired")
	ErrConnectionCodeExpired       = New(ErrorTypePermanent, "connection code has expired")
	ErrConnectionCodeAlreadyUsed   = New(ErrorTypePermanent, "connection code has already been used")
	ErrConnectionCodeRevoked       = New(ErrorTypePermanent, "connection code has been revoked")
	ErrMappingNotFound             = New(ErrorTypePermanent, "mapping not found or expired")
	ErrMappingExpired              = New(ErrorTypePermanent, "mapping has expired")
	ErrMappingRevoked              = New(ErrorTypePermanent, "mapping has been revoked")
	ErrMappingNotAuthorized        = New(ErrorTypeAuth, "client is not authorized to use this mapping")
	
	// 连接管理 Sentinel Errors
	ErrConnectionAlreadyExists = New(ErrorTypePermanent, "connection already exists")
)

