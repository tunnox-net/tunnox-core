package dispose

import (
	"context"
)

// ResourceFactory 资源工厂，用于创建标准化的资源实例
type ResourceFactory struct{}

// NewResourceFactory 创建资源工厂实例
func NewResourceFactory() *ResourceFactory {
	return &ResourceFactory{}
}

// NewManager 创建标准管理器基类
func (rf *ResourceFactory) NewManager(name string, parentCtx context.Context) *ManagerBase {
	manager := &ManagerBase{
		ResourceBase: NewResourceBase(name),
	}
	manager.Initialize(parentCtx)
	return manager
}

// NewService 创建标准服务基类
func (rf *ResourceFactory) NewService(name string, parentCtx context.Context) *ServiceBase {
	service := &ServiceBase{
		ResourceBase: NewResourceBase(name),
	}
	service.Initialize(parentCtx)
	return service
}

// ManagerBase 标准管理器基类
type ManagerBase struct {
	*ResourceBase
}

// ServiceBase 标准服务基类
type ServiceBase struct {
	*ResourceBase
}

// 全局资源工厂实例
var GlobalResourceFactory = NewResourceFactory()

// NewManager 便捷函数：创建标准管理器
func NewManager(name string, parentCtx context.Context) *ManagerBase {
	return GlobalResourceFactory.NewManager(name, parentCtx)
}

// NewService 便捷函数：创建标准服务
func NewService(name string, parentCtx context.Context) *ServiceBase {
	return GlobalResourceFactory.NewService(name, parentCtx)
}
