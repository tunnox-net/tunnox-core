package services

import (
	"fmt"
	"time"
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

	return fmt.Errorf("%s: %w", message, err)
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

	return fmt.Errorf("%s: %w", message, err)
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

	return fmt.Errorf("%s: %w", message, err)
}

// WrapError 包装错误，提供统一的错误格式
func (s *BaseService) WrapError(err error, operation string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s: %w", operation, err)
}

// WrapErrorWithID 包装带ID的错误
func (s *BaseService) WrapErrorWithID(err error, operation, id string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s %s: %w", operation, id, err)
}

// WrapErrorWithInt64ID 包装带int64 ID的错误
func (s *BaseService) WrapErrorWithInt64ID(err error, operation string, id int64) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s %d: %w", operation, id, err)
}

// LogCreated 记录创建成功日志
func (s *BaseService) LogCreated(resourceType, id string, args ...interface{}) {
	if len(args) > 0 {
		utils.Infof("Created %s: %s", resourceType, fmt.Sprintf(id, args...))
	} else {
		utils.Infof("Created %s: %s", resourceType, id)
	}
}

// LogUpdated 记录更新成功日志
func (s *BaseService) LogUpdated(resourceType, id string, args ...interface{}) {
	if len(args) > 0 {
		utils.Infof("Updated %s: %s", resourceType, fmt.Sprintf(id, args...))
	} else {
		utils.Infof("Updated %s: %s", resourceType, id)
	}
}

// LogDeleted 记录删除成功日志
func (s *BaseService) LogDeleted(resourceType, id string, args ...interface{}) {
	if len(args) > 0 {
		utils.Infof("Deleted %s: %s", resourceType, fmt.Sprintf(id, args...))
	} else {
		utils.Infof("Deleted %s: %s", resourceType, id)
	}
}

// LogWarning 记录警告日志
func (s *BaseService) LogWarning(operation string, err error, args ...interface{}) {
	if len(args) > 0 {
		utils.Warnf("Failed to %s: %v", fmt.Sprintf(operation, args...), err)
	} else {
		utils.Warnf("Failed to %s: %v", operation, err)
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
