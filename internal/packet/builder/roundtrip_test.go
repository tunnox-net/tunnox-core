// Package builder 提供数据包构建/解析/验证的往返测试
package builder

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/packet/parser"
	"tunnox-core/internal/packet/validator"
)

// ============================================================================
// 往返测试：构建 -> 解析 -> 验证
// ============================================================================

func TestRoundTrip_BuildParseValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		packetType  packet.Type
		commandType packet.CommandType
		commandID   string
		token       string
		senderID    string
		receiverID  string
		commandBody string
	}{
		{
			name:        "Connect command",
			packetType:  packet.JsonCommand,
			commandType: packet.Connect,
			commandID:   "cmd-connect-001",
			token:       "jwt-token-abc123",
			senderID:    "client-12345",
			receiverID:  "server-main",
			commandBody: `{"version":"1.0.0","protocol":"tcp"}`,
		},
		{
			name:        "Disconnect command",
			packetType:  packet.JsonCommand,
			commandType: packet.Disconnect,
			commandID:   "cmd-disconnect-001",
			token:       "jwt-token-xyz789",
			senderID:    "client-67890",
			receiverID:  "server-main",
			commandBody: `{"reason":"user_request"}`,
		},
		{
			name:        "TcpMapCreate command",
			packetType:  packet.JsonCommand,
			commandType: packet.TcpMapCreate,
			commandID:   "cmd-tcp-map-001",
			token:       "jwt-token-map",
			senderID:    "client-mapper",
			receiverID:  "server-main",
			commandBody: `{"local_port":8080,"remote_port":80,"target_host":"localhost"}`,
		},
		{
			name:        "Heartbeat command",
			packetType:  packet.JsonCommand,
			commandType: packet.HeartbeatCmd,
			commandID:   "cmd-heartbeat-001",
			token:       "",
			senderID:    "client-1",
			receiverID:  "server-1",
			commandBody: "",
		},
		{
			name:        "DataTransferStart command",
			packetType:  packet.JsonCommand,
			commandType: packet.DataTransferStart,
			commandID:   "cmd-transfer-001",
			token:       "session-token",
			senderID:    "client-transfer",
			receiverID:  "server-transfer",
			commandBody: `{"tunnel_id":"tunnel-001","mapping_id":"map-001"}`,
		},
		{
			name:        "RpcInvoke command",
			packetType:  packet.JsonCommand,
			commandType: packet.RpcInvoke,
			commandID:   "cmd-rpc-001",
			token:       "rpc-token",
			senderID:    "rpc-client",
			receiverID:  "rpc-server",
			commandBody: `{"method":"echo","params":["hello"]}`,
		},
		{
			name:        "ConnectionCodeGenerate command",
			packetType:  packet.JsonCommand,
			commandType: packet.ConnectionCodeGenerate,
			commandID:   "cmd-code-gen-001",
			token:       "auth-token",
			senderID:    "code-client",
			receiverID:  "code-server",
			commandBody: `{"ttl_seconds":3600,"max_uses":1}`,
		},
		{
			name:        "HTTPDomainCreate command",
			packetType:  packet.JsonCommand,
			commandType: packet.HTTPDomainCreate,
			commandID:   "cmd-http-domain-001",
			token:       "http-token",
			senderID:    "http-client",
			receiverID:  "http-server",
			commandBody: `{"subdomain":"myapp","base_domain":"tunnox.net","target_url":"http://localhost:3000"}`,
		},
	}

	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Step 1: 构建命令包
			cmdPacket, err := b.BuildCommandPacket(
				tt.commandType,
				tt.commandID,
				tt.token,
				tt.senderID,
				tt.receiverID,
				tt.commandBody,
			)
			if err != nil {
				t.Fatalf("BuildCommandPacket() error = %v", err)
			}

			// Step 2: 构建传输包
			transferPacket := b.BuildTransferPacket(tt.packetType, cmdPacket)

			// Step 3: 序列化到缓冲区
			var buf bytes.Buffer
			err = b.BuildPacket(&buf, transferPacket)
			if err != nil {
				t.Fatalf("BuildPacket() error = %v", err)
			}

			// Step 4: 解析数据包
			parsedPacket, err := p.ParsePacket(&buf)
			if err != nil {
				t.Fatalf("ParsePacket() error = %v", err)
			}

			// Step 5: 验证数据包
			err = v.ValidateTransferPacket(parsedPacket)
			if err != nil {
				t.Fatalf("ValidateTransferPacket() error = %v", err)
			}

			// Step 6: 验证数据完整性
			if parsedPacket.PacketType != tt.packetType {
				t.Errorf("PacketType = %v, want %v", parsedPacket.PacketType, tt.packetType)
			}
			if parsedPacket.CommandPacket == nil {
				t.Fatal("CommandPacket is nil")
			}
			if parsedPacket.CommandPacket.CommandType != tt.commandType {
				t.Errorf("CommandType = %v, want %v", parsedPacket.CommandPacket.CommandType, tt.commandType)
			}
			if parsedPacket.CommandPacket.CommandId != tt.commandID {
				t.Errorf("CommandId = %s, want %s", parsedPacket.CommandPacket.CommandId, tt.commandID)
			}
			if parsedPacket.CommandPacket.Token != tt.token {
				t.Errorf("Token = %s, want %s", parsedPacket.CommandPacket.Token, tt.token)
			}
			if parsedPacket.CommandPacket.SenderId != tt.senderID {
				t.Errorf("SenderId = %s, want %s", parsedPacket.CommandPacket.SenderId, tt.senderID)
			}
			if parsedPacket.CommandPacket.ReceiverId != tt.receiverID {
				t.Errorf("ReceiverId = %s, want %s", parsedPacket.CommandPacket.ReceiverId, tt.receiverID)
			}
			if parsedPacket.CommandPacket.CommandBody != tt.commandBody {
				t.Errorf("CommandBody = %s, want %s", parsedPacket.CommandPacket.CommandBody, tt.commandBody)
			}
		})
	}
}

