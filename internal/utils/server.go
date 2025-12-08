package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/dispose"
)

// ServiceConfig 服务配置
type ServiceConfig struct {
	// 优雅关闭超时时间
	GracefulShutdownTimeout time.Duration
	// 资源释放超时时间
	ResourceDisposeTimeout time.Duration
	// 是否启用信号处理
	EnableSignalHandling bool
	// 自定义资源管理器
	ResourceManager *dispose.ResourceManager
}

// DefaultServiceConfig 默认服务配置
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		GracefulShutdownTimeout: 30 * time.Second,
		ResourceDisposeTimeout:  10 * time.Second,
		EnableSignalHandling:    true,
		ResourceManager:         nil, // 使用全局资源管理器
	}
}

// Service 服务接口，所有服务都应该实现这个接口
type Service interface {
	// Start 启动服务
	Start(ctx context.Context) error
	// Stop 停止服务
	Stop(ctx context.Context) error
	// Name 服务名称
	Name() string
}

// HTTPService HTTP服务实现
type HTTPService struct {
	addr    string
	handler http.Handler
	server  *http.Server
	mu      sync.Mutex
}

// NewHTTPService 创建HTTP服务
func NewHTTPService(addr string, handler http.Handler) *HTTPService {
	return &HTTPService{
		addr:    addr,
		handler: handler,
	}
}

func (h *HTTPService) Name() string {
	return fmt.Sprintf("HTTP-Server-%s", h.addr)
}

func (h *HTTPService) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.server = &http.Server{
		Addr:    h.addr,
		Handler: h.handler,
	}

	Infof("Starting HTTP service on %s", h.addr)
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			Errorf("HTTP service error: %v", err)
		}
	}()

	return nil
}

func (h *HTTPService) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.server == nil {
		return nil
	}

	Infof("Stopping HTTP service on %s", h.addr)
	return h.server.Shutdown(ctx)
}

// ServiceManager 服务管理器，支持多协议服务
//
// 职责：
//   - 管理多个服务的生命周期（启动、停止）
//   - 管理资源的注册和释放
//   - 提供优雅关闭机制
//   - 支持信号处理和上下文取消
//
// 设计：
//   - 使用 ManagerBase 作为基类，遵循 dispose 体系
//   - Context 从 parentCtx 派生，确保正确的上下文树结构
//   - 资源管理器独立管理，不依赖 dispose 体系
type ServiceManager struct {
	*dispose.ManagerBase
	config        *ServiceConfig
	services      map[string]Service
	resourceMgr   *dispose.ResourceManager
	shutdownChan  chan struct{}
	disposeResult *dispose.DisposeResult
	mu            sync.RWMutex
}

// NewServiceManager 创建新的服务管理器
//
// 参数：
//   - parentCtx: 父上下文，用于派生服务管理器的上下文（必须从 dispose 体系下合适的子树节点分配）
//   - config: 服务配置，如果为 nil 则使用默认配置
//
// 返回：
//   - *ServiceManager: 服务管理器实例
//
// 注意：
//   - parentCtx 不应该为 nil，应该从应用的主 context 或 dispose 体系下的资源派生
//   - 如果 parentCtx 为 nil，将使用 context.Background()（仅用于独立模式或测试）
func NewServiceManager(parentCtx context.Context, config *ServiceConfig) *ServiceManager {
	if config == nil {
		config = DefaultServiceConfig()
	}

	// 使用配置的资源管理器或全局资源管理器
	resourceMgr := config.ResourceManager
	if resourceMgr == nil {
		resourceMgr = dispose.NewResourceManager()
	}

	// 如果没有提供 parentCtx，使用 Background（仅用于独立模式或测试）
	// 注意：在生产环境中，应该始终提供有效的 parentCtx
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	// 使用 ManagerBase 作为基类，遵循 dispose 体系
	manager := &ServiceManager{
		ManagerBase:  dispose.NewManager("ServiceManager", parentCtx),
		config:       config,
		services:     make(map[string]Service),
		resourceMgr:  resourceMgr,
		shutdownChan: make(chan struct{}),
	}

	// 添加清理回调
	manager.AddCleanHandler(manager.onClose)
	return manager
}

// RegisterService 注册服务
func (sm *ServiceManager) RegisterService(service Service) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	name := service.Name()
	if _, exists := sm.services[name]; exists {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "service %s already registered", name)
	}

	sm.services[name] = service
	Infof("Service registered: %s", name)
	return nil
}

// UnregisterService 注销服务
func (sm *ServiceManager) UnregisterService(name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.services[name]; !exists {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "service %s not found", name)
	}

	delete(sm.services, name)
	Infof("Service unregistered: %s", name)
	return nil
}

