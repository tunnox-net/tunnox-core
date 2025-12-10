package session

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// TestHandshakeFailure_ConnectionClosed 测试握手失败后连接是否被关闭
func TestHandshakeFailure_ConnectionClosed(t *testing.T) {
	// 创建测试环境
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建存储
	storageFactory := storage.NewStorageFactory(ctx)
	testStorage, err := storageFactory.CreateStorage(storage.StorageTypeMemory, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer testStorage.Close()

	// 创建 IDManager
	idManager := idgen.NewIDManager(testStorage, ctx)

	// 创建 SessionManager
	sessionMgr := NewSessionManager(idManager, ctx)

	// 创建模拟的 AuthHandler（总是返回失败）
	mockAuthHandler := &mockAuthHandlerAlwaysFail{}
	sessionMgr.SetAuthHandler(mockAuthHandler)

	// 创建测试连接
	connID := "test-conn-001"
	mockStream := &mockStreamProcessor{}
	mockConn := &types.Connection{
		ID:       connID,
		Stream:   mockStream,
		Protocol: "tcp",
	}

	// 注册连接
	sessionMgr.connLock.Lock()
	sessionMgr.connMap[connID] = mockConn
	sessionMgr.connLock.Unlock()

	// 构造握手请求
	handshakeReq := &packet.HandshakeRequest{
		ClientID: 12345,
		Token:    "invalid-token", // 无效 token
		Version:  "1.0",
		Protocol: "tcp",
	}
	reqData, _ := json.Marshal(handshakeReq)

	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet: &packet.TransferPacket{
			PacketType: packet.Handshake,
			Payload:    reqData,
		},
	}

	// 处理握手（应该失败）
	err = sessionMgr.HandlePacket(streamPacket)
	if err == nil {
		t.Fatal("Expected handshake to fail, but it succeeded")
	}

	// 等待异步关闭完成（100ms 延迟 + 一些余量）
	time.Sleep(200 * time.Millisecond)

	// 验证连接已被关闭
	sessionMgr.connLock.RLock()
	_, exists := sessionMgr.connMap[connID]
	sessionMgr.connLock.RUnlock()

	if exists {
		t.Error("Expected connection to be closed after handshake failure, but it still exists")
	}
}

// mockAuthHandlerAlwaysFail 模拟总是失败的 AuthHandler
type mockAuthHandlerAlwaysFail struct{}

func (m *mockAuthHandlerAlwaysFail) HandleHandshake(conn ControlConnectionInterface, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	return nil, &mockAuthError{message: "authentication failed"}
}

func (m *mockAuthHandlerAlwaysFail) GetClientConfig(conn ControlConnectionInterface) (string, error) {
	return "", &mockAuthError{message: "not implemented"}
}

// mockAuthError 模拟认证错误
type mockAuthError struct {
	message string
}

func (e *mockAuthError) Error() string {
	return e.message
}

// mockStreamProcessor 模拟 StreamProcessor
type mockStreamProcessor struct {
	packets []*packet.TransferPacket
}

func (m *mockStreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	m.packets = append(m.packets, pkt)
	return 0, nil
}

func (m *mockStreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	return nil, 0, nil
}

func (m *mockStreamProcessor) ReadExact(length int) ([]byte, error) {
	return nil, nil
}

func (m *mockStreamProcessor) WriteExact(data []byte) error {
	return nil
}

func (m *mockStreamProcessor) GetReader() io.Reader {
	return nil
}

func (m *mockStreamProcessor) GetWriter() io.Writer {
	return nil
}

func (m *mockStreamProcessor) Close() {
}
