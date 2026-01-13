package dispose

import (
	"context"
	"fmt"
	"sync"
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

// Disposable 统一的资源释放接口
type Disposable interface {
	Dispose() error
}

// Dispose 资源管理结构体
type Dispose struct {
	currentLock   sync.Mutex
	closed        bool
	ctx           context.Context
	cancel        context.CancelFunc
	cleanHandlers []func() error
	linkLock      sync.Mutex
	errors        []*DisposeError
}

// NewDispose 创建并初始化 Dispose（推荐使用）
// 避免"先创建后初始化"的问题
func NewDispose(parent context.Context, onClose func() error) *Dispose {
	d := &Dispose{}
	d.SetCtx(parent, onClose)
	return d
}

// NewDisposeWithNoOp 创建 Dispose，使用空操作的清理回调
func NewDisposeWithNoOp(parent context.Context) *Dispose {
	return NewDispose(parent, func() error { return nil })
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

	// 复制 cleanHandlers 以避免并发访问问题
	c.linkLock.Lock()
	handlers := make([]func() error, len(c.cleanHandlers))
	copy(handlers, c.cleanHandlers)
	c.linkLock.Unlock()

	for i, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(); err != nil {
			disposeErr := &DisposeError{
				HandlerIndex: i,
				Err:          err,
			}
			result.Errors = append(result.Errors, disposeErr)
			c.errors = append(c.errors, disposeErr)

			// 记录错误日志，但不中断其他清理过程
			Errorf("Cleanup handler[%d] failed: %v", i, err)
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
	c.currentLock.Lock()
	defer c.currentLock.Unlock()

	if c.ctx != nil {
		Warn("ctx already set, ignoring SetCtx call")
		return
	}

	curParent := parent
	if curParent == nil {
		// 防御性代码：当调用方传入 nil 时，使用 context.Background()
		// 注意：正常情况下调用方应该传入有效的 parent context
		curParent = context.Background()
	}

	// 只有当 onClose 不为 nil 时才添加到清理处理器
	if onClose != nil {
		c.AddCleanHandler(onClose)
	}

	c.ctx, c.cancel = context.WithCancel(curParent)
	c.closed = false

	// 注意：不再启动 goroutine 监听 context 取消
	// 原因：依赖 context 取消来执行清理是不可靠的设计，会导致 goroutine 泄露
	// 清理逻辑应该通过显式调用 Close() 方法来触发
	// context 取消会让使用该 context 的 I/O 操作返回错误，从而触发上层调用 Close()
}

// SetCtxWithNoOpOnClose 设置上下文并使用空操作的清理回调
func (c *Dispose) SetCtxWithNoOpOnClose(parent context.Context) {
	c.SetCtx(parent, func() error { return nil })
}

// SetCtxWithSelfOnClose 设置上下文并使用自身的 onClose 方法
func (c *Dispose) SetCtxWithSelfOnClose(parent context.Context, selfOnClose func() error) {
	c.SetCtx(parent, selfOnClose)
}
