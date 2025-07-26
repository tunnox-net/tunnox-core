package dispose

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ResourceManager 资源管理器，负责统一管理所有可释放资源
type ResourceManager struct {
	resources map[string]Disposable
	mu        sync.RWMutex
	order     []string // 资源释放顺序
	disposing bool     // 标记是否正在释放资源
}

// NewResourceManager 创建新的资源管理器
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		resources: make(map[string]Disposable),
		order:     make([]string, 0),
	}
}

// Register 注册资源，按注册顺序进行释放
func (rm *ResourceManager) Register(name string, resource Disposable) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[name]; exists {
		return fmt.Errorf("resource %s already registered", name)
	}

	rm.resources[name] = resource
	rm.order = append(rm.order, name)
	Debugf("Registered resource: %s", name)
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
	Debugf("Unregistered resource: %s", name)
	return nil
}

// GetResource 获取指定名称的资源
func (rm *ResourceManager) GetResource(name string) (Disposable, bool) {
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

	rm.disposing = true

	// 保存当前资源列表的副本
	resources := make(map[string]Disposable)
	order := make([]string, len(rm.order))
	copy(order, rm.order)

	// 清空资源列表
	rm.resources = make(map[string]Disposable)
	rm.order = make([]string, 0)

	rm.mu.Unlock()

	// 按注册的相反顺序释放资源
	result := &DisposeResult{Errors: make([]*DisposeError, 0)}
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		resource := resources[name]

		if resource != nil {
			IncrementDisposeCount()
			if err := resource.Dispose(); err != nil {
				disposeErr := &DisposeError{
					HandlerIndex: len(order) - 1 - i,
					ResourceName: name,
					Err:          err,
				}
				result.Errors = append(result.Errors, disposeErr)
				Errorf("Failed to dispose resource %s: %v", name, err)
			} else {
				Debugf("Successfully disposed resource: %s", name)
			}
		}
	}

	rm.mu.Lock()
	rm.disposing = false
	rm.mu.Unlock()

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

// 全局资源管理器实例
var globalResourceManager = NewResourceManager()

// RegisterGlobalResource 注册全局资源
func RegisterGlobalResource(name string, resource Disposable) error {
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

// 全局释放计数器
var disposeCount int64

// IncrementDisposeCount 增加释放计数（用于监控）
func IncrementDisposeCount() {
	disposeCount++
}
