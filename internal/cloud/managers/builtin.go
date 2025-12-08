package managers

import (
	"context"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils"
)

// BuiltinCloudControl 内置云控实现，继承 CloudControl，注入 MemoryStorage
type BuiltinCloudControl struct {
	*CloudControl
}

func NewBuiltinCloudControl(config *ControlConfig, parentCtx context.Context) *BuiltinCloudControl {
	if parentCtx == nil {
		// 如果没有提供 context，创建一个新的（仅用于独立模式）
		// 注意：这应该只在 main 函数或测试中使用
		parentCtx = context.Background()
	}
	memoryStorage := storage.NewMemoryStorage(parentCtx)
	base, err := NewCloudControl(config, memoryStorage, parentCtx)
	if err != nil {
		return nil
	}
	control := &BuiltinCloudControl{
		CloudControl: base,
	}
	return control
}

// NewBuiltinCloudControlWithStorage 创建内置云控实例，使用指定的存储实例
func NewBuiltinCloudControlWithStorage(config *ControlConfig, storage storage.Storage, parentCtx context.Context) *BuiltinCloudControl {
	if parentCtx == nil {
		// 如果没有提供 context，创建一个新的（仅用于独立模式）
		// 注意：这应该只在 main 函数或测试中使用
		parentCtx = context.Background()
	}
	base, err := NewCloudControl(config, storage, parentCtx)
	if err != nil {
		return nil
	}
	control := &BuiltinCloudControl{
		CloudControl: base,
	}
	return control
}

// 只在这里实现 BuiltinCloudControl 特有的逻辑，通用逻辑全部在 CloudControl

// Start 启动内置云控
func (b *BuiltinCloudControl) Start() error {
	utils.Infof("Starting builtin cloud control...")

	// 启动清理例程
	go b.cleanupRoutine()

	utils.Infof("Builtin cloud control started successfully")
	return nil
}

// cleanupRoutine 清理例程
func (b *BuiltinCloudControl) cleanupRoutine() {
	utils.Infof("Cleanup routine started")

	for {
		select {
		case <-b.cleanupTicker.C:
			// 执行清理任务
			// Performing cleanup tasks (removed debug log)
			// 这里可以添加具体的清理逻辑

		case <-b.CloudControl.ResourceBase.Dispose.Ctx().Done():
			utils.Infof("Cleanup routine stopped")
			return
		}
	}
}

// Close 实现 CloudControlAPI 接口的 Close 方法
// 统一使用 Close 方法，移除 Stop 方法以保持一致性
func (b *BuiltinCloudControl) Close() error {
	// 调用父类的 Close 方法（父类已经处理了 cleanupTicker 和 done 通道）
	return b.CloudControl.Close()
}
