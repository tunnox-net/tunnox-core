package unit

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/packet/builder"
	"tunnox-core/internal/packet/parser"
)

// TestPacketBuilder_CommandPacket 测试命令包构建
func TestPacketBuilder_CommandPacket(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	var buf bytes.Buffer

	cmdPkt, err := b.BuildCommandPacket(
		packet.Connect,
		"cmd-123",
		"token-456",
		"sender-789",
		"receiver-012",
		`{"key":"value"}`,
	)
	require.NoError(t, err)

	pkt := b.BuildTransferPacket(packet.JsonCommand, cmdPkt)

	err = b.BuildPacket(&buf, pkt)
	require.NoError(t, err)

	// 解析验证
	p := parser.NewDefaultPacketParser()
	parsed, err := p.ParsePacket(&buf)
	require.NoError(t, err)
	assert.Equal(t, packet.JsonCommand, parsed.PacketType)
	assert.NotNil(t, parsed.CommandPacket)
	assert.Equal(t, packet.Connect, parsed.CommandPacket.CommandType)
	assert.Equal(t, "cmd-123", parsed.CommandPacket.CommandId)
	assert.Equal(t, "token-456", parsed.CommandPacket.Token)
}

// TestPacketBuilder_AllCommandTypes 测试所有命令类型
func TestPacketBuilder_AllCommandTypes(t *testing.T) {
	commandTypes := []packet.CommandType{
		packet.Connect,
		packet.Disconnect,
		packet.HeartbeatCmd,
		packet.ConfigSet,
		packet.ConfigGet,
		packet.TunnelOpenRequestCmd,
		packet.TcpMapCreate,
		packet.TcpMapDelete,
		packet.KickClient,
	}

	b := builder.NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()

	for _, cmdType := range commandTypes {
		t.Run(string(cmdType), func(t *testing.T) {
			var buf bytes.Buffer

			cmdPkt, err := b.BuildCommandPacket(
				cmdType,
				"cmd-id",
				"token",
				"sender",
				"receiver",
				`{"test":"data"}`,
			)
			require.NoError(t, err)

			pkt := b.BuildTransferPacket(packet.JsonCommand, cmdPkt)

			err = b.BuildPacket(&buf, pkt)
			require.NoError(t, err)

			parsed, err := p.ParsePacket(&buf)
			require.NoError(t, err)
			assert.Equal(t, packet.JsonCommand, parsed.PacketType)
			assert.Equal(t, cmdType, parsed.CommandPacket.CommandType)
		})
	}
}

// TestCommandPacket_ParseJSON 测试命令包JSON解析
func TestCommandPacket_ParseJSON(t *testing.T) {
	jsonStr := `{
		"CommandType": 10,
		"CommandId": "cmd-123",
		"Token": "token-456",
		"SenderId": "sender-789",
		"ReceiverId": "receiver-012",
		"CommandBody": "{\"key\":\"value\"}"
	}`

	var cmdPkt packet.CommandPacket
	err := json.Unmarshal([]byte(jsonStr), &cmdPkt)
	require.NoError(t, err)
	assert.Equal(t, packet.Connect, cmdPkt.CommandType)
	assert.Equal(t, "cmd-123", cmdPkt.CommandId)
	assert.Equal(t, "token-456", cmdPkt.Token)
}

// TestPacketBuilder_ConcurrentBuild 测试并发构建
func TestPacketBuilder_ConcurrentBuild(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()

	const goroutines = 100
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			var buf bytes.Buffer
			
			cmdPkt, err := b.BuildCommandPacket(
				packet.Connect,
				"cmd-id",
				"token",
				"sender",
				"receiver",
				`{"id":`+string(rune(id+'0'))+`}`,
			)
			assert.NoError(t, err)

			pkt := b.BuildTransferPacket(packet.JsonCommand, cmdPkt)
			err = b.BuildPacket(&buf, pkt)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// TestPacketParser_ConcurrentParse 测试并发解析
func TestPacketParser_ConcurrentParse(t *testing.T) {
	// 准备测试数据
	b := builder.NewDefaultPacketBuilder()
	var testData bytes.Buffer
	
	cmdPkt, err := b.BuildCommandPacket(
		packet.Connect,
		"cmd-id",
		"token",
		"sender",
		"receiver",
		`{"test":"data"}`,
	)
	require.NoError(t, err)

	pkt := b.BuildTransferPacket(packet.JsonCommand, cmdPkt)
	err = b.BuildPacket(&testData, pkt)
	require.NoError(t, err)

	p := parser.NewDefaultPacketParser()
	const goroutines = 100
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			buf := bytes.NewReader(testData.Bytes())
			_, err := p.ParsePacket(buf)
			assert.NoError(t, err)

			done <- true
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// TestHandshakeRequest_JSON 测试握手请求JSON序列化
func TestHandshakeRequest_JSON(t *testing.T) {
	req := &packet.HandshakeRequest{
		ClientID: 12345,
		Token:    "test-auth-token",
		Version:  "1.0.0",
		Protocol: "tcp",
	}

	// 序列化
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var parsed packet.HandshakeRequest
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), parsed.ClientID)
	assert.Equal(t, "test-auth-token", parsed.Token)
	assert.Equal(t, "1.0.0", parsed.Version)
	assert.Equal(t, "tcp", parsed.Protocol)
}

