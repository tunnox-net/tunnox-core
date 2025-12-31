// Package parser 提供数据包解析器的测试
package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"

	"tunnox-core/internal/packet"
)

// ═══════════════════════════════════════════════════════════════════
// NewDefaultPacketParser 测试
// ═══════════════════════════════════════════════════════════════════

func TestNewDefaultPacketParser(t *testing.T) {
	t.Parallel()

	parser := NewDefaultPacketParser()
	if parser == nil {
		t.Error("NewDefaultPacketParser() returned nil")
	}
}

// ═══════════════════════════════════════════════════════════════════
// ParsePacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketParser_ParsePacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    func() []byte
		wantErr bool
		check   func(*testing.T, *packet.TransferPacket)
	}{
		{
			name: "valid json command packet",
			data: func() []byte {
				cmd := &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-123",
					Token:       "token-abc",
					SenderId:    "sender-1",
					ReceiverId:  "receiver-1",
					CommandBody: `{"action":"connect"}`,
				}
				cmdData, _ := json.Marshal(cmd)
				buf := make([]byte, 1+4+len(cmdData))
				buf[0] = byte(packet.JsonCommand)
				binary.BigEndian.PutUint32(buf[1:5], uint32(len(cmdData)))
				copy(buf[5:], cmdData)
				return buf
			},
			wantErr: false,
			check: func(t *testing.T, tp *packet.TransferPacket) {
				if tp.PacketType != packet.JsonCommand {
					t.Errorf("PacketType = %v, want %v", tp.PacketType, packet.JsonCommand)
				}
				if tp.CommandPacket == nil {
					t.Error("CommandPacket is nil")
					return
				}
				if tp.CommandPacket.CommandId != "cmd-123" {
					t.Errorf("CommandId = %s, want cmd-123", tp.CommandPacket.CommandId)
				}
				if tp.CommandPacket.Token != "token-abc" {
					t.Errorf("Token = %s, want token-abc", tp.CommandPacket.Token)
				}
			},
		},
		{
			name: "unknown packet type",
			data: func() []byte {
				buf := make([]byte, 1+4)
				buf[0] = 0xFF // 未知类型
				binary.BigEndian.PutUint32(buf[1:5], 0)
				return buf
			},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parser := NewDefaultPacketParser()
			reader := bytes.NewReader(tt.data())

			result, err := parser.ParsePacket(reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestDefaultPacketParser_ParsePacket_ReadErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty reader",
			data: []byte{},
		},
		{
			name: "only packet type",
			data: []byte{byte(packet.JsonCommand)},
		},
		{
			name: "incomplete length",
			data: []byte{byte(packet.JsonCommand), 0, 0},
		},
		{
			name: "incomplete data",
			data: func() []byte {
				buf := make([]byte, 5)
				buf[0] = byte(packet.JsonCommand)
				binary.BigEndian.PutUint32(buf[1:5], 100) // 声明100字节但没有数据
				return buf
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parser := NewDefaultPacketParser()
			reader := bytes.NewReader(tt.data)

			_, err := parser.ParsePacket(reader)
			if err == nil {
				t.Error("ParsePacket() should return error for incomplete data")
			}
		})
	}
}

func TestDefaultPacketParser_ParsePacket_InvalidJSON(t *testing.T) {
	t.Parallel()

	parser := NewDefaultPacketParser()

	// 创建一个包含无效 JSON 的数据包
	invalidJSON := []byte("not valid json")
	buf := make([]byte, 1+4+len(invalidJSON))
	buf[0] = byte(packet.JsonCommand)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(invalidJSON)))
	copy(buf[5:], invalidJSON)

	reader := bytes.NewReader(buf)
	_, err := parser.ParsePacket(reader)
	if err == nil {
		t.Error("ParsePacket() should return error for invalid JSON")
	}
}

// ═══════════════════════════════════════════════════════════════════
// ParseCommandPacket 测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketParser_ParseCommandPacket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     *packet.CommandPacket
		wantErr bool
	}{
		{
			name: "valid connect command",
			cmd: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-001",
				Token:       "jwt-token",
				SenderId:    "client-1",
				ReceiverId:  "server-1",
				CommandBody: `{"version":"1.0"}`,
			},
			wantErr: false,
		},
		{
			name: "valid tcp map command",
			cmd: &packet.CommandPacket{
				CommandType: packet.TcpMapCreate,
				CommandId:   "cmd-002",
				Token:       "jwt-token-2",
				SenderId:    "client-2",
				ReceiverId:  "server-1",
				CommandBody: `{"local_port":8080,"remote_port":80}`,
			},
			wantErr: false,
		},
		{
			name: "empty body command",
			cmd: &packet.CommandPacket{
				CommandType: packet.HeartbeatCmd,
				CommandId:   "cmd-003",
				Token:       "",
				SenderId:    "client-3",
				ReceiverId:  "server-1",
				CommandBody: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parser := NewDefaultPacketParser()

			// 序列化命令
			data, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("Failed to marshal command: %v", err)
			}

			// 解析命令
			result, err := parser.ParseCommandPacket(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCommandPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.CommandType != tt.cmd.CommandType {
					t.Errorf("CommandType = %v, want %v", result.CommandType, tt.cmd.CommandType)
				}
				if result.CommandId != tt.cmd.CommandId {
					t.Errorf("CommandId = %s, want %s", result.CommandId, tt.cmd.CommandId)
				}
				if result.Token != tt.cmd.Token {
					t.Errorf("Token = %s, want %s", result.Token, tt.cmd.Token)
				}
				if result.SenderId != tt.cmd.SenderId {
					t.Errorf("SenderId = %s, want %s", result.SenderId, tt.cmd.SenderId)
				}
				if result.ReceiverId != tt.cmd.ReceiverId {
					t.Errorf("ReceiverId = %s, want %s", result.ReceiverId, tt.cmd.ReceiverId)
				}
				if result.CommandBody != tt.cmd.CommandBody {
					t.Errorf("CommandBody = %s, want %s", result.CommandBody, tt.cmd.CommandBody)
				}
			}
		})
	}
}

