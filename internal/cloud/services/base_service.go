package services

import (
corelog "tunnox-core/internal/core/log"
	"fmt"
	"time"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"
)

// BaseService 基础服务结构，提供通用的错误处理工具
type BaseService struct{}

// NewBaseService 创建基础服务实例
func NewBaseService() *BaseService {
	return &BaseService{}
}

// HandleErrorWithIDRelease 处理需要释放ID的错误
// 这是一个通用的错误处理模式，用于在操作失败时自动释放已分配的ID
func (s *BaseService) HandleErrorWithIDRelease(err error, id interface{}, releaseFunc func(interface{}) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID
	if releaseFunc != nil {
		_ = releaseFunc(id)
	}

	// 根据原始错误类型选择合适的错误类型
	errType := errors.GetErrorType(err)
	if errType == errors.ErrorTypePermanent {
		errType = errors.ErrorTypeStorage // 默认使用 Storage 类型，因为通常是资源操作失败
	}
	return errors.Wrap(err, errType, message)
}

// HandleErrorWithIDReleaseInt64 处理需要释放int64类型ID的错误
func (s *BaseService) HandleErrorWithIDReleaseInt64(err error, id int64, releaseFunc func(int64) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID
	if releaseFunc != nil {
		_ = releaseFunc(id)
	}

	// 根据原始错误类型选择合适的错误类型
	errType := errors.GetErrorType(err)
	if errType == errors.ErrorTypePermanent {
		errType = errors.ErrorTypeStorage // 默认使用 Storage 类型，因为通常是资源操作失败
	}
	return errors.Wrap(err, errType, message)
}

// HandleErrorWithIDReleaseString 处理需要释放string类型ID的错误
func (s *BaseService) HandleErrorWithIDReleaseString(err error, id string, releaseFunc func(string) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID
	if releaseFunc != nil {
		_ = releaseFunc(id)
	}

	// 根据原始错误类型选择合适的错误类型
	errType := errors.GetErrorType(err)
	if errType == errors.ErrorTypePermanent {
		errType = errors.ErrorTypeStorage // 默认使用 Storage 类型，因为通常是资源操作失败
	}
	return errors.Wrap(err, errType, message)
}

// WrapError 包装错误，提供统一的错误格式
func (s *BaseService) WrapError(err error, operation string) error {
	if err == nil {
		return nil
	}
	errType := errors.GetErrorType(err)
	if errType == errors.ErrorTypePermanent {
		errType = errors.ErrorTypeStorage // 默认使用 Storage 类型
	}
	return errors.Wrapf(err, errType, "failed to %s", operation)
}

// WrapErrorWithID 包装带ID的错误
func (s *BaseService) WrapErrorWithID(err error, operation, id string) error {
	if err == nil {
		return nil
	}
	errType := errors.GetErrorType(err)
	if errType == errors.ErrorTypePermanent {
		errType = errors.ErrorTypeStorage // 默认使用 Storage 类型
	}
	return errors.Wrapf(err, errType, "failed to %s %s", operation, id)
}

// WrapErrorWithInt64ID 包装带int64 ID的错误
func (s *BaseService) WrapErrorWithInt64ID(err error, operation string, id int64) error {
	if err == nil {
		return nil
	}
	errType := errors.GetErrorType(err)
	if errType == errors.ErrorTypePermanent {
		errType = errors.ErrorTypeStorage // 默认使用 Storage 类型
	}
	return errors.Wrapf(err, errType, "failed to %s %d", operation, id)
}

// LogCreated 记录创建成功日志
func (s *BaseService) LogCreated(resourceType, identifier string) {
	corelog.Infof("Created %s: %s", resourceType, identifier)
}

// LogUpdated 记录更新成功日志
func (s *BaseService) LogUpdated(resourceType, identifier string) {
	corelog.Infof("Updated %s: %s", resourceType, identifier)
}

// LogDeleted 记录删除成功日志
func (s *BaseService) LogDeleted(resourceType, identifier string) {
	corelog.Infof("Deleted %s: %s", resourceType, identifier)
}

// LogWarning 记录警告日志
func (s *BaseService) LogWarning(operation string, err error, args ...interface{}) {
	if len(args) > 0 {
		utils.LogErrorf(err, "Failed to %s", fmt.Sprintf(operation, args...))
	} else {
		utils.LogErrorf(err, "Failed to %s", operation)
	}
}

// SetTimestamps 设置时间戳
func (s *BaseService) SetTimestamps(createdAt, updatedAt *time.Time) {
	now := time.Now()
	if createdAt != nil {
		*createdAt = now
	}
	if updatedAt != nil {
		*updatedAt = now
	}
}

// SetUpdatedTimestamp 设置更新时间戳
func (s *BaseService) SetUpdatedTimestamp(updatedAt *time.Time) {
	if updatedAt != nil {
		*updatedAt = time.Now()
	}
}
