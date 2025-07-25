package managers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/distributed"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
)

// CleanupManager 清理管理器
type CleanupManager struct {
	*dispose.ResourceBase
	storage storages.Storage
	lock    distributed.DistributedLock
	ticker  *time.Ticker
	done    chan bool
}

// CleanupTask 清理任务信息
type CleanupTask struct {
	TaskID   string        `json:"task_id"`
	Type     string        `json:"type"`
	LastRun  time.Time     `json:"last_run"`
	NextRun  time.Time     `json:"next_run"`
	Interval time.Duration `json:"interval"`
	Status   string        `json:"status"` // "running", "completed", "failed"
	Error    string        `json:"error,omitempty"`
}

// NewCleanupManager 创建新的清理管理器
func NewCleanupManager(storage storages.Storage, lock distributed.DistributedLock, ctx context.Context) *CleanupManager {
	manager := &CleanupManager{
		ResourceBase: dispose.NewResourceBase("CleanupManager"),
		storage:      storage,
		lock:         lock,
		ticker:       time.NewTicker(5 * time.Minute), // 每5分钟清理一次
		done:         make(chan bool),
	}
	manager.Initialize(ctx)
	return manager
}

// RegisterCleanupTask 注册清理任务
func (cm *CleanupManager) RegisterCleanupTask(ctx context.Context, taskType string, interval time.Duration) error {
	taskID := fmt.Sprintf("cleanup_%s", taskType)

	// 检查任务是否已存在
	key := fmt.Sprintf("%s:cleanup_task:%s", constants.KeyPrefixCleanup, taskID)
	exists, err := cm.storage.Exists(key)
	if err != nil {
		return fmt.Errorf("check task exists failed: %w", err)
	}

	if exists {
		return nil // 任务已存在
	}

	// 创建新任务
	task := &CleanupTask{
		TaskID:   taskID,
		Type:     taskType,
		LastRun:  time.Time{}, // 零值表示从未运行
		NextRun:  time.Now().Add(interval),
		Interval: interval,
		Status:   "pending",
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task failed: %w", err)
	}

	if err := cm.storage.Set(key, string(data), 0); err != nil {
		return fmt.Errorf("save task failed: %w", err)
	}

	return nil
}

// AcquireCleanupTask 获取清理任务执行权
func (cm *CleanupManager) AcquireCleanupTask(ctx context.Context, taskType string) (*CleanupTask, bool, error) {
	taskID := fmt.Sprintf("cleanup_%s", taskType)
	lockKey := fmt.Sprintf("lock:cleanup_task:%s", taskID)

	// 使用存储层的原子操作获取锁
	lockValue := fmt.Sprintf("cleanup_manager:%d", time.Now().UnixNano())
	acquired, err := cm.storage.SetNX(lockKey, lockValue, 5*time.Minute) // 5分钟锁超时
	if err != nil {
		return nil, false, fmt.Errorf("acquire lock failed: %w", err)
	}
	if !acquired {
		return nil, false, nil // 任务正在被其他实例执行
	}

	// 获取任务信息
	key := fmt.Sprintf("%s:cleanup_task:%s", constants.KeyPrefixCleanup, taskID)
	data, err := cm.storage.Get(key)
	if err != nil {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, fmt.Errorf("get task failed: %w", err)
	}

	taskData, ok := data.(string)
	if !ok {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, fmt.Errorf("invalid task data type")
	}

	var task CleanupTask
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, fmt.Errorf("unmarshal task failed: %w", err)
	}

	// 检查是否需要执行
	if time.Now().Before(task.NextRun) {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, nil     // 还未到执行时间
	}

	// 更新任务状态为运行中
	task.Status = "running"
	task.LastRun = time.Now()
	task.NextRun = time.Now().Add(task.Interval)

	dataBytes, err := json.Marshal(task)
	if err != nil {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, fmt.Errorf("marshal updated task failed: %w", err)
	}

	// 使用原子操作更新任务状态
	success, err := cm.storage.CompareAndSwap(key, taskData, string(dataBytes), 0)
	if err != nil {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, fmt.Errorf("update task failed: %w", err)
	}

	if !success {
		cm.storage.Delete(lockKey) // 释放锁
		return nil, false, fmt.Errorf("task was modified by another process")
	}

	return &task, true, nil
}

// CompleteCleanupTask 完成清理任务
func (cm *CleanupManager) CompleteCleanupTask(ctx context.Context, taskType string, err error) error {
	taskID := fmt.Sprintf("cleanup_%s", taskType)
	lockKey := fmt.Sprintf("lock:cleanup_task:%s", taskID)

	defer cm.storage.Delete(lockKey) // 释放锁

	// 更新任务状态
	key := fmt.Sprintf("%s:cleanup_task:%s", constants.KeyPrefixCleanup, taskID)
	data, err := cm.storage.Get(key)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}

	taskData, ok := data.(string)
	if !ok {
		return fmt.Errorf("invalid task data type")
	}

	var task CleanupTask
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		return fmt.Errorf("unmarshal task failed: %w", err)
	}

	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		task.Status = "completed"
		task.Error = ""
	}

	dataBytes, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal completed task failed: %w", err)
	}

	// 使用原子操作更新任务状态
	success, err := cm.storage.CompareAndSwap(key, taskData, string(dataBytes), 0)
	if err != nil {
		return fmt.Errorf("update completed task failed: %w", err)
	}

	if !success {
		return fmt.Errorf("task was modified by another process")
	}

	return nil
}

// GetCleanupTasks 获取所有清理任务
func (cm *CleanupManager) GetCleanupTasks(ctx context.Context) ([]*CleanupTask, error) {
	// 这里简化实现，实际应该支持模式匹配查询
	// 对于内存存储，我们可以遍历所有键来查找清理任务

	// 注意：这是一个简化的实现，实际生产环境需要更高效的查询方式
	var tasks []*CleanupTask

	// 预定义的任务类型
	taskTypes := []string{"expired_tokens", "orphaned_connections", "stale_mappings"}

	for _, taskType := range taskTypes {
		taskID := fmt.Sprintf("cleanup_%s", taskType)
		key := fmt.Sprintf("%s:cleanup_task:%s", constants.KeyPrefixCleanup, taskID)

		data, err := cm.storage.Get(key)
		if err != nil {
			continue // 任务不存在，跳过
		}

		taskData, ok := data.(string)
		if !ok {
			continue
		}

		var task CleanupTask
		if err := json.Unmarshal([]byte(taskData), &task); err != nil {
			continue
		}

		tasks = append(tasks, &task)
	}

	return tasks, nil
}
