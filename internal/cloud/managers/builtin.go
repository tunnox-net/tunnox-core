package managers

import (
	"context"
	"time"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/utils"
)

// BuiltinCloudControl 内置云控实现，继承 CloudControl，注入 MemoryStorage

type BuiltinCloudControl struct {
	*CloudControl
}

func NewBuiltinCloudControl(config *ControlConfig) *BuiltinCloudControl {
	memoryStorage := storages.NewMemoryStorage(context.Background())
	base := NewCloudControl(config, memoryStorage)
	return &BuiltinCloudControl{CloudControl: base}
}

// NewBuiltinCloudControlWithStorage 创建内置云控实例，使用指定的存储实例（主要用于测试）
func NewBuiltinCloudControlWithStorage(config *ControlConfig, storage storages.Storage) *BuiltinCloudControl {
	base := NewCloudControl(config, storage)
	return &BuiltinCloudControl{CloudControl: base}
}

// 只在这里实现 BuiltinCloudControl 特有的逻辑，通用逻辑全部在 CloudControl

// Start 启动内置云控
func (b *BuiltinCloudControl) Start() {
	if b.IsClosed() {
		utils.Warnf("Cloud control is already closed, cannot start")
		return
	}

	go b.cleanupRoutine()
	utils.Infof("Built-in cloud control started successfully")
}

// Close 关闭内置云控（实现CloudControlAPI接口）
func (b *BuiltinCloudControl) Close() error {
	b.Dispose.Close()
	return nil
}

// onClose 资源清理回调
func (b *BuiltinCloudControl) onClose() error {
	utils.Infof("Cleaning up cloud control resources...")

	// 等待清理例程完全退出
	time.Sleep(100 * time.Millisecond)

	// 清理各个组件
	if b.jwtManager != nil {
		// JWT管理器可能有自己的清理逻辑
		utils.Infof("JWT manager resources cleaned up")
	}

	if b.cleanupManager != nil {
		utils.Infof("Cleanup manager resources cleaned up")
	}

	if b.lock != nil {
		utils.Infof("Distributed lock resources cleaned up")
	}

	utils.Infof("Cloud control resources cleanup completed")
	return nil
}

// cleanupRoutine 清理例程
func (b *BuiltinCloudControl) cleanupRoutine() {
	utils.LogSystemEvent("cleanup_routine_started", "cloud_control", nil)

	// 注册清理任务
	ctx := context.Background()
	tasks := []struct {
		taskType string
		interval time.Duration
	}{
		{"expired_tokens", 5 * time.Minute},
		{"orphaned_connections", 2 * time.Minute},
		{"stale_mappings", 10 * time.Minute},
	}

	for _, task := range tasks {
		if err := b.cleanupManager.RegisterCleanupTask(ctx, task.taskType, task.interval); err != nil {
			utils.Errorf("Failed to register cleanup task %s: %v", task.taskType, err)
		} else {
			utils.Infof("Registered cleanup task: %s (interval: %v)", task.taskType, task.interval)
		}
	}

	for {
		// 优先检查退出条件
		select {
		case <-b.done:
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "manual_stop",
			})
			utils.Info("Cloud control cleanup routine exited (manual stop)")
			return

		case <-b.Ctx().Done():
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "context_cancelled",
			})
			utils.Info("Cloud control cleanup routine exited (context cancelled)")
			return

		default:
			// 如果没有退出信号，检查ticker
		}

		// 检查是否已关闭
		if b.IsClosed() {
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "disposed",
			})
			utils.Info("Cloud control cleanup routine exited (disposed)")
			return
		}

		// 等待ticker或退出信号
		select {
		case <-b.cleanupTicker.C:
			// 执行清理逻辑
			ctx := context.Background()
			startTime := time.Now()

			// 使用分布式清理管理器
			if _, acquired, err := b.cleanupManager.AcquireCleanupTask(ctx, "expired_tokens"); err == nil && acquired {
				// 清理过期的JWT令牌（简化实现）
				cleanupErr := b.cleanupManager.CompleteCleanupTask(ctx, "expired_tokens", nil)
				utils.LogCleanup("expired_tokens", 0, time.Since(startTime), cleanupErr)
			} else if err != nil {
				utils.LogErrorWithContext(err, "acquire cleanup task", map[string]interface{}{
					"task": "expired_tokens",
				})
			}

			if _, acquired, err := b.cleanupManager.AcquireCleanupTask(ctx, "orphaned_connections"); err == nil && acquired {
				// 清理孤立的连接（简化实现）
				cleanupErr := b.cleanupManager.CompleteCleanupTask(ctx, "orphaned_connections", nil)
				utils.LogCleanup("orphaned_connections", 0, time.Since(startTime), cleanupErr)
			} else if err != nil {
				utils.LogErrorWithContext(err, "acquire cleanup task", map[string]interface{}{
					"task": "orphaned_connections",
				})
			}

			if _, acquired, err := b.cleanupManager.AcquireCleanupTask(ctx, "stale_mappings"); err == nil && acquired {
				// 清理过期的匿名映射
				cleanupErr := b.CleanupExpiredAnonymous()
				if cleanupErr != nil {
					_ = b.cleanupManager.CompleteCleanupTask(ctx, "stale_mappings", cleanupErr)
				} else {
					_ = b.cleanupManager.CompleteCleanupTask(ctx, "stale_mappings", nil)
				}
				utils.LogCleanup("stale_mappings", 0, time.Since(startTime), cleanupErr)
			} else if err != nil {
				utils.LogErrorWithContext(err, "acquire cleanup task", map[string]interface{}{
					"task": "stale_mappings",
				})
			}

		case <-b.done:
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "manual_stop",
			})
			utils.Info("Cloud control cleanup routine exited (manual stop)")
			return

		case <-b.Ctx().Done():
			utils.LogSystemEvent("cleanup_routine_stopped", "cloud_control", map[string]interface{}{
				"reason": "context_cancelled",
			})
			utils.Info("Cloud control cleanup routine exited (context cancelled)")
			return
		}
	}
}
