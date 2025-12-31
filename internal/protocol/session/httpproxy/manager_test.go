// Package httpproxy HTTP 代理管理器测试
package httpproxy

import (
	"context"
	"sync"
	"testing"
	"time"

	"tunnox-core/internal/protocol/httptypes"
)

// ============================================================================
// Manager 创建测试
// ============================================================================

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager should not return nil")
	}

	if m.pendingRequests == nil {
		t.Error("pendingRequests should be initialized")
	}

	if m.defaultTimeout != 30*time.Second {
		t.Errorf("defaultTimeout should be 30s, got %v", m.defaultTimeout)
	}
}

// ============================================================================
// RegisterPendingRequest 测试
// ============================================================================

func TestManager_RegisterPendingRequest(t *testing.T) {
	m := NewManager()

	ch := m.RegisterPendingRequest("req-001")

	if ch == nil {
		t.Fatal("RegisterPendingRequest should return a channel")
	}

	// 验证 channel 容量为 1
	select {
	case ch <- &httptypes.HTTPProxyResponse{RequestID: "req-001"}:
		// 成功发送
	default:
		t.Error("channel should have capacity of 1")
	}

	// 验证请求已注册
	m.pendingMu.RLock()
	_, exists := m.pendingRequests["req-001"]
	m.pendingMu.RUnlock()

	if !exists {
		t.Error("request should be registered")
	}
}

func TestManager_RegisterPendingRequest_Multiple(t *testing.T) {
	m := NewManager()

	ch1 := m.RegisterPendingRequest("req-001")
	ch2 := m.RegisterPendingRequest("req-002")
	ch3 := m.RegisterPendingRequest("req-003")

	if ch1 == nil || ch2 == nil || ch3 == nil {
		t.Error("all channels should be created")
	}

	// 验证各自独立
	if ch1 == ch2 || ch2 == ch3 || ch1 == ch3 {
		t.Error("each request should have its own channel")
	}

	m.pendingMu.RLock()
	count := len(m.pendingRequests)
	m.pendingMu.RUnlock()

	if count != 3 {
		t.Errorf("should have 3 pending requests, got %d", count)
	}
}

// ============================================================================
// UnregisterPendingRequest 测试
// ============================================================================

func TestManager_UnregisterPendingRequest(t *testing.T) {
	m := NewManager()

	// 先注册
	m.RegisterPendingRequest("req-unregister")

	// 注销
	m.UnregisterPendingRequest("req-unregister")

	// 验证已删除
	m.pendingMu.RLock()
	_, exists := m.pendingRequests["req-unregister"]
	m.pendingMu.RUnlock()

	if exists {
		t.Error("request should be unregistered")
	}
}

func TestManager_UnregisterPendingRequest_NonExistent(t *testing.T) {
	m := NewManager()

	// 注销不存在的请求不应该 panic
	m.UnregisterPendingRequest("non-existent")
}

// ============================================================================
// HandleResponse 测试
// ============================================================================

