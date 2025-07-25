package tests

import (
	"bytes"
	"context"
	"testing"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestIDManager 创建测试用的ID管理器
func createTestIDManager() *generators.IDManager {
	storage := storages.NewMemoryStorage(context.Background())
	return generators.NewIDManager(storage, context.Background())
}

func TestNewConnectionSession(t *testing.T) {
	// 创建新的连接会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 验证初始状态
	activeCount := session.GetActiveConnections()
	assert.Equal(t, 0, activeCount, "Expected 0 active connections")
}

func TestSessionInitConnection(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 创建测试数据
	var buf bytes.Buffer
	reader := &buf
	writer := &buf

	// 初始化连接
	connInfo, err := session.AcceptConnection(reader, writer)
	require.NoError(t, err, "Failed to initialize connection")

	// 验证连接信息
	assert.NotEmpty(t, connInfo.ID, "Expected non-empty connection ID")
	assert.NotNil(t, connInfo.Stream, "Expected non-nil stream")

	// 验证活跃连接数量
	activeCount := session.GetActiveConnections()
	assert.Equal(t, 1, activeCount, "Expected 1 active connection")

	// 获取连接信息
	retrievedInfo, exists := session.GetStreamConnectionInfo(connInfo.ID)
	assert.True(t, exists, "Expected connection to exist")
	assert.Equal(t, connInfo.ID, retrievedInfo.ID, "Connection ID mismatch")
}

func TestSessionCloseConnection(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 创建测试数据
	var buf bytes.Buffer
	reader := &buf
	writer := &buf

	// 初始化连接
	connInfo, err := session.AcceptConnection(reader, writer)
	require.NoError(t, err, "Failed to initialize connection")

	// 验证连接存在
	activeCount := session.GetActiveConnections()
	assert.Equal(t, 1, activeCount, "Expected 1 active connection")

	// 关闭连接
	err = session.CloseConnection(connInfo.ID)
	require.NoError(t, err, "Failed to close connection")

	// 验证连接已关闭
	activeCount = session.GetActiveConnections()
	assert.Equal(t, 0, activeCount, "Expected 0 active connections after close")

	// 验证连接信息不存在
	_, exists := session.GetStreamConnectionInfo(connInfo.ID)
	assert.False(t, exists, "Expected connection to not exist after close")
}

func TestSessionHandlePacket(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 创建测试数据包
	testPacket := &protocol.StreamPacket{
		ConnectionID: "test_conn_123",
		Packet: &packet.TransferPacket{
			PacketType: packet.Heartbeat,
		},
		Timestamp: time.Now(),
	}

	// 处理数据包（应该成功，因为没有连接依赖）
	err := session.HandlePacket(testPacket)
	assert.NoError(t, err, "Failed to handle packet")
}

func TestSessionMultipleConnections(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 创建多个连接
	var bufs []bytes.Buffer
	var connInfos []*protocol.StreamConnection

	for i := 0; i < 3; i++ {
		var buf bytes.Buffer
		bufs = append(bufs, buf)

		connInfo, err := session.AcceptConnection(&buf, &buf)
		require.NoError(t, err, "Failed to initialize connection %d", i)
		connInfos = append(connInfos, connInfo)
	}

	// 验证连接数量
	activeCount := session.GetActiveConnections()
	assert.Equal(t, 3, activeCount, "Expected 3 active connections")

	// 关闭部分连接
	err := session.CloseConnection(connInfos[0].ID)
	require.NoError(t, err, "Failed to close connection")

	// 验证剩余连接数量
	activeCount = session.GetActiveConnections()
	assert.Equal(t, 2, activeCount, "Expected 2 active connections after close")
}

func TestSessionCloseNonExistentConnection(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 尝试关闭不存在的连接
	err := session.CloseConnection("non_existent_connection")
	assert.Error(t, err, "Expected error when closing non-existent connection")
}

func TestSessionConnectionIDGenerator(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 创建多个连接，验证ID唯一性
	var buf bytes.Buffer
	connInfo1, err := session.AcceptConnection(&buf, &buf)
	require.NoError(t, err, "Failed to initialize first connection")

	connInfo2, err := session.AcceptConnection(&buf, &buf)
	require.NoError(t, err, "Failed to initialize second connection")

	// 验证ID唯一性
	assert.NotEqual(t, connInfo1.ID, connInfo2.ID, "Connection IDs should be unique")

	// 验证ID格式（应该以conn_开头）
	assert.Contains(t, connInfo1.ID, "conn_", "Connection ID should start with conn_")
	assert.Contains(t, connInfo2.ID, "conn_", "Connection ID should start with conn_")
}

func TestSessionGetStreamManager(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	// 获取流管理器
	streamMgr := session.GetStreamManager()
	assert.NotNil(t, streamMgr, "Expected non-nil stream manager")

	// 验证流管理器初始状态
	streamCount := streamMgr.GetStreamCount()
	assert.Equal(t, 0, streamCount, "Expected 0 streams initially")
}

func TestSessionDispose(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := protocol.NewConnectionSession(idManager, context.Background())

	// 创建一些连接
	var buf bytes.Buffer
	_, err := session.AcceptConnection(&buf, &buf)
	require.NoError(t, err, "Failed to initialize connection")

	// 验证连接存在
	activeCount := session.GetActiveConnections()
	assert.Equal(t, 1, activeCount, "Expected 1 active connection")

	// 关闭会话
	session.Close()

	// 验证所有连接都被清理
	activeCount = session.GetActiveConnections()
	assert.Equal(t, 0, activeCount, "Expected 0 active connections after dispose")
}