// ============================================================================
// 往返测试：带标志位的数据包
// ============================================================================

func TestRoundTrip_PacketWithFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		packetType packet.Type
	}{
		{
			name:       "JsonCommand without flags",
			packetType: packet.JsonCommand,
		},
		{
			name:       "JsonCommand with compressed flag",
			packetType: packet.JsonCommand | packet.Compressed,
		},
		{
			name:       "JsonCommand with encrypted flag",
			packetType: packet.JsonCommand | packet.Encrypted,
		},
		{
			name:       "JsonCommand with both flags",
			packetType: packet.JsonCommand | packet.Compressed | packet.Encrypted,
		},
	}

	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmdPacket := &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "flag-test-001",
				Token:       "test-token",
				SenderId:    "sender",
				ReceiverId:  "receiver",
				CommandBody: `{"test":"data"}`,
			}

			transferPacket := &packet.TransferPacket{
				PacketType:    tt.packetType,
				CommandPacket: cmdPacket,
			}

			var buf bytes.Buffer
			err := b.BuildPacket(&buf, transferPacket)
			if err != nil {
				t.Fatalf("BuildPacket() error = %v", err)
			}

			parsedPacket, err := p.ParsePacket(&buf)
			if err != nil {
				t.Fatalf("ParsePacket() error = %v", err)
			}

			// 验证标志位被保留
			if parsedPacket.PacketType != tt.packetType {
				t.Errorf("PacketType = %v, want %v", parsedPacket.PacketType, tt.packetType)
			}

			// 验证标志位检测方法
			if tt.packetType.IsCompressed() != parsedPacket.PacketType.IsCompressed() {
				t.Errorf("IsCompressed() = %v, want %v",
					parsedPacket.PacketType.IsCompressed(), tt.packetType.IsCompressed())
			}
			if tt.packetType.IsEncrypted() != parsedPacket.PacketType.IsEncrypted() {
				t.Errorf("IsEncrypted() = %v, want %v",
					parsedPacket.PacketType.IsEncrypted(), tt.packetType.IsEncrypted())
			}
		})
	}
}

// ============================================================================
// 往返测试：大数据量命令体
// ============================================================================

