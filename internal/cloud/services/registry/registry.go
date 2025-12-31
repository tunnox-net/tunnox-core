package registry

import (
	"fmt"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/services/base"
)

// Registry 服务注册器，提供依赖注入和错误处理
type Registry struct {
	container   *container.Container
	baseService *base.Service
}

// NewRegistry 创建服务注册器
func NewRegistry(c *container.Container) *Registry {
	return &Registry{
		container:   c,
		baseService: base.NewService(),
	}
}

// wrapResolveError 包装服务解析错误
func (r *Registry) wrapResolveError(err error, serviceName string) error {
	return r.baseService.WrapError(err, fmt.Sprintf("resolve %s", serviceName))
}

// Container 获取容器
func (r *Registry) Container() *container.Container {
	return r.container
}
