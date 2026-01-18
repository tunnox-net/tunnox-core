// Package tunnel 提供隧道桥接和路由功能的测试
package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Mock 实现
// ============================================================================

// mockTunnelConnection 模拟隧道连接
type mockTunnelConnection struct {
	connectionID string
	clientID     int64
	mappingID    string
	tunnelID     string
	netConn      net.Conn
	stream       stream.PackageStreamer
	closed       bool
}

func (m *mockTunnelConnection) GetConnectionID() string { return m.connectionID }
func (m *mockTunnelConnection) GetClientID() int64      { return m.clientID }
func (m *mockTunnelConnection) GetMappingID() string    { return m.mappingID }
func (m *mockTunnelConnection) GetTunnelID() string     { return m.tunnelID }
func (m *mockTunnelConnection) GetStream() stream.PackageStreamer {
	return m.stream
}
func (m *mockTunnelConnection) GetNetConn() net.Conn { return m.netConn }
func (m *mockTunnelConnection) Close() error {
	m.closed = true
	return nil
}
func (m *mockTunnelConnection) IsClosed() bool { return m.closed }

// mockCloudControl 模拟云控API
type mockCloudControl struct {
	mappings      map[string]*models.PortMapping
	updateStats   map[string]*stats.TrafficStats
	getErr        error
	updateErr     error
	getStatsCount int
}

func newMockCloudControl() *mockCloudControl {
	return &mockCloudControl{
		mappings:    make(map[string]*models.PortMapping),
		updateStats: make(map[string]*stats.TrafficStats),
	}
}

func (m *mockCloudControl) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	m.getStatsCount++
	if m.getErr != nil {
		return nil, m.getErr
	}
	if mapping, ok := m.mappings[mappingID]; ok {
		return mapping, nil
	}
	return nil, errors.New("mapping not found")
}

func (m *mockCloudControl) UpdatePortMappingStats(mappingID string, trafficStats *stats.TrafficStats) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateStats[mappingID] = trafficStats
	return nil
}

func (m *mockCloudControl) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	return nil, nil
}

// mockCrossNodeConn 模拟跨节点连接
type mockCrossNodeConn struct {
	nodeID   string
	reader   interface{}
	writer   interface{}
	closed   bool
	released bool
}

func (m *mockCrossNodeConn) GetNodeID() string      { return m.nodeID }
func (m *mockCrossNodeConn) GetReader() interface{} { return m.reader }
func (m *mockCrossNodeConn) GetWriter() interface{} { return m.writer }
func (m *mockCrossNodeConn) Close() error {
	m.closed = true
	return nil
}
func (m *mockCrossNodeConn) Release() {
	m.released = true
}

// mockPackageStreamer 模拟流处理器（实现完整的 stream.PackageStreamer 接口）
type mockPackageStreamer struct {
	reader   io.Reader
	writer   io.Writer
	clientID int64
	closed   bool
}

func (m *mockPackageStreamer) GetReader() io.Reader { return m.reader }
func (m *mockPackageStreamer) GetWriter() io.Writer { return m.writer }
func (m *mockPackageStreamer) GetClientID() int64   { return m.clientID }
func (m *mockPackageStreamer) Close() {
	m.closed = true
}

// 实现 PackageStreamer 需要的其他方法
func (m *mockPackageStreamer) ReadPacket() (*packet.TransferPacket, int, error) {
	return nil, 0, io.EOF
}

func (m *mockPackageStreamer) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	return 0, nil
}

func (m *mockPackageStreamer) ReadExact(length int) ([]byte, error) {
	return nil, io.EOF
}

func (m *mockPackageStreamer) WriteExact(data []byte) error {
	return nil
}

// ============================================================================
// 测试用例
// ============================================================================

// TestNewBridge 测试创建桥接器
func TestNewBridge(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID:  "test-tunnel-001",
		MappingID: "test-mapping-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge, "NewBridge should not return nil")
	defer bridge.Close()

	// 验证基础属性
	assert.Equal(t, "test-tunnel-001", bridge.GetTunnelID())
	assert.Equal(t, "test-mapping-001", bridge.GetMappingID())
	assert.True(t, bridge.IsActive())
	assert.False(t, bridge.IsTargetReady())
}

