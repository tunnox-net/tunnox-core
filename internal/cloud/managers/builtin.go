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

func NewBuiltinCloudControl(parentCtx context.Context, config *ControlConfig) *BuiltinCloudControl {
	memoryStorage := storage.NewMemoryStorage(parentCtx)
	// 创建默认的 CloudControlDeps（使用 nil 服务，后续可按需初始化）
	deps := &CloudControlDeps{}
	base := NewCloudControl(parentCtx, config, memoryStorage, deps)
	control := &BuiltinCloudControl{
		CloudControl: base,
	}
	return control
}

// NewBuiltinCloudControlWithStorage 创建内置云控实例，使用指定的存储实例（主要用于测试）
// 注意：此方法不初始化 Services，建议使用 factories.NewBuiltinCloudControlWithStorageAndServices
func NewBuiltinCloudControlWithStorage(parentCtx context.Context, config *ControlConfig, stor storage.Storage) *BuiltinCloudControl {
	// 创建默认的 CloudControlDeps（使用 nil 服务，后续可按需初始化）
	deps := &CloudControlDeps{}
	base := NewCloudControl(parentCtx, config, stor, deps)
	control := &BuiltinCloudControl{
		CloudControl: base,
	}
	return control
}

// NewBuiltinCloudControlWithDeps 创建内置云控实例，使用指定的存储和依赖
// 这是完整版本，包含所有 Services 支持
func NewBuiltinCloudControlWithDeps(parentCtx context.Context, config *ControlConfig, stor storage.Storage, deps *CloudControlDeps) *BuiltinCloudControl {
	base := NewCloudControl(parentCtx, config, stor, deps)
	return &BuiltinCloudControl{
		CloudControl: base,
	}
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

		case <-b.CloudControl.ManagerBase.Ctx().Done():
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
	if b.CloudControl == nil || b.CloudControl.nodeManager == nil {
		return fmt.Errorf("nodeManager not initialized")
	}
	// 通过 NodeManager 注册节点
	// 版本从 Meta 中获取（如果有），否则留空
	version := ""
	if node.Meta != nil {
		version = node.Meta["version"]
	}
	req := &models.NodeRegisterRequest{
		Address: node.Address,
		Version: version,
		Meta:    node.Meta,
	}
	_, err := b.CloudControl.NodeRegister(req)
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}
	return nil
}