// TestHandshakeResponse_JSON 测试握手响应JSON序列化
func TestHandshakeResponse_JSON(t *testing.T) {
	resp := &packet.HandshakeResponse{
		Success: true,
		Message: "Handshake successful",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed packet.HandshakeResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.True(t, parsed.Success)
	assert.Equal(t, "Handshake successful", parsed.Message)
}

// TestAcceptPacket_JSON 测试接受包JSON序列化
func TestAcceptPacket_JSON(t *testing.T) {
	resp := &packet.AcceptPacket{
		Success:  true,
		Message:  "Connection accepted",
		ClientID: "client-123",
		Token:    "session-token",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed packet.AcceptPacket
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.True(t, parsed.Success)
	assert.Equal(t, "Connection accepted", parsed.Message)
	assert.Equal(t, "client-123", parsed.ClientID)
	assert.Equal(t, "session-token", parsed.Token)
}

// TestTunnelOpenRequest_JSON 测试隧道打开请求JSON序列化
func TestTunnelOpenRequest_JSON(t *testing.T) {
	req := &packet.TunnelOpenRequest{
		TunnelID:  "tunnel-123",
		MappingID: "mapping-456",
		SecretKey: "secret-key-789",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var parsed packet.TunnelOpenRequest
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "tunnel-123", parsed.TunnelID)
	assert.Equal(t, "mapping-456", parsed.MappingID)
	assert.Equal(t, "secret-key-789", parsed.SecretKey)
}

// TestTunnelOpenAckResponse_JSON 测试隧道打开确认响应JSON序列化
func TestTunnelOpenAckResponse_JSON(t *testing.T) {
	resp := &packet.TunnelOpenAckResponse{
		Success:  true,
		TunnelID: "tunnel-123",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed packet.TunnelOpenAckResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.True(t, parsed.Success)
	assert.Equal(t, "tunnel-123", parsed.TunnelID)
}

// TestPacketType_Methods 测试PacketType的方法
func TestPacketType_Methods(t *testing.T) {
	tests := []struct {
		name        string
		packetType  packet.Type
		isHeartbeat bool
		isCommand   bool
		isTunnel    bool
		isHandshake bool
	}{
		{"Heartbeat", packet.Heartbeat, true, false, false, false},
		{"JsonCommand", packet.JsonCommand, false, true, false, false},
		{"Handshake", packet.Handshake, false, false, false, true},
		{"TunnelOpen", packet.TunnelOpen, false, false, true, false},
		{"TunnelData", packet.TunnelData, false, false, true, false},
		{"TunnelClose", packet.TunnelClose, false, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isHeartbeat, tt.packetType.IsHeartbeat())
			assert.Equal(t, tt.isCommand, tt.packetType.IsJsonCommand())
			assert.Equal(t, tt.isTunnel, tt.packetType.IsTunnelPacket())
			assert.Equal(t, tt.isHandshake, tt.packetType.IsHandshake())
		})
	}
}

// TestPacketType_Flags 测试PacketType的标志位
func TestPacketType_Flags(t *testing.T) {
	// 测试压缩标志
	compressed := packet.JsonCommand | packet.Compressed
	assert.True(t, compressed.IsCompressed())
	assert.True(t, compressed.IsJsonCommand())

	// 测试加密标志
	encrypted := packet.JsonCommand | packet.Encrypted
	assert.True(t, encrypted.IsEncrypted())
	assert.True(t, encrypted.IsJsonCommand())

	// 测试压缩+加密
	both := packet.JsonCommand | packet.Compressed | packet.Encrypted
	assert.True(t, both.IsCompressed())
	assert.True(t, both.IsEncrypted())
	assert.True(t, both.IsJsonCommand())
}

// TestCommandPacket_EmptyBody 测试空命令体
func TestCommandPacket_EmptyBody(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	
	cmdPkt, err := b.BuildCommandPacket(
		packet.HeartbeatCmd,
		"heartbeat-1",
		"token",
		"sender",
		"receiver",
		"", // 空命令体
	)
	require.NoError(t, err)
	assert.Empty(t, cmdPkt.CommandBody)

	// 序列化
	data, err := json.Marshal(cmdPkt)
	require.NoError(t, err)

	// 反序列化
	var parsed packet.CommandPacket
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Empty(t, parsed.CommandBody)
}

// TestCommandPacket_LargeBody 测试大命令体
func TestCommandPacket_LargeBody(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	
	// 创建1MB的命令体
	largeBody := string(make([]byte, 1024*1024))
	
	cmdPkt, err := b.BuildCommandPacket(
		packet.ConfigSet,
		"config-1",
		"token",
		"sender",
		"receiver",
		largeBody,
	)
	require.NoError(t, err)
	assert.Len(t, cmdPkt.CommandBody, 1024*1024)
}

// TestPacketRoundTrip 测试完整往返
func TestPacketRoundTrip(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()

	cmdPkt, err := b.BuildCommandPacket(
		packet.Connect,
		"cmd-123",
		"token-abc",
		"sender-1",
		"receiver-2",
		`{"client_id":1,"version":"1.0"}`,
	)
	require.NoError(t, err)

	transferPkt := b.BuildTransferPacket(packet.JsonCommand, cmdPkt)

	// 序列化
	var buf bytes.Buffer
	err = b.BuildPacket(&buf, transferPkt)
	require.NoError(t, err)

	// 反序列化
	parsedTransferPkt, err := p.ParsePacket(&buf)
	require.NoError(t, err)
	assert.NotNil(t, parsedTransferPkt)
	assert.Equal(t, packet.JsonCommand, parsedTransferPkt.PacketType)
	assert.NotNil(t, parsedTransferPkt.CommandPacket)
	assert.Equal(t, cmdPkt.CommandType, parsedTransferPkt.CommandPacket.CommandType)
	assert.Equal(t, cmdPkt.CommandId, parsedTransferPkt.CommandPacket.CommandId)
	assert.Equal(t, cmdPkt.Token, parsedTransferPkt.CommandPacket.Token)
	assert.Equal(t, cmdPkt.SenderId, parsedTransferPkt.CommandPacket.SenderId)
	assert.Equal(t, cmdPkt.ReceiverId, parsedTransferPkt.CommandPacket.ReceiverId)
	assert.Equal(t, cmdPkt.CommandBody, parsedTransferPkt.CommandPacket.CommandBody)
}

// TestInitPacket_JSON 测试初始化包JSON序列化
func TestInitPacket_JSON(t *testing.T) {
	initPkt := &packet.InitPacket{
		Version:   "1.0.0",
		ClientID:  "client-123",
		AuthCode:  "auth-code",
		SecretKey: "secret-key",
		NodeID:    "node-1",
		IPAddress: "192.168.1.100",
		Type:      "registered",
	}

	data, err := json.Marshal(initPkt)
	require.NoError(t, err)

	var parsed packet.InitPacket
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", parsed.Version)
	assert.Equal(t, "client-123", parsed.ClientID)
	assert.Equal(t, "auth-code", parsed.AuthCode)
}

// TestPacketBuilder_NilCommandPacket 测试nil命令包
func TestPacketBuilder_NilCommandPacket(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	var buf bytes.Buffer

	// 构建一个没有命令包的传输包（使用JsonCommand类型但CommandPacket为nil）
	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: nil,
	}

	err := b.BuildPacket(&buf, pkt)
	require.NoError(t, err)
	
	// 应该只写入类型和长度（0）
	assert.Greater(t, buf.Len(), 0)
}