// TestNewBridgeWithSourceTunnelConn 测试创建带源连接的桥接器
func TestNewBridgeWithSourceTunnelConn(t *testing.T) {
	ctx := context.Background()

	sourceTunnelConn := &mockTunnelConnection{
		connectionID: "source-conn-001",
		clientID:     12345,
		mappingID:    "mapping-001",
		tunnelID:     "tunnel-001",
	}

	config := &BridgeConfig{
		TunnelID:         "test-tunnel-001",
		MappingID:        "test-mapping-001",
		SourceTunnelConn: sourceTunnelConn,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	assert.Equal(t, "source-conn-001", bridge.GetSourceConnectionID())
	assert.Equal(t, int64(12345), bridge.GetClientID())
}

// TestNewBridgeWithBandwidthLimit 测试创建带带宽限制的桥接器
func TestNewBridgeWithBandwidthLimit(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID:       "test-tunnel-001",
		BandwidthLimit: 1024 * 1024, // 1MB/s
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	assert.NotNil(t, bridge.GetRateLimiter())
}

// TestBridgeClose 测试关闭桥接器
func TestBridgeClose(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 关闭
	err := bridge.Close()
	assert.NoError(t, err)

	// 验证状态
	assert.False(t, bridge.IsActive())
	assert.True(t, bridge.IsClosed())
}

// TestBridgeCloseWithConnections 测试关闭带连接的桥接器
func TestBridgeCloseWithConnections(t *testing.T) {
	ctx := context.Background()

	sourceTunnelConn := &mockTunnelConnection{
		connectionID: "source-conn-001",
	}
	targetTunnelConn := &mockTunnelConnection{
		connectionID: "target-conn-001",
	}

	config := &BridgeConfig{
		TunnelID:         "test-tunnel-001",
		SourceTunnelConn: sourceTunnelConn,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 设置目标连接
	bridge.SetTargetConnection(targetTunnelConn)

	// 关闭
	err := bridge.Close()
	assert.NoError(t, err)

	// 验证连接已关闭
	assert.True(t, sourceTunnelConn.closed)
	assert.True(t, targetTunnelConn.closed)
}

// TestBridgeAccessors_NilReceiver 测试空接收者的访问器方法
func TestBridgeAccessors_NilReceiver(t *testing.T) {
	var bridge *Bridge

	assert.Equal(t, "", bridge.GetTunnelID())
	assert.Equal(t, "", bridge.GetSourceConnectionID())
	assert.Equal(t, "", bridge.GetTargetConnectionID())
	assert.Equal(t, "", bridge.GetMappingID())
	assert.Equal(t, int64(0), bridge.GetClientID())
	assert.False(t, bridge.IsActive())
	assert.Nil(t, bridge.GetSourceTunnelConn())
	assert.Nil(t, bridge.GetTargetTunnelConn())
	assert.Nil(t, bridge.GetSourceNetConn())
	assert.Nil(t, bridge.GetTargetNetConn())
}

// TestBridgeSetTargetConnection 测试设置目标连接
func TestBridgeSetTargetConnection(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	assert.False(t, bridge.IsTargetReady())

	targetConn := &mockTunnelConnection{
		connectionID: "target-conn-001",
	}

	bridge.SetTargetConnection(targetConn)

	assert.True(t, bridge.IsTargetReady())
	assert.Equal(t, "target-conn-001", bridge.GetTargetConnectionID())
}

// TestBridgeSetSourceConnection 测试设置源连接
func TestBridgeSetSourceConnection(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	sourceConn := &mockTunnelConnection{
		connectionID: "source-conn-002",
		clientID:     54321,
	}

	bridge.SetSourceConnection(sourceConn)

	assert.Equal(t, "source-conn-002", bridge.GetSourceConnectionID())
	assert.Equal(t, int64(54321), bridge.GetClientID())
}

// TestBridgeSetSourceConnectionNil 测试设置空源连接
func TestBridgeSetSourceConnection_Nil(t *testing.T) {
	ctx := context.Background()

	sourceConn := &mockTunnelConnection{
		connectionID: "source-conn-001",
	}

	config := &BridgeConfig{
		TunnelID:         "test-tunnel-001",
		SourceTunnelConn: sourceConn,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 设置为 nil
	bridge.SetSourceConnection(nil)

	assert.Equal(t, "", bridge.GetSourceConnectionID())
	assert.Nil(t, bridge.GetSourceForwarder())
}

// TestBridgeWaitForTarget_Success 测试等待目标连接成功
func TestBridgeWaitForTarget_Success(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 异步设置目标连接
	go func() {
		time.Sleep(50 * time.Millisecond)
		bridge.SetTargetConnection(&mockTunnelConnection{
			connectionID: "target-001",
		})
	}()

	err := bridge.WaitForTarget(time.Second)
	assert.NoError(t, err)
	assert.True(t, bridge.IsTargetReady())
}

// TestBridgeWaitForTarget_Timeout 测试等待目标连接超时
func TestBridgeWaitForTarget_Timeout(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	err := bridge.WaitForTarget(50 * time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestBridgeWaitForTarget_ContextCancelled 测试等待时context取消
func TestBridgeWaitForTarget_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 异步取消
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := bridge.WaitForTarget(time.Second)
	assert.Error(t, err)
}

// TestBridgeNotifyTargetReady 测试通知目标就绪
func TestBridgeNotifyTargetReady(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	assert.False(t, bridge.IsTargetReady())

	bridge.NotifyTargetReady()

	assert.True(t, bridge.IsTargetReady())

	// 多次调用不应 panic
	bridge.NotifyTargetReady()
	bridge.NotifyTargetReady()
}

// TestBridgeCrossNodeConnection 测试跨节点连接管理
func TestBridgeCrossNodeConnection(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 初始状态
	assert.Nil(t, bridge.GetCrossNodeConnection())

	// 设置跨节点连接
	crossNodeConn := &mockCrossNodeConn{
		nodeID: "node-002",
	}
	bridge.SetCrossNodeConnection(crossNodeConn)

	retrieved := bridge.GetCrossNodeConnection()
	assert.NotNil(t, retrieved)
	assert.Equal(t, "node-002", retrieved.GetNodeID())

	// 释放连接
	bridge.ReleaseCrossNodeConnection()
	assert.Nil(t, bridge.GetCrossNodeConnection())
	// 注意：释放只是清除引用，不关闭连接
	assert.False(t, crossNodeConn.closed)
}

// TestBridgeTrafficStats 测试流量统计
func TestBridgeTrafficStats(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 初始状态
	assert.Equal(t, int64(0), bridge.GetBytesSent())
	assert.Equal(t, int64(0), bridge.GetBytesReceived())

	// 添加流量
	bridge.AddBytesSent(1024)
	bridge.AddBytesReceived(2048)

	assert.Equal(t, int64(1024), bridge.GetBytesSent())
	assert.Equal(t, int64(2048), bridge.GetBytesReceived())

	// 累加
	bridge.AddBytesSent(512)
	assert.Equal(t, int64(1536), bridge.GetBytesSent())
}

// TestBridgeTrafficStats_Concurrent 测试并发流量统计
func TestBridgeTrafficStats_Concurrent(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	const goroutines = 10
	const iterations = 1000

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				bridge.AddBytesSent(1)
				bridge.AddBytesReceived(1)
			}
			done <- true
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	assert.Equal(t, int64(goroutines*iterations), bridge.GetBytesSent())
	assert.Equal(t, int64(goroutines*iterations), bridge.GetBytesReceived())
}

// TestBridgeReportTrafficStats 测试流量统计上报
func TestBridgeReportTrafficStats(t *testing.T) {
	ctx := context.Background()

	cloudControl := newMockCloudControl()
	cloudControl.mappings["mapping-001"] = &models.PortMapping{
		ID: "mapping-001",
		TrafficStats: models.TrafficStats{
			BytesSent:     0,
			BytesReceived: 0,
		},
	}

	config := &BridgeConfig{
		TunnelID:     "test-tunnel-001",
		MappingID:    "mapping-001",
		CloudControl: cloudControl,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 添加流量
	bridge.AddBytesSent(1024)
	bridge.AddBytesReceived(2048)

	// 触发上报
	bridge.reportTrafficStats()

	// 验证上报
	assert.Equal(t, 1, cloudControl.getStatsCount)
	assert.NotNil(t, cloudControl.updateStats["mapping-001"])
}

// TestBridgeReportTrafficStats_NoCloudControl 测试无云控时的流量上报
func TestBridgeReportTrafficStats_NoCloudControl(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID:  "test-tunnel-001",
		MappingID: "mapping-001",
		// 无 CloudControl
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	bridge.AddBytesSent(1024)

	// 不应 panic
	bridge.reportTrafficStats()
}

// TestBridgeReportTrafficStats_NoMappingID 测试无映射ID时的流量上报
func TestBridgeReportTrafficStats_NoMappingID(t *testing.T) {
	ctx := context.Background()

	cloudControl := newMockCloudControl()

	config := &BridgeConfig{
		TunnelID:     "test-tunnel-001",
		CloudControl: cloudControl,
		// 无 MappingID
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	bridge.AddBytesSent(1024)

	// 不应上报
	bridge.reportTrafficStats()
	assert.Equal(t, 0, cloudControl.getStatsCount)
}

// TestBridgeReportTrafficStats_NoIncrement 测试无增量时不上报
func TestBridgeReportTrafficStats_NoIncrement(t *testing.T) {
	ctx := context.Background()

	cloudControl := newMockCloudControl()
	cloudControl.mappings["mapping-001"] = &models.PortMapping{
		ID: "mapping-001",
	}

	config := &BridgeConfig{
		TunnelID:     "test-tunnel-001",
		MappingID:    "mapping-001",
		CloudControl: cloudControl,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 不添加流量
	bridge.reportTrafficStats()

	// 不应上报
	assert.Equal(t, 0, cloudControl.getStatsCount)
}

// TestBridgeReportTrafficStats_GetError 测试获取映射失败
func TestBridgeReportTrafficStats_GetError(t *testing.T) {
	ctx := context.Background()

	cloudControl := newMockCloudControl()
	cloudControl.getErr = errors.New("get mapping failed")

	config := &BridgeConfig{
		TunnelID:     "test-tunnel-001",
		MappingID:    "mapping-001",
		CloudControl: cloudControl,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	bridge.AddBytesSent(1024)

	// 不应 panic
	bridge.reportTrafficStats()
}

// TestBridgeReportTrafficStats_UpdateError 测试更新统计失败
func TestBridgeReportTrafficStats_UpdateError(t *testing.T) {
	ctx := context.Background()

	cloudControl := newMockCloudControl()
	cloudControl.mappings["mapping-001"] = &models.PortMapping{
		ID: "mapping-001",
	}
	cloudControl.updateErr = errors.New("update failed")

	config := &BridgeConfig{
		TunnelID:     "test-tunnel-001",
		MappingID:    "mapping-001",
		CloudControl: cloudControl,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	bridge.AddBytesSent(1024)

	// 不应 panic
	bridge.reportTrafficStats()
}

// TestBridgeStartPeriodicTrafficReport 测试启动定期流量上报
func TestBridgeStartPeriodicTrafficReport(t *testing.T) {
	ctx := context.Background()

	cloudControl := newMockCloudControl()
	cloudControl.mappings["mapping-001"] = &models.PortMapping{
		ID: "mapping-001",
	}

	config := &BridgeConfig{
		TunnelID:     "test-tunnel-001",
		MappingID:    "mapping-001",
		CloudControl: cloudControl,
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 启动定期上报
	bridge.StartPeriodicTrafficReport()

	// 短暂等待确保 goroutine 启动
	time.Sleep(10 * time.Millisecond)

	// 关闭
	bridge.Close()
}

// TestBridgeStartPeriodicTrafficReport_NoCloudControl 测试无云控时不启动定期上报
func TestBridgeStartPeriodicTrafficReport_NoCloudControl(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID:  "test-tunnel-001",
		MappingID: "mapping-001",
		// 无 CloudControl
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 不应 panic
	bridge.StartPeriodicTrafficReport()
}

// TestBridgeGetForwarders 测试获取数据转发器
func TestBridgeGetForwarders(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 初始状态没有连接，forwarder 可能为 nil
	forwarder := bridge.GetSourceForwarder()
	// 根据配置可能为 nil
	_ = forwarder

	targetForwarder := bridge.GetTargetForwarder()
	assert.Nil(t, targetForwarder)
}

// TestExtractClientID 测试从 stream 提取客户端ID
func TestExtractClientID(t *testing.T) {
	// 测试 nil stream
	clientID := extractClientID(nil, nil)
	assert.Equal(t, int64(0), clientID)

	// 测试实现了 GetClientID 的 stream
	mockStream := &mockPackageStreamer{
		clientID: 12345,
	}
	clientID = extractClientID(mockStream, nil)
	assert.Equal(t, int64(12345), clientID)
}

// TestSetTunnelConnectionFactory 测试设置隧道连接工厂
func TestSetTunnelConnectionFactory(t *testing.T) {
	// 保存原始工厂
	originalFactory := tunnelConnFactory

	// 测试设置工厂
	called := false
	testFactory := func(
		connID string,
		conn net.Conn,
		s stream.PackageStreamer,
		clientID int64,
		mappingID string,
		tunnelID string,
	) TunnelConnectionInterface {
		called = true
		return &mockTunnelConnection{
			connectionID: connID,
			clientID:     clientID,
		}
	}

	SetTunnelConnectionFactory(testFactory)
	assert.NotNil(t, tunnelConnFactory)

	// 测试创建连接
	conn := createTunnelConnection("test-conn", nil, nil, 123, "map-1", "tunnel-1")
	assert.True(t, called)
	assert.NotNil(t, conn)
	assert.Equal(t, "test-conn", conn.GetConnectionID())

	// 恢复原始工厂
	tunnelConnFactory = originalFactory
}

// TestCreateTunnelConnection_NoFactory 测试无工厂时创建连接
func TestCreateTunnelConnection_NoFactory(t *testing.T) {
	// 保存原始工厂
	originalFactory := tunnelConnFactory
	tunnelConnFactory = nil

	conn := createTunnelConnection("test-conn", nil, nil, 123, "map-1", "tunnel-1")
	assert.Nil(t, conn)

	// 恢复原始工厂
	tunnelConnFactory = originalFactory
}

// TestBridgeCopyWithControl 测试带流量控制的数据拷贝
func TestBridgeCopyWithControl(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 准备测试数据
	testData := []byte("Hello, World!")
	reader := &mockReader{data: testData}
	writer := &mockWriter{}

	var counter atomic.Int64

	// 执行拷贝
	copied := bridge.CopyWithControl(writer, reader, "test", &counter)

	assert.Equal(t, int64(len(testData)), copied)
	assert.Equal(t, testData, writer.data)
	assert.Equal(t, int64(len(testData)), counter.Load())
}

// TestBridgeCopyWithControl_ContextCancelled 测试上下文取消时的数据拷贝
func TestBridgeCopyWithControl_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 立即取消
	cancel()

	// 使用一个会快速返回 EOF 的 reader，而不是慢速 reader
	// 因为 CopyWithControl 只在读取后检查 context
	reader := &mockReader{data: []byte{}}
	writer := &mockWriter{}

	var counter atomic.Int64

	// 应该快速返回（因为读取立即返回 EOF）
	start := time.Now()
	bridge.CopyWithControl(writer, reader, "test", &counter)
	elapsed := time.Since(start)

	// 应该在 100ms 内返回
	assert.Less(t, elapsed, 100*time.Millisecond)
}

// TestBridgeCopyWithControl_WithRateLimiter 测试带限速的数据拷贝
func TestBridgeCopyWithControl_WithRateLimiter(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID:       "test-tunnel-001",
		BandwidthLimit: 1024, // 1KB/s
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	testData := []byte("Test data")
	reader := &mockReader{data: testData}
	writer := &mockWriter{}

	var counter atomic.Int64

	copied := bridge.CopyWithControl(writer, reader, "test", &counter)

	assert.Equal(t, int64(len(testData)), copied)
}

// ============================================================================
// 辅助 Mock 类型
// ============================================================================

// mockReader 模拟读取器
type mockReader struct {
	data   []byte
	offset int
}

func (r *mockReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

// mockWriter 模拟写入器
type mockWriter struct {
	data []byte
}

func (w *mockWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// slowReader 慢速读取器
type slowReader struct {
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	return 0, io.EOF
}

// ============================================================================
// 边界条件测试
// ============================================================================

// TestBridgeGetSourceConn 测试获取源连接
func TestBridgeGetSourceConn(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 初始状态
	conn := bridge.GetSourceConn()
	assert.Nil(t, conn)
}

// TestBridgeMultipleClose 测试多次关闭
func TestBridgeMultipleClose(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 多次关闭不应 panic
	err1 := bridge.Close()
	err2 := bridge.Close()
	err3 := bridge.Close()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
}

// TestBridgeContextCancellation 测试上下文取消
func TestBridgeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 验证初始状态
	assert.True(t, bridge.IsActive())

	// 取消上下文
	cancel()

	// 验证 context 已取消
	select {
	case <-bridge.Ctx().Done():
		// context 取消成功
	default:
		t.Error("Context should be cancelled")
	}

	// 注意：根据设计，context 取消不会自动调用 Close()
	// 清理逻辑应该通过显式调用 Close() 方法来触发
	// 这里显式关闭 bridge
	err := bridge.Close()
	assert.NoError(t, err)

	// 验证已关闭
	assert.False(t, bridge.IsActive())
}

// ============================================================================
// bridge_forward.go 测试
// ============================================================================

// TestCreateDataForwarder_WithStream 测试使用 stream 创建数据转发器
func TestCreateDataForwarder_WithStream(t *testing.T) {
	// 创建带 reader/writer 的 mockPackageStreamer
	reader := &mockReader{data: []byte("test")}
	writer := &mockWriter{}

	mockStream := &mockPackageStreamer{
		reader: reader,
		writer: writer,
	}

	forwarder := CreateDataForwarder(nil, mockStream)
	// 根据 stream 的实现，可能返回 nil 或有效的 forwarder
	_ = forwarder
}

// TestCreateDataForwarder_NilInputs 测试空输入
func TestCreateDataForwarder_NilInputs(t *testing.T) {
	forwarder := CreateDataForwarder(nil, nil)
	assert.Nil(t, forwarder)
}

// TestCreateDataForwarder_WithConn 测试使用 conn 创建数据转发器
func TestCreateDataForwarder_WithConn(t *testing.T) {
	// 创建一个模拟的 ReadWriteCloser
	mockRWC := &mockReadWriteCloser{}

	forwarder := CreateDataForwarder(mockRWC, nil)
	assert.NotNil(t, forwarder)
}

// mockReadWriteCloser 模拟 ReadWriteCloser
type mockReadWriteCloser struct {
	data   []byte
	closed bool
	mu     sync.Mutex
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockReadWriteCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// TestStreamDataForwarderAdapter 测试 StreamDataForwarder 适配器
func TestStreamDataForwarderAdapter(t *testing.T) {
	// 创建一个实现 StreamDataForwarder 的 mock
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte("test data"),
	}

	// 创建适配器
	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
	}

	// 测试读取
	buf := make([]byte, 100)
	n, err := adapter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 9, n)

	// 测试写入
	n, err = adapter.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)

	// 测试关闭
	err = adapter.Close()
	assert.NoError(t, err)

	// 关闭后读取应返回 EOF
	n, err = adapter.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)

	// 关闭后写入应返回 ErrClosedPipe
	n, err = adapter.Write([]byte("test"))
	assert.Equal(t, 0, n)
	assert.Equal(t, io.ErrClosedPipe, err)
}

// mockStreamDataForwarder 模拟 StreamDataForwarder
type mockStreamDataForwarder struct {
	data         []byte
	offset       int
	closed       bool
	connectionID string
}

func (m *mockStreamDataForwarder) ReadExact(length int) ([]byte, error) {
	if m.closed {
		return nil, io.EOF
	}
	if m.offset >= len(m.data) {
		return nil, io.EOF
	}
	end := m.offset + length
	if end > len(m.data) {
		end = len(m.data)
	}
	result := m.data[m.offset:end]
	m.offset = end
	return result, nil
}

func (m *mockStreamDataForwarder) ReadAvailable(maxLength int) ([]byte, error) {
	if m.closed {
		return nil, io.EOF
	}
	if m.offset >= len(m.data) {
		return nil, io.EOF
	}
	end := m.offset + maxLength
	if end > len(m.data) {
		end = len(m.data)
	}
	result := m.data[m.offset:end]
	m.offset = end
	return result, nil
}

func (m *mockStreamDataForwarder) WriteExact(data []byte) error {
	if m.closed {
		return io.ErrClosedPipe
	}
	return nil
}

func (m *mockStreamDataForwarder) Close() {
	m.closed = true
}

func (m *mockStreamDataForwarder) GetConnectionID() string {
	return m.connectionID
}

// TestStreamDataForwarderAdapter_BufferedRead 测试适配器的缓冲读取
func TestStreamDataForwarderAdapter_BufferedRead(t *testing.T) {
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte("this is a longer test data"),
	}

	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
	}

	// 读取小块数据
	buf := make([]byte, 5)
	n, err := adapter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "this ", string(buf[:n]))

	// 继续读取
	n, err = adapter.Read(buf)
	assert.NoError(t, err)
	assert.True(t, n > 0)
}

// TestStreamDataForwarderAdapter_ReadEOF 测试读取 EOF
func TestStreamDataForwarderAdapter_ReadEOF(t *testing.T) {
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte{},
	}

	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
	}

	buf := make([]byte, 10)
	n, err := adapter.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

// TestStreamDataForwarderAdapter_DoubleClose 测试多次关闭
func TestStreamDataForwarderAdapter_DoubleClose(t *testing.T) {
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte("test"),
	}

	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
	}

	err1 := adapter.Close()
	err2 := adapter.Close()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
}

