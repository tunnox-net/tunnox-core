package dispose

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewDispose 测试 NewDispose 工厂函数
func TestNewDispose(t *testing.T) {
	ctx := context.Background()
	called := false

	d := NewDispose(ctx, func() error {
		called = true
		return nil
	})

	if d == nil {
		t.Fatal("NewDispose should not return nil")
	}

	if d.Ctx() == nil {
		t.Error("Context should be set")
	}

	if d.IsClosed() {
		t.Error("Should not be closed initially")
	}

	// 关闭并验证回调被调用
	result := d.Close()
	if result.HasErrors() {
		t.Errorf("Close should not have errors: %v", result.Error())
	}

	if !called {
		t.Error("onClose callback should be called")
	}

	if !d.IsClosed() {
		t.Error("Should be closed after Close()")
	}
}

// TestNewDisposeWithNoOp 测试 NewDisposeWithNoOp
func TestNewDisposeWithNoOp(t *testing.T) {
	ctx := context.Background()
	d := NewDisposeWithNoOp(ctx)

	if d == nil {
		t.Fatal("NewDisposeWithNoOp should not return nil")
	}

	if d.Ctx() == nil {
		t.Error("Context should be set")
	}

	result := d.Close()
	if result.HasErrors() {
		t.Errorf("Close should not have errors: %v", result.Error())
	}
}

// TestDisposeSetCtxOnce 测试 SetCtx 只能调用一次
func TestDisposeSetCtxOnce(t *testing.T) {
	d := &Dispose{}
	ctx := context.Background()

	// 第一次调用应该成功
	d.SetCtx(ctx, nil)
	if d.Ctx() == nil {
		t.Error("Context should be set after first SetCtx")
	}

	// 第二次调用应该被忽略（不会 panic）
	d.SetCtx(ctx, nil)
}

// TestDisposeAddCleanHandler 测试添加清理处理器
func TestDisposeAddCleanHandler(t *testing.T) {
	ctx := context.Background()
	order := make([]int, 0)

	d := NewDispose(ctx, func() error {
		order = append(order, 1)
		return nil
	})

	d.AddCleanHandler(func() error {
		order = append(order, 2)
		return nil
	})

	d.AddCleanHandler(func() error {
		order = append(order, 3)
		return nil
	})

	d.Close()

	// 验证所有处理器都被调用
	if len(order) != 3 {
		t.Errorf("Expected 3 handlers called, got %d", len(order))
	}
}

// TestDisposeCleanHandlerError 测试清理处理器返回错误
func TestDisposeCleanHandlerError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("cleanup error")

	d := NewDispose(ctx, func() error {
		return expectedErr
	})

	result := d.Close()

	if !result.HasErrors() {
		t.Error("Should have errors")
	}

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}

	if result.Errors[0].Err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, result.Errors[0].Err)
	}
}

// TestDisposeContextCancellation 测试上下文取消触发清理
func TestDisposeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan bool, 1)

	d := NewDispose(ctx, func() error {
		called <- true
		return nil
	})

	// 取消上下文
	cancel()

	// 等待清理被调用
	select {
	case <-called:
		// 成功
	case <-time.After(time.Second):
		t.Error("onClose should be called when context is cancelled")
	}

	// 验证已关闭
	if !d.IsClosed() {
		t.Error("Should be closed after context cancellation")
	}
}

// TestDisposeCloseIdempotent 测试 Close 是幂等的
func TestDisposeCloseIdempotent(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	d := NewDispose(ctx, func() error {
		callCount++
		return nil
	})

	// 多次调用 Close
	d.Close()
	d.Close()
	d.Close()

	// 清理处理器应该只被调用一次
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

// TestDisposeCloseWithError 测试 CloseWithError 方法
func TestDisposeCloseWithError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("cleanup error")

	d := NewDispose(ctx, func() error {
		return expectedErr
	})

	err := d.CloseWithError()
	if err == nil {
		t.Error("CloseWithError should return error")
	}

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

// TestServiceBase 测试 ServiceBase
func TestServiceBase(t *testing.T) {
	ctx := context.Background()
	service := NewService("TestService", ctx)

	if service == nil {
		t.Fatal("NewService should not return nil")
	}

	if service.GetName() != "TestService" {
		t.Errorf("Expected name 'TestService', got '%s'", service.GetName())
	}

	if service.Ctx() == nil {
		t.Error("Context should be set")
	}

	err := service.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

// TestManagerBase 测试 ManagerBase
func TestManagerBase(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("TestManager", ctx)

	if manager == nil {
		t.Fatal("NewManager should not return nil")
	}

	if manager.GetName() != "TestManager" {
		t.Errorf("Expected name 'TestManager', got '%s'", manager.GetName())
	}

	if manager.Ctx() == nil {
		t.Error("Context should be set")
	}

	err := manager.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}
