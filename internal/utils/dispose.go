package utils

import (
	"context"
	"sync"
)

type Dispose struct {
	currentLock   sync.Mutex
	closed        bool
	ctx           context.Context
	cancel        context.CancelFunc
	cleanHandlers []func()
	linkLock      sync.Mutex
}

func (c *Dispose) Ctx() context.Context {
	return c.ctx
}

func (c *Dispose) IsClosed() bool {
	c.currentLock.Lock()
	defer c.currentLock.Unlock()
	return c.closed
}

func (c *Dispose) Close() {
	c.currentLock.Lock()
	defer c.currentLock.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	c.runCleanHandlers()
}

func (c *Dispose) runCleanHandlers() {
	if (c.cleanHandlers != nil) && (len(c.cleanHandlers) > 0) {
		for i, closeLink := range c.cleanHandlers {
			closeLink()
			_ = i // 避免 linter 错误
		}
	}
}

func (c *Dispose) AddCleanHandler(f func()) {
	c.linkLock.Lock()
	defer c.linkLock.Unlock()

	if c.cleanHandlers == nil {
		c.cleanHandlers = make([]func(), 0)
	}
	c.cleanHandlers = append(c.cleanHandlers, f)
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
					c.runCleanHandlers()
					c.closed = true
				}
			}
		}()
	}
}
