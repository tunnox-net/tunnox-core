package services

import (
	"fmt"
	"tunnox-core/internal/cloud/container"
)

// ServiceRegistry 服务注册器，提供依赖注入和错误处理
type ServiceRegistry struct {
	container   *container.Container
	baseService *BaseService
}

// NewServiceRegistry 创建服务注册器
func NewServiceRegistry(container *container.Container) *ServiceRegistry {
	return &ServiceRegistry{
		container:   container,
		baseService: NewBaseService(),
	}
}

// wrapResolveError 包装服务解析错误
func (r *ServiceRegistry) wrapResolveError(err error, serviceName string) error {
	return r.baseService.WrapError(err, fmt.Sprintf("resolve %s", serviceName))
}
