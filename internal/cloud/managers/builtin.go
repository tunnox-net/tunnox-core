package managers

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// BuiltinCloudControl 内置云控实现，继承 CloudControl，注入 MemoryStorage
type BuiltinCloudControl struct {
	*CloudControl
}

func NewBuiltinCloudControl(config *ControlConfig) *BuiltinCloudControl {
	memoryStorage := storage.NewMemoryStorage(context.Background())
	base := NewCloudControl(config, memoryStorage)
	control := &BuiltinCloudControl{
		CloudControl: base,
	}
	return control
}

// NewBuiltinCloudControlWithStorage 创建内置云控实例，使用指定的存储实例（主要用于测试）
func NewBuiltinCloudControlWithStorage(config *ControlConfig, storage storage.Storage) *BuiltinCloudControl {
	base := NewCloudControl(config, storage)
	control := &BuiltinCloudControl{
		CloudControl: base,
	}
	return control
}

// 只在这里实现 BuiltinCloudControl 特有的逻辑，通用逻辑全部在 CloudControl

// Start 启动内置云控
func (b *BuiltinCloudControl) Start() error {
	corelog.Infof("Starting builtin cloud control...")

	// 启动清理例程
	go b.cleanupRoutine()

	corelog.Infof("Builtin cloud control started successfully")
	return nil
}

// Stop 停止内置云控
func (b *BuiltinCloudControl) Stop() error {
	corelog.Infof("Stopping builtin cloud control...")

	// 停止清理例程
	close(b.done)

	// 等待清理例程完全退出
	time.Sleep(100 * time.Millisecond)

	corelog.Infof("Builtin cloud control stopped successfully")
	return nil
}

// cleanupRoutine 清理例程
func (b *BuiltinCloudControl) cleanupRoutine() {
	corelog.Infof("Cleanup routine started")

	for {
		select {
		case <-b.cleanupTicker.C:
			// 执行清理任务
			corelog.Debugf("Performing cleanup tasks...")
			// 这里可以添加具体的清理逻辑

		case <-b.CloudControl.ResourceBase.Dispose.Ctx().Done():
			corelog.Infof("Cleanup routine stopped")
			return
		}
	}
}

// Close 实现 CloudControlAPI 接口的 Close 方法
func (b *BuiltinCloudControl) Close() error {
	// 调用父类的 Close 方法
	return b.CloudControl.Close()
}

// RegisterNodeDirect 直接注册节点（用于服务器启动时注册自己）
func (b *BuiltinCloudControl) RegisterNodeDirect(node *models.Node) error {
	if b.CloudControl == nil || b.CloudControl.nodeRepo == nil {
		return fmt.Errorf("nodeRepo not initialized")
	}
	// 保存节点记录（创建或更新）
	if err := b.CloudControl.nodeRepo.SaveNode(node); err != nil {
		return fmt.Errorf("failed to save node: %w", err)
	}
	// 添加到节点列表
	if err := b.CloudControl.nodeRepo.AddNodeToList(node); err != nil {
		return fmt.Errorf("failed to add node to list: %w", err)
	}
	return nil
}
