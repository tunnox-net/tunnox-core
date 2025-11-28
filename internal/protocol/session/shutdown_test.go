package session

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"

	"github.com/stretchr/testify/assert"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// BroadcastShutdown 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestBroadcastShutdown_NoConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 SessionManager
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)
	sessionMgr := NewSessionManager(idManager, ctx)

	// 测试无连接时的广播
	successCount, failureCount := sessionMgr.BroadcastShutdown(
		ShutdownReasonMaintenance,
		30,
		true,
		"Test shutdown",
	)

	assert.Equal(t, 0, successCount+failureCount, "Should have no connections to notify")
}

func TestBroadcastShutdown_WithConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 SessionManager
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)
	sessionMgr := NewSessionManager(idManager, ctx)

	// 创建测试用的控制连接
	conn1ID, _ := idManager.GenerateConnectionID()
	conn2ID, _ := idManager.GenerateConnectionID()

	// 创建mock stream
	mockStream1 := &mockWriteStream{packets: make([]interface{}, 0)}
	mockStream2 := &mockWriteStream{packets: make([]interface{}, 0)}

	// 注册控制连接
	sessionMgr.controlConnLock.Lock()
	sessionMgr.controlConnMap[conn1ID] = &ControlConnection{
		ConnID:   conn1ID,
		ClientID: 101,
		Stream:   mockStream1,
	}
	sessionMgr.clientIDIndexMap[101] = sessionMgr.controlConnMap[conn1ID]

	sessionMgr.controlConnMap[conn2ID] = &ControlConnection{
		ConnID:   conn2ID,
		ClientID: 102,
		Stream:   mockStream2,
	}
	sessionMgr.clientIDIndexMap[102] = sessionMgr.controlConnMap[conn2ID]
	sessionMgr.controlConnLock.Unlock()

	// 测试广播
	successCount, failureCount := sessionMgr.BroadcastShutdown(
		ShutdownReasonRollingUpdate,
		60,
		true,
		"Rolling update in progress",
	)

	// 等待异步发送完成
	time.Sleep(1 * time.Second)

	assert.Equal(t, 2, successCount+failureCount, "Should notify 2 connections")
	assert.GreaterOrEqual(t, mockStream1.PacketCount(), 0, "Stream1 should receive packets")
	assert.GreaterOrEqual(t, mockStream2.PacketCount(), 0, "Stream2 should receive packets")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GetActiveTunnelCount 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestGetActiveTunnelCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 SessionManager
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)
	sessionMgr := NewSessionManager(idManager, ctx)

	// 初始应该为0
	assert.Equal(t, 0, sessionMgr.GetActiveTunnelCount(), "Initial tunnel count should be 0")

	// 添加隧道连接
	tunnel1ID, _ := idManager.GenerateTunnelID()
	tunnel2ID, _ := idManager.GenerateTunnelID()
	conn1ID, _ := idManager.GenerateConnectionID()
	conn2ID, _ := idManager.GenerateConnectionID()

	sessionMgr.tunnelConnLock.Lock()
	sessionMgr.tunnelConnMap[conn1ID] = &TunnelConnection{
		ConnID:   conn1ID,
		TunnelID: tunnel1ID,
	}
	sessionMgr.tunnelIDMap[tunnel1ID] = sessionMgr.tunnelConnMap[conn1ID]

	sessionMgr.tunnelConnMap[conn2ID] = &TunnelConnection{
		ConnID:   conn2ID,
		TunnelID: tunnel2ID,
	}
	sessionMgr.tunnelIDMap[tunnel2ID] = sessionMgr.tunnelConnMap[conn2ID]
	sessionMgr.tunnelConnLock.Unlock()

	assert.Equal(t, 2, sessionMgr.GetActiveTunnelCount(), "Should have 2 active tunnels")

	// 移除一个隧道
	sessionMgr.tunnelConnLock.Lock()
	delete(sessionMgr.tunnelConnMap, conn1ID)
	delete(sessionMgr.tunnelIDMap, tunnel1ID)
	sessionMgr.tunnelConnLock.Unlock()

	assert.Equal(t, 1, sessionMgr.GetActiveTunnelCount(), "Should have 1 active tunnel")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// WaitForTunnelsToComplete 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestWaitForTunnelsToComplete_NoTunnels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 SessionManager
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)
	sessionMgr := NewSessionManager(idManager, ctx)

	// 无隧道时应该立即返回true
	result := sessionMgr.WaitForTunnelsToComplete(5)
	assert.True(t, result, "Should return true when no tunnels")
}

