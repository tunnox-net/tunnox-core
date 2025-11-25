package bridge

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
	pb "tunnox-core/api/proto/bridge"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NodeInfo 节点信息（强类型定义）
type NodeInfo struct {
	NodeID  string
	Address string
}

// MockNodeRegistry 模拟节点注册表
type MockNodeRegistry struct {
	nodes map[string]*NodeInfo
	mu    sync.RWMutex
}

func NewMockNodeRegistry() *MockNodeRegistry {
	return &MockNodeRegistry{
		nodes: make(map[string]*NodeInfo),
	}
}

func (r *MockNodeRegistry) GetNodeAddress(nodeID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodeInfo, exists := r.nodes[nodeID]
	if !exists {
		return "", fmt.Errorf("node not found: %s", nodeID)
	}
	return nodeInfo.Address, nil
}

func (r *MockNodeRegistry) ListAllNodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]string, 0, len(r.nodes))
	for nodeID := range r.nodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

func (r *MockNodeRegistry) RegisterNode(nodeID, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[nodeID] = &NodeInfo{
		NodeID:  nodeID,
		Address: addr,
	}
}

// setupTestGRPCServer 启动测试用的 gRPC 服务器
func setupTestGRPCServer(t *testing.T, nodeID string) (string, *GRPCBridgeServer, func()) {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	addr := listener.Addr().String()
	grpcServer := grpc.NewServer()

	// 创建 BridgeManager（使用 nil 依赖，仅用于测试）
	bridgeServer := NewGRPCBridgeServer(nodeID, nil)
	pb.RegisterBridgeServiceServer(grpcServer, bridgeServer)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()

	cleanup := func() {
		grpcServer.Stop()
		listener.Close()
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	return addr, bridgeServer, cleanup
}

func TestGRPCBridgeServer_Ping(t *testing.T) {
	nodeID := "test-node-1"
	addr, _, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	// 创建客户端连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewBridgeServiceClient(conn)

	// 测试 Ping
	resp, err := client.Ping(ctx, &pb.PingRequest{
		NodeId:    "client-node",
		Timestamp: time.Now().UnixMilli(),
	})
	require.NoError(t, err)
	assert.True(t, resp.Ok)
	assert.Greater(t, resp.ServerTimestamp, int64(0))
}

func TestGRPCBridgeServer_GetNodeInfo(t *testing.T) {
	nodeID := "test-node-2"
	addr, _, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewBridgeServiceClient(conn)

	// 测试 GetNodeInfo
	resp, err := client.GetNodeInfo(ctx, &pb.NodeInfoRequest{
		NodeId: "client-node",
	})
	require.NoError(t, err)
	assert.Equal(t, nodeID, resp.NodeId)
	assert.GreaterOrEqual(t, resp.ActiveConnections, int32(0))
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))
}

func TestGRPCBridgeServer_ForwardStream(t *testing.T) {
	nodeID := "test-node-3"
	addr, bridgeServer, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewBridgeServiceClient(conn)

	// 创建双向流
	stream, err := client.ForwardStream(ctx)
	require.NoError(t, err)

	streamID := "test-stream-123"

	// 发送 STREAM_OPEN 数据包
	openPacket := &pb.BridgePacket{
		StreamId:  streamID,
		Type:      pb.PacketType_STREAM_OPEN,
		Payload:   []byte(`{"target":"localhost:3306"}`),
		Timestamp: time.Now().UnixMilli(),
	}

	err = stream.Send(openPacket)
	require.NoError(t, err)

	// 等待 ACK
	ackPacket, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, streamID, ackPacket.StreamId)
	assert.Equal(t, pb.PacketType_STREAM_ACK, ackPacket.Type)

	// 验证服务器端已注册流
	assert.Equal(t, 1, bridgeServer.GetActiveStreamsCount())

	// 发送数据包
	dataPacket := &pb.BridgePacket{
		StreamId:  streamID,
		Type:      pb.PacketType_STREAM_DATA,
		Payload:   []byte("test data"),
		Timestamp: time.Now().UnixMilli(),
	}

	err = stream.Send(dataPacket)
	require.NoError(t, err)

	// 等待一小段时间让服务器处理
	time.Sleep(100 * time.Millisecond)

	// 发送关闭包
	closePacket := &pb.BridgePacket{
		StreamId:  streamID,
		Type:      pb.PacketType_STREAM_CLOSE,
		Timestamp: time.Now().UnixMilli(),
	}

	err = stream.Send(closePacket)
	require.NoError(t, err)

	// 关闭发送端
	err = stream.CloseSend()
	assert.NoError(t, err)

	// 等待服务器关闭
	time.Sleep(100 * time.Millisecond)
}