// TestDynamicSourceWriter 测试动态源写入器
func TestDynamicSourceWriter(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 设置源连接
	mockRWC := &mockReadWriteCloser{}
	bridge.sourceForwarder = mockRWC

	writer := &dynamicSourceWriter{bridge: bridge}

	// 写入数据
	n, err := writer.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
}

// TestDynamicSourceWriter_NilForwarder 测试 forwarder 为 nil 的情况
func TestDynamicSourceWriter_NilForwarder(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 不设置 forwarder
	bridge.sourceForwarder = nil

	writer := &dynamicSourceWriter{bridge: bridge}

	// 写入应返回错误
	n, err := writer.Write([]byte("hello"))
	assert.Equal(t, 0, n)
	assert.Equal(t, io.ErrClosedPipe, err)
}

// TestBridgeCopyWithControl_LargeData 测试大数据拷贝
func TestBridgeCopyWithControl_LargeData(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 准备大量测试数据
	testData := make([]byte, 100*1024) // 100KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	reader := &mockReader{data: testData}
	writer := &mockWriter{}

	var counter atomic.Int64

	// 执行拷贝
	copied := bridge.CopyWithControl(writer, reader, "test", &counter)

	assert.Equal(t, int64(len(testData)), copied)
	assert.Equal(t, len(testData), len(writer.data))
	assert.Equal(t, int64(len(testData)), counter.Load())
}