func TestManager_HandleResponse(t *testing.T) {
	m := NewManager()

	// 注册请求
	ch := m.RegisterPendingRequest("req-response")

	// 处理响应
	resp := &httptypes.HTTPProxyResponse{
		RequestID:  "req-response",
		StatusCode: 200,
		Body:       []byte("OK"),
	}
	m.HandleResponse(resp)

	// 验证响应已发送到 channel
	select {
	case received := <-ch:
		if received.RequestID != "req-response" {
			t.Errorf("RequestID should be 'req-response', got %s", received.RequestID)
		}
		if received.StatusCode != 200 {
			t.Errorf("StatusCode should be 200, got %d", received.StatusCode)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("should receive response on channel")
	}
}

func TestManager_HandleResponse_NonExistent(t *testing.T) {
	m := NewManager()

	// 处理不存在的请求的响应（不应该 panic）
	resp := &httptypes.HTTPProxyResponse{
		RequestID:  "non-existent",
		StatusCode: 200,
	}
	m.HandleResponse(resp)
}

func TestManager_HandleResponse_ChannelFull(t *testing.T) {
	m := NewManager()

	// 注册请求
	ch := m.RegisterPendingRequest("req-full")

	// 填满 channel
	resp1 := &httptypes.HTTPProxyResponse{RequestID: "req-full", StatusCode: 200}
	ch <- resp1

	// 再次发送（channel 已满，应该不阻塞）
	resp2 := &httptypes.HTTPProxyResponse{RequestID: "req-full", StatusCode: 201}
	m.HandleResponse(resp2) // 不应该阻塞

	// 验证第一个响应仍在 channel
	select {
	case received := <-ch:
		if received.StatusCode != 200 {
			t.Errorf("first response should have StatusCode 200, got %d", received.StatusCode)
		}
	default:
		t.Error("should have first response in channel")
	}
}

// ============================================================================
// WaitForResponse 测试
// ============================================================================

func TestManager_WaitForResponse(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	// 启动 goroutine 稍后发送响应
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.HandleResponse(&httptypes.HTTPProxyResponse{
			RequestID:  "req-wait",
			StatusCode: 200,
			Body:       []byte("success"),
		})
	}()

	// 等待响应
	resp, err := m.WaitForResponse(ctx, "req-wait", 1*time.Second)
	if err != nil {
		t.Errorf("WaitForResponse should not return error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode should be 200, got %d", resp.StatusCode)
	}

	// 验证请求已注销
	m.pendingMu.RLock()
	_, exists := m.pendingRequests["req-wait"]
	m.pendingMu.RUnlock()

	if exists {
		t.Error("request should be unregistered after WaitForResponse")
	}
}

func TestManager_WaitForResponse_Timeout(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	// 等待超时
	_, err := m.WaitForResponse(ctx, "req-timeout", 50*time.Millisecond)
	if err == nil {
		t.Error("WaitForResponse should return timeout error")
	}

	// 验证请求已注销
	m.pendingMu.RLock()
	_, exists := m.pendingRequests["req-timeout"]
	m.pendingMu.RUnlock()

	if exists {
		t.Error("request should be unregistered after timeout")
	}
}

func TestManager_WaitForResponse_ContextCancelled(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())

	// 启动 goroutine 取消 context
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// 等待响应
	_, err := m.WaitForResponse(ctx, "req-cancel", 1*time.Second)
	if err == nil {
		t.Error("WaitForResponse should return error when context is cancelled")
	}
}

func TestManager_WaitForResponse_DefaultTimeout(t *testing.T) {
	m := NewManager()
	m.defaultTimeout = 50 * time.Millisecond // 缩短默认超时用于测试

	ctx := context.Background()

	// 使用 0 超时（应该使用默认值）
	_, err := m.WaitForResponse(ctx, "req-default-timeout", 0)
	if err == nil {
		t.Error("WaitForResponse should return timeout error")
	}
}

// ============================================================================
// 全局管理器测试
// ============================================================================

func TestGetGlobalManager(t *testing.T) {
	m1 := GetGlobalManager()
	m2 := GetGlobalManager()

	if m1 == nil {
		t.Fatal("GetGlobalManager should not return nil")
	}

	if m1 != m2 {
		t.Error("GetGlobalManager should return the same instance")
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestManager_ConcurrentRegisterUnregister(t *testing.T) {
	m := NewManager()

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines * 2)

	// 并发注册
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			m.RegisterPendingRequest("req-concurrent-" + string(rune('A'+id%26)))
		}(i)
	}

	// 并发注销
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			m.UnregisterPendingRequest("req-concurrent-" + string(rune('A'+id%26)))
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}

func TestManager_ConcurrentHandleResponse(t *testing.T) {
	m := NewManager()

	// 注册一些请求
	for i := 0; i < 10; i++ {
		m.RegisterPendingRequest("req-concurrent-resp-" + string(rune('A'+i)))
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			resp := &httptypes.HTTPProxyResponse{
				RequestID:  "req-concurrent-resp-" + string(rune('A'+id%10)),
				StatusCode: 200,
			}
			m.HandleResponse(resp)
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}

func TestManager_ConcurrentWaitForResponse(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 20

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			reqID := "req-wait-concurrent-" + string(rune('A'+id))

			// 启动 goroutine 发送响应
			go func() {
				time.Sleep(10 * time.Millisecond)
				m.HandleResponse(&httptypes.HTTPProxyResponse{
					RequestID:  reqID,
					StatusCode: 200,
				})
			}()

			m.WaitForResponse(ctx, reqID, 100*time.Millisecond)
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}
