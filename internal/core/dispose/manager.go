package dispose

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/utils"
)

// DisposeError 清理过程中的错误信息
type DisposeError struct {
	HandlerIndex int
	ResourceName string
	Err          error
}

func (e *DisposeError) Error() string {
	if e.ResourceName != "" {
		return fmt.Sprintf("cleanup resource[%s] handler[%d] failed: %v", e.ResourceName, e.HandlerIndex, e.Err)
	}
	return fmt.Sprintf("cleanup handler[%d] failed: %v", e.HandlerIndex, e.Err)
}

// DisposeResult 清理结果
type DisposeResult struct {
	Errors         []*DisposeError
	ActualDisposal bool // 标记是否实际执行了释放操作
}

func (r *DisposeResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *DisposeResult) Error() string {
	if !r.HasErrors() {
		return ""
	}
	return fmt.Sprintf("dispose cleanup failed with %d errors", len(r.Errors))
}

// ResourceManager 资源管理器，负责统一管理所有可释放资源
type ResourceManager struct {
	resources map[string]types.Disposable
	mu        sync.RWMutex
	order     []string // 资源释放顺序
	disposing bool     // 标记是否正在释放资源
}

// NewResourceManager 创建新的资源管理器
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		resources: make(map[string]types.Disposable),
		order:     make([]string, 0),
	}
}

// Register 注册资源，按注册顺序进行释放
func (rm *ResourceManager) Register(name string, resource types.Disposable) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[name]; exists {
		return fmt.Errorf("resource %s already registered", name)
	}

	rm.resources[name] = resource
	rm.order = append(rm.order, name)
	utils.Debugf("Registered resource: %s", name)
	return nil
}

// Unregister 注销资源
func (rm *ResourceManager) Unregister(name string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[name]; !exists {
		return fmt.Errorf("resource %s not found", name)
	}

	delete(rm.resources, name)
	// 从顺序列表中移除
	for i, resourceName := range rm.order {
		if resourceName == name {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	utils.Debugf("Unregistered resource: %s", name)
	return nil
}

// GetResource 获取指定名称的资源
func (rm *ResourceManager) GetResource(name string) (types.Disposable, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resource, exists := rm.resources[name]
	return resource, exists
}

// ListResources 列出所有资源名称
func (rm *ResourceManager) ListResources() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	names := make([]string, len(rm.order))
	copy(names, rm.order)
	return names
}

// GetResourceCount 获取资源数量
func (rm *ResourceManager) GetResourceCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.resources)
}

// DisposeAll 释放所有资源，按注册的相反顺序
func (rm *ResourceManager) DisposeAll() *DisposeResult {
	rm.mu.Lock()

	// 如果正在释放或已经没有资源，返回空结果
	if rm.disposing || len(rm.resources) == 0 {
		rm.mu.Unlock()
		return &DisposeResult{Errors: make([]*DisposeError, 0)}
	}

	// 标记正在释放
	rm.disposing = true

	// 保存当前资源列表的副本
	resources := make(map[string]types.Disposable)
	order := make([]string, len(rm.order))
	copy(order, rm.order)

	for name, resource := range rm.resources {
		resources[name] = resource
	}

	// 清空资源列表
	rm.resources = make(map[string]types.Disposable)
	rm.order = make([]string, 0)

	rm.mu.Unlock()

	result := &DisposeResult{Errors: make([]*DisposeError, 0)}

	// 按相反顺序释放资源
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		resource := resources[name]

		if err := resource.Dispose(); err != nil {
			disposeErr := &DisposeError{
				HandlerIndex: len(order) - 1 - i,
				ResourceName: name,
				Err:          err,
			}
			result.Errors = append(result.Errors, disposeErr)
			utils.Errorf("Failed to dispose resource %s: %v", name, err)
		} else {
			utils.Debugf("Successfully disposed resource: %s", name)
		}
	}

	// 重置释放标记
	rm.mu.Lock()
	rm.disposing = false
	rm.mu.Unlock()

	// 添加一个特殊标记表示这是实际执行释放的结果
	result.ActualDisposal = true

	// 增加释放计数用于监控
	IncrementDisposeCount()

	return result
}

// DisposeWithTimeout 带超时的资源释放
func (rm *ResourceManager) DisposeWithTimeout(timeout time.Duration) *DisposeResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan *DisposeResult, 1)

	go func() {
		resultChan <- rm.DisposeAll()
	}()

	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		return &DisposeResult{
			Errors: []*DisposeError{
				{
					HandlerIndex: -1,
					ResourceName: "timeout",
					Err:          fmt.Errorf("dispose timeout after %v", timeout),
				},
			},
		}
	}
}

