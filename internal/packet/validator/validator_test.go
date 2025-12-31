// Package validator 提供数据包验证器的测试
package validator

import (
	"testing"

	"tunnox-core/internal/packet"
)

// ═══════════════════════════════════════════════════════════════════
// NewDefaultPacketValidator 测试
// ═══════════════════════════════════════════════════════════════════

func TestNewDefaultPacketValidator(t *testing.T) {
	t.Parallel()

	validator := NewDefaultPacketValidator()
	if validator == nil {
		t.Error("NewDefaultPacketValidator() returned nil")
	}
}

// ═══════════════════════════════════════════════════════════════════
// ValidateTransferPacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketValidator_ValidateTransferPacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		transferPacket *packet.TransferPacket
		wantErr        bool
	}{
		{
			name:           "nil packet",
			transferPacket: nil,
			wantErr:        true,
		},
		{
			name: "valid json command packet with command",
			transferPacket: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-123",
					Token:       "token",
					SenderId:    "sender",
					ReceiverId:  "receiver",
				},
			},
			wantErr: false,
		},
		{
			name: "valid heartbeat packet without command",
			transferPacket: &packet.TransferPacket{
				PacketType:    packet.Heartbeat,
				CommandPacket: nil,
			},
			wantErr: false,
		},
		{
			name: "invalid packet type",
			transferPacket: &packet.TransferPacket{
				PacketType:    packet.Type(0xFF),
				CommandPacket: nil,
			},
			wantErr: true,
		},
		{
			name: "valid packet type but invalid command",
			transferPacket: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "", // 空命令ID
					SenderId:    "sender",
					ReceiverId:  "receiver",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator := NewDefaultPacketValidator()

			err := validator.ValidateTransferPacket(tt.transferPacket)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransferPacket() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// ValidateCommandPacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketValidator_ValidateCommandPacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		commandPacket *packet.CommandPacket
		wantErr       bool
		errContains   string
	}{
		{
			name:          "nil packet",
			commandPacket: nil,
			wantErr:       true,
			errContains:   "nil",
		},
		{
			name: "valid command packet",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-123",
				Token:       "token",
				SenderId:    "sender",
				ReceiverId:  "receiver",
				CommandBody: `{"action":"connect"}`,
			},
			wantErr: false,
		},
		{
			name: "empty command ID",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "",
				Token:       "token",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
			wantErr:     true,
			errContains: "command ID",
		},
		{
			name: "empty sender ID",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-123",
				Token:       "token",
				SenderId:    "",
				ReceiverId:  "receiver",
			},
			wantErr:     true,
			errContains: "sender ID",
		},
		{
			name: "empty receiver ID",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-123",
				Token:       "token",
				SenderId:    "sender",
				ReceiverId:  "",
			},
			wantErr:     true,
			errContains: "receiver ID",
		},
		{
			name: "invalid command type",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.CommandType(255),
				CommandId:   "cmd-123",
				Token:       "token",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
			wantErr:     true,
			errContains: "command type",
		},
		{
			name: "empty token is allowed",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-123",
				Token:       "", // 空token是允许的
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
			wantErr: false,
		},
		{
			name: "empty body is allowed",
			commandPacket: &packet.CommandPacket{
				CommandType: packet.HeartbeatCmd,
				CommandId:   "cmd-123",
				SenderId:    "sender",
				ReceiverId:  "receiver",
				CommandBody: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator := NewDefaultPacketValidator()

			err := validator.ValidateCommandPacket(tt.commandPacket)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommandPacket() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// ValidatePacketType 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketValidator_ValidatePacketType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		packetType packet.Type
		wantErr    bool
	}{
		{"json command", packet.JsonCommand, false},
		{"compressed", packet.Compressed, false},
		{"encrypted", packet.Encrypted, false},
		{"heartbeat", packet.Heartbeat, false},
		{"handshake", packet.Handshake, false},
		{"tunnel open", packet.TunnelOpen, false},
		{"json command with compressed flag", packet.JsonCommand | packet.Compressed, false},
		{"json command with encrypted flag", packet.JsonCommand | packet.Encrypted, false},
		{"json command with both flags", packet.JsonCommand | packet.Compressed | packet.Encrypted, false},
		{"unknown type 0x00", packet.Type(0x00), true},
		{"unknown type 0xFF", packet.Type(0xFF), true},
		{"unknown type 0x05", packet.Type(0x05), true}, // 未定义的基础类型
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator := NewDefaultPacketValidator()

			err := validator.ValidatePacketType(tt.packetType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePacketType(%v) error = %v, wantErr %v", tt.packetType, err, tt.wantErr)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// ValidateCommandType 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketValidator_ValidateCommandType(t *testing.T) {
	t.Parallel()

	// 测试所有有效的命令类型
	validCommands := []struct {
		name string
		cmd  packet.CommandType
	}{
		// 连接管理类命令
		{"Connect", packet.Connect},
		{"Disconnect", packet.Disconnect},
		{"Reconnect", packet.Reconnect},
		{"HeartbeatCmd", packet.HeartbeatCmd},

		// 端口映射类命令
		{"TcpMapCreate", packet.TcpMapCreate},
		{"TcpMapDelete", packet.TcpMapDelete},
		{"TcpMapUpdate", packet.TcpMapUpdate},
		{"TcpMapList", packet.TcpMapList},
		{"TcpMapStatus", packet.TcpMapStatus},

		{"HttpMapCreate", packet.HttpMapCreate},
		{"HttpMapDelete", packet.HttpMapDelete},
		{"HttpMapUpdate", packet.HttpMapUpdate},
		{"HttpMapList", packet.HttpMapList},
		{"HttpMapStatus", packet.HttpMapStatus},

		{"SocksMapCreate", packet.SocksMapCreate},
		{"SocksMapDelete", packet.SocksMapDelete},
		{"SocksMapUpdate", packet.SocksMapUpdate},
		{"SocksMapList", packet.SocksMapList},
		{"SocksMapStatus", packet.SocksMapStatus},

		// 数据传输类命令
		{"DataTransferStart", packet.DataTransferStart},
		{"DataTransferStop", packet.DataTransferStop},
		{"DataTransferStatus", packet.DataTransferStatus},
		{"ProxyForward", packet.ProxyForward},
		{"DataTransferOut", packet.DataTransferOut},

		// 系统管理类命令
		{"ConfigGet", packet.ConfigGet},
		{"ConfigSet", packet.ConfigSet},
		{"StatsGet", packet.StatsGet},
		{"LogGet", packet.LogGet},
		{"HealthCheck", packet.HealthCheck},

		// RPC类命令
		{"RpcInvoke", packet.RpcInvoke},
		{"RpcRegister", packet.RpcRegister},
		{"RpcUnregister", packet.RpcUnregister},
		{"RpcList", packet.RpcList},
	}

	for _, tt := range validCommands {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator := NewDefaultPacketValidator()

			err := validator.ValidateCommandType(tt.cmd)
			if err != nil {
				t.Errorf("ValidateCommandType(%v) = %v, want nil", tt.cmd, err)
			}
		})
	}

	// 测试无效的命令类型
	invalidCommands := []struct {
		name string
		cmd  packet.CommandType
	}{
		{"invalid 0", packet.CommandType(0)},
		{"invalid 1", packet.CommandType(1)},
		{"invalid 255", packet.CommandType(255)},
		{"invalid 128", packet.CommandType(128)},
	}

	for _, tt := range invalidCommands {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator := NewDefaultPacketValidator()

			err := validator.ValidateCommandType(tt.cmd)
			if err == nil {
				t.Errorf("ValidateCommandType(%v) = nil, want error", tt.cmd)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// 接口兼容性测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketValidator_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ PacketValidator = (*DefaultPacketValidator)(nil)
}

// ═══════════════════════════════════════════════════════════════════
// 边界条件测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketValidator_ValidateTransferPacket_AllValidTypes(t *testing.T) {
	t.Parallel()

	validTypes := []struct {
		name string
		pt   packet.Type
	}{
		{"JsonCommand", packet.JsonCommand},
		{"Compressed", packet.Compressed},
		{"Encrypted", packet.Encrypted},
		{"Heartbeat", packet.Heartbeat},
		{"Handshake", packet.Handshake},
		{"TunnelOpen", packet.TunnelOpen},
		{"TunnelData", packet.TunnelData},
		{"JsonCommand with compressed", packet.JsonCommand | packet.Compressed},
	}

	for _, tc := range validTypes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			validator := NewDefaultPacketValidator()

			tp := &packet.TransferPacket{
				PacketType:    tc.pt,
				CommandPacket: nil,
			}

			err := validator.ValidateTransferPacket(tp)
			if err != nil {
				t.Errorf("ValidateTransferPacket() with type %v error = %v", tc.pt, err)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// 基准测试
// ═══════════════════════════════════════════════════════════════════

func BenchmarkDefaultPacketValidator_ValidateTransferPacket(b *testing.B) {
	validator := NewDefaultPacketValidator()
	tp := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.TcpMapCreate,
			CommandId:   "bench-cmd",
			Token:       "bench-token",
			SenderId:    "bench-sender",
			ReceiverId:  "bench-receiver",
			CommandBody: `{"port":8080}`,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateTransferPacket(tp)
	}
}

func BenchmarkDefaultPacketValidator_ValidateCommandPacket(b *testing.B) {
	validator := NewDefaultPacketValidator()
	cmd := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		CommandId:   "bench-cmd",
		Token:       "bench-token",
		SenderId:    "bench-sender",
		ReceiverId:  "bench-receiver",
		CommandBody: `{"port":8080}`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateCommandPacket(cmd)
	}
}

func BenchmarkDefaultPacketValidator_ValidatePacketType(b *testing.B) {
	validator := NewDefaultPacketValidator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidatePacketType(packet.JsonCommand)
	}
}

func BenchmarkDefaultPacketValidator_ValidateCommandType(b *testing.B) {
	validator := NewDefaultPacketValidator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateCommandType(packet.TcpMapCreate)
	}
}
