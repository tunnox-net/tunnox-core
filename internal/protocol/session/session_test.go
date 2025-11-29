package session

import (
	"bytes"
	"context"
	"testing"
	"time"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestIDManager 创建测试用的ID管理器
func createTestIDManager() *idgen.IDManager {
	storage := storage.NewMemoryStorage(context.Background())
	return idgen.NewIDManager(storage, context.Background())
}

func TestNewSessionManager(t *testing.T) {
	// 创建新的会话管理器
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 验证初始状态
	activeCount := session.GetActiveConnections()
	assert.Equal(t, 0, activeCount, "Expected 0 active connections")
}

func TestSessionInitConnection(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
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

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// AcceptConnection 只是创建连接，不会自动注册，所以这里不检查连接数
	// 连接需要经过握手后才能注册为控制连接或隧道连接

	// 获取连接信息
	retrievedInfo, exists := session.GetStreamConnectionInfo(connInfo.ID)
	assert.True(t, exists, "Expected connection to exist")
	assert.Equal(t, connInfo.ID, retrievedInfo.ID, "Connection ID mismatch")
}

func TestSessionCloseConnection(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 创建测试数据
	var buf bytes.Buffer
	reader := &buf
	writer := &buf

	// 初始化连接
	connInfo, err := session.AcceptConnection(reader, writer)
	require.NoError(t, err, "Failed to initialize connection")

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// AcceptConnection 只是创建连接，不会自动注册，所以这里不检查连接数

	// 关闭连接
	err = session.CloseConnection(connInfo.ID)
	require.NoError(t, err, "Failed to close connection")

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// 由于连接未注册，关闭后连接数仍为0

	// 验证连接信息不存在
	_, exists := session.GetStreamConnectionInfo(connInfo.ID)
	assert.False(t, exists, "Expected connection to not exist after close")
}

func TestSessionHandlePacket(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 创建测试数据包
	testPacket := &types.StreamPacket{
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

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 创建多个连接
	var bufs []bytes.Buffer
	var connInfos []*types.StreamConnection

	for i := 0; i < 3; i++ {
		var buf bytes.Buffer
		bufs = append(bufs, buf)
		reader := &bufs[i]
		writer := &bufs[i]

		connInfo, err := session.AcceptConnection(reader, writer)
		require.NoError(t, err, "Failed to initialize connection %d", i)
		connInfos = append(connInfos, connInfo)
	}

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// AcceptConnection 只是创建连接，不会自动注册，所以这里不检查连接数

	// 关闭所有连接
	for _, connInfo := range connInfos {
		err := session.CloseConnection(connInfo.ID)
		require.NoError(t, err, "Failed to close connection %s", connInfo.ID)
	}

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// 由于连接未注册，关闭后连接数仍为0
}

func TestSessionCloseNonExistentConnection(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 尝试关闭不存在的连接
	// 注意：CloseConnection 不会返回错误，即使连接不存在也会成功
	err := session.CloseConnection("non_existent_conn")
	assert.NoError(t, err, "CloseConnection should not error even for non-existent connection")
}

func TestSessionConnectionIDGenerator(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 创建多个连接，验证ID生成器工作正常
	var buf bytes.Buffer
	reader := &buf
	writer := &buf

	connInfo1, err := session.AcceptConnection(reader, writer)
	require.NoError(t, err, "Failed to initialize first connection")
	assert.NotEmpty(t, connInfo1.ID, "Expected non-empty connection ID")

	connInfo2, err := session.AcceptConnection(reader, writer)
	require.NoError(t, err, "Failed to initialize second connection")
	assert.NotEmpty(t, connInfo2.ID, "Expected non-empty connection ID")

	// 验证两个连接ID不同
	assert.NotEqual(t, connInfo1.ID, connInfo2.ID, "Expected different connection IDs")
}

func TestSessionGetStreamManager(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())
	defer session.Close()

	// 获取流管理器
	streamMgr := session.GetStreamManager()
	assert.NotNil(t, streamMgr, "Expected non-nil stream manager")
}

func TestSessionDispose(t *testing.T) {
	// 创建会话
	idManager := createTestIDManager()
	defer idManager.Close()

	session := NewSessionManager(idManager, context.Background())

	// 创建一些连接
	var buf bytes.Buffer
	reader := &buf
	writer := &buf

	_, err := session.AcceptConnection(reader, writer)
	require.NoError(t, err, "Failed to initialize connection")

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// AcceptConnection 只是创建连接，不会自动注册，所以这里不检查连接数

	// 释放会话
	result := session.Close()
	require.False(t, result.HasErrors(), "Failed to dispose session: %v", result.Error())

	// 注意：GetActiveConnections() 只统计已注册的控制连接和隧道连接
	// 由于连接未注册，关闭后连接数仍为0
}