func TestRoundTrip_LargeCommandBody(t *testing.T) {
	t.Parallel()

	// 测试不同大小的命令体
	sizes := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10240},
		{"100KB", 102400},
	}

	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 生成大的 JSON 命令体
			largeData := make([]byte, tc.size)
			for i := range largeData {
				largeData[i] = byte('a' + (i % 26))
			}

			cmdBody, _ := json.Marshal(map[string]string{
				"large_field": string(largeData),
			})

			cmdPacket, err := b.BuildCommandPacket(
				packet.DataTransferStart,
				"large-cmd-001",
				"token",
				"sender",
				"receiver",
				string(cmdBody),
			)
			if err != nil {
				t.Fatalf("BuildCommandPacket() error = %v", err)
			}

			transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

			var buf bytes.Buffer
			err = b.BuildPacket(&buf, transferPacket)
			if err != nil {
				t.Fatalf("BuildPacket() error = %v", err)
			}

			parsedPacket, err := p.ParsePacket(&buf)
			if err != nil {
				t.Fatalf("ParsePacket() error = %v", err)
			}

			err = v.ValidateTransferPacket(parsedPacket)
			if err != nil {
				t.Fatalf("ValidateTransferPacket() error = %v", err)
			}

			// 验证命令体完整性
			if parsedPacket.CommandPacket.CommandBody != string(cmdBody) {
				t.Errorf("CommandBody length = %d, want %d",
					len(parsedPacket.CommandPacket.CommandBody), len(cmdBody))
			}
		})
	}
}

// ============================================================================
// 往返测试：特殊字符命令体
// ============================================================================

func TestRoundTrip_SpecialCharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		commandBody string
	}{
		{
			name:        "Unicode characters",
			commandBody: `{"message":"Hello, \u4e16\u754c! \u0424\u044b\u0432\u0430"}`,
		},
		{
			name:        "Escaped characters",
			commandBody: `{"path":"C:\\Users\\test\\file.txt","tab":"\t","newline":"\n"}`,
		},
		{
			name:        "JSON with nested objects",
			commandBody: `{"outer":{"inner":{"deep":{"value":123}}}}`,
		},
		{
			name:        "JSON with arrays",
			commandBody: `{"items":[1,2,3,"a","b","c",true,false,null]}`,
		},
		{
			name:        "Empty JSON object",
			commandBody: `{}`,
		},
		{
			name:        "Complex nested structure",
			commandBody: `{"config":{"servers":[{"host":"localhost","port":8080},{"host":"example.com","port":443}],"options":{"timeout":30,"retries":3}}}`,
		},
	}

	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmdPacket, err := b.BuildCommandPacket(
				packet.ConfigSet,
				"special-cmd-001",
				"token",
				"sender",
				"receiver",
				tt.commandBody,
			)
			if err != nil {
				t.Fatalf("BuildCommandPacket() error = %v", err)
			}

			transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

			var buf bytes.Buffer
			err = b.BuildPacket(&buf, transferPacket)
			if err != nil {
				t.Fatalf("BuildPacket() error = %v", err)
			}

			parsedPacket, err := p.ParsePacket(&buf)
			if err != nil {
				t.Fatalf("ParsePacket() error = %v", err)
			}

			err = v.ValidateTransferPacket(parsedPacket)
			if err != nil {
				t.Fatalf("ValidateTransferPacket() error = %v", err)
			}

			if parsedPacket.CommandPacket.CommandBody != tt.commandBody {
				t.Errorf("CommandBody = %q, want %q",
					parsedPacket.CommandPacket.CommandBody, tt.commandBody)
			}
		})
	}
}

// ============================================================================
// 往返测试：所有命令类型
// ============================================================================