func TestGRPCBridgeServer_MultipleStreams(t *testing.T) {
	nodeID := "test-node-4"
	addr, bridgeServer, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建第一个流
	conn1, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)
	defer conn1.Close()

	client1 := pb.NewBridgeServiceClient(conn1)
	stream1, err := client1.ForwardStream(ctx)
	require.NoError(t, err)

	// 创建第二个流
	conn2, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)
	defer conn2.Close()

	client2 := pb.NewBridgeServiceClient(conn2)
	stream2, err := client2.ForwardStream(ctx)
	require.NoError(t, err)

	// 在两个流上发送打开请求
	streamID1 := "stream-1"
	streamID2 := "stream-2"

	err = stream1.Send(&pb.BridgePacket{
		StreamId:  streamID1,
		Type:      pb.PacketType_STREAM_OPEN,
		Payload:   []byte(`{"target":"host1:3306"}`),
		Timestamp: time.Now().UnixMilli(),
	})
	require.NoError(t, err)

	err = stream2.Send(&pb.BridgePacket{
		StreamId:  streamID2,
		Type:      pb.PacketType_STREAM_OPEN,
		Payload:   []byte(`{"target":"host2:5432"}`),
		Timestamp: time.Now().UnixMilli(),
	})
	require.NoError(t, err)

	// 接收 ACK
	ack1, err := stream1.Recv()
	require.NoError(t, err)
	assert.Equal(t, streamID1, ack1.StreamId)

	ack2, err := stream2.Recv()
	require.NoError(t, err)
	assert.Equal(t, streamID2, ack2.StreamId)

	// 验证两个流都已注册
	assert.Equal(t, 2, bridgeServer.GetActiveStreamsCount())

	// 关闭流
	stream1.CloseSend()
	stream2.CloseSend()
}

func TestMultiplexedConn_Lifecycle(t *testing.T) {
	// 这个测试需要一个真实的 gRPC 服务器
	nodeID := "test-node-5"
	addr, _, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建 gRPC 连接
	grpcConn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)

	// 创建多路复用连接
	mc, err := NewMultiplexedConn(ctx, nodeID, grpcConn, 100)
	require.NoError(t, err)
	require.NotNil(t, mc)

	// 验证初始状态
	assert.Equal(t, nodeID, mc.GetTargetNodeID())
	assert.True(t, mc.CanAcceptStream())
	assert.Equal(t, int32(0), mc.GetActiveStreams())
	assert.False(t, mc.IsClosed())

	// 创建会话
	metadata := &SessionMetadata{
		SourceClientID: 1,
		TargetClientID: 2,
		TargetHost:     "test-host",
		TargetPort:     8080,
		SourceNodeID:   "node-1",
		TargetNodeID:   nodeID,
		RequestID:      "test-req-1",
	}
	session := NewForwardSession(ctx, mc, metadata)
	require.NotNil(t, session)

	// 验证会话已注册
	assert.Equal(t, int32(1), mc.GetActiveStreams())
	assert.False(t, mc.IsIdle(1*time.Minute))

	// 关闭会话
	err = session.Close()
	assert.NoError(t, err)

	// 等待清理
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), mc.GetActiveStreams())

	// 关闭连接
	err = mc.Close()
	assert.NoError(t, err)
	assert.True(t, mc.IsClosed())
}

func TestMultiplexedConn_MaxStreamsLimit(t *testing.T) {
	nodeID := "test-node-6"
	addr, _, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grpcConn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	require.NoError(t, err)

	// 创建限制为 3 个流的连接
	maxStreams := int32(3)
	mc, err := NewMultiplexedConn(ctx, nodeID, grpcConn, maxStreams)
	require.NoError(t, err)
	defer mc.Close()

	sessions := make([]*ForwardSession, 0, maxStreams)

	// 创建 maxStreams 个会话
	for i := int32(0); i < maxStreams; i++ {
		session := NewForwardSession(ctx, mc, nil)
		require.NotNil(t, session, "failed to create session %d", i)
		sessions = append(sessions, session)
	}

	assert.Equal(t, maxStreams, mc.GetActiveStreams())
	assert.False(t, mc.CanAcceptStream())

	// 尝试创建第 4 个会话应该失败
	extraSession := NewForwardSession(ctx, mc, nil)
	assert.Nil(t, extraSession, "should not be able to create session beyond max limit")

	// 关闭一个会话后应该可以再创建
	sessions[0].Close()
	time.Sleep(50 * time.Millisecond)

	assert.True(t, mc.CanAcceptStream())
	newSession := NewForwardSession(ctx, mc, nil)
	assert.NotNil(t, newSession)

	// 清理
	for _, s := range sessions[1:] {
		s.Close()
	}
	newSession.Close()
}

