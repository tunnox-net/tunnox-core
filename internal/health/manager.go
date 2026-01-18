package health

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 健康状态管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HealthStatus 健康状态
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"   // 健康，可以接受新连接
	HealthStatusDraining  HealthStatus = "draining"  // 排空中，不接受新连接，但处理现有连接
	HealthStatusUnhealthy HealthStatus = "unhealthy" // 不健康，不可用
)

// HealthInfo 健康信息
type HealthInfo struct {
	Status            HealthStatus      `json:"status"`
	ActiveConnections int               `json:"active_connections"`
	ActiveTunnels     int               `json:"active_tunnels"`
	Uptime            int64             `json:"uptime_seconds"`
	NodeID            string            `json:"node_id,omitempty"`
	Version           string            `json:"version,omitempty"`
	Details           map[string]string `json:"details,omitempty"`
	LastStatusChange  time.Time         `json:"last_status_change"`
	AcceptingNewConns bool              `json:"accepting_new_connections"`
}

// StatsProvider 提供统计信息的接口
type StatsProvider interface {
	GetActiveConnections() int
	GetActiveTunnels() int
}

// HealthManager 健康状态管理器
//
// 职责：
// 1. 管理服务器健康状态（healthy/draining/unhealthy）
// 2. 提供健康检查信息给负载均衡器（如Nginx）
// 3. 在优雅关闭时将状态切换为draining，提前摘除节点
type HealthManager struct {
	mu sync.RWMutex

	status           HealthStatus
	startTime        time.Time
	lastStatusChange time.Time
	nodeID           string
	version          string
	details          map[string]string

	// 外部状态提供者
	statsProvider StatsProvider

	dispose.ServiceBase
}

// NewHealthManager 创建健康状态管理器
func NewHealthManager(nodeID, version string, parentCtx context.Context) *HealthManager {
	now := time.Now()

	m := &HealthManager{
		ServiceBase:      *dispose.NewService("HealthManager", parentCtx),
		status:           HealthStatusHealthy,
		startTime:        now,
		lastStatusChange: now,
		nodeID:           nodeID,
		version:          version,
		details:          make(map[string]string),
	}

	return m
}

// SetStatsProvider 设置统计信息提供者
func (m *HealthManager) SetStatsProvider(provider StatsProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.statsProvider = provider
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 状态管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetStatus 获取当前健康状态
func (m *HealthManager) GetStatus() HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.status
}

// SetStatus 设置健康状态
func (m *HealthManager) SetStatus(status HealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status != status {
		m.status = status
		m.lastStatusChange = time.Now()
	}
}

// IsHealthy 是否健康
func (m *HealthManager) IsHealthy() bool {
	return m.GetStatus() == HealthStatusHealthy
}

// IsDraining 是否在排空中
func (m *HealthManager) IsDraining() bool {
	return m.GetStatus() == HealthStatusDraining
}

// IsAcceptingConnections 是否接受新连接
//
// 只有在healthy状态下才接受新连接
func (m *HealthManager) IsAcceptingConnections() bool {
	return m.GetStatus() == HealthStatusHealthy
}

// MarkDraining 标记为排空中（优雅关闭）
func (m *HealthManager) MarkDraining() {
	m.SetStatus(HealthStatusDraining)
}

// MarkUnhealthy 标记为不健康
func (m *HealthManager) MarkUnhealthy(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.status = HealthStatusUnhealthy
	m.lastStatusChange = time.Now()
	m.details["unhealthy_reason"] = reason
}

// SetDetail 设置详细信息
func (m *HealthManager) SetDetail(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.details[key] = value
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 健康信息
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetHealthInfo 获取完整健康信息
func (m *HealthManager) GetHealthInfo() *HealthInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var activeConns, activeTunnels int
	if m.statsProvider != nil {
		activeConns = m.statsProvider.GetActiveConnections()
		activeTunnels = m.statsProvider.GetActiveTunnels()
	}

	// 复制details以避免并发修改
	detailsCopy := make(map[string]string, len(m.details))
	for k, v := range m.details {
		detailsCopy[k] = v
	}

	return &HealthInfo{
		Status:            m.status,
		ActiveConnections: activeConns,
		ActiveTunnels:     activeTunnels,
		Uptime:            int64(time.Since(m.startTime).Seconds()),
		NodeID:            m.nodeID,
		Version:           m.version,
		Details:           detailsCopy,
		LastStatusChange:  m.lastStatusChange,
		AcceptingNewConns: m.status == HealthStatusHealthy,
	}
}
