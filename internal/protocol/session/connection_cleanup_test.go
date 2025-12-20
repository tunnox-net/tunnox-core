package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/stream"
)

// TestConnectionLimit_EnforcesMaxConnections 验证连接数限制
func TestConnectionLimit_EnforcesMaxConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	config := &SessionConfig{
		MaxConnections:  2,
		CleanupInterval: 30 * time.Second,
	}
	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, config)
	defer sessionMgr.Close()

	// 创建两个连接应该成功
	conn1, err := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})
	require.NoError(t, err)
	assert.NotNil(t, conn1)

	conn2, err := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})
	require.NoError(t, err)
	assert.NotNil(t, conn2)

	// 第三个连接应该失败
	conn3, err := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})
	assert.Error(t, err)
	assert.Nil(t, conn3)
	assert.Contains(t, err.Error(), "connection limit reached")

	// 关闭一个连接后，应该可以创建新连接
	err = sessionMgr.CloseConnection(conn1.ID)
	require.NoError(t, err)

	conn4, err := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})
	require.NoError(t, err)
	assert.NotNil(t, conn4)
}

// TestControlConnectionLimit_EnforcesMaxControlConnections 验证控制连接数限制
func TestControlConnectionLimit_EnforcesMaxControlConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	config := &SessionConfig{
		MaxControlConnections: 2,
		CleanupInterval:       30 * time.Second,
	}
	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, config)
	defer sessionMgr.Close()

	// 创建流工厂
	streamFactory := stream.NewDefaultStreamFactory(ctx)
	mockStream1 := streamFactory.CreateStreamProcessor(&mockReader{}, &mockWriter{})
	mockStream2 := streamFactory.CreateStreamProcessor(&mockReader{}, &mockWriter{})
	mockStream3 := streamFactory.CreateStreamProcessor(&mockReader{}, &mockWriter{})

	// 注册两个控制连接应该成功
	conn1 := NewControlConnection("conn1", mockStream1, nil, "tcp")
	sessionMgr.RegisterControlConnection(conn1)

	conn2 := NewControlConnection("conn2", mockStream2, nil, "tcp")
	sessionMgr.RegisterControlConnection(conn2)

	// 第三个连接应该触发清理最旧的连接
	conn3 := NewControlConnection("conn3", mockStream3, nil, "tcp")
	sessionMgr.RegisterControlConnection(conn3)

	// 验证连接数不超过限制
	stats := sessionMgr.GetConnectionStats()
	assert.LessOrEqual(t, stats.ControlConnections, config.MaxControlConnections)
}

// TestConnectionStats_ReturnsCorrectCounts 验证连接统计信息
func TestConnectionStats_ReturnsCorrectCounts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	config := &SessionConfig{
		MaxConnections:        10,
		MaxControlConnections: 5,
		CleanupInterval:       30 * time.Second,
	}
	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, config)
	defer sessionMgr.Close()

	// 创建一些连接
	conn1, _ := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})
	conn2, _ := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})

	streamFactory := stream.NewDefaultStreamFactory(ctx)
	controlConn := NewControlConnection("control1", streamFactory.CreateStreamProcessor(&mockReader{}, &mockWriter{}), nil, "tcp")
	sessionMgr.RegisterControlConnection(controlConn)

	// 获取统计信息
	stats := sessionMgr.GetConnectionStats()
	assert.Equal(t, 2, stats.TotalConnections)
	assert.Equal(t, 1, stats.ControlConnections)
	assert.Equal(t, 0, stats.TunnelConnections)
	assert.Equal(t, 10, stats.MaxConnections)
	assert.Equal(t, 5, stats.MaxControlConnections)

	// 清理连接
	sessionMgr.CloseConnection(conn1.ID)
	sessionMgr.CloseConnection(conn2.ID)

	stats = sessionMgr.GetConnectionStats()
	assert.Equal(t, 0, stats.TotalConnections)
}

// TestCloseConnection_ReleasesAllResources 验证关闭连接时释放所有资源
func TestCloseConnection_ReleasesAllResources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, DefaultSessionConfig())
	defer sessionMgr.Close()

	// 创建连接
	conn, err := sessionMgr.CreateConnection(&mockReader{}, &mockWriter{})
	require.NoError(t, err)

	// 注册为控制连接
	streamFactory := stream.NewDefaultStreamFactory(ctx)
	controlConn := NewControlConnection(conn.ID, streamFactory.CreateStreamProcessor(&mockReader{}, &mockWriter{}), nil, "tcp")
	sessionMgr.RegisterControlConnection(controlConn)

	// 关闭连接
	err = sessionMgr.CloseConnection(conn.ID)
	require.NoError(t, err)

	// 验证连接已被移除
	_, exists := sessionMgr.GetConnection(conn.ID)
	assert.False(t, exists)

	// 验证控制连接已被移除
	controlConn2 := sessionMgr.GetControlConnection(conn.ID)
	assert.Nil(t, controlConn2)
}

// mockReader 模拟 Reader
type mockReader struct{}

func (m *mockReader) Read(p []byte) (n int, err error) {
	return 0, nil
}

// mockWriter 模拟 Writer
type mockWriter struct{}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