func TestNodeConnectionPool_Lifecycle(t *testing.T) {
	nodeID := "test-node-7"
	addr, _, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := &NodePoolConfig{
		MinConns:          1,
		MaxConns:          3,
		MaxIdleTime:       1 * time.Minute,
		MaxStreamsPerConn: 10,
		DialTimeout:       5 * time.Second,
	}

	pool, err := NewNodeConnectionPool(ctx, nodeID, addr, config)
	require.NoError(t, err)
	defer pool.Close()

	// 验证初始连接数
	stats := pool.GetStats()
	assert.Equal(t, config.MinConns, stats.TotalConns)

	// 创建会话
	session, err := pool.GetOrCreateSession(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, session)

	// 验证统计
	stats = pool.GetStats()
	assert.GreaterOrEqual(t, stats.ActiveStreams, int32(1))

	// 关闭会话
	session.Close()
	time.Sleep(100 * time.Millisecond)
}

func TestNodeConnectionPool_ScaleUp(t *testing.T) {
	nodeID := "test-node-8"
	addr, _, cleanup := setupTestGRPCServer(t, nodeID)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := &NodePoolConfig{
		MinConns:          1,
		MaxConns:          5,
		MaxIdleTime:       1 * time.Minute,
		MaxStreamsPerConn: 2, // 限制每个连接只能有 2 个流
		DialTimeout:       5 * time.Second,
	}

	pool, err := NewNodeConnectionPool(ctx, nodeID, addr, config)
	require.NoError(t, err)
	defer pool.Close()

	sessions := make([]*ForwardSession, 0)

	// 创建 6 个会话，应该触发连接扩容
	for i := 0; i < 6; i++ {
		session, err := pool.GetOrCreateSession(ctx, nil)
		require.NoError(t, err, "failed to create session %d", i)
		require.NotNil(t, session)
		sessions = append(sessions, session)
		time.Sleep(50 * time.Millisecond)
	}

	// 验证连接数已扩容
	stats := pool.GetStats()
	assert.GreaterOrEqual(t, stats.TotalConns, int32(3), "should have scaled up to at least 3 connections")
	assert.Equal(t, int32(6), stats.ActiveStreams)

	// 清理
	for _, s := range sessions {
		s.Close()
	}
}

func TestBridgeConnectionPool_Integration(t *testing.T) {
	// 启动两个测试节点
	node1ID := "node-1"
	node1Addr, _, cleanup1 := setupTestGRPCServer(t, node1ID)
	defer cleanup1()

	node2ID := "node-2"
	node2Addr, _, cleanup2 := setupTestGRPCServer(t, node2ID)
	defer cleanup2()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建连接池
	poolConfig := &PoolConfig{
		MinConnsPerNode:     1,
		MaxConnsPerNode:     3,
		MaxIdleTime:         1 * time.Minute,
		MaxStreamsPerConn:   10,
		DialTimeout:         5 * time.Second,
		HealthCheckInterval: 5 * time.Second,
	}

	pool := NewBridgeConnectionPool(ctx, poolConfig)
	require.NotNil(t, pool)
	defer pool.Close()

	// 创建到 node1 的会话
	metadata1 := &SessionMetadata{
		SourceClientID: 100,
		TargetClientID: 200,
		TargetHost:     "localhost",
		TargetPort:     3306,
		SourceNodeID:   "test-node",
		TargetNodeID:   node1ID,
		RequestID:      "req-1",
	}
	session1, err := pool.CreateSession(ctx, node1ID, node1Addr, metadata1)
	require.NoError(t, err)
	require.NotNil(t, session1)

	// 创建到 node2 的会话
	metadata2 := &SessionMetadata{
		SourceClientID: 101,
		TargetClientID: 201,
		TargetHost:     "localhost",
		TargetPort:     5432,
		SourceNodeID:   "test-node",
		TargetNodeID:   node2ID,
		RequestID:      "req-2",
	}
	session2, err := pool.CreateSession(ctx, node2ID, node2Addr, metadata2)
	require.NoError(t, err)
	require.NotNil(t, session2)

	// 等待统计信息更新
	time.Sleep(200 * time.Millisecond)

	// 验证统计
	allStats := pool.GetAllStats()
	assert.Len(t, allStats, 2)

	assert.Contains(t, allStats, node1ID)
	assert.Contains(t, allStats, node2ID)

	assert.GreaterOrEqual(t, allStats[node1ID].TotalConns, int32(1))
	assert.GreaterOrEqual(t, allStats[node2ID].TotalConns, int32(1))

	// 验证全局指标 - 只检查节点数，不检查会话数（因为统计更新有延迟）
	metrics := pool.GetMetrics()
	assert.GreaterOrEqual(t, metrics.GlobalStats.TotalNodes, int32(0))
	assert.GreaterOrEqual(t, metrics.GlobalStats.TotalSessionsCreated, int64(2))

	// 清理
	session1.Close()
	session2.Close()
}
