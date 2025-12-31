package constants

import (
	coreerrors "tunnox-core/internal/core/errors"
)

// ============================================================================
// 云控相关错误（使用 core/errors 包的哨兵错误）
// ============================================================================

// 认证相关错误 - 直接使用 core/errors 包的哨兵错误
var (
	ErrAuthenticationFailed = coreerrors.ErrAuthFailed
	ErrInvalidToken         = coreerrors.ErrInvalidToken
	ErrTokenExpired         = coreerrors.ErrTokenExpired
	ErrTokenRevoked         = coreerrors.ErrTokenRevoked
	ErrInvalidAuthCode      = coreerrors.ErrInvalidAuthCode
	ErrInvalidSecretKey     = coreerrors.ErrInvalidSecretKey
)

// 实体不存在错误
var (
	ErrClientNotFound      = coreerrors.ErrClientNotFound
	ErrNodeNotFound        = coreerrors.ErrNodeNotFound
	ErrUserNotFound        = coreerrors.ErrUserNotFound
	ErrPortMappingNotFound = coreerrors.ErrMappingNotFound
	ErrConnectionNotFound  = coreerrors.ErrNotFound
	ErrEntityDoesNotExist  = coreerrors.ErrNotFound
)

// 实体已存在错误
var (
	ErrEntityAlreadyExists = coreerrors.ErrAlreadyExists
)

// 业务逻辑错误
var (
	ErrClientBlocked      = coreerrors.ErrClientBlocked
	ErrNetworkUnavailable = coreerrors.ErrUnavailable
	ErrInvalidConfig      = coreerrors.ErrInvalidRequest
)

// 系统错误
var (
	ErrInternalError         = coreerrors.ErrInternal
	ErrStorageError          = coreerrors.ErrStorageError
	ErrLockAcquisitionFailed = coreerrors.ErrInternal
	ErrCleanupTaskFailed     = coreerrors.ErrInternal
	ErrConfigUpdateFailed    = coreerrors.ErrInternal
)

// ============================================================================
// 错误检查函数（委托给 core/errors 包）
// ============================================================================

// IsNotFoundError 检查是否为资源不存在错误
func IsNotFoundError(err error) bool {
	return coreerrors.IsNotFound(err)
}

// IsAuthenticationError 检查是否为认证错误
func IsAuthenticationError(err error) bool {
	return coreerrors.IsAuthError(err)
}

// IsSystemError 检查是否为系统错误
func IsSystemError(err error) bool {
	return coreerrors.IsSystemError(err)
}

// ============================================================================
// 创建带上下文的错误（使用 core/errors 包）
// ============================================================================

// NewNotFoundError 创建资源不存在错误
func NewNotFoundError(entityType, entityID string) error {
	return coreerrors.Newf(coreerrors.CodeNotFound, "%s with ID %s not found", entityType, entityID)
}

// NewAuthenticationError 创建认证错误
func NewAuthenticationError(reason string) error {
	return coreerrors.Newf(coreerrors.CodeAuthFailed, "authentication failed: %s", reason)
}

// NewStorageError 创建存储错误
func NewStorageError(operation string) error {
	return coreerrors.Newf(coreerrors.CodeStorageError, "storage operation '%s' failed", operation)
}

// NewLockError 创建锁获取错误
func NewLockError(operation string) error {
	return coreerrors.Newf(coreerrors.CodeInternal, "lock acquisition for '%s' failed", operation)
}

// NewValidationError 创建验证错误
func NewValidationError(field, reason string) error {
	return coreerrors.Newf(coreerrors.CodeValidationError, "validation failed for %s: %s", field, reason)
}

// WrapError 包装错误（兼容旧代码）
func WrapError(err error, context string) error {
	if err == nil {
		return nil
	}
	return coreerrors.Wrap(err, coreerrors.CodeInternal, context)
}