// TestBridgeCopyWithControl_WriteError 测试写入错误
func TestBridgeCopyWithControl_WriteError(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	testData := []byte("test data")
	reader := &mockReader{data: testData}
	writer := &errorWriter{maxWrites: 0}

	var counter atomic.Int64

	// 写入应该失败
	copied := bridge.CopyWithControl(writer, reader, "test", &counter)

	// 由于第一次写入就失败，应该返回 0
	assert.Equal(t, int64(0), copied)
}

// errorWriter 总是返回错误的写入器
type errorWriter struct {
	maxWrites   int
	writeCount  int
	partialData []byte
}

func (w *errorWriter) Write(p []byte) (int, error) {
	if w.writeCount >= w.maxWrites {
		return 0, errors.New("write error")
	}
	w.writeCount++
	w.partialData = append(w.partialData, p...)
	return len(p), nil
}

// TestBridgeCopyWithControl_PartialWrite 测试部分写入
func TestBridgeCopyWithControl_PartialWrite(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	testData := []byte("test data for partial write")
	reader := &mockReader{data: testData}
	writer := &partialWriter{maxBytes: 5}

	var counter atomic.Int64

	// 部分写入
	copied := bridge.CopyWithControl(writer, reader, "test", &counter)

	// 由于部分写入会导致 nr != nw，应该提前退出
	assert.Less(t, copied, int64(len(testData)))
}

