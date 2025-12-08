package registry

import (
	"tunnox-core/internal/cloud/container"
)

// Container 依赖注入容器接口
// 职责：
// 1. 服务的注册和解析（依赖注入）
// 2. 类型安全的服务解析
// 3. 服务存在性检查
// 注意：使用接口而非直接依赖具体类型，遵循依赖倒置原则
type Container interface {
	// Resolve 解析服务
	Resolve(name string) (interface{}, error)

	// ResolveTyped 解析指定类型的服务（类型安全）
	ResolveTyped(name string, target interface{}) error

	// HasService 检查服务是否存在
	HasService(name string) bool

	// ListServices 列出所有服务
	ListServices() []string
}

// containerAdapter 适配现有 container.Container
// 将 container.Container 适配为 registry.Container 接口
type containerAdapter struct {
	container *container.Container
}

// NewContainerAdapter 创建容器适配器
func NewContainerAdapter(c *container.Container) Container {
	return &containerAdapter{container: c}
}

// Resolve 解析服务
func (a *containerAdapter) Resolve(name string) (interface{}, error) {
	return a.container.Resolve(name)
}

// ResolveTyped 解析指定类型的服务（类型安全）
func (a *containerAdapter) ResolveTyped(name string, target interface{}) error {
	// 直接使用现有容器的 ResolveTyped 方法（已实现类型安全）
	return a.container.ResolveTyped(name, target)
}

// HasService 检查服务是否存在
func (a *containerAdapter) HasService(name string) bool {
	return a.container.HasService(name)
}

// ListServices 列出所有服务
func (a *containerAdapter) ListServices() []string {
	return a.container.ListServices()
}

