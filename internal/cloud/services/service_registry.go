package services

import (
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/services/registry"
)

// ServiceRegistry 服务注册器，提供依赖注入和错误处理
// 向后兼容：重新导出 registry.Registry
type ServiceRegistry = registry.Registry

// NewServiceRegistry 创建服务注册器
func NewServiceRegistry(c *container.Container) *ServiceRegistry {
	return registry.NewRegistry(c)
}

// ManagerFactories 管理器工厂函数集合
// 向后兼容：重新导出 registry.ManagerFactories
type ManagerFactories = registry.ManagerFactories