// TestCommandPacket_SpecialCharacters 测试特殊字符
func TestCommandPacket_SpecialCharacters(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	
	specialChars := `{"message":"Hello\nWorld\t\"quoted\""}`
	
	cmdPkt, err := b.BuildCommandPacket(
		packet.ConfigSet,
		"cmd-1",
		"token",
		"sender",
		"receiver",
		specialChars,
	)
	require.NoError(t, err)

	// 序列化
	data, err := json.Marshal(cmdPkt)
	require.NoError(t, err)

	// 反序列化
	var parsed packet.CommandPacket
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, specialChars, parsed.CommandBody)
}

// TestPacketType_Validation 测试PacketType的有效性
func TestPacketType_Validation(t *testing.T) {
	validTypes := []packet.Type{
		packet.Handshake,
		packet.HandshakeResp,
		packet.Heartbeat,
		packet.JsonCommand,
		packet.CommandResp,
		packet.TunnelOpen,
		packet.TunnelOpenAck,
		packet.TunnelData,
		packet.TunnelClose,
	}

	for _, pt := range validTypes {
		t.Run(string([]byte{byte(pt)}), func(t *testing.T) {
			// 验证类型有效
			assert.NotEqual(t, packet.Type(0), pt)
		})
	}
}

// TestPacketBuilder_MultiplePackets 测试构建多个包
func TestPacketBuilder_MultiplePackets(t *testing.T) {
	b := builder.NewDefaultPacketBuilder()
	p := parser.NewDefaultPacketParser()
	var buf bytes.Buffer

	// 构建多个包
	for i := 0; i < 10; i++ {
		cmdPkt, err := b.BuildCommandPacket(
			packet.Connect,
			"cmd-"+string(rune(i+'0')),
			"token",
			"sender",
			"receiver",
			`{"id":`+string(rune(i+'0'))+`}`,
		)
		require.NoError(t, err)

		transferPkt := b.BuildTransferPacket(packet.JsonCommand, cmdPkt)
		err = b.BuildPacket(&buf, transferPkt)
		require.NoError(t, err)
	}

	// 解析所有包
	for i := 0; i < 10; i++ {
		parsed, err := p.ParsePacket(&buf)
		require.NoError(t, err)
		assert.NotNil(t, parsed)
		assert.Equal(t, packet.JsonCommand, parsed.PacketType)
	}
}
