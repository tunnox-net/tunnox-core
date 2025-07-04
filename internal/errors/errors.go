package errors

import (
	"errors"
	"fmt"
)

// 预定义错误
var (
	ErrStreamClosed           = errors.New("stream is closed")
	ErrReaderNil              = errors.New("reader is nil")
	ErrWriterNil              = errors.New("writer is nil")
	ErrUnexpectedEOF          = errors.New("unexpected end of file")
	ErrInvalidPacketType      = errors.New("invalid packet type")
	ErrInvalidBodySize        = errors.New("invalid body size")
	ErrCompressionFailed      = errors.New("compression failed")
	ErrDecompressionFailed    = errors.New("decompression failed")
	ErrRateLimitExceeded      = errors.New("rate limit exceeded")
	ErrContextCancelled       = errors.New("context cancelled")
	ErrInvalidRate            = errors.New("invalid rate limit")
	ErrResourceNotInitialized = errors.New("resource not initialized")
)

// PacketError 数据包相关错误
type PacketError struct {
	Type    string
	Message string
	Cause   error
}

func (e *PacketError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("packet error [%s]: %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("packet error [%s]: %s", e.Type, e.Message)
}

func (e *PacketError) Unwrap() error {
	return e.Cause
}

// NewPacketError 创建数据包错误
func NewPacketError(packetType, message string, cause error) *PacketError {
	return &PacketError{
		Type:    packetType,
		Message: message,
		Cause:   cause,
	}
}

// StreamError 流相关错误
type StreamError struct {
	Operation string
	Message   string
	Cause     error
}

func (e *StreamError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("stream error [%s]: %s: %v", e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("stream error [%s]: %s", e.Operation, e.Message)
}

func (e *StreamError) Unwrap() error {
	return e.Cause
}

// NewStreamError 创建流错误
func NewStreamError(operation, message string, cause error) *StreamError {
	return &StreamError{
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}

// RateLimitError 限速相关错误
type RateLimitError struct {
	Rate    int64
	Message string
	Cause   error
}

func (e *RateLimitError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("rate limit error [%d bytes/s]: %s: %v", e.Rate, e.Message, e.Cause)
	}
	return fmt.Sprintf("rate limit error [%d bytes/s]: %s", e.Rate, e.Message)
}

func (e *RateLimitError) Unwrap() error {
	return e.Cause
}

// NewRateLimitError 创建限速错误
func NewRateLimitError(rate int64, message string, cause error) *RateLimitError {
	return &RateLimitError{
		Rate:    rate,
		Message: message,
		Cause:   cause,
	}
}

// CompressionError 压缩相关错误
type CompressionError struct {
	Operation string
	Message   string
	Cause     error
}

func (e *CompressionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("compression error [%s]: %s: %v", e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("compression error [%s]: %s", e.Operation, e.Message)
}

func (e *CompressionError) Unwrap() error {
	return e.Cause
}

// NewCompressionError 创建压缩错误
func NewCompressionError(operation, message string, cause error) *CompressionError {
	return &CompressionError{
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}

// WrapError 包装错误，添加上下文信息
func WrapError(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// WrapErrorf 格式化包装错误
func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}

// IsTemporaryError 判断是否为临时错误（可重试）
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}
	// 可以根据具体错误类型判断是否为临时错误
	// 这里简单实现，实际项目中可能需要更复杂的判断逻辑
	return false
}

// IsFatalError 判断是否为致命错误（不可重试）
func IsFatalError(err error) bool {
	if err == nil {
		return false
	}
	// 可以根据具体错误类型判断是否为致命错误
	// 这里简单实现，实际项目中可能需要更复杂的判断逻辑
	return true
}
