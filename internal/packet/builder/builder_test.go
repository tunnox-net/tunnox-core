// Package builder 提供数据包构建器的测试
package builder

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"tunnox-core/internal/packet"
)

// ═══════════════════════════════════════════════════════════════════
// NewDefaultPacketBuilder 测试
// ═══════════════════════════════════════════════════════════════════

func TestNewDefaultPacketBuilder(t *testing.T) {
	t.Parallel()

	builder := NewDefaultPacketBuilder()
	if builder == nil {
		t.Error("NewDefaultPacketBuilder() returned nil")
	}
}

// ═══════════════════════════════════════════════════════════════════
// BuildPacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketBuilder_BuildPacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		transferPacket *packet.TransferPacket
		wantErr        bool
	}{
		{
			name: "valid packet with command",
			transferPacket: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-123",
					Token:       "token-abc",
					SenderId:    "sender-1",
					ReceiverId:  "receiver-1",
					CommandBody: `{"action":"connect"}`,
				},
			},
			wantErr: false,
		},
		{
			name: "packet without command",
			transferPacket: &packet.TransferPacket{
				PacketType:    packet.Heartbeat,
				CommandPacket: nil,
			},
			wantErr: false,
		},
		{
			name: "packet with different type",
			transferPacket: &packet.TransferPacket{
				PacketType: packet.Compressed,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.TcpMapCreate,
					CommandId:   "cmd-456",
					Token:       "token-def",
					SenderId:    "sender-2",
					ReceiverId:  "receiver-2",
					CommandBody: `{"port":8080}`,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			builder := NewDefaultPacketBuilder()
			var buf bytes.Buffer

			err := builder.BuildPacket(&buf, tt.transferPacket)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// 验证输出格式
				data := buf.Bytes()
				if len(data) < 5 {
					t.Errorf("BuildPacket() output too short: %d bytes", len(data))
					return
				}

				// 验证数据包类型
				if packet.Type(data[0]) != tt.transferPacket.PacketType {
					t.Errorf("PacketType = %v, want %v", packet.Type(data[0]), tt.transferPacket.PacketType)
				}

				// 验证长度字段
				length := binary.BigEndian.Uint32(data[1:5])
				if int(length) != len(data)-5 {
					t.Errorf("Length = %d, want %d", length, len(data)-5)
				}
			}
		})
	}
}

func TestDefaultPacketBuilder_BuildPacket_WriteError(t *testing.T) {
	t.Parallel()

	builder := NewDefaultPacketBuilder()
	transferPacket := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.Connect,
			CommandId:   "cmd-123",
			SenderId:    "sender",
			ReceiverId:  "receiver",
		},
	}

	// 使用一个会失败的 writer
	errWriter := &errorWriter{failAfter: 0}
	err := builder.BuildPacket(errWriter, transferPacket)
	if err == nil {
		t.Error("BuildPacket() should fail with error writer")
	}
}

// errorWriter 是一个在写入一定字节后返回错误的 writer
type errorWriter struct {
	failAfter int
	written   int
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	if w.written >= w.failAfter {
		return 0, errors.New("write error")
	}
	w.written += len(p)
	return len(p), nil
}

