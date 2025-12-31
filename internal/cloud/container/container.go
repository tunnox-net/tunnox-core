package container

import (
	"context"
	"reflect"
	"sync"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// ServiceProvider 服务提供者接口
// 注意: 使用 any 返回类型是 DI 容器的标准模式，因为容器需要存储和返回任意类型的服务
// 调用方通过 ResolveTyped 或类型断言获取具体类型
type ServiceProvider interface {
	// GetService 获取服务实例
	GetService() (any, error)
	// Close 关闭服务
	Close() error
}

// SingletonProvider 单例服务提供者
// 注意: instance 和 creator 使用 any 是 DI 容器的必要设计，用于存储任意类型服务
type SingletonProvider struct {
	instance any
	creator  func() (any, error)
	mu       sync.RWMutex
}

// NewSingletonProvider 创建单例提供者
func NewSingletonProvider(creator func() (any, error)) *SingletonProvider {
	return &SingletonProvider{
		creator: creator,
	}
}

// GetService 获取单例服务
func (s *SingletonProvider) GetService() (any, error) {
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create service instance")
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
// 注意: creator 使用 any 是 DI 容器的必要设计
type TransientProvider struct {
	creator func() (any, error)
}

// NewTransientProvider 创建瞬态提供者
func NewTransientProvider(creator func() (any, error)) *TransientProvider {
	return &TransientProvider{
		creator: creator,
	}
}

// GetService 获取瞬态服务
func (t *TransientProvider) GetService() (any, error) {
	return t.creator()
}

// Close 瞬态服务无需关闭
func (t *TransientProvider) Close() error {
	return nil
}

// Container 依赖注入容器
type Container struct {
	services map[string]ServiceProvider
	mu       sync.RWMutex
	ctx      context.Context
	dispose.Dispose
}

// NewContainer 创建新的容器
func NewContainer(parentCtx context.Context) *Container {
	container := &Container{
		services: make(map[string]ServiceProvider),
	}
	container.SetCtx(parentCtx, container.onClose)
	return container
}

// onClose 资源清理回调
func (c *Container) onClose() error {
	corelog.Infof("Cleaning up container resources...")

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
			corelog.Errorf("Failed to close service %s: %v", name, err)
		} else {
			corelog.Infof("Successfully closed service: %s", name)
		}
	}

	// 清空服务映射
	c.services = make(map[string]ServiceProvider)
	corelog.Infof("Container resources cleanup completed")
	return nil
}

// RegisterSingleton 注册单例服务
// 注意: creator 返回 any 是 DI 容器的标准模式，调用方使用 Resolve + 类型断言获取具体类型
func (c *Container) RegisterSingleton(name string, creator func() (any, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.services[name]; exists {
		corelog.Warnf("Service %s already registered, overwriting", name)
	}

	c.services[name] = NewSingletonProvider(creator)
	corelog.Infof("Registered singleton service: %s", name)
}

// RegisterTransient 注册瞬态服务
// 注意: creator 返回 any 是 DI 容器的标准模式
func (c *Container) RegisterTransient(name string, creator func() (any, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.services[name]; exists {
		corelog.Warnf("Service %s already registered, overwriting", name)
	}

	c.services[name] = NewTransientProvider(creator)
	corelog.Infof("Registered transient service: %s", name)
}

// Resolve 解析服务
// 注意: 返回 any 是 DI 容器的标准模式，调用方需要类型断言
func (c *Container) Resolve(name string) (any, error) {
	c.mu.RLock()
	provider, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil, coreerrors.Newf(coreerrors.CodeNotFound, "service %s not found", name)
	}

	return provider.GetService()
}

// ResolveTyped 解析指定类型的服务
// 注意: target 使用 any 是反射操作的必要条件
func (c *Container) ResolveTyped(name string, target any) error {
	service, err := c.Resolve(name)
	if err != nil {
		return err
	}

	// 使用反射设置目标值
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr {
		return coreerrors.New(coreerrors.CodeInvalidParam, "target must be a pointer")
	}

	serviceValue := reflect.ValueOf(service)
	if !serviceValue.Type().AssignableTo(targetValue.Elem().Type()) {
		return coreerrors.Newf(coreerrors.CodeInvalidParam, "service type %T is not assignable to target type %T", service, target)
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
	result := c.Dispose.Close()
	if result.HasErrors() {
		return coreerrors.Newf(coreerrors.CodeInternal, "container cleanup failed: %s", result.Error())
	}
	return nil
}
