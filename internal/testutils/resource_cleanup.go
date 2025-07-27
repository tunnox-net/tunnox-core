package testutils

import (
	"tunnox-core/internal/core/dispose"
)

// ResourceCleanup 资源清理工具，用于测试中的资源管理
type ResourceCleanup struct {
	resources []dispose.Disposable
}

// NewResourceCleanup 创建资源清理工具
func NewResourceCleanup() *ResourceCleanup {
	return &ResourceCleanup{
		resources: make([]dispose.Disposable, 0),
	}
}

// AddResource 添加需要清理的资源
func (rc *ResourceCleanup) AddResource(resource dispose.Disposable) {
	rc.resources = append(rc.resources, resource)
}

// AddCloser 添加实现了Close方法的资源
func (rc *ResourceCleanup) AddCloser(closer interface{ Close() error }) {
	// 创建一个包装器，将Close方法适配为Disposable接口
	wrapper := &closerWrapper{closer: closer}
	rc.AddResource(wrapper)
}

// Cleanup 清理所有资源
func (rc *ResourceCleanup) Cleanup() {
	for _, resource := range rc.resources {
		if resource != nil {
			_ = resource.Dispose()
		}
	}
	rc.resources = make([]dispose.Disposable, 0)
}

// DeferCleanup 返回一个函数，用于在defer中调用
func (rc *ResourceCleanup) DeferCleanup() func() {
	return rc.Cleanup
}

// closerWrapper 包装实现了Close方法的资源
type closerWrapper struct {
	closer interface{ Close() error }
}

// Dispose 实现Disposable接口
func (cw *closerWrapper) Dispose() error {
	return cw.closer.Close()
}

// WithCleanup 使用资源清理的辅助函数
func WithCleanup(fn func(*ResourceCleanup)) {
	cleanup := NewResourceCleanup()
	defer cleanup.Cleanup()
	fn(cleanup)
}