func TestDefaultPacketParser_ParseCommandPacket_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{"empty data", []byte{}},
		{"invalid json", []byte("not json")},
		{"incomplete json", []byte(`{"CommandType":`)},
		{"wrong type", []byte(`["array"]`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parser := NewDefaultPacketParser()

			_, err := parser.ParseCommandPacket(tt.data)
			if err == nil {
				t.Error("ParseCommandPacket() should return error for invalid JSON")
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// 边界条件测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketParser_ParsePacket_ZeroLengthData(t *testing.T) {
	t.Parallel()

	parser := NewDefaultPacketParser()

	// 创建一个长度为0的数据包
	buf := make([]byte, 5)
	buf[0] = byte(packet.JsonCommand)
	binary.BigEndian.PutUint32(buf[1:5], 0)

	reader := bytes.NewReader(buf)
	_, err := parser.ParsePacket(reader)
	// 零长度数据解析空 JSON 应该失败
	if err == nil {
		t.Error("ParsePacket() should return error for zero-length JSON command")
	}
}

func TestDefaultPacketParser_ParsePacket_LargeData(t *testing.T) {
	t.Parallel()

	parser := NewDefaultPacketParser()

	// 创建一个大的命令包
	cmd := &packet.CommandPacket{
		CommandType: packet.DataTransferStart,
		CommandId:   "large-cmd",
		Token:       "token",
		SenderId:    "sender",
		ReceiverId:  "receiver",
		CommandBody: string(make([]byte, 10000)), // 10KB body
	}
	cmdData, _ := json.Marshal(cmd)

	buf := make([]byte, 1+4+len(cmdData))
	buf[0] = byte(packet.JsonCommand)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(cmdData)))
	copy(buf[5:], cmdData)

	reader := bytes.NewReader(buf)
	result, err := parser.ParsePacket(reader)
	if err != nil {
		t.Errorf("ParsePacket() error = %v for large data", err)
		return
	}

	if result.CommandPacket.CommandId != "large-cmd" {
		t.Errorf("CommandId = %s, want large-cmd", result.CommandPacket.CommandId)
	}
}

// ═══════════════════════════════════════════════════════════════════
// 压缩/加密标志测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketParser_ParsePacket_WithFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		packetType packet.Type
	}{
		{"json command", packet.JsonCommand},
		{"compressed json command", packet.JsonCommand | packet.Compressed},
		{"encrypted json command", packet.JsonCommand | packet.Encrypted},
		{"compressed and encrypted", packet.JsonCommand | packet.Compressed | packet.Encrypted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parser := NewDefaultPacketParser()

			cmd := &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "flag-test",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			}
			cmdData, _ := json.Marshal(cmd)

			buf := make([]byte, 1+4+len(cmdData))
			buf[0] = byte(tt.packetType)
			binary.BigEndian.PutUint32(buf[1:5], uint32(len(cmdData)))
			copy(buf[5:], cmdData)

			reader := bytes.NewReader(buf)
			result, err := parser.ParsePacket(reader)
			if err != nil {
				t.Errorf("ParsePacket() error = %v", err)
				return
			}

			if result.PacketType != tt.packetType {
				t.Errorf("PacketType = %v, want %v", result.PacketType, tt.packetType)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// 接口兼容性测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketParser_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ PacketParser = (*DefaultPacketParser)(nil)
}

// ═══════════════════════════════════════════════════════════════════
// EOF 处理测试
// ═══════════════════════════════════════════════════════════════════

func TestDefaultPacketParser_ParsePacket_EOF(t *testing.T) {
	t.Parallel()

	parser := NewDefaultPacketParser()
	reader := bytes.NewReader([]byte{})

	_, err := parser.ParsePacket(reader)
	if err != io.EOF {
		t.Errorf("ParsePacket() error = %v, want io.EOF", err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// 基准测试
// ═══════════════════════════════════════════════════════════════════

func BenchmarkDefaultPacketParser_ParsePacket(b *testing.B) {
	parser := NewDefaultPacketParser()

	cmd := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		CommandId:   "bench-cmd",
		Token:       "bench-token",
		SenderId:    "bench-sender",
		ReceiverId:  "bench-receiver",
		CommandBody: `{"port":8080,"host":"localhost"}`,
	}
	cmdData, _ := json.Marshal(cmd)

	buf := make([]byte, 1+4+len(cmdData))
	buf[0] = byte(packet.JsonCommand)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(cmdData)))
	copy(buf[5:], cmdData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(buf)
		parser.ParsePacket(reader)
	}
}

func BenchmarkDefaultPacketParser_ParseCommandPacket(b *testing.B) {
	parser := NewDefaultPacketParser()

	cmd := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		CommandId:   "bench-cmd",
		Token:       "bench-token",
		SenderId:    "bench-sender",
		ReceiverId:  "bench-receiver",
		CommandBody: `{"port":8080,"host":"localhost"}`,
	}
	cmdData, _ := json.Marshal(cmd)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseCommandPacket(cmdData)
	}
}
