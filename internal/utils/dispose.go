package utils

import (
	"context"
	"fmt"
	"sync"
)

// DisposeError 清理过程中的错误信息
type DisposeError struct {
	HandlerIndex int
	Err          error
}

func (e *DisposeError) Error() string {
	return fmt.Sprintf("cleanup handler[%d] failed: %v", e.HandlerIndex, e.Err)
}

// DisposeResult 清理结果
type DisposeResult struct {
	Errors []*DisposeError
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

// AddCleanHandlerNoError 添加不返回错误的清理处理器（向后兼容）
func (c *Dispose) AddCleanHandlerNoError(f func()) {
	c.AddCleanHandler(func() error {
		f()
		return nil
	})
}

// GetErrors 获取清理过程中的错误
func (c *Dispose) GetErrors() []*DisposeError {
	c.currentLock.Lock()
	defer c.currentLock.Unlock()
	return c.errors
}

func (c *Dispose) SetCtx(parent context.Context, onClose func()) {
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
		c.AddCleanHandlerNoError(onClose)
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