// GetService 获取服务
func (sm *ServiceManager) GetService(name string) (Service, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	service, exists := sm.services[name]
	return service, exists
}

// ListServices 列出所有服务
func (sm *ServiceManager) ListServices() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	names := make([]string, 0, len(sm.services))
	for name := range sm.services {
		names = append(names, name)
	}
	return names
}

// GetServiceCount 获取服务数量
func (sm *ServiceManager) GetServiceCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.services)
}

// RegisterResource 注册资源到服务管理器
func (sm *ServiceManager) RegisterResource(name string, resource dispose.Disposable) error {
	return sm.resourceMgr.Register(name, resource)
}

// UnregisterResource 从服务管理器注销资源
func (sm *ServiceManager) UnregisterResource(name string) error {
	return sm.resourceMgr.Unregister(name)
}

// ListResources 列出所有注册的资源
func (sm *ServiceManager) ListResources() []string {
	return sm.resourceMgr.ListResources()
}

// GetResourceCount 获取资源数量
func (sm *ServiceManager) GetResourceCount() int {
	return sm.resourceMgr.GetResourceCount()
}

// StartAllServices 启动所有服务
func (sm *ServiceManager) StartAllServices() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	Infof("Starting %d services...", len(sm.services))

	// 使用 ManagerBase 的 Context，确保从 dispose 体系派生
	ctx := sm.Ctx()
	for name, service := range sm.services {
		if err := service.Start(ctx); err != nil {
			Errorf("Failed to start service %s: %v", name, err)
			return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to start service %s", name)
		}
		Infof("Service started: %s", name)
	}

	return nil
}

// StopAllServices 停止所有服务
func (sm *ServiceManager) StopAllServices() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	Infof("Stopping %d services...", len(sm.services))

	// 从 ManagerBase 的 Context 派生超时上下文，确保正确的上下文树结构
	// 这样优雅关闭的超时控制会正确传播到所有子服务
	shutdownCtx, cancel := context.WithTimeout(sm.Ctx(), sm.config.GracefulShutdownTimeout)
	defer cancel()

	var lastErr error
	for name, service := range sm.services {
		Infof("Stopping service: %s", name)
		if err := service.Stop(shutdownCtx); err != nil {
			Errorf("Failed to stop service %s: %v", name, err)
			lastErr = err
		} else {
			Infof("Service stopped: %s", name)
		}
	}

	return lastErr
}

// Run 运行服务管理器
func (sm *ServiceManager) Run() error {
	// 设置信号处理
	if sm.config.EnableSignalHandling {
		sm.setupSignalHandling()
	}

	// 启动所有服务
	if err := sm.StartAllServices(); err != nil {
		// 确保错误信息输出到控制台
		fmt.Fprintf(os.Stderr, "ERROR: Failed to start services: %v\n", err)
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to start services")
	}

	// 等待关闭信号
	sm.waitForShutdown()

	// 执行优雅关闭
	return sm.gracefulShutdown()
}

// RunWithContext 使用指定上下文运行服务管理器
func (sm *ServiceManager) RunWithContext(ctx context.Context) error {
	// 设置信号处理
	if sm.config.EnableSignalHandling {
		sm.setupSignalHandling()
	}

	// 启动所有服务
	if err := sm.StartAllServices(); err != nil {
		// 确保错误信息输出到控制台
		fmt.Fprintf(os.Stderr, "ERROR: Failed to start services: %v\n", err)
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to start services")
	}

	// 等待上下文取消或关闭信号
	select {
	case <-ctx.Done():
		Infof("Context cancelled, initiating shutdown")
	case <-sm.shutdownChan:
		Infof("Shutdown signal received")
	}

	// 执行优雅关闭
	return sm.gracefulShutdown()
}

// waitForShutdown 等待关闭信号
func (sm *ServiceManager) waitForShutdown() {
	<-sm.shutdownChan
}

// setupSignalHandling 设置信号处理
func (sm *ServiceManager) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		Infof("Received signal: %v", sig)
		close(sm.shutdownChan)
	}()
}

// gracefulShutdown 优雅关闭
func (sm *ServiceManager) gracefulShutdown() error {
	Infof("Starting graceful shutdown...")

	// 1. 停止所有服务
	if err := sm.StopAllServices(); err != nil {
		Errorf("Service shutdown error: %v", err)
	}

	// 2. 释放所有资源
	Infof("Disposing resources...")
	sm.disposeResult = sm.resourceMgr.DisposeWithTimeout(sm.config.ResourceDisposeTimeout)

	if sm.disposeResult.HasErrors() {
		Errorf("Resource disposal completed with errors: %v", sm.disposeResult.Error())
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "resource disposal failed: %v", sm.disposeResult.Error())
	}

	// 3. 关闭 ManagerBase（会取消 Context 并执行清理回调）
	sm.Close()

	Infof("Graceful shutdown completed successfully")
	return nil
}