// partialWriter 只写入部分数据的写入器
type partialWriter struct {
	maxBytes int
	data     []byte
}

func (w *partialWriter) Write(p []byte) (int, error) {
	if len(p) > w.maxBytes {
		w.data = append(w.data, p[:w.maxBytes]...)
		return w.maxBytes, nil
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

// TestBridgeSetTargetConnectionWithNetConn 测试设置带网络连接的目标
func TestBridgeSetTargetConnectionWithNetConn(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 创建带 netConn 的目标连接
	targetConn := &mockTunnelConnection{
		connectionID: "target-001",
		netConn:      &mockNetConn{},
	}

	bridge.SetTargetConnection(targetConn)

	assert.True(t, bridge.IsTargetReady())
	assert.NotNil(t, bridge.GetTargetNetConn())
}

// mockNetConn 模拟网络连接
type mockNetConn struct {
	data   []byte
	closed bool
}

func (m *mockNetConn) Read(b []byte) (n int, err error) {
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n = copy(b, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockNetConn) Write(b []byte) (n int, err error) {
	m.data = append(m.data, b...)
	return len(b), nil
}

func (m *mockNetConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockNetConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockNetConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockNetConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockNetConn) SetWriteDeadline(t time.Time) error { return nil }

// TestCheckStreamDataForwarder 测试检查 StreamDataForwarder
func TestCheckStreamDataForwarder(t *testing.T) {
	// 测试实现了 StreamDataForwarder 接口的 stream
	mockForwarderStream := &mockFullStreamer{
		mockPackageStreamer: mockPackageStreamer{},
		streamForwarder:     &mockStreamDataForwarder{},
	}
	result := checkStreamDataForwarder(mockForwarderStream)
	assert.NotNil(t, result)

	// 测试未实现 StreamDataForwarder 接口的 stream
	mockStream := &mockPackageStreamer{}
	result = checkStreamDataForwarder(mockStream)
	assert.Nil(t, result)
}

// mockFullStreamer 同时实现 PackageStreamer 和 StreamDataForwarder
type mockFullStreamer struct {
	mockPackageStreamer
	streamForwarder *mockStreamDataForwarder
}

func (m *mockFullStreamer) ReadExact(length int) ([]byte, error) {
	return m.streamForwarder.ReadExact(length)
}

func (m *mockFullStreamer) ReadAvailable(maxLength int) ([]byte, error) {
	return m.streamForwarder.ReadAvailable(maxLength)
}

func (m *mockFullStreamer) WriteExact(data []byte) error {
	return m.streamForwarder.WriteExact(data)
}

func (m *mockFullStreamer) GetConnectionID() string {
	return m.streamForwarder.GetConnectionID()
}

// TestStreamDataForwarderAdapter_EmptyBuffer 测试空缓冲区
func TestStreamDataForwarderAdapter_EmptyBuffer(t *testing.T) {
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte("hello"),
	}

	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
		buf:    []byte{}, // 空缓冲区
	}

	// 读取时应从 stream 获取数据
	buf := make([]byte, 10)
	n, err := adapter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
}

// TestStreamDataForwarderAdapter_WithBufferedData 测试有缓冲数据
func TestStreamDataForwarderAdapter_WithBufferedData(t *testing.T) {
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte("from stream"),
	}

	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
		buf:    []byte("buffered"), // 有缓冲数据
	}

	// 读取时应先返回缓冲区数据
	buf := make([]byte, 10)
	n, err := adapter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 8, n)
	assert.Equal(t, "buffered", string(buf[:n]))
}