func TestRoundTrip_AllCommandTypes(t *testing.T) {
	t.Parallel()

	commandTypes := []packet.CommandType{
		// 连接管理类
		packet.Connect, packet.Disconnect, packet.Reconnect, packet.HeartbeatCmd,
		// 端口映射类
		packet.TcpMapCreate, packet.TcpMapDelete, packet.TcpMapUpdate, packet.TcpMapList, packet.TcpMapStatus,
		packet.HttpMapCreate, packet.HttpMapDelete, packet.HttpMapUpdate, packet.HttpMapList, packet.HttpMapStatus,
		packet.SocksMapCreate, packet.SocksMapDelete, packet.SocksMapUpdate, packet.SocksMapList, packet.SocksMapStatus,
		// 数据传输类
		packet.DataTransferStart, packet.DataTransferStop, packet.DataTransferStatus, packet.ProxyForward, packet.DataTransferOut,
		// 系统管理类
		packet.ConfigGet, packet.ConfigSet, packet.StatsGet, packet.LogGet, packet.HealthCheck,
		// RPC类
		packet.RpcInvoke, packet.RpcRegister, packet.RpcUnregister, packet.RpcList,
	}

	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	for _, cmdType := range commandTypes {
		t.Run(string(rune('0'+int(cmdType)/10))+string(rune('0'+int(cmdType)%10)), func(t *testing.T) {
			t.Parallel()

			cmdPacket, _ := b.BuildCommandPacket(
				cmdType,
				"all-types-test",
				"token",
				"sender",
				"receiver",
				"{}",
			)

			transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

			var buf bytes.Buffer
			b.BuildPacket(&buf, transferPacket)

			parsedPacket, err := p.ParsePacket(&buf)
			if err != nil {
				t.Fatalf("ParsePacket() error = %v for command type %d", err, cmdType)
			}

			err = v.ValidateCommandType(parsedPacket.CommandPacket.CommandType)
			if err != nil {
				t.Errorf("ValidateCommandType() error = %v for command type %d", err, cmdType)
			}
		})
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestRoundTrip_BinaryFormat(t *testing.T) {
	t.Parallel()

	b := NewDefaultPacketBuilder()

	cmdPacket := &packet.CommandPacket{
		CommandType: packet.Connect,
		CommandId:   "binary-test",
		Token:       "token",
		SenderId:    "sender",
		ReceiverId:  "receiver",
		CommandBody: `{"test":"value"}`,
	}

	transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

	var buf bytes.Buffer
	err := b.BuildPacket(&buf, transferPacket)
	if err != nil {
		t.Fatalf("BuildPacket() error = %v", err)
	}

	data := buf.Bytes()

	// 验证二进制格式
	// Byte 0: 包类型
	if packet.Type(data[0]) != packet.JsonCommand {
		t.Errorf("Byte 0 (PacketType) = %d, want %d", data[0], packet.JsonCommand)
	}

	// Bytes 1-4: 长度（大端序）
	length := binary.BigEndian.Uint32(data[1:5])
	if int(length) != len(data)-5 {
		t.Errorf("Length = %d, want %d", length, len(data)-5)
	}

	// Bytes 5+: JSON 数据
	var parsedCmd packet.CommandPacket
	err = json.Unmarshal(data[5:], &parsedCmd)
	if err != nil {
		t.Errorf("JSON unmarshal error = %v", err)
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestRoundTrip_ConcurrentBuildParse(t *testing.T) {
	t.Parallel()

	const numGoroutines = 100

	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			cmdPacket, _ := b.BuildCommandPacket(
				packet.Connect,
				"concurrent-test",
				"token",
				"sender",
				"receiver",
				`{"id":`+string(rune('0'+id%10))+`}`,
			)

			transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

			var buf bytes.Buffer
			b.BuildPacket(&buf, transferPacket)

			parsedPacket, err := p.ParsePacket(&buf)
			if err != nil {
				t.Errorf("goroutine %d: ParsePacket() error = %v", id, err)
				return
			}

			err = v.ValidateTransferPacket(parsedPacket)
			if err != nil {
				t.Errorf("goroutine %d: ValidateTransferPacket() error = %v", id, err)
			}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// ============================================================================
// 基准测试
// ============================================================================

func BenchmarkRoundTrip_SmallPacket(bench *testing.B) {
	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	cmdPacket := &packet.CommandPacket{
		CommandType: packet.Connect,
		CommandId:   "bench-cmd",
		Token:       "bench-token",
		SenderId:    "bench-sender",
		ReceiverId:  "bench-receiver",
		CommandBody: `{"test":"small"}`,
	}

	transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		var buf bytes.Buffer
		b.BuildPacket(&buf, transferPacket)
		parsedPacket, _ := p.ParsePacket(&buf)
		v.ValidateTransferPacket(parsedPacket)
	}
}

func BenchmarkRoundTrip_LargePacket(bench *testing.B) {
	b := NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	v := validator.NewDefaultPacketValidator()

	// 10KB 命令体
	largeData := make([]byte, 10240)
	for i := range largeData {
		largeData[i] = byte('a' + (i % 26))
	}

	cmdPacket := &packet.CommandPacket{
		CommandType: packet.DataTransferStart,
		CommandId:   "bench-large-cmd",
		Token:       "bench-token",
		SenderId:    "bench-sender",
		ReceiverId:  "bench-receiver",
		CommandBody: string(largeData),
	}

	transferPacket := b.BuildTransferPacket(packet.JsonCommand, cmdPacket)

	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		var buf bytes.Buffer
		b.BuildPacket(&buf, transferPacket)
		parsedPacket, _ := p.ParsePacket(&buf)
		v.ValidateTransferPacket(parsedPacket)
	}
}
