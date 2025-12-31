// Package httpproxy 隧道等待管理器测试
package httpproxy

import (
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Mock TunnelConnection
// ============================================================================

type mockTunnelConnection struct {
	tunnelID string
	closed   bool
}

func (m *mockTunnelConnection) GetTunnelID() string {
	return m.tunnelID
}

func (m *mockTunnelConnection) Close() error {
	m.closed = true
	return nil
}

func newMockTunnelConnection(tunnelID string) *mockTunnelConnection {
	return &mockTunnelConnection{
		tunnelID: tunnelID,
	}
}

// ============================================================================
// TunnelWaitManager 创建测试
// ============================================================================

func TestNewTunnelWaitManager(t *testing.T) {
	m := NewTunnelWaitManager()

	if m == nil {
		t.Fatal("NewTunnelWaitManager should not return nil")
	}

	if m.pendingTunnels == nil {
		t.Error("pendingTunnels should be initialized")
	}
}

// ============================================================================
// RegisterPendingTunnel 测试
// ============================================================================

func TestTunnelWaitManager_RegisterPendingTunnel(t *testing.T) {
	m := NewTunnelWaitManager()

	ch := m.RegisterPendingTunnel("tunnel-001")

	if ch == nil {
		t.Fatal("RegisterPendingTunnel should return a channel")
	}

	// 验证 channel 容量为 1
	conn := newMockTunnelConnection("tunnel-001")
	select {
	case ch <- conn:
		// 成功发送
	default:
		t.Error("channel should have capacity of 1")
	}

	// 验证隧道已注册
	m.mu.RLock()
	_, exists := m.pendingTunnels["tunnel-001"]
	m.mu.RUnlock()

	if !exists {
		t.Error("tunnel should be registered")
	}
}

func TestTunnelWaitManager_RegisterPendingTunnel_Multiple(t *testing.T) {
	m := NewTunnelWaitManager()

	ch1 := m.RegisterPendingTunnel("tunnel-001")
	ch2 := m.RegisterPendingTunnel("tunnel-002")
	ch3 := m.RegisterPendingTunnel("tunnel-003")

	if ch1 == nil || ch2 == nil || ch3 == nil {
		t.Error("all channels should be created")
	}

	// 验证各自独立
	if ch1 == ch2 || ch2 == ch3 || ch1 == ch3 {
		t.Error("each tunnel should have its own channel")
	}

	m.mu.RLock()
	count := len(m.pendingTunnels)
	m.mu.RUnlock()

	if count != 3 {
		t.Errorf("should have 3 pending tunnels, got %d", count)
	}
}

// ============================================================================
// UnregisterPendingTunnel 测试
// ============================================================================

func TestTunnelWaitManager_UnregisterPendingTunnel(t *testing.T) {
	m := NewTunnelWaitManager()

	// 先注册
	m.RegisterPendingTunnel("tunnel-unregister")

	// 注销
	m.UnregisterPendingTunnel("tunnel-unregister")

	// 验证已删除
	m.mu.RLock()
	_, exists := m.pendingTunnels["tunnel-unregister"]
	m.mu.RUnlock()

	if exists {
		t.Error("tunnel should be unregistered")
	}
}

func TestTunnelWaitManager_UnregisterPendingTunnel_NonExistent(t *testing.T) {
	m := NewTunnelWaitManager()

	// 注销不存在的隧道不应该 panic
	m.UnregisterPendingTunnel("non-existent")
}

// ============================================================================
// NotifyTunnelEstablished 测试
// ============================================================================

func TestTunnelWaitManager_NotifyTunnelEstablished(t *testing.T) {
	m := NewTunnelWaitManager()

	// 注册隧道
	ch := m.RegisterPendingTunnel("tunnel-notify")

	// 通知隧道已建立
	conn := newMockTunnelConnection("tunnel-notify")
	m.NotifyTunnelEstablished("tunnel-notify", conn)

	// 验证连接已发送到 channel
	select {
	case received := <-ch:
		if received.GetTunnelID() != "tunnel-notify" {
			t.Errorf("TunnelID should be 'tunnel-notify', got %s", received.GetTunnelID())
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("should receive connection on channel")
	}
}

func TestTunnelWaitManager_NotifyTunnelEstablished_NonExistent(t *testing.T) {
	m := NewTunnelWaitManager()

	// 通知不存在的隧道（不应该 panic）
	conn := newMockTunnelConnection("non-existent")
	m.NotifyTunnelEstablished("non-existent", conn)
}

func TestTunnelWaitManager_NotifyTunnelEstablished_ChannelFull(t *testing.T) {
	m := NewTunnelWaitManager()

	// 注册隧道
	ch := m.RegisterPendingTunnel("tunnel-full")

	// 填满 channel
	conn1 := newMockTunnelConnection("tunnel-full-1")
	ch <- conn1

	// 再次通知（channel 已满，应该不阻塞）
	conn2 := newMockTunnelConnection("tunnel-full-2")
	m.NotifyTunnelEstablished("tunnel-full", conn2) // 不应该阻塞

	// 验证第一个连接仍在 channel
	select {
	case received := <-ch:
		if received.GetTunnelID() != "tunnel-full-1" {
			t.Errorf("first connection should have TunnelID 'tunnel-full-1', got %s", received.GetTunnelID())
		}
	default:
		t.Error("should have first connection in channel")
	}
}

// ============================================================================
// 完整流程测试
// ============================================================================

func TestTunnelWaitManager_FullFlow(t *testing.T) {
	m := NewTunnelWaitManager()

	// 模拟等待隧道建立的流程
	done := make(chan bool)
	var receivedConn TunnelConnection

	go func() {
		ch := m.RegisterPendingTunnel("tunnel-flow")
		defer m.UnregisterPendingTunnel("tunnel-flow")

		select {
		case conn := <-ch:
			receivedConn = conn
			done <- true
		case <-time.After(1 * time.Second):
			done <- false
		}
	}()

	// 等待注册完成
	time.Sleep(10 * time.Millisecond)

	// 通知隧道建立
	conn := newMockTunnelConnection("tunnel-flow")
	m.NotifyTunnelEstablished("tunnel-flow", conn)

	// 等待结果
	success := <-done
	if !success {
		t.Error("should receive tunnel connection")
	}

	if receivedConn == nil {
		t.Error("received connection should not be nil")
	}

	if receivedConn.GetTunnelID() != "tunnel-flow" {
		t.Errorf("TunnelID should be 'tunnel-flow', got %s", receivedConn.GetTunnelID())
	}
}

func TestTunnelWaitManager_FullFlow_Timeout(t *testing.T) {
	m := NewTunnelWaitManager()

	// 模拟等待超时
	done := make(chan bool)

	go func() {
		ch := m.RegisterPendingTunnel("tunnel-timeout")
		defer m.UnregisterPendingTunnel("tunnel-timeout")

		select {
		case <-ch:
			done <- true
		case <-time.After(50 * time.Millisecond):
			done <- false
		}
	}()

	// 不通知隧道建立

	// 等待结果
	success := <-done
	if success {
		t.Error("should timeout")
	}

	// 验证隧道已注销
	m.mu.RLock()
	_, exists := m.pendingTunnels["tunnel-timeout"]
	m.mu.RUnlock()

	if exists {
		t.Error("tunnel should be unregistered after timeout")
	}
}

// ============================================================================
// 全局管理器测试
// ============================================================================

func TestGetGlobalTunnelWaitManager(t *testing.T) {
	m1 := GetGlobalTunnelWaitManager()
	m2 := GetGlobalTunnelWaitManager()

	if m1 == nil {
		t.Fatal("GetGlobalTunnelWaitManager should not return nil")
	}

	if m1 != m2 {
		t.Error("GetGlobalTunnelWaitManager should return the same instance")
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestTunnelWaitManager_ConcurrentRegisterUnregister(t *testing.T) {
	m := NewTunnelWaitManager()

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines * 2)

	// 并发注册
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			m.RegisterPendingTunnel("tunnel-concurrent-" + string(rune('A'+id%26)))
		}(i)
	}

	// 并发注销
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			m.UnregisterPendingTunnel("tunnel-concurrent-" + string(rune('A'+id%26)))
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}

func TestTunnelWaitManager_ConcurrentNotify(t *testing.T) {
	m := NewTunnelWaitManager()

	// 注册一些隧道
	for i := 0; i < 10; i++ {
		m.RegisterPendingTunnel("tunnel-notify-concurrent-" + string(rune('A'+i)))
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			tunnelID := "tunnel-notify-concurrent-" + string(rune('A'+id%10))
			conn := newMockTunnelConnection(tunnelID)
			m.NotifyTunnelEstablished(tunnelID, conn)
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}

func TestTunnelWaitManager_ConcurrentFullFlow(t *testing.T) {
	m := NewTunnelWaitManager()

	var wg sync.WaitGroup
	numGoroutines := 20

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			tunnelID := "tunnel-flow-concurrent-" + string(rune('A'+id))

			// 注册并等待
			ch := m.RegisterPendingTunnel(tunnelID)

			// 启动 goroutine 通知
			go func() {
				time.Sleep(10 * time.Millisecond)
				conn := newMockTunnelConnection(tunnelID)
				m.NotifyTunnelEstablished(tunnelID, conn)
			}()

			select {
			case <-ch:
				// 收到连接
			case <-time.After(100 * time.Millisecond):
				// 超时
			}

			m.UnregisterPendingTunnel(tunnelID)
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}

// ============================================================================
// TunnelConnection 接口测试
// ============================================================================

func TestTunnelConnection_Interface(t *testing.T) {
	// 验证 mockTunnelConnection 实现了 TunnelConnection 接口
	var _ TunnelConnection = (*mockTunnelConnection)(nil)

	conn := newMockTunnelConnection("test-tunnel")

	if conn.GetTunnelID() != "test-tunnel" {
		t.Errorf("GetTunnelID should return 'test-tunnel', got %s", conn.GetTunnelID())
	}

	if conn.closed {
		t.Error("connection should not be closed initially")
	}

	err := conn.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}

	if !conn.closed {
		t.Error("connection should be closed after Close()")
	}
}
