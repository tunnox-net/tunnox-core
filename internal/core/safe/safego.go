// Package safe 提供安全的 Goroutine 管理
//
// 设计原则：
// 1. 所有 Goroutine 必须有 panic 恢复
// 2. 支持 Goroutine 计数和跟踪
// 3. 支持 context 取消
package safe

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"

	corelog "tunnox-core/internal/core/log"
)

// GoroutineManager 全局 Goroutine 管理器
var globalManager = &Manager{
	activeCount: atomic.Int64{},
	totalCount:  atomic.Int64{},
}

// Manager Goroutine 管理器
type Manager struct {
	activeCount atomic.Int64 // 当前活跃 Goroutine 数量
	totalCount  atomic.Int64 // 累计创建的 Goroutine 数量
	mu          sync.RWMutex
	panicCount  atomic.Int64 // panic 次数
}

// Stats Goroutine 统计信息
type Stats struct {
	Active     int64 // 当前活跃数量
	Total      int64 // 累计创建数量
	PanicCount int64 // panic 次数
}

// GetStats 获取统计信息
func GetStats() Stats {
	return Stats{
		Active:     globalManager.activeCount.Load(),
		Total:      globalManager.totalCount.Load(),
		PanicCount: globalManager.panicCount.Load(),
	}
}

// Go 安全启动 Goroutine（带 panic 恢复）
// name 用于日志标识
func Go(name string, fn func()) {
	globalManager.totalCount.Add(1)
	globalManager.activeCount.Add(1)

	go func() {
		defer func() {
			globalManager.activeCount.Add(-1)
			if r := recover(); r != nil {
				globalManager.panicCount.Add(1)
				stack := string(debug.Stack())
				corelog.Errorf("SafeGo[%s]: panic recovered: %v\n%s", name, r, stack)
			}
		}()
		fn()
	}()
}

// GoWithContext 带 context 的安全 Goroutine
// 当 context 取消时，fn 应该检查 ctx.Done() 并退出
func GoWithContext(ctx context.Context, name string, fn func(ctx context.Context)) {
	globalManager.totalCount.Add(1)
	globalManager.activeCount.Add(1)

	go func() {
		defer func() {
			globalManager.activeCount.Add(-1)
			if r := recover(); r != nil {
				globalManager.panicCount.Add(1)
				stack := string(debug.Stack())
				corelog.Errorf("SafeGo[%s]: panic recovered: %v\n%s", name, r, stack)
			}
		}()
		fn(ctx)
	}()
}

// GoWithCallback 带回调的安全 Goroutine
// onPanic 在发生 panic 时调用，用于自定义处理
func GoWithCallback(name string, fn func(), onPanic func(recovered interface{})) {
	globalManager.totalCount.Add(1)
	globalManager.activeCount.Add(1)

	go func() {
		defer func() {
			globalManager.activeCount.Add(-1)
			if r := recover(); r != nil {
				globalManager.panicCount.Add(1)
				stack := string(debug.Stack())
				corelog.Errorf("SafeGo[%s]: panic recovered: %v\n%s", name, r, stack)
				if onPanic != nil {
					onPanic(r)
				}
			}
		}()
		fn()
	}()
}

// GoLoop 安全启动循环 Goroutine
// 适用于需要持续运行的后台任务
// 如果 fn 返回 error，会记录日志但不会 panic
func GoLoop(ctx context.Context, name string, fn func(ctx context.Context) error) {
	globalManager.totalCount.Add(1)
	globalManager.activeCount.Add(1)

	go func() {
		defer func() {
			globalManager.activeCount.Add(-1)
			if r := recover(); r != nil {
				globalManager.panicCount.Add(1)
				stack := string(debug.Stack())
				corelog.Errorf("SafeGo[%s]: panic recovered in loop: %v\n%s", name, r, stack)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				corelog.Debugf("SafeGo[%s]: context cancelled, exiting loop", name)
				return
			default:
				if err := fn(ctx); err != nil {
					corelog.Warnf("SafeGo[%s]: loop iteration error: %v", name, err)
				}
			}
		}
	}()
}

// WaitGroup 封装的 WaitGroup，自动跟踪 Goroutine
type WaitGroup struct {
	wg   sync.WaitGroup
	name string
}

// NewWaitGroup 创建新的 WaitGroup
func NewWaitGroup(name string) *WaitGroup {
	return &WaitGroup{name: name}
}

// Go 在 WaitGroup 中安全启动 Goroutine
func (w *WaitGroup) Go(fn func()) {
	w.wg.Add(1)
	globalManager.totalCount.Add(1)
	globalManager.activeCount.Add(1)

	go func() {
		defer func() {
			w.wg.Done()
			globalManager.activeCount.Add(-1)
			if r := recover(); r != nil {
				globalManager.panicCount.Add(1)
				stack := string(debug.Stack())
				corelog.Errorf("SafeGo[%s]: panic recovered in WaitGroup: %v\n%s", w.name, r, stack)
			}
		}()
		fn()
	}()
}

// Wait 等待所有 Goroutine 完成
func (w *WaitGroup) Wait() {
	w.wg.Wait()
}

// Pool Goroutine 池
type Pool struct {
	name       string
	maxWorkers int32
	active     atomic.Int32
	queue      chan func()
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewPool 创建 Goroutine 池
func NewPool(ctx context.Context, name string, maxWorkers int32, queueSize int32) *Pool {
	poolCtx, cancel := context.WithCancel(ctx)
	p := &Pool{
		name:       name,
		maxWorkers: maxWorkers,
		queue:      make(chan func(), queueSize),
		ctx:        poolCtx,
		cancel:     cancel,
	}

	// 启动工作 Goroutine
	for i := int32(0); i < maxWorkers; i++ {
		workerName := fmt.Sprintf("%s-worker-%d", name, i)
		GoWithContext(poolCtx, workerName, func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done():
					return
				case fn, ok := <-p.queue:
					if !ok {
						return
					}
					p.active.Add(1)
					func() {
						defer func() {
							p.active.Add(-1)
							if r := recover(); r != nil {
								globalManager.panicCount.Add(1)
								stack := string(debug.Stack())
								corelog.Errorf("SafeGo[%s]: panic in pool task: %v\n%s", workerName, r, stack)
							}
						}()
						fn()
					}()
				}
			}
		})
	}

	return p
}

// Submit 提交任务到池
func (p *Pool) Submit(fn func()) bool {
	select {
	case p.queue <- fn:
		return true
	default:
		return false // 队列已满
	}
}

// SubmitWait 提交任务并等待队列有空位
func (p *Pool) SubmitWait(ctx context.Context, fn func()) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.queue <- fn:
		return nil
	}
}

// ActiveCount 获取活跃工作数
func (p *Pool) ActiveCount() int32 {
	return p.active.Load()
}

// Close 关闭池
func (p *Pool) Close() {
	p.cancel()
	close(p.queue)
}