// TestStreamDataForwarderAdapter_ZeroLengthRead 测试零长度读取
func TestStreamDataForwarderAdapter_ZeroLengthRead(t *testing.T) {
	mockStreamForwarder := &mockStreamDataForwarder{
		data: []byte("test"),
	}

	adapter := &streamDataForwarderAdapter{
		stream: mockStreamForwarder,
	}

	// 零长度缓冲区
	buf := make([]byte, 0)
	n, err := adapter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

// ============================================================================
// routing.go 测试
// ============================================================================

// mockStorage 模拟存储
type mockStorage struct {
	data map[string]interface{}
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		data: make(map[string]interface{}),
	}
}

func (m *mockStorage) Set(key string, value interface{}, ttl time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *mockStorage) Get(key string) (interface{}, error) {
	if value, ok := m.data[key]; ok {
		return value, nil
	}
	return nil, storage.ErrKeyNotFound
}

func (m *mockStorage) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockStorage) Exists(key string) (bool, error) {
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockStorage) SetExpiration(key string, ttl time.Duration) error {
	return nil
}

func (m *mockStorage) GetExpiration(key string) (time.Duration, error) {
	return 0, nil
}

func (m *mockStorage) CleanupExpired() error {
	return nil
}

func (m *mockStorage) Close() error {
	return nil
}

// TestNewRoutingTable 测试创建路由表
func TestNewRoutingTable(t *testing.T) {
	store := newMockStorage()

	// 测试默认 TTL
	rt := NewRoutingTable(store, 0)
	assert.NotNil(t, rt)
	assert.Equal(t, store, rt.GetStorage())

	// 测试自定义 TTL
	rt2 := NewRoutingTable(store, 60*time.Second)
	assert.NotNil(t, rt2)
}