// Dispose 原有的Dispose结构体，保持向后兼容
type Dispose struct {
	currentLock   sync.Mutex
	closed        bool
	ctx           context.Context
	cancel        context.CancelFunc
	cleanHandlers []func() error
	linkLock      sync.Mutex
	errors        []*DisposeError
}

func (c *Dispose) Ctx() context.Context {
	return c.ctx
}

func (c *Dispose) IsClosed() bool {
	c.currentLock.Lock()
	defer c.currentLock.Unlock()
	return c.closed
}

// Close 关闭并返回清理结果
func (c *Dispose) Close() *DisposeResult {
	c.currentLock.Lock()
	defer c.currentLock.Unlock()
	if c.closed {
		return &DisposeResult{Errors: c.errors}
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	return c.runCleanHandlers()
}

// CloseWithError 兼容旧版本的 Close 方法，返回 error
func (c *Dispose) CloseWithError() error {
	result := c.Close()
	if result.HasErrors() {
		// 返回第一个错误的具体消息，保持向后兼容
		if len(result.Errors) > 0 {
			return result.Errors[0].Err
		}
		return result
	}
	return nil
}

func (c *Dispose) runCleanHandlers() *DisposeResult {
	result := &DisposeResult{Errors: make([]*DisposeError, 0)}

	if (c.cleanHandlers != nil) && (len(c.cleanHandlers) > 0) {
		for i, handler := range c.cleanHandlers {
			if err := handler(); err != nil {
				disposeErr := &DisposeError{
					HandlerIndex: i,
					Err:          err,
				}
				result.Errors = append(result.Errors, disposeErr)
				c.errors = append(c.errors, disposeErr)

				// 记录错误日志，但不中断其他清理过程
				utils.Errorf("Cleanup handler[%d] failed: %v", i, err)
			}
		}
	}

	return result
}

// AddCleanHandler 添加返回错误的清理处理器
func (c *Dispose) AddCleanHandler(f func() error) {
	c.linkLock.Lock()
	defer c.linkLock.Unlock()

	if c.cleanHandlers == nil {
		c.cleanHandlers = make([]func() error, 0)
	}
	c.cleanHandlers = append(c.cleanHandlers, f)
}

// GetErrors 获取清理过程中的错误
func (c *Dispose) GetErrors() []*DisposeError {
	c.currentLock.Lock()
	defer c.currentLock.Unlock()
	return c.errors
}

func (c *Dispose) SetCtx(parent context.Context, onClose func() error) {
	if c.ctx != nil {
		utils.Warn("ctx already set")
		return
	}

	curParent := parent
	if curParent == nil {
		curParent = context.Background()
	}

	// 只有当 onClose 不为 nil 时才添加到清理处理器
	if onClose != nil {
		c.AddCleanHandler(onClose)
	}

	if curParent != nil {
		if c.ctx != nil && !c.closed {
			utils.Warn("context is not nil and context is not closed")
		}
		c.ctx, c.cancel = context.WithCancel(curParent)
		c.closed = false
		go func() {
			select {
			case <-c.ctx.Done():
				defer c.currentLock.Unlock()
				c.currentLock.Lock()

				if !c.closed {
					result := c.runCleanHandlers()
					if result.HasErrors() {
						utils.Errorf("Context cancellation cleanup failed: %v", result.Error())
					}
					c.closed = true
				}
			}
		}()
	}
}

// 全局资源管理器实例
var globalResourceManager = NewResourceManager()

// RegisterGlobalResource 注册全局资源
func RegisterGlobalResource(name string, resource types.Disposable) error {
	return globalResourceManager.Register(name, resource)
}

// UnregisterGlobalResource 注销全局资源
func UnregisterGlobalResource(name string) error {
	return globalResourceManager.Unregister(name)
}

// DisposeAllGlobalResources 释放所有全局资源
func DisposeAllGlobalResources() *DisposeResult {
	return globalResourceManager.DisposeAll()
}

// DisposeAllGlobalResourcesWithTimeout 带超时的全局资源释放
func DisposeAllGlobalResourcesWithTimeout(timeout time.Duration) *DisposeResult {
	return globalResourceManager.DisposeWithTimeout(timeout)
}

// IncrementDisposeCount 增加释放计数（用于监控）
func IncrementDisposeCount() {
	// 这里可以实现更复杂的监控逻辑
}
