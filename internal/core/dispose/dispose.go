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

	// 使用 linkLock 保护 cleanHandlers 的读取，防止与 AddCleanHandler 竞争
	c.linkLock.Lock()
	handlers := make([]func() error, len(c.cleanHandlers))
	copy(handlers, c.cleanHandlers)
	c.linkLock.Unlock()

	if len(handlers) > 0 {
		for i, handler := range handlers {
			if err := handler(); err != nil {
				disposeErr := &DisposeError{
					HandlerIndex: i,
					Err:          err,
				}
				result.Errors = append(result.Errors, disposeErr)

				// 保护 errors 的写入
				c.currentLock.Lock()
				c.errors = append(c.errors, disposeErr)
				c.currentLock.Unlock()

				// 记录错误日志，但不中断其他清理过程
				Errorf("Cleanup handler[%d] failed: %v", i, err)
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
		Warn("ctx already set")
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
			Warn("context is not nil and context is not closed")
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
						Errorf("Context cancellation cleanup failed: %v", result.Error())
					}
					c.closed = true
				}
			}
		}()
	}
}

// SetCtxWithNoOpOnClose 设置上下文并使用空操作的清理回调
func (c *Dispose) SetCtxWithNoOpOnClose(parent context.Context) {
	c.SetCtx(parent, func() error { return nil })
}

// SetCtxWithSelfOnClose 设置上下文并使用自身的 onClose 方法
func (c *Dispose) SetCtxWithSelfOnClose(parent context.Context, selfOnClose func() error) {
	c.SetCtx(parent, selfOnClose)
}