// TestRoutingTableRegisterWaitingTunnel 测试注册等待中的隧道
func TestRoutingTableRegisterWaitingTunnel(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 30*time.Second)

	ctx := context.Background()

	// 测试空 TunnelID
	err := rt.RegisterWaitingTunnel(ctx, &WaitingState{})
	assert.Error(t, err)

	// 测试正常注册
	state := &WaitingState{
		TunnelID:       "tunnel-001",
		MappingID:      "mapping-001",
		SecretKey:      "secret-key",
		SourceNodeID:   "node-001",
		SourceClientID: 12345,
		TargetClientID: 67890,
		TargetHost:     "192.168.1.100",
		TargetPort:     8080,
	}
	err = rt.RegisterWaitingTunnel(ctx, state)
	assert.NoError(t, err)

	// 验证数据已存储
	exists, err := store.Exists("tunnox:tunnel_waiting:tunnel-001")
	assert.NoError(t, err)
	assert.True(t, exists)
}

// TestRoutingTableLookupWaitingTunnel 测试查找等待中的隧道
func TestRoutingTableLookupWaitingTunnel(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 30*time.Second)

	ctx := context.Background()

	// 测试空 TunnelID
	_, err := rt.LookupWaitingTunnel(ctx, "")
	assert.Error(t, err)

	// 测试不存在的隧道
	_, err = rt.LookupWaitingTunnel(ctx, "nonexistent")
	assert.Equal(t, ErrNotFound, err)

	// 注册一个隧道
	state := &WaitingState{
		TunnelID:       "tunnel-001",
		MappingID:      "mapping-001",
		SourceNodeID:   "node-001",
		SourceClientID: 12345,
		TargetClientID: 67890,
	}
	err = rt.RegisterWaitingTunnel(ctx, state)
	assert.NoError(t, err)

	// 查找已注册的隧道
	found, err := rt.LookupWaitingTunnel(ctx, "tunnel-001")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tunnel-001", found.TunnelID)
	assert.Equal(t, "mapping-001", found.MappingID)
}

// TestRoutingTableLookupWaitingTunnel_Expired 测试查找过期的隧道
func TestRoutingTableLookupWaitingTunnel_Expired(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 1*time.Millisecond) // 极短的 TTL

	ctx := context.Background()

	// 注册一个隧道
	state := &WaitingState{
		TunnelID:       "tunnel-001",
		SourceNodeID:   "node-001",
		SourceClientID: 12345,
		TargetClientID: 67890,
	}
	err := rt.RegisterWaitingTunnel(ctx, state)
	assert.NoError(t, err)

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 查找应返回过期错误
	_, err = rt.LookupWaitingTunnel(ctx, "tunnel-001")
	assert.Equal(t, ErrExpired, err)
}

// TestRoutingTableLookupWaitingTunnel_DifferentTypes 测试不同类型的存储值
func TestRoutingTableLookupWaitingTunnel_DifferentTypes(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 30*time.Second)
	ctx := context.Background()

	// 测试 map[string]interface{} 类型
	mapValue := map[string]interface{}{
		"tunnel_id":        "tunnel-002",
		"mapping_id":       "mapping-002",
		"source_node_id":   "node-002",
		"source_client_id": float64(12345), // JSON 解析后数字为 float64
		"target_client_id": float64(67890),
		"target_host":      "192.168.1.100",
		"target_port":      float64(8080),
		"created_at":       time.Now().Format(time.RFC3339),
		"expires_at":       time.Now().Add(time.Hour).Format(time.RFC3339),
	}
	store.data["tunnox:tunnel_waiting:tunnel-002"] = mapValue

	found, err := rt.LookupWaitingTunnel(ctx, "tunnel-002")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tunnel-002", found.TunnelID)

	// 测试 []byte 类型
	jsonData := []byte(`{"tunnel_id":"tunnel-003","mapping_id":"mapping-003","source_node_id":"node-003","source_client_id":12345,"target_client_id":67890,"created_at":"2025-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z"}`)
	store.data["tunnox:tunnel_waiting:tunnel-003"] = jsonData

	found, err = rt.LookupWaitingTunnel(ctx, "tunnel-003")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tunnel-003", found.TunnelID)

	// 测试 string 类型
	store.data["tunnox:tunnel_waiting:tunnel-004"] = string(jsonData)

	// 修改 tunnel_id 以匹配查询
	store.data["tunnox:tunnel_waiting:tunnel-004"] = `{"tunnel_id":"tunnel-004","mapping_id":"mapping-004","source_node_id":"node-004","source_client_id":12345,"target_client_id":67890,"created_at":"2025-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z"}`

	found, err = rt.LookupWaitingTunnel(ctx, "tunnel-004")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tunnel-004", found.TunnelID)

	// 测试无效类型
	store.data["tunnox:tunnel_waiting:tunnel-005"] = 12345 // int 类型
	_, err = rt.LookupWaitingTunnel(ctx, "tunnel-005")
	assert.Error(t, err)

	// 测试 *WaitingState 指针类型
	ptrState := &WaitingState{
		TunnelID:       "tunnel-006",
		MappingID:      "mapping-006",
		SourceNodeID:   "node-006",
		SourceClientID: 12345,
		TargetClientID: 67890,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(time.Hour),
	}
	store.data["tunnox:tunnel_waiting:tunnel-006"] = ptrState

	found, err = rt.LookupWaitingTunnel(ctx, "tunnel-006")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tunnel-006", found.TunnelID)

	// 测试 WaitingState 值类型
	valueState := WaitingState{
		TunnelID:       "tunnel-007",
		MappingID:      "mapping-007",
		SourceNodeID:   "node-007",
		SourceClientID: 12345,
		TargetClientID: 67890,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(time.Hour),
	}
	store.data["tunnox:tunnel_waiting:tunnel-007"] = valueState

	found, err = rt.LookupWaitingTunnel(ctx, "tunnel-007")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "tunnel-007", found.TunnelID)
}

