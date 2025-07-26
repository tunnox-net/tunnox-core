package dispose

import (
	"context"
)

// ResourceBase 通用资源管理基类
type ResourceBase struct {
	Dispose
	name string
}

// NewResourceBase 创建新的资源基类
func NewResourceBase(name string) *ResourceBase {
	return &ResourceBase{
		name: name,
	}
}

// Initialize 初始化资源，设置上下文和清理回调
func (r *ResourceBase) Initialize(parentCtx context.Context) {
	r.SetCtx(parentCtx, r.onClose)
}

// onClose 通用资源清理回调
func (r *ResourceBase) onClose() error {
	Infof("%s resources cleaned up", r.name)
	return nil
}

// GetName 获取资源名称
func (r *ResourceBase) GetName() string {
	return r.name
}

// SetName 设置资源名称
func (r *ResourceBase) SetName(name string) {
	r.name = name
}

// DisposableResource 可释放资源接口
type DisposableResource interface {
	Initialize(context.Context)
	GetName() string
	SetName(string)
	Disposable
}

// NewDisposableResource 创建可释放资源的通用构造函数
func NewDisposableResource[T DisposableResource](name string, parentCtx context.Context, factory func() T) T {
	resource := factory()
	resource.SetName(name)
	resource.Initialize(parentCtx)
	return resource
}

// ResourceInitializer 资源初始化器接口
type ResourceInitializer interface {
	Initialize(context.Context)
}

// InitializeResource 初始化资源的通用函数
func InitializeResource(resource ResourceInitializer, parentCtx context.Context) {
	resource.Initialize(parentCtx)
}