func TestWaitForTunnelsToComplete_WithTunnels_Timeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 SessionManager
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)
	sessionMgr := NewSessionManager(idManager, ctx)

	// 添加一个隧道
	tunnelID, _ := idManager.GenerateTunnelID()
	connID, _ := idManager.GenerateConnectionID()

	sessionMgr.tunnelConnLock.Lock()
	sessionMgr.tunnelConnMap[connID] = &TunnelConnection{
		ConnID:   connID,
		TunnelID: tunnelID,
	}
	sessionMgr.tunnelIDMap[tunnelID] = sessionMgr.tunnelConnMap[connID]
	sessionMgr.tunnelConnLock.Unlock()

	// 测试超时（2秒）
	start := time.Now()
	result := sessionMgr.WaitForTunnelsToComplete(2)
	elapsed := time.Since(start)

	assert.False(t, result, "Should return false due to timeout")
	assert.GreaterOrEqual(t, elapsed, 2*time.Second, "Should wait at least 2 seconds")
	assert.LessOrEqual(t, elapsed, 3*time.Second, "Should not wait much more than 2 seconds")
}

func TestWaitForTunnelsToComplete_WithTunnels_Complete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 SessionManager
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)
	sessionMgr := NewSessionManager(idManager, ctx)

	// 添加一个隧道
	tunnelID, _ := idManager.GenerateTunnelID()
	connID, _ := idManager.GenerateConnectionID()

	sessionMgr.tunnelConnLock.Lock()
	sessionMgr.tunnelConnMap[connID] = &TunnelConnection{
		ConnID:   connID,
		TunnelID: tunnelID,
	}
	sessionMgr.tunnelIDMap[tunnelID] = sessionMgr.tunnelConnMap[connID]
	sessionMgr.tunnelConnLock.Unlock()

	// 1秒后移除隧道（模拟完成）
	go func() {
		time.Sleep(1 * time.Second)
		sessionMgr.tunnelConnLock.Lock()
		delete(sessionMgr.tunnelConnMap, connID)
		delete(sessionMgr.tunnelIDMap, tunnelID)
		sessionMgr.tunnelConnLock.Unlock()
	}()

	// 测试等待完成
	start := time.Now()
	result := sessionMgr.WaitForTunnelsToComplete(5)
	elapsed := time.Since(start)

	assert.True(t, result, "Should return true when tunnels complete")
	assert.GreaterOrEqual(t, elapsed, 1*time.Second, "Should wait at least 1 second")
	assert.LessOrEqual(t, elapsed, 2*time.Second, "Should not wait much more than 1 second")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Mock Stream for Testing
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

type mockWriteStream struct {
	mu      sync.Mutex
	packets []interface{}
}

func (m *mockWriteStream) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.packets = append(m.packets, pkt)
	return 0, nil
}

func (m *mockWriteStream) PacketCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.packets)
}

// 实现 stream.PackageStreamer 接口的其他方法（即使测试中不使用）
func (m *mockWriteStream) ReadPacket() (*packet.TransferPacket, int, error) {
	return nil, 0, nil
}

func (m *mockWriteStream) ReadExact(length int) ([]byte, error) {
	return nil, nil
}

func (m *mockWriteStream) WriteExact(data []byte) error {
	return nil
}

func (m *mockWriteStream) Close() {
	// No-op
}

func (m *mockWriteStream) GetWriter() io.Writer {
	return nil
}

func (m *mockWriteStream) GetReader() io.Reader {
	return nil
}

func (m *mockWriteStream) GetConnectionID() string {
	return "mock-conn-id"
}

func (m *mockWriteStream) SendHeartbeat() error {
	return nil
}

func (m *mockWriteStream) GetLastHeartbeat() time.Time {
	return time.Now()
}

func (m *mockWriteStream) Stop() {}

// 确保实现 stream.PackageStreamer 接口
var _ stream.PackageStreamer = (*mockWriteStream)(nil)

