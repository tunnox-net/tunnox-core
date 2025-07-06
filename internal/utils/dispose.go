package utils

import (
	"context"
	"sync"
)

type Dispose struct {
	currentLock sync.Mutex
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
	onClose     func()
	closeLinks  []func()
	linkLock    sync.Mutex
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
	if c.onClose != nil {
		c.onClose()
	}
	c.closeLinksRun()
}

func (c *Dispose) closeLinksRun() {
	if (c.closeLinks != nil) && (len(c.closeLinks) > 0) {
		for i, closeLink := range c.closeLinks {
			closeLink()
			_ = i // 避免 linter 错误
		}
	}
}

func (c *Dispose) AddCloseFunc(f func()) {
	c.linkLock.Lock()
	defer c.linkLock.Unlock()

	if c.closeLinks == nil {
		c.closeLinks = make([]func(), 0)
	}
	c.closeLinks = append(c.closeLinks, f)
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

	if curParent != nil {
		if c.ctx != nil && !c.closed {
			Warn("context is not nil and context is not closed")
		}
		c.onClose = onClose
		c.ctx, c.cancel = context.WithCancel(curParent)
		c.closed = false
		go func() {
			select {
			case <-c.ctx.Done():
				defer c.currentLock.Unlock()
				c.currentLock.Lock()

				if !c.closed {
					if c.onClose != nil {
						c.onClose()
						c.closeLinksRun()
					}
					c.closed = true
				}
			}
		}()
	}
}
