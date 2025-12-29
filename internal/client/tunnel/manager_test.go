package tunnel

import (
	"context"
	"testing"
)

func TestNewTunnelManager(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)

	if manager == nil {
		t.Fatal("NewTunnelManager returned nil")
	}
	if manager.role != TunnelRoleListen {
		t.Errorf("Expected role=%v, got %v", TunnelRoleListen, manager.role)
	}

	manager.Close()
}

func TestTunnelManager_RegisterTunnel(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	// 创建隧道
	tunnel := NewTunnel(&TunnelConfig{
		ID:        "test-tunnel-1",
		Protocol:  "tcp",
	})

	// 注册
	err := manager.RegisterTunnel(tunnel)
	if err != nil {
		t.Fatalf("RegisterTunnel failed: %v", err)
	}

	// 验证注册成功
	if manager.CountTunnels() != 1 {
		t.Errorf("Expected 1 tunnel, got %d", manager.CountTunnels())
	}

	// 重复注册应该失败
	err = manager.RegisterTunnel(tunnel)
	if err == nil {
		t.Error("Duplicate registration should fail")
	}
}

func TestTunnelManager_RegisterNilTunnel(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	err := manager.RegisterTunnel(nil)
	if err == nil {
		t.Error("RegisterTunnel(nil) should return error")
	}
}

func TestTunnelManager_UnregisterTunnel(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	// 创建并注册隧道
	tunnel := NewTunnel(&TunnelConfig{
		ID:       "test-tunnel-1",
		Protocol: "tcp",
	})
	manager.RegisterTunnel(tunnel)

	// 注销
	deleted := manager.UnregisterTunnel("test-tunnel-1")
	if !deleted {
		t.Error("UnregisterTunnel should return true for existing tunnel")
	}

	// 验证注销成功
	if manager.CountTunnels() != 0 {
		t.Errorf("Expected 0 tunnels, got %d", manager.CountTunnels())
	}

	// 再次注销应该返回 false
	deleted = manager.UnregisterTunnel("test-tunnel-1")
	if deleted {
		t.Error("UnregisterTunnel should return false for non-existing tunnel")
	}
}

func TestTunnelManager_GetTunnel(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	// 获取不存在的隧道
	tunnel := manager.GetTunnel("non-existing")
	if tunnel != nil {
		t.Error("GetTunnel should return nil for non-existing tunnel")
	}

	// 创建并注册
	newTunnel := NewTunnel(&TunnelConfig{
		ID:       "test-tunnel-1",
		Protocol: "tcp",
	})
	manager.RegisterTunnel(newTunnel)

	// 获取存在的隧道
	tunnel = manager.GetTunnel("test-tunnel-1")
	if tunnel == nil {
		t.Fatal("GetTunnel should return tunnel for existing ID")
	}
	if tunnel.id != "test-tunnel-1" {
		t.Errorf("Expected ID=test-tunnel-1, got %s", tunnel.id)
	}
}

func TestTunnelManager_ListTunnels(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	// 空列表
	tunnels := manager.ListTunnels()
	if len(tunnels) != 0 {
		t.Errorf("Expected 0 tunnels, got %d", len(tunnels))
	}

	// 添加多个隧道
	for i := 1; i <= 3; i++ {
		tunnel := NewTunnel(&TunnelConfig{
			ID:       "test-tunnel-" + string(rune('0'+i)),
			Protocol: "tcp",
		})
		manager.RegisterTunnel(tunnel)
	}

	tunnels = manager.ListTunnels()
	if len(tunnels) != 3 {
		t.Errorf("Expected 3 tunnels, got %d", len(tunnels))
	}
}

func TestTunnelManager_CountTunnels(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	if manager.CountTunnels() != 0 {
		t.Errorf("Initial count should be 0, got %d", manager.CountTunnels())
	}

	// 添加
	for i := 1; i <= 5; i++ {
		tunnel := NewTunnel(&TunnelConfig{
			ID:       "test-tunnel-" + string(rune('0'+i)),
			Protocol: "tcp",
		})
		manager.RegisterTunnel(tunnel)
	}

	if manager.CountTunnels() != 5 {
		t.Errorf("Expected 5 tunnels, got %d", manager.CountTunnels())
	}
}

func TestTunnelManager_CloseTunnel_NotFound(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	err := manager.CloseTunnel("non-existing", CloseReasonNormal)
	if err == nil {
		t.Error("CloseTunnel should return error for non-existing tunnel")
	}
}

func TestTunnelManager_NotificationHandlers(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	// 这些方法应该不 panic
	manager.OnSystemMessage("title", "message", "info")
	manager.OnQuotaWarning("bandwidth", 80.5, "warning message")
	manager.OnTunnelOpened("tunnel-1", "mapping-1", 12345)
	manager.OnTunnelError("tunnel-1", "mapping-1", "ERR001", "error message", true)
	manager.OnTunnelError("tunnel-1", "mapping-1", "ERR002", "fatal error", false)
	manager.OnCustomNotification(12345, "action", nil, "raw")
	// Note: OnGenericNotification(nil) panics - this is expected behavior for nil input
}

func TestTunnelManager_OnTunnelClosed(t *testing.T) {
	ctx := context.Background()
	manager := NewTunnelManager(ctx, TunnelRoleListen)
	defer manager.Close()

	// 创建并注册隧道
	tunnel := NewTunnel(&TunnelConfig{
		ID:        "test-tunnel-1",
		MappingID: "mapping-1",
		Protocol:  "tcp",
	})
	tunnel.state.Store(int32(TunnelStateConnected))
	manager.RegisterTunnel(tunnel)

	// 模拟 context（需要设置才能关闭）
	tunnel.SetCtx(ctx, tunnel.onClose)

	// 调用 OnTunnelClosed
	manager.OnTunnelClosed("test-tunnel-1", "mapping-1", "normal", 1000, 2000, 5000)

	// 验证隧道被关闭（状态变为 Closing 或 Closed）
	state := tunnel.GetState()
	if state != TunnelStateClosing && state != TunnelStateClosed {
		t.Errorf("Expected tunnel to be closing/closed, got %v", state)
	}
}
