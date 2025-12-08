package container

import (
	"context"
	"reflect"
	"sync"
	"tunnox-core/internal/core/dispose"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"
)

// ServiceProvider 服务提供者接口
type ServiceProvider interface {
	// GetService 获取服务实例
	GetService() (interface{}, error)
	// Close 关闭服务
	Close() error
}

// SingletonProvider 单例服务提供者
type SingletonProvider struct {
	instance interface{}
	creator  func() (interface{}, error)
	mu       sync.RWMutex
}

// NewSingletonProvider 创建单例提供者
func NewSingletonProvider(creator func() (interface{}, error)) *SingletonProvider {
	return &SingletonProvider{
		creator: creator,
	}
}

// GetService 获取单例服务
func (s *SingletonProvider) GetService() (interface{}, error) {
	s.mu.RLock()
	if s.instance != nil {
		s.mu.RUnlock()
		return s.instance, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// 双重检查锁定
	if s.instance != nil {
		return s.instance, nil
	}

	instance, err := s.creator()
	if err != nil {
		return nil, coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to create service instance")
	}

	s.instance = instance
	return s.instance, nil
}

// Close 关闭服务
func (s *SingletonProvider) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.instance != nil {
		if closer, ok := s.instance.(interface{ Close() error }); ok {
			return closer.Close()
		}
	}
	return nil
}

// TransientProvider 瞬态服务提供者
type TransientProvider struct {
	creator func() (interface{}, error)
}

// NewTransientProvider 创建瞬态提供者
func NewTransientProvider(creator func() (interface{}, error)) *TransientProvider {
	return &TransientProvider{
		creator: creator,
	}
}

// GetService 获取瞬态服务
func (t *TransientProvider) GetService() (interface{}, error) {
	return t.creator()
}

// Close 瞬态服务无需关闭
func (t *TransientProvider) Close() error {
	return nil
}

// Container 依赖注入容器
type Container struct {
	*dispose.ManagerBase
	services map[string]ServiceProvider
	mu       sync.RWMutex
	ctx      context.Context
}

// NewContainer 创建新的容器
func NewContainer(parentCtx context.Context) *Container {
	container := &Container{
		ManagerBase: dispose.NewManager("Container", parentCtx),
		services:    make(map[string]ServiceProvider),
		ctx:         parentCtx,
	}
	container.AddCleanHandler(container.onClose)
	return container
}

// onClose 资源清理回调
func (c *Container) onClose() error {
	utils.Infof("Cleaning up container resources...")

	c.mu.Lock()
	defer c.mu.Unlock()

	// 按注册的相反顺序关闭服务
	serviceNames := make([]string, 0, len(c.services))
	for name := range c.services {
		serviceNames = append(serviceNames, name)
	}

	for i := len(serviceNames) - 1; i >= 0; i-- {
		name := serviceNames[i]
		provider := c.services[name]

		if err := provider.Close(); err != nil {
			utils.Errorf("Failed to close service %s: %v", name, err)
		} else {
			utils.Infof("Successfully closed service: %s", name)
		}
	}

	// 清空服务映射
	c.services = make(map[string]ServiceProvider)
	utils.Infof("Container resources cleanup completed")
	return nil
}

// RegisterSingleton 注册单例服务
func (c *Container) RegisterSingleton(name string, creator func() (interface{}, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.services[name]; exists {
		utils.Warnf("Service %s already registered, overwriting", name)
	}

	c.services[name] = NewSingletonProvider(creator)
	utils.Infof("Registered singleton service: %s", name)
}

// RegisterTransient 注册瞬态服务
func (c *Container) RegisterTransient(name string, creator func() (interface{}, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.services[name]; exists {
		utils.Warnf("Service %s already registered, overwriting", name)
	}

	c.services[name] = NewTransientProvider(creator)
	utils.Infof("Registered transient service: %s", name)
}

// Resolve 解析服务
func (c *Container) Resolve(name string) (interface{}, error) {
	c.mu.RLock()
	provider, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil, coreErrors.Newf(coreErrors.ErrorTypePermanent, "service %s not found", name)
	}

	return provider.GetService()
}

// ResolveTyped 解析指定类型的服务
func (c *Container) ResolveTyped(name string, target interface{}) error {
	service, err := c.Resolve(name)
	if err != nil {
		return err
	}

	// 使用反射设置目标值
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "target must be a pointer")
	}

	serviceValue := reflect.ValueOf(service)
	if !serviceValue.Type().AssignableTo(targetValue.Elem().Type()) {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "service type %T is not assignable to target type %T", service, target)
	}

	targetValue.Elem().Set(serviceValue)
	return nil
}

// HasService 检查服务是否已注册
func (c *Container) HasService(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.services[name]
	return exists
}

// ListServices 列出所有已注册的服务
func (c *Container) ListServices() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	services := make([]string, 0, len(c.services))
	for name := range c.services {
		services = append(services, name)
	}
	return services
}

// Close 关闭容器
func (c *Container) Close() error {
	return c.ManagerBase.Close()
}