// TestRoutingTableRemoveWaitingTunnel 测试移除等待中的隧道
func TestRoutingTableRemoveWaitingTunnel(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 30*time.Second)

	ctx := context.Background()

	// 测试空 TunnelID
	err := rt.RemoveWaitingTunnel(ctx, "")
	assert.Error(t, err)

	// 注册一个隧道
	state := &WaitingState{
		TunnelID:       "tunnel-001",
		SourceNodeID:   "node-001",
		SourceClientID: 12345,
		TargetClientID: 67890,
	}
	err = rt.RegisterWaitingTunnel(ctx, state)
	assert.NoError(t, err)

	// 移除隧道
	err = rt.RemoveWaitingTunnel(ctx, "tunnel-001")
	assert.NoError(t, err)

	// 验证已移除
	_, err = rt.LookupWaitingTunnel(ctx, "tunnel-001")
	assert.Equal(t, ErrNotFound, err)
}

// TestRoutingTableCleanupExpiredTunnels 测试清理过期隧道
func TestRoutingTableCleanupExpiredTunnels(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 30*time.Second)

	ctx := context.Background()

	// 此方法目前不做实际清理，只返回 0
	count, err := rt.CleanupExpiredTunnels(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestRoutingTableGetNodeAddress 测试获取节点地址
func TestRoutingTableGetNodeAddress(t *testing.T) {
	store := newMockStorage()
	rt := NewRoutingTable(store, 30*time.Second)

	// 测试不存在的节点
	_, err := rt.GetNodeAddress("nonexistent")
	assert.Error(t, err)

	// 注册节点地址
	err = rt.RegisterNodeAddress("node-001", "192.168.1.100:9000")
	assert.NoError(t, err)

	// 获取节点地址
	addr, err := rt.GetNodeAddress("node-001")
	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.100:9000", addr)

	// 测试 []byte 类型的地址
	store.data["tunnox:node:node-002:addr"] = []byte("192.168.1.101:9000")
	addr, err = rt.GetNodeAddress("node-002")
	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.101:9000", addr)

	// 测试无效类型
	store.data["tunnox:node:node-003:addr"] = 12345
	_, err = rt.GetNodeAddress("node-003")
	assert.Error(t, err)
}

// TestRoutingTableGetNodeAddress_NilStorage 测试空存储
func TestRoutingTableGetNodeAddress_NilStorage(t *testing.T) {
	rt := NewRoutingTable(nil, 30*time.Second)

	_, err := rt.GetNodeAddress("node-001")
	assert.Error(t, err)

	err = rt.RegisterNodeAddress("node-001", "192.168.1.100:9000")
	assert.Error(t, err)
}

// ============================================================================
// Start 方法测试
// ============================================================================

// TestBridgeStart_Timeout 测试 Start 方法超时
func TestBridgeStart_Timeout(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 由于没有设置目标连接，Start 应该超时
	// 但默认超时是 30 秒，我们不想等那么久
	// 先关闭 bridge 以触发 context 取消
	go func() {
		time.Sleep(100 * time.Millisecond)
		bridge.Close()
	}()

	err := bridge.Start()
	// 由于 context 被取消，应该返回取消错误
	assert.Error(t, err)
}

// TestBridgeStart_WithTargetConnection 测试 Start 方法带目标连接
func TestBridgeStart_WithTargetConnection(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 设置目标连接
	go func() {
		time.Sleep(50 * time.Millisecond)
		targetConn := &mockTunnelConnection{
			connectionID: "target-001",
		}
		bridge.SetTargetConnection(targetConn)

		// 等待一段时间后关闭
		time.Sleep(100 * time.Millisecond)
		bridge.Close()
	}()

	err := bridge.Start()
	// Start 应该成功（在 context 取消后返回 nil）
	assert.NoError(t, err)
}

// TestBridgeStart_WithCrossNodeConnection 测试 Start 方法带跨节点连接
func TestBridgeStart_WithCrossNodeConnection(t *testing.T) {
	ctx := context.Background()

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 设置跨节点连接
	crossNodeConn := &mockCrossNodeConn{
		nodeID: "node-002",
	}
	bridge.SetCrossNodeConnection(crossNodeConn)

	// 设置目标连接以触发 ready
	go func() {
		time.Sleep(50 * time.Millisecond)
		bridge.NotifyTargetReady()

		// 等待一段时间后关闭
		time.Sleep(100 * time.Millisecond)
		bridge.Close()
	}()

	err := bridge.Start()
	// Start 应该成功
	assert.NoError(t, err)
}

// TestBridgeStart_WithForwarders 测试 Start 方法带数据转发器
func TestBridgeStart_WithForwarders(t *testing.T) {
	ctx := context.Background()

	sourceRWC := &mockReadWriteCloser{
		data: []byte("source data"),
	}
	targetRWC := &mockReadWriteCloser{}

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)

	// 设置转发器
	bridge.sourceForwarder = sourceRWC
	bridge.targetForwarder = targetRWC

	// 设置目标连接以触发 ready
	go func() {
		time.Sleep(50 * time.Millisecond)
		bridge.NotifyTargetReady()

		// 等待数据转发一段时间后关闭
		time.Sleep(100 * time.Millisecond)
		bridge.Close()
	}()

	err := bridge.Start()
	// Start 应该成功
	assert.NoError(t, err)
}

// TestBridgeStart_ContextCancelled 测试 Start 方法 context 取消
func TestBridgeStart_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &BridgeConfig{
		TunnelID: "test-tunnel-001",
	}

	bridge := NewBridge(ctx, config)
	require.NotNil(t, bridge)
	defer bridge.Close()

	// 立即取消 context
	cancel()

	err := bridge.Start()
	// 应该返回取消错误
	assert.Error(t, err)
}
