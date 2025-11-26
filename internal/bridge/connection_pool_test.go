package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBridgeConnectionPool_Creation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := DefaultPoolConfig()
	pool := NewBridgeConnectionPool(ctx, config)
	require.NotNil(t, pool)
	defer pool.Close()

	assert.Equal(t, config.MinConnsPerNode, pool.config.MinConnsPerNode)
	assert.Equal(t, config.MaxConnsPerNode, pool.config.MaxConnsPerNode)
	assert.Equal(t, config.MaxStreamsPerConn, pool.config.MaxStreamsPerConn)
}

func TestBridgeConnectionPool_GetMetrics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewBridgeConnectionPool(ctx, DefaultPoolConfig())
	require.NotNil(t, pool)
	defer pool.Close()

	metrics := pool.GetMetrics()
	require.NotNil(t, metrics)
	assert.NotNil(t, metrics.GlobalStats)
	assert.NotNil(t, metrics.NodeStats)
	assert.Equal(t, int32(0), metrics.GlobalStats.TotalNodes)
	assert.Equal(t, int32(0), metrics.GlobalStats.TotalConnections)
}

func TestBridgeConnectionPool_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewBridgeConnectionPool(ctx, DefaultPoolConfig())
	require.NotNil(t, pool)

	err := pool.Close()
	assert.NoError(t, err)

	// 关闭后再次关闭应该不出错
	err = pool.Close()
	assert.NoError(t, err)
}

func TestMetricsCollector_RecordSessionCreated(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	nodeID := "test-node-1"
	collector.RecordSessionCreated(nodeID)
	collector.RecordSessionCreated(nodeID)

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)
	
	nodeStats, exists := metrics.NodeStats[nodeID]
	require.True(t, exists)
	assert.Equal(t, int64(2), nodeStats.SessionsCreated)
	assert.Equal(t, int64(2), metrics.GlobalStats.TotalSessionsCreated)
}

func TestMetricsCollector_RecordSessionClosed(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	nodeID := "test-node-2"
	collector.RecordSessionClosed(nodeID)
	collector.RecordSessionClosed(nodeID)
	collector.RecordSessionClosed(nodeID)

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)
	
	nodeStats, exists := metrics.NodeStats[nodeID]
	require.True(t, exists)
	assert.Equal(t, int64(3), nodeStats.SessionsClosed)
	assert.Equal(t, int64(3), metrics.GlobalStats.TotalSessionsClosed)
}

func TestMetricsCollector_RecordError(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	nodeID := "test-node-3"
	errorMsg := "connection failed"
	
	collector.RecordError(nodeID, errorMsg)

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)
	
	nodeStats, exists := metrics.NodeStats[nodeID]
	require.True(t, exists)
	assert.Equal(t, int64(1), nodeStats.ErrorCount)
	assert.Equal(t, errorMsg, nodeStats.LastError)
	assert.Equal(t, int64(1), metrics.GlobalStats.TotalErrors)
}

func TestMetricsCollector_UpdatePoolStats(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	nodeID := "test-node-4"
	collector.UpdatePoolStats(nodeID, 5, 20)

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)
	
	nodeStats, exists := metrics.NodeStats[nodeID]
	require.True(t, exists)
	assert.Equal(t, int32(5), nodeStats.TotalConnections)
	assert.Equal(t, int32(20), nodeStats.ActiveStreams)
	assert.Equal(t, int32(1), metrics.GlobalStats.TotalNodes)
	assert.Equal(t, int32(5), metrics.GlobalStats.TotalConnections)
	assert.Equal(t, int32(20), metrics.GlobalStats.TotalActiveStreams)
}

func TestMetricsCollector_MultipleNodes(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	// 节点 1
	collector.RecordSessionCreated("node-1")
	collector.RecordSessionCreated("node-1")
	collector.UpdatePoolStats("node-1", 3, 10)

	// 节点 2
	collector.RecordSessionCreated("node-2")
	collector.RecordSessionClosed("node-2")
	collector.UpdatePoolStats("node-2", 2, 5)

	// 节点 3
	collector.RecordError("node-3", "test error")
	collector.UpdatePoolStats("node-3", 1, 2)

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)

	// 验证全局统计
	assert.Equal(t, int32(3), metrics.GlobalStats.TotalNodes)
	assert.Equal(t, int32(6), metrics.GlobalStats.TotalConnections) // 3+2+1
	assert.Equal(t, int32(17), metrics.GlobalStats.TotalActiveStreams) // 10+5+2
	assert.Equal(t, int64(3), metrics.GlobalStats.TotalSessionsCreated)
	assert.Equal(t, int64(1), metrics.GlobalStats.TotalSessionsClosed)
	assert.Equal(t, int64(1), metrics.GlobalStats.TotalErrors)

	// 验证各节点统计
	assert.Len(t, metrics.NodeStats, 3)
	assert.Equal(t, int64(2), metrics.NodeStats["node-1"].SessionsCreated)
	assert.Equal(t, int64(1), metrics.NodeStats["node-2"].SessionsClosed)
	assert.Equal(t, int64(1), metrics.NodeStats["node-3"].ErrorCount)
}

func TestMetricsCollector_RemoveNodeStats(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	collector.RecordSessionCreated("node-1")
	collector.UpdatePoolStats("node-1", 2, 5)
	collector.RecordSessionCreated("node-2")
	collector.UpdatePoolStats("node-2", 3, 8)

	// 移除节点 1
	collector.RemoveNodeStats("node-1")

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)

	assert.Equal(t, int32(1), metrics.GlobalStats.TotalNodes)
	assert.Equal(t, int32(3), metrics.GlobalStats.TotalConnections)
	assert.Equal(t, int32(8), metrics.GlobalStats.TotalActiveStreams)
	_, exists := metrics.NodeStats["node-1"]
	assert.False(t, exists)
}

func TestMetricsCollector_Reset(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	collector.RecordSessionCreated("node-1")
	collector.RecordError("node-2", "test")
	collector.UpdatePoolStats("node-1", 5, 10)

	// 重置
	collector.Reset()

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)

	assert.Equal(t, int32(0), metrics.GlobalStats.TotalNodes)
	assert.Equal(t, int32(0), metrics.GlobalStats.TotalConnections)
	assert.Equal(t, int64(0), metrics.GlobalStats.TotalSessionsCreated)
	assert.Equal(t, int64(0), metrics.GlobalStats.TotalErrors)
	assert.Len(t, metrics.NodeStats, 0)
}

func TestMetricsCollector_Uptime(t *testing.T) {
	collector := NewMetricsCollector(context.Background())
	require.NotNil(t, collector)

	time.Sleep(100 * time.Millisecond)

	metrics := collector.GetMetrics()
	require.NotNil(t, metrics)

	assert.GreaterOrEqual(t, metrics.GlobalStats.Uptime, 100*time.Millisecond)
	assert.Less(t, metrics.GlobalStats.Uptime, 1*time.Second)
}

