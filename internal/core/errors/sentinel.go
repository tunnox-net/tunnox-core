package errors

// 预定义哨兵错误（用于 errors.Is 比较）
// 这些错误用于快速类型检查，不包含详细信息
var (
	// 认证相关
	ErrAuthFailed       = New(CodeAuthFailed, "authentication failed")
	ErrInvalidToken     = New(CodeInvalidToken, "invalid token")
	ErrTokenExpired     = New(CodeTokenExpired, "token expired")
	ErrTokenRevoked     = New(CodeTokenRevoked, "token revoked")
	ErrInvalidAuthCode  = New(CodeInvalidAuthCode, "invalid auth code")
	ErrInvalidSecretKey = New(CodeInvalidSecretKey, "invalid secret key")

	// 资源不存在
	ErrNotFound        = New(CodeNotFound, "resource not found")
	ErrClientNotFound  = New(CodeClientNotFound, "client not found")
	ErrUserNotFound    = New(CodeUserNotFound, "user not found")
	ErrNodeNotFound    = New(CodeNodeNotFound, "node not found")
	ErrMappingNotFound = New(CodeMappingNotFound, "mapping not found")

	// 资源冲突
	ErrAlreadyExists = New(CodeAlreadyExists, "resource already exists")
	ErrConflict      = New(CodeConflict, "resource conflict")

	// 请求错误
	ErrInvalidRequest  = New(CodeInvalidRequest, "invalid request")
	ErrInvalidParam    = New(CodeInvalidParam, "invalid parameter")
	ErrMissingParam    = New(CodeMissingParam, "missing required parameter")
	ErrValidationError = New(CodeValidationError, "validation error")

	// 权限错误
	ErrForbidden     = New(CodeForbidden, "access forbidden")
	ErrClientBlocked = New(CodeClientBlocked, "client is blocked")
	ErrQuotaExceeded = New(CodeQuotaExceeded, "quota exceeded")
	ErrRateLimited   = New(CodeRateLimited, "rate limit exceeded")

	// 系统错误
	ErrInternal      = New(CodeInternal, "internal error")
	ErrStorageError  = New(CodeStorageError, "storage error")
	ErrNetworkError  = New(CodeNetworkError, "network error")
	ErrTimeout       = New(CodeTimeout, "operation timeout")
	ErrUnavailable   = New(CodeUnavailable, "service unavailable")
	ErrNotConfigured = New(CodeNotConfigured, "not configured")

	// 流/连接错误
	ErrStreamClosed    = New(CodeStreamClosed, "stream closed")
	ErrConnectionError = New(CodeConnectionError, "connection error")
	ErrHandshakeFailed = New(CodeHandshakeFailed, "handshake failed")
	ErrTunnelError     = New(CodeTunnelError, "tunnel error")

	// 数据包错误
	ErrInvalidPacket    = New(CodeInvalidPacket, "invalid packet")
	ErrPacketTooLarge   = New(CodePacketTooLarge, "packet too large")
	ErrCompressionError = New(CodeCompressionError, "compression error")
	ErrEncryptionError  = New(CodeEncryptionError, "encryption error")

	// ============================================================================
	// 流处理相关错误（从 internal/errors 迁移）
	// ============================================================================
	ErrReaderNil              = New(CodeStreamClosed, "reader is nil")
	ErrWriterNil              = New(CodeStreamClosed, "writer is nil")
	ErrUnexpectedEOF          = New(CodeStreamClosed, "unexpected end of file")
	ErrInvalidPacketType      = New(CodeInvalidPacket, "invalid packet type")
	ErrInvalidBodySize        = New(CodeInvalidPacket, "invalid body size")
	ErrContextCancelled       = New(CodeTimeout, "context cancelled")
	ErrInvalidRate            = New(CodeInvalidParam, "invalid rate limit")
	ErrResourceNotInitialized = New(CodeNotConfigured, "resource not initialized")
)

// 错误检查辅助函数

// IsNotFound 检查是否为资源不存在错误
func IsNotFound(err error) bool {
	return IsCode(err, CodeNotFound) ||
		IsCode(err, CodeClientNotFound) ||
		IsCode(err, CodeUserNotFound) ||
		IsCode(err, CodeNodeNotFound) ||
		IsCode(err, CodeMappingNotFound)
}

// IsAuthError 检查是否为认证错误
func IsAuthError(err error) bool {
	return IsCode(err, CodeAuthFailed) ||
		IsCode(err, CodeInvalidToken) ||
		IsCode(err, CodeTokenExpired) ||
		IsCode(err, CodeTokenRevoked) ||
		IsCode(err, CodeInvalidAuthCode) ||
		IsCode(err, CodeInvalidSecretKey)
}

// IsPermissionError 检查是否为权限错误
func IsPermissionError(err error) bool {
	return IsCode(err, CodeForbidden) ||
		IsCode(err, CodeClientBlocked) ||
		IsCode(err, CodeQuotaExceeded) ||
		IsCode(err, CodeRateLimited)
}

// IsSystemError 检查是否为系统错误
func IsSystemError(err error) bool {
	return IsCode(err, CodeInternal) ||
		IsCode(err, CodeStorageError) ||
		IsCode(err, CodeNetworkError) ||
		IsCode(err, CodeTimeout) ||
		IsCode(err, CodeUnavailable)
}

// IsRetryable 检查错误是否可重试
func IsRetryable(err error) bool {
	code := GetCode(err)
	switch code {
	case CodeTimeout, CodeUnavailable, CodeNetworkError, CodeRateLimited:
		return true
	default:
		return false
	}
}