// GetDisposeResult 获取资源释放结果
func (sm *ServiceManager) GetDisposeResult() *dispose.DisposeResult {
	return sm.disposeResult
}

// ForceShutdown 强制关闭
//
// 注意：强制关闭会立即停止所有服务，不等待优雅关闭超时
// 应该仅在紧急情况下使用（如 panic 恢复、资源耗尽等）
func (sm *ServiceManager) ForceShutdown() error {
	Infof("Force shutdown initiated")

	// 强制停止所有服务
	// 使用 ManagerBase 的 Context，但设置很短的超时（1秒）以确保快速响应
	// 如果 Context 已取消，则使用 Background 作为最后手段
	forceCtx := sm.Ctx()
	if forceCtx.Err() != nil {
		// Context 已取消，使用很短的超时作为最后手段
		var cancel context.CancelFunc
		forceCtx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
	} else {
		// 从当前 Context 派生，但使用很短的超时
		var cancel context.CancelFunc
		forceCtx, cancel = context.WithTimeout(forceCtx, 1*time.Second)
		defer cancel()
	}

	sm.mu.RLock()
	for name, service := range sm.services {
		Infof("Force stopping service: %s", name)
		if err := service.Stop(forceCtx); err != nil {
			Errorf("Force stop service %s error: %v", name, err)
		}
	}
	sm.mu.RUnlock()

	// 释放资源
	sm.disposeResult = sm.resourceMgr.DisposeAll()

	if sm.disposeResult.HasErrors() {
		Errorf("Force shutdown resource disposal errors: %v", sm.disposeResult.Error())
	}

	// 关闭 ManagerBase（会取消 Context 并执行清理回调）
	sm.Close()

	return nil
}

// TriggerShutdown 触发关闭
func (sm *ServiceManager) TriggerShutdown() {
	select {
	case <-sm.shutdownChan:
		// 已经关闭
	default:
		close(sm.shutdownChan)
	}
}

// GetContext 获取服务管理器的上下文
//
// 返回 ManagerBase 的 Context，确保从 dispose 体系派生
func (sm *ServiceManager) GetContext() context.Context {
	return sm.Ctx()
}

// Close 关闭服务管理器，返回 error 类型以兼容测试
//
// 注意：此方法会调用 ManagerBase 的 Close，会取消 Context 并执行清理回调
func (sm *ServiceManager) Close() error {
	return sm.ManagerBase.Close()
}

// onClose 资源清理回调
//
// 当 ManagerBase 的 Context 被取消时，会自动调用此方法
func (sm *ServiceManager) onClose() error {
	Infof("Cleaning up service manager resources...")

	// 停止所有服务（使用已取消的 Context，服务应该快速响应）
	// 注意：此时 Context 可能已取消，但 StopAllServices 会创建超时上下文
	if err := sm.StopAllServices(); err != nil {
		Errorf("Failed to stop all services: %v", err)
	}

	// 释放所有资源
	sm.disposeResult = sm.resourceMgr.DisposeAll()
	if sm.disposeResult.HasErrors() {
		Errorf("Resource disposal errors: %v", sm.disposeResult.Error())
	}

	Infof("Service manager resources cleanup completed")
	return nil
}

// 便捷函数

// StartHTTPServiceWithCleanup 便捷函数：启动带资源管理的HTTP服务
//
// 参数：
//   - ctx: 父上下文，用于派生服务管理器的上下文（必须从 dispose 体系下合适的子树节点分配）
//   - addr: HTTP 服务地址
//   - handler: HTTP 处理器
func StartHTTPServiceWithCleanup(ctx context.Context, addr string, handler http.Handler) error {
	config := DefaultServiceConfig()
	manager := NewServiceManager(ctx, config)

	httpService := NewHTTPService(addr, handler)
	if err := manager.RegisterService(httpService); err != nil {
		return err
	}

	return manager.RunWithContext(ctx)
}

// RunServicesWithCleanup 便捷函数：运行带资源管理的服务
//
// 参数：
//   - ctx: 父上下文，用于派生服务管理器的上下文（必须从 dispose 体系下合适的子树节点分配）
//   - config: 服务配置，如果为 nil 则使用默认配置
//   - services: 要运行的服务列表
func RunServicesWithCleanup(ctx context.Context, config *ServiceConfig, services ...Service) error {
	manager := NewServiceManager(ctx, config)

	for _, service := range services {
		if err := manager.RegisterService(service); err != nil {
			return err
		}
	}

	return manager.RunWithContext(ctx)
}
