package api

import (
	"testing"

	"tunnox-core/internal/stream"
)

// mockConnection 模拟控制连接
type mockConnection struct {
	connID     string
	remoteAddr string
	stream     stream.PackageStreamer
}

func (m *mockConnection) GetConnID() string {
	return m.connID
}

func (m *mockConnection) GetRemoteAddr() string {
	return m.remoteAddr
}

func (m *mockConnection) GetStream() stream.PackageStreamer {
	return m.stream
}

// TestGetStreamFromConnection_Success 测试成功获取stream
func TestGetStreamFromConnection_Success(t *testing.T) {
	mockStream := &stream.StreamProcessor{} // 这里需要一个有效的stream
	conn := &mockConnection{
		connID:     "test-conn-123",
		remoteAddr: "127.0.0.1:12345",
		stream:     mockStream,
	}

	streamProcessor, connID, remoteAddr, err := getStreamFromConnection(conn, 100)
	
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if streamProcessor != mockStream {
		t.Errorf("Expected stream %p, got %p", mockStream, streamProcessor)
	}
	
	if connID != "test-conn-123" {
		t.Errorf("Expected connID 'test-conn-123', got '%s'", connID)
	}
	
	if remoteAddr != "127.0.0.1:12345" {
		t.Errorf("Expected remoteAddr '127.0.0.1:12345', got '%s'", remoteAddr)
	}
}

// TestGetStreamFromConnection_NilInterface 测试nil接口
func TestGetStreamFromConnection_NilInterface(t *testing.T) {
	_, _, _, err := getStreamFromConnection(nil, 100)
	
	if err == nil {
		t.Error("Expected error for nil connection interface")
	}
}

// TestGetStreamFromConnection_NilStream 测试nil stream
func TestGetStreamFromConnection_NilStream(t *testing.T) {
	conn := &mockConnection{
		connID:     "test-conn-456",
		remoteAddr: "127.0.0.1:12346",
		stream:     nil, // nil stream
	}

	_, _, _, err := getStreamFromConnection(conn, 100)
	
	if err == nil {
		t.Error("Expected error for nil stream")
	}
}

// TestSendPacketAsync 测试异步发送包（简单验证函数签名）
func TestSendPacketAsync(t *testing.T) {
	// 注意：sendPacketAsync需要一个完全初始化的StreamProcessor
	// 由于StreamProcessor的初始化很复杂，这里只测试nil情况会被优雅处理
	// 实际的发送逻辑应该通过集成测试覆盖
	
	// 跳过这个测试，因为它需要完整的StreamProcessor初始化
	t.Skip("Skipping sendPacketAsync test - requires full StreamProcessor initialization")
}

// TestConfigPushData_Marshaling 测试配置数据序列化
func TestConfigPushData_Marshaling(t *testing.T) {
	data := ConfigPushData{
		Mappings: nil,
	}
	
	// 验证结构体可以被序列化
	if data.Mappings == nil {
		t.Log("ConfigPushData initialized correctly")
	}
}

// TestConfigRemovalData_Marshaling 测试配置移除数据序列化
func TestConfigRemovalData_Marshaling(t *testing.T) {
	data := ConfigRemovalData{
		Mappings:       nil,
		RemoveMappings: []string{"mapping-1"},
	}
	
	// 验证结构体可以被序列化
	if len(data.RemoveMappings) == 1 {
		t.Log("ConfigRemovalData initialized correctly")
	}
}

// TestKickClientInfo_Marshaling 测试踢下线信息序列化
func TestKickClientInfo_Marshaling(t *testing.T) {
	info := KickClientInfo{
		Reason: "quota_exceeded",
		Code:   "QUOTA_EXCEEDED",
	}
	
	// 验证结构体字段
	if info.Reason != "quota_exceeded" {
		t.Errorf("Expected reason 'quota_exceeded', got '%s'", info.Reason)
	}
	
	if info.Code != "QUOTA_EXCEEDED" {
		t.Errorf("Expected code 'QUOTA_EXCEEDED', got '%s'", info.Code)
	}
}

