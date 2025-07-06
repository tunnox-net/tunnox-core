package tests

import (
	"context"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/utils"
)

func TestDisposeCtx(t *testing.T) {
	// 测试获取上下文
	dispose := &utils.Dispose{}

	// 初始状态应该返回nil上下文
	if dispose.Ctx() != nil {
		t.Error("Initial context should be nil")
	}

	// 设置上下文后应该能正确获取
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, nil)

	if dispose.Ctx() == nil {
		t.Error("Context should not be nil after SetCtx")
	}
}

func TestDisposeIsClosed(t *testing.T) {
	// 测试关闭状态检查
	dispose := &utils.Dispose{}

	// 初始状态应该未关闭
	if dispose.IsClosed() {
		t.Error("Initial state should not be closed")
	}

	// 设置上下文
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, nil)

	// 设置后应该未关闭
	if dispose.IsClosed() {
		t.Error("Should not be closed after SetCtx")
	}
}

func TestDisposeClose(t *testing.T) {
	// 测试关闭功能
	dispose := &utils.Dispose{}

	// 设置上下文
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, nil)

	// 关闭
	dispose.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !dispose.IsClosed() {
		t.Error("Should be closed after calling Close()")
	}
}

func TestDisposeSetCtx(t *testing.T) {
	// 测试设置上下文
	dispose := &utils.Dispose{}

	// 设置上下文
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, nil)

	// 验证上下文已设置
	if dispose.Ctx() == nil {
		t.Error("Context should be set after SetCtx")
	}

	// 验证未关闭
	if dispose.IsClosed() {
		t.Error("Should not be closed after SetCtx")
	}
}

func TestDisposeSetCtxWithOnClose(t *testing.T) {
	// 测试设置带清理函数的上下文
	dispose := &utils.Dispose{}

	called := false
	onClose := func() {
		called = true
	}

	// 设置上下文和清理函数
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, onClose)

	// 关闭
	dispose.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证清理函数被调用
	if !called {
		t.Error("OnClose function should be called")
	}

	// 验证已关闭
	if !dispose.IsClosed() {
		t.Error("Should be closed after calling Close()")
	}
}

func TestDisposeContextCancellation(t *testing.T) {
	// 测试上下文取消时的行为
	dispose := &utils.Dispose{}

	called := false
	onClose := func() {
		called = true
	}

	// 设置上下文和清理函数
	parentCtx, cancel := context.WithCancel(context.Background())
	dispose.SetCtx(parentCtx, onClose)

	// 取消上下文
	cancel()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证清理函数被调用
	if !called {
		t.Error("OnClose function should be called on context cancellation")
	}

	// 验证已关闭
	if !dispose.IsClosed() {
		t.Error("Should be closed after context cancellation")
	}
}

func TestDisposeConcurrentAccess(t *testing.T) {
	// 测试并发访问的安全性
	dispose := &utils.Dispose{}

	// 设置上下文
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, nil)

	// 并发访问
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 并发检查关闭状态
			_ = dispose.IsClosed()

			// 模拟一些工作
			time.Sleep(1 * time.Millisecond)
		}()
	}

	wg.Wait()

	// 验证仍然可用
	if dispose.IsClosed() {
		t.Error("Should not be closed after concurrent access")
	}
}

func TestDisposeMultipleClose(t *testing.T) {
	// 测试多次调用Close的安全性
	dispose := &utils.Dispose{}

	// 设置上下文
	parentCtx := context.Background()
	dispose.SetCtx(parentCtx, nil)

	// 多次调用Close
	dispose.Close()
	dispose.Close()
	dispose.Close()

	// 等待goroutine执行完成
	time.Sleep(10 * time.Millisecond)

	// 验证已关闭
	if !dispose.IsClosed() {
		t.Error("Should be closed after calling Close()")
	}
}

func TestDisposeWithNilParentCtx(t *testing.T) {
	// 测试nil父上下文
	dispose := &utils.Dispose{}

	// 设置nil上下文
	dispose.SetCtx(nil, nil)

	// 验证未关闭
	if dispose.IsClosed() {
		t.Error("Should not be closed with nil parent context")
	}
}

func TestDisposeCloseWithoutSetCtx(t *testing.T) {
	// 测试未设置上下文时调用Close
	dispose := &utils.Dispose{}

	// 直接调用Close
	dispose.Close()

	// 验证已关闭（无论是否设置上下文）
	if !dispose.IsClosed() {
		t.Error("Should be closed after Close() even if no context is set")
	}
}

func TestDisposeIsClosedWithoutSetCtx(t *testing.T) {
	// 测试未设置上下文时的关闭状态
	dispose := &utils.Dispose{}

	// 初始状态应该未关闭
	if dispose.IsClosed() {
		t.Error("Should not be closed initially")
	}
}
