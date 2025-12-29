package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
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
		if releaseErr := releaseFunc(id); releaseErr != nil {
			corelog.Warnf("HandleErrorWithIDRelease: failed to release ID %v: %v", id, releaseErr)
		}
	}

	// 根据原始错误类型选择合适的错误码
	errCode := errors.GetCode(err)
	if errCode == errors.CodeInternal {
		errCode = errors.CodeStorageError // 默认使用 Storage 类型，因为通常是资源操作失败
	}
	return errors.Wrap(err, errCode, message)
}

// HandleErrorWithIDReleaseInt64 处理需要释放int64类型ID的错误
func (s *BaseService) HandleErrorWithIDReleaseInt64(err error, id int64, releaseFunc func(int64) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID
	if releaseFunc != nil {
		if releaseErr := releaseFunc(id); releaseErr != nil {
			corelog.Warnf("HandleErrorWithIDReleaseInt64: failed to release ID %d: %v", id, releaseErr)
		}
	}

	// 根据原始错误类型选择合适的错误码
	errCode := errors.GetCode(err)
	if errCode == errors.CodeInternal {
		errCode = errors.CodeStorageError // 默认使用 Storage 类型，因为通常是资源操作失败
	}
	return errors.Wrap(err, errCode, message)
}

// HandleErrorWithIDReleaseString 处理需要释放string类型ID的错误
func (s *BaseService) HandleErrorWithIDReleaseString(err error, id string, releaseFunc func(string) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID
	if releaseFunc != nil {
		if releaseErr := releaseFunc(id); releaseErr != nil {
			corelog.Warnf("HandleErrorWithIDReleaseString: failed to release ID %s: %v", id, releaseErr)
		}
	}

	// 根据原始错误类型选择合适的错误码
	errCode := errors.GetCode(err)
	if errCode == errors.CodeInternal {
		errCode = errors.CodeStorageError // 默认使用 Storage 类型，因为通常是资源操作失败
	}
	return errors.Wrap(err, errCode, message)
}

// WrapError 包装错误，提供统一的错误格式
func (s *BaseService) WrapError(err error, operation string) error {
	if err == nil {
		return nil
	}
	errCode := errors.GetCode(err)
	if errCode == errors.CodeInternal {
		errCode = errors.CodeStorageError // 默认使用 Storage 类型
	}
	return errors.Wrapf(err, errCode, "failed to %s", operation)
}

// WrapErrorWithID 包装带ID的错误
func (s *BaseService) WrapErrorWithID(err error, operation, id string) error {
	if err == nil {
		return nil
	}
	errCode := errors.GetCode(err)
	if errCode == errors.CodeInternal {
		errCode = errors.CodeStorageError // 默认使用 Storage 类型
	}
	return errors.Wrapf(err, errCode, "failed to %s %s", operation, id)
}

// WrapErrorWithInt64ID 包装带int64 ID的错误
func (s *BaseService) WrapErrorWithInt64ID(err error, operation string, id int64) error {
	if err == nil {
		return nil
	}
	errCode := errors.GetCode(err)
	if errCode == errors.CodeInternal {
		errCode = errors.CodeStorageError // 默认使用 Storage 类型
	}
	return errors.Wrapf(err, errCode, "failed to %s %d", operation, id)
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

// simpleStatsProvider 简化版统计提供者
// 仅提供 StatsCounter，用于 Services 初始化时的依赖注入
// 这是为了打破 StatsManager <-> Services 之间的循环依赖
type simpleStatsProvider struct {
	counter *stats.StatsCounter
}

// NewSimpleStatsProvider 创建简化版统计提供者
func NewSimpleStatsProvider(storage storage.Storage, parentCtx context.Context) (StatsProvider, error) {
	counter, err := stats.NewStatsCounter(storage, parentCtx)
	if err != nil {
		dispose.Warnf("simpleStatsProvider: failed to create counter: %v", err)
		// 返回一个空的 counter，降级模式
		return &simpleStatsProvider{counter: nil}, nil
	}

	// 初始化计数器
	if err := counter.Initialize(); err != nil {
		dispose.Warnf("simpleStatsProvider: failed to initialize counter: %v", err)
		// 返回一个空的 counter，降级模式
		return &simpleStatsProvider{counter: nil}, nil
	}

	return &simpleStatsProvider{counter: counter}, nil
}

// GetCounter 获取统计计数器
func (s *simpleStatsProvider) GetCounter() *stats.StatsCounter {
	return s.counter
}

// GetUserStats 获取用户统计信息
// 简化版返回基本统计信息，完整实现需要通过 StatsManager 获取
func (s *simpleStatsProvider) GetUserStats(userID string) (*stats.UserStats, error) {
	return &stats.UserStats{
		UserID: userID,
	}, nil
}

// GetClientStats 获取客户端统计信息
// 简化版返回基本统计信息，完整实现需要通过 StatsManager 获取
func (s *simpleStatsProvider) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	return &stats.ClientStats{
		ClientID: clientID,
	}, nil
}
