package bridge

import (
	"context"
	"testing"
	"time"
	pb "tunnox-core/api/proto/bridge"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMultiplexedConn 用于测试的模拟多路复用连接
type MockMultiplexedConn struct {
	sessions map[string]*ForwardSession
	closed   bool
}

func NewMockMultiplexedConn() *MockMultiplexedConn {
	return &MockMultiplexedConn{
		sessions: make(map[string]*ForwardSession),
	}
}

func (m *MockMultiplexedConn) RegisterSession(streamID string, session *ForwardSession) error {
	if m.closed {
		return assert.AnError
	}
	m.sessions[streamID] = session
	return nil
}

func (m *MockMultiplexedConn) UnregisterSession(streamID string) {
	delete(m.sessions, streamID)
}

func (m *MockMultiplexedConn) CanAcceptStream() bool {
	return !m.closed && len(m.sessions) < 100
}

func (m *MockMultiplexedConn) GetActiveStreams() int32 {
	return int32(len(m.sessions))
}

func (m *MockMultiplexedConn) IsIdle(maxIdleTime time.Duration) bool {
	return len(m.sessions) == 0
}

func (m *MockMultiplexedConn) GetTargetNodeID() string {
	return "mock-node"
}

func (m *MockMultiplexedConn) Close() error {
	m.closed = true
	return nil
}

func (m *MockMultiplexedConn) IsClosed() bool {
	return m.closed
}

func TestForwardSession_Creation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	metadata := &SessionMetadata{
		SourceClientID: 12345,
		TargetClientID: 67890,
		TargetHost:     "localhost",
		TargetPort:     8080,
		SourceNodeID:   "node-1",
		TargetNodeID:   "node-2",
		RequestID:      "test-request-123",
	}

	session := NewForwardSession(ctx, mockConn, metadata)
	require.NotNil(t, session)
	defer session.Close()

	assert.NotEmpty(t, session.StreamID())
	assert.Equal(t, metadata, session.GetMetadata())
	assert.True(t, session.IsActive())
}

func TestForwardSession_SendReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)
	defer session.Close()

	// 测试发送
	testData := []byte("test payload")
	err := session.Send(testData)
	assert.NoError(t, err)

	// 从发送通道读取数据包
	sendChan := session.getSendChannel()
	select {
	case packet := <-sendChan:
		assert.Equal(t, session.StreamID(), packet.StreamId)
		assert.Equal(t, pb.PacketType_STREAM_DATA, packet.Type)
		assert.Equal(t, testData, packet.Payload)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for sent packet")
	}
}

func TestForwardSession_DeliverPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)
	defer session.Close()

	// 模拟接收数据包
	testPayload := []byte("incoming data")
	packet := &pb.BridgePacket{
		StreamId:  session.StreamID(),
		Type:      pb.PacketType_STREAM_DATA,
		Payload:   testPayload,
		Timestamp: time.Now().UnixMilli(),
	}

	err := session.deliverPacket(packet)
	assert.NoError(t, err)

	// 接收数据
	receivedData, err := session.Receive()
	assert.NoError(t, err)
	assert.Equal(t, testPayload, receivedData)
}

func TestForwardSession_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)

	streamID := session.StreamID()

	// 验证会话已注册
	_, exists := mockConn.sessions[streamID]
	assert.True(t, exists)

	// 关闭会话
	err := session.Close()
	assert.NoError(t, err)

	// 验证会话已注销
	_, exists = mockConn.sessions[streamID]
	assert.False(t, exists)

	// 再次关闭应该不出错
	err = session.Close()
	assert.NoError(t, err)
}

func TestForwardSession_SendPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)
	defer session.Close()

	// 发送打开流数据包
	openPayload := []byte(`{"target":"localhost:3306"}`)
	err := session.SendPacket(pb.PacketType_STREAM_OPEN, openPayload)
	assert.NoError(t, err)

	// 验证数据包
	sendChan := session.getSendChannel()
	select {
	case packet := <-sendChan:
		assert.Equal(t, pb.PacketType_STREAM_OPEN, packet.Type)
		assert.Equal(t, openPayload, packet.Payload)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestForwardSession_ReceivePacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)
	defer session.Close()

	// 投递确认数据包
	ackPacket := &pb.BridgePacket{
		StreamId:  session.StreamID(),
		Type:      pb.PacketType_STREAM_ACK,
		Payload:   []byte("ack"),
		Timestamp: time.Now().UnixMilli(),
	}

	err := session.deliverPacket(ackPacket)
	assert.NoError(t, err)

	// 接收数据包
	receivedPacket, err := session.ReceivePacket()
	assert.NoError(t, err)
	assert.Equal(t, pb.PacketType_STREAM_ACK, receivedPacket.Type)
	assert.Equal(t, []byte("ack"), receivedPacket.Payload)
}

func TestForwardSession_UpdateLastActive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)
	defer session.Close()

	// 记录初始时间
	time.Sleep(50 * time.Millisecond)

	// 更新活跃时间
	session.UpdateLastActive()

	// 验证会话仍然活跃
	assert.True(t, session.IsActive())
}

func TestForwardSession_IsActive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockConn := NewMockMultiplexedConn()
	session := NewForwardSession(ctx, mockConn, nil)
	require.NotNil(t, session)
	defer session.Close()

	// 新创建的会话应该是活跃的
	assert.True(t, session.IsActive())

	// 发送数据后仍然活跃
	session.Send([]byte("test"))
	assert.True(t, session.IsActive())
}

