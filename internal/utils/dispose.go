package utils

import (
	"context"
	"log"
	"sync"
)

type Dispose struct {
	currentLock sync.Mutex
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
	onClose     func()
}

func (c *Dispose) Ctx() context.Context {
	return c.ctx
}

func (c *Dispose) IsClosed() bool {
	defer c.currentLock.Unlock()
	c.currentLock.Lock()
	return c.closed
}

func (c *Dispose) Close() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Dispose) SetCtx(parent context.Context, onClose func()) {
	if c.ctx != nil {
		log.Println("[Warning] ctx already set")
		return
	}

	if parent != nil {
		if c.ctx != nil && !c.closed {
			log.Println("[Warning] context is not nil and context is not closed")
		}
		c.onClose = onClose
		c.ctx, c.cancel = context.WithCancel(parent)
		c.closed = false
		go func() {
			select {
			case <-c.ctx.Done():
				defer c.currentLock.Unlock()
				c.currentLock.Lock()

				if !c.closed {
					if c.onClose != nil {
						c.onClose()
					}
					c.closed = true
				}
			}
		}()
	}
}
