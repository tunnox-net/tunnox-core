package cloud

import (
	"errors"
	"fmt"
	coreerrors "tunnox-core/internal/errors"
)

// 云控相关错误
var (
	// 认证相关错误
	ErrAuthenticationFailed = errors.New(ErrMsgAuthenticationFailed)
	ErrInvalidToken         = errors.New(ErrMsgTokenInvalid)
	ErrTokenExpired         = errors.New(ErrMsgTokenExpired)
	ErrTokenRevoked         = errors.New(ErrMsgTokenRevoked)
	ErrInvalidAuthCode      = errors.New(ErrMsgInvalidAuthCode)
	ErrInvalidSecretKey     = errors.New(ErrMsgInvalidSecretKey)

	// 实体不存在错误
	ErrClientNotFound      = errors.New(ErrMsgClientNotFound)
	ErrNodeNotFound        = errors.New(ErrMsgNodeNotFound)
	ErrUserNotFound        = errors.New(ErrMsgUserNotFound)
	ErrPortMappingNotFound = errors.New(ErrMsgMappingNotFound)
	ErrConnectionNotFound  = errors.New(ErrMsgConnectionNotFound)
	ErrEntityDoesNotExist  = errors.New(ErrMsgEntityDoesNotExist)

	// 实体已存在错误
	ErrEntityAlreadyExists = errors.New(ErrMsgEntityAlreadyExists)

	// 业务逻辑错误
	ErrClientBlocked      = errors.New(ErrMsgClientBlocked)
	ErrNetworkUnavailable = errors.New("network unavailable")
	ErrInvalidConfig      = errors.New(ErrMsgInvalidRequest)

	// 系统错误
	ErrInternalError         = errors.New(ErrMsgInternalError)
	ErrStorageError          = errors.New(ErrMsgStorageError)
	ErrLockAcquisitionFailed = errors.New(ErrMsgLockAcquisitionFailed)
	ErrCleanupTaskFailed     = errors.New(ErrMsgCleanupTaskFailed)
	ErrConfigUpdateFailed    = errors.New(ErrMsgConfigUpdateFailed)
)

// 错误检查函数，用于检查特定错误类型
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrClientNotFound) ||
		errors.Is(err, ErrNodeNotFound) ||
		errors.Is(err, ErrUserNotFound) ||
		errors.Is(err, ErrPortMappingNotFound) ||
		errors.Is(err, ErrConnectionNotFound) ||
		errors.Is(err, ErrEntityDoesNotExist)
}

func IsAuthenticationError(err error) bool {
	return errors.Is(err, ErrAuthenticationFailed) ||
		errors.Is(err, ErrInvalidToken) ||
		errors.Is(err, ErrTokenExpired) ||
		errors.Is(err, ErrTokenRevoked) ||
		errors.Is(err, ErrInvalidAuthCode) ||
		errors.Is(err, ErrInvalidSecretKey)
}

func IsSystemError(err error) bool {
	return errors.Is(err, ErrInternalError) ||
		errors.Is(err, ErrStorageError) ||
		errors.Is(err, ErrLockAcquisitionFailed) ||
		errors.Is(err, ErrCleanupTaskFailed) ||
		errors.Is(err, ErrConfigUpdateFailed)
}

// 创建带上下文的错误
func NewNotFoundError(entityType, entityID string) error {
	return coreerrors.WrapError(ErrEntityDoesNotExist, fmt.Sprintf("%s with ID %s", entityType, entityID))
}

func NewAuthenticationError(reason string) error {
	return coreerrors.WrapError(ErrAuthenticationFailed, reason)
}

func NewStorageError(operation string) error {
	return coreerrors.WrapError(ErrStorageError, fmt.Sprintf("storage operation '%s' failed", operation))
}

func NewLockError(operation string) error {
	return coreerrors.WrapError(ErrLockAcquisitionFailed, fmt.Sprintf("lock acquisition for '%s' failed", operation))
}
