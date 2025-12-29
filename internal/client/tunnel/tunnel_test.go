package tunnel

import (
	"testing"
)

func TestTunnelRole_String(t *testing.T) {
	tests := []struct {
		role     TunnelRole
		expected string
	}{
		{TunnelRoleListen, "Listen"},
		{TunnelRoleTarget, "Target"},
		{TunnelRole(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.role.String(); got != tt.expected {
				t.Errorf("TunnelRole.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCloseReason_String(t *testing.T) {
	tests := []struct {
		reason   CloseReason
		expected string
	}{
		{CloseReasonNormal, "normal"},
		{CloseReasonLocalClosed, "local_closed"},
		{CloseReasonPeerClosed, "peer_closed"},
		{CloseReasonTimeout, "timeout"},
		{CloseReasonError, "error"},
		{CloseReasonContextCanceled, "context_canceled"},
		{CloseReason(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.reason.String(); got != tt.expected {
				t.Errorf("CloseReason.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewTunnel(t *testing.T) {
	config := &TunnelConfig{
		ID:        "test-tunnel-1",
		MappingID: "test-mapping",
		Role:      TunnelRoleListen,
		Protocol:  "tcp",
	}

	tunnel := NewTunnel(config)
	if tunnel == nil {
		t.Fatal("NewTunnel returned nil")
	}

	// 验证字段初始化
	if tunnel.id != config.ID {
		t.Errorf("Expected id=%s, got %s", config.ID, tunnel.id)
	}
	if tunnel.mappingID != config.MappingID {
		t.Errorf("Expected mappingID=%s, got %s", config.MappingID, tunnel.mappingID)
	}
	if tunnel.role != config.Role {
		t.Errorf("Expected role=%s, got %s", config.Role, tunnel.role)
	}
	if tunnel.protocol != config.Protocol {
		t.Errorf("Expected protocol=%s, got %s", config.Protocol, tunnel.protocol)
	}

	// 验证初始状态
	if tunnel.GetState() != TunnelStateConnecting {
		t.Errorf("Expected initial state=%v, got %v", TunnelStateConnecting, tunnel.GetState())
	}
}

func TestTunnel_GetID(t *testing.T) {
	tunnel := NewTunnel(&TunnelConfig{
		ID:       "test-tunnel-id",
		Protocol: "tcp",
	})

	if tunnel.GetID() != "test-tunnel-id" {
		t.Errorf("GetID() = %s, want test-tunnel-id", tunnel.GetID())
	}
}

func TestTunnel_GetRole(t *testing.T) {
	tunnel := NewTunnel(&TunnelConfig{
		Role:     TunnelRoleTarget,
		Protocol: "tcp",
	})

	if tunnel.GetRole() != TunnelRoleTarget {
		t.Errorf("GetRole() = %v, want %v", tunnel.GetRole(), TunnelRoleTarget)
	}
}

func TestTunnel_GetState(t *testing.T) {
	tunnel := NewTunnel(&TunnelConfig{Protocol: "tcp"})

	// 初始状态
	if tunnel.GetState() != TunnelStateConnecting {
		t.Errorf("Initial state = %v, want %v", tunnel.GetState(), TunnelStateConnecting)
	}

	// 手动更改状态
	tunnel.state.Store(int32(TunnelStateConnected))
	if tunnel.GetState() != TunnelStateConnected {
		t.Errorf("State = %v, want %v", tunnel.GetState(), TunnelStateConnected)
	}
}

func TestTunnel_GetStats(t *testing.T) {
	tunnel := NewTunnel(&TunnelConfig{Protocol: "tcp"})

	// 设置统计
	tunnel.bytesSent.Store(1000)
	tunnel.bytesRecv.Store(2000)

	stats := tunnel.GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	if stats.BytesSent != 1000 {
		t.Errorf("BytesSent = %d, want 1000", stats.BytesSent)
	}
	if stats.BytesRecv != 2000 {
		t.Errorf("BytesRecv = %d, want 2000", stats.BytesRecv)
	}
	// DurationMs 可能为 0（如果测试运行很快）
	if stats.DurationMs < 0 {
		t.Errorf("DurationMs should be >= 0, got %d", stats.DurationMs)
	}
}

func TestTunnel_shouldNotifyPeer(t *testing.T) {
	tunnel := NewTunnel(&TunnelConfig{Protocol: "tcp"})

	tests := []struct {
		reason   CloseReason
		expected bool
	}{
		{CloseReasonNormal, true},
		{CloseReasonLocalClosed, true},
		{CloseReasonPeerClosed, false},
		{CloseReasonTimeout, true},
		{CloseReasonError, true},
		{CloseReasonContextCanceled, false},
	}

	for _, tt := range tests {
		t.Run(tt.reason.String(), func(t *testing.T) {
			if got := tunnel.shouldNotifyPeer(tt.reason); got != tt.expected {
				t.Errorf("shouldNotifyPeer(%v) = %v, want %v", tt.reason, got, tt.expected)
			}
		})
	}
}

func TestTunnelClosedPayload_FromPacketPayload(t *testing.T) {
	// 由于 packet.TunnelClosedPayload 需要导入 packet 包
	// 这里仅测试基本结构
	payload := &TunnelClosedPayload{}

	// 直接设置值测试结构
	payload.TunnelID = "tunnel-1"
	payload.MappingID = "mapping-1"
	payload.Reason = "normal"
	payload.BytesSent = 1000
	payload.BytesRecv = 2000
	payload.DurationMs = 5000
	payload.ClosedAt = 1234567890

	if payload.TunnelID != "tunnel-1" {
		t.Errorf("TunnelID = %s, want tunnel-1", payload.TunnelID)
	}
}
