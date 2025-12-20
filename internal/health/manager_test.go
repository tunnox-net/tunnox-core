package health

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HealthManager 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestNewHealthManager(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	assert.NotNil(t, manager)
	assert.Equal(t, HealthStatusHealthy, manager.GetStatus())
	assert.True(t, manager.IsHealthy())
	assert.False(t, manager.IsDraining())
	assert.True(t, manager.IsAcceptingConnections())
}

func TestHealthManager_SetStatus(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	// 初始状态
	assert.Equal(t, HealthStatusHealthy, manager.GetStatus())

	// 切换到draining
	manager.SetStatus(HealthStatusDraining)
	assert.Equal(t, HealthStatusDraining, manager.GetStatus())
	assert.False(t, manager.IsHealthy())
	assert.True(t, manager.IsDraining())
	assert.False(t, manager.IsAcceptingConnections())

	// 切换到unhealthy
	manager.SetStatus(HealthStatusUnhealthy)
	assert.Equal(t, HealthStatusUnhealthy, manager.GetStatus())
	assert.False(t, manager.IsHealthy())
	assert.False(t, manager.IsDraining())
	assert.False(t, manager.IsAcceptingConnections())
}

func TestHealthManager_MarkDraining(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	manager.MarkDraining()
	assert.Equal(t, HealthStatusDraining, manager.GetStatus())
	assert.True(t, manager.IsDraining())
}

func TestHealthManager_MarkUnhealthy(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	manager.MarkUnhealthy("test reason")
	assert.Equal(t, HealthStatusUnhealthy, manager.GetStatus())

	info := manager.GetHealthInfo()
	assert.Equal(t, "test reason", info.Details["unhealthy_reason"])
}

func TestHealthManager_GetHealthInfo(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-123", "2.0.0", ctx)

	// 设置mock stats provider
	mockProvider := &mockStatsProvider{
		activeConns:   10,
		activeTunnels: 5,
	}
	manager.SetStatsProvider(mockProvider)

	// 设置详细信息
	manager.SetDetail("custom_key", "custom_value")

	// 获取健康信息
	info := manager.GetHealthInfo()

	assert.Equal(t, HealthStatusHealthy, info.Status)
	assert.Equal(t, 10, info.ActiveConnections)
	assert.Equal(t, 5, info.ActiveTunnels)
	assert.Equal(t, "node-123", info.NodeID)
	assert.Equal(t, "2.0.0", info.Version)
	assert.Equal(t, "custom_value", info.Details["custom_key"])
	assert.True(t, info.AcceptingNewConns)
	assert.GreaterOrEqual(t, info.Uptime, int64(0))
}

func TestHealthManager_StatusChange_Timestamp(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	// 记录初始时间
	initialChange := manager.GetHealthInfo().LastStatusChange

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// 改变状态
	manager.SetStatus(HealthStatusDraining)

	// 验证时间戳已更新
	newChange := manager.GetHealthInfo().LastStatusChange
	assert.True(t, newChange.After(initialChange), "LastStatusChange should be updated")
}

func TestHealthManager_Uptime(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)

	info := manager.GetHealthInfo()
	assert.GreaterOrEqual(t, info.Uptime, int64(0), "Uptime should be >= 0")
}

func TestHealthManager_Concurrent(t *testing.T) {
	ctx := context.Background()
	manager := NewHealthManager("node-1", "1.0.0", ctx)

	// 并发读写测试
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			manager.SetStatus(HealthStatusHealthy)
			_ = manager.GetStatus()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			manager.SetStatus(HealthStatusDraining)
			_ = manager.GetHealthInfo()
		}
		done <- true
	}()

	<-done
	<-done

	// 测试通过即表示无数据竞争
	assert.NotNil(t, manager)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Mock StatsProvider
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

type mockStatsProvider struct {
	activeConns   int
	activeTunnels int
}

func (m *mockStatsProvider) GetActiveConnections() int {
	return m.activeConns
}

func (m *mockStatsProvider) GetActiveTunnels() int {
	return m.activeTunnels
}