// ═══════════════════════════════════════════════════════════════════
// BuildCommandPacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketBuilder_BuildCommandPacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		commandType packet.CommandType
		commandID   string
		token       string
		senderID    string
		receiverID  string
		commandBody string
	}{
		{
			name:        "connect command",
			commandType: packet.Connect,
			commandID:   "cmd-001",
			token:       "jwt-token",
			senderID:    "client-1",
			receiverID:  "server-1",
			commandBody: `{"version":"1.0"}`,
		},
		{
			name:        "tcp map create command",
			commandType: packet.TcpMapCreate,
			commandID:   "cmd-002",
			token:       "jwt-token-2",
			senderID:    "client-2",
			receiverID:  "server-1",
			commandBody: `{"local_port":8080,"remote_port":80}`,
		},
		{
			name:        "empty body command",
			commandType: packet.HeartbeatCmd,
			commandID:   "cmd-003",
			token:       "",
			senderID:    "client-3",
			receiverID:  "server-1",
			commandBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			builder := NewDefaultPacketBuilder()

			cmdPacket, err := builder.BuildCommandPacket(
				tt.commandType,
				tt.commandID,
				tt.token,
				tt.senderID,
				tt.receiverID,
				tt.commandBody,
			)

			if err != nil {
				t.Errorf("BuildCommandPacket() error = %v", err)
				return
			}

			if cmdPacket == nil {
				t.Error("BuildCommandPacket() returned nil")
				return
			}

			if cmdPacket.CommandType != tt.commandType {
				t.Errorf("CommandType = %v, want %v", cmdPacket.CommandType, tt.commandType)
			}
			if cmdPacket.CommandId != tt.commandID {
				t.Errorf("CommandId = %s, want %s", cmdPacket.CommandId, tt.commandID)
			}
			if cmdPacket.Token != tt.token {
				t.Errorf("Token = %s, want %s", cmdPacket.Token, tt.token)
			}
			if cmdPacket.SenderId != tt.senderID {
				t.Errorf("SenderId = %s, want %s", cmdPacket.SenderId, tt.senderID)
			}
			if cmdPacket.ReceiverId != tt.receiverID {
				t.Errorf("ReceiverId = %s, want %s", cmdPacket.ReceiverId, tt.receiverID)
			}
			if cmdPacket.CommandBody != tt.commandBody {
				t.Errorf("CommandBody = %s, want %s", cmdPacket.CommandBody, tt.commandBody)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// BuildTransferPacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketBuilder_BuildTransferPacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		packetType    packet.Type
		commandPacket *packet.CommandPacket
	}{
		{
			name:       "json command packet",
			packetType: packet.JsonCommand,
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-1",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
		},
		{
			name:          "heartbeat packet without command",
			packetType:    packet.Heartbeat,
			commandPacket: nil,
		},
		{
			name:       "compressed packet",
			packetType: packet.Compressed,
			commandPacket: &packet.CommandPacket{
				CommandType: packet.DataTransferStart,
				CommandId:   "cmd-2",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			builder := NewDefaultPacketBuilder()

			transferPacket := builder.BuildTransferPacket(tt.packetType, tt.commandPacket)

			if transferPacket == nil {
				t.Error("BuildTransferPacket() returned nil")
				return
			}

			if transferPacket.PacketType != tt.packetType {
				t.Errorf("PacketType = %v, want %v", transferPacket.PacketType, tt.packetType)
			}

			if tt.commandPacket != nil {
				if transferPacket.CommandPacket == nil {
					t.Error("CommandPacket should not be nil")
				} else if transferPacket.CommandPacket.CommandId != tt.commandPacket.CommandId {
					t.Errorf("CommandId = %s, want %s", transferPacket.CommandPacket.CommandId, tt.commandPacket.CommandId)
				}
			} else {
				if transferPacket.CommandPacket != nil {
					t.Error("CommandPacket should be nil")
				}
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// 集成测试：构建完整数据包并验证格式
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketBuilder_Integration(t *testing.T) {
	t.Parallel()

	builder := NewDefaultPacketBuilder()

	// 1. 构建命令包
	cmdPacket, err := builder.BuildCommandPacket(
		packet.TcpMapCreate,
		"integration-test-cmd",
		"test-token",
		"test-sender",
		"test-receiver",
		`{"port":8080}`,
	)
	if err != nil {
		t.Fatalf("BuildCommandPacket() error = %v", err)
	}

	// 2. 构建传输包
	transferPacket := builder.BuildTransferPacket(packet.JsonCommand, cmdPacket)

	// 3. 序列化到缓冲区
	var buf bytes.Buffer
	err = builder.BuildPacket(&buf, transferPacket)
	if err != nil {
		t.Fatalf("BuildPacket() error = %v", err)
	}

	// 4. 验证输出
	data := buf.Bytes()

	// 验证类型
	if packet.Type(data[0]) != packet.JsonCommand {
		t.Errorf("PacketType = %v, want %v", packet.Type(data[0]), packet.JsonCommand)
	}

	// 验证长度
	length := binary.BigEndian.Uint32(data[1:5])

	// 验证 JSON 内容
	jsonData := data[5 : 5+length]
	var parsedCmd packet.CommandPacket
	if err := json.Unmarshal(jsonData, &parsedCmd); err != nil {
		t.Fatalf("Failed to unmarshal command packet: %v", err)
	}

	if parsedCmd.CommandId != "integration-test-cmd" {
		t.Errorf("CommandId = %s, want integration-test-cmd", parsedCmd.CommandId)
	}
	if parsedCmd.Token != "test-token" {
		t.Errorf("Token = %s, want test-token", parsedCmd.Token)
	}
}

// ═══════════════════════════════════════════════════════════════════
// 接口兼容性测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketBuilder_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ PacketBuilder = (*DefaultPacketBuilder)(nil)
}

// ═══════════════════════════════════════════════════════════════════
// 基准测试
// ═══════════════════════════════════════════════════════════════════

func BenchmarkDefaultPacketBuilder_BuildPacket(b *testing.B) {
	builder := NewDefaultPacketBuilder()
	transferPacket := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.TcpMapCreate,
			CommandId:   "bench-cmd",
			Token:       "bench-token",
			SenderId:    "bench-sender",
			ReceiverId:  "bench-receiver",
			CommandBody: `{"port":8080,"host":"localhost"}`,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		builder.BuildPacket(&buf, transferPacket)
	}
}

func BenchmarkDefaultPacketBuilder_BuildCommandPacket(b *testing.B) {
	builder := NewDefaultPacketBuilder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.BuildCommandPacket(
			packet.TcpMapCreate,
			"bench-cmd",
			"bench-token",
			"bench-sender",
			"bench-receiver",
			`{"port":8080}`,
		)
	}
}

func BenchmarkDefaultPacketBuilder_BuildTransferPacket(b *testing.B) {
	builder := NewDefaultPacketBuilder()
	cmdPacket := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		CommandId:   "bench-cmd",
		SenderId:    "sender",
		ReceiverId:  "receiver",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.BuildTransferPacket(packet.JsonCommand, cmdPacket)
	}
}
