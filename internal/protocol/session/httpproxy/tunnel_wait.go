package httpproxy

import (
	"sync"

	corelog "tunnox-core/internal/core/log"
)

// TunnelConnection 隧道连接接口（避免循环依赖）
type TunnelConnection interface {
	GetTunnelID() string
	Close() error
}

// TunnelWaitManager 隧道等待管理器
// 管理等待建立的隧道连接
type TunnelWaitManager struct {
	pendingTunnels map[string]chan TunnelConnection
	mu             sync.RWMutex
}

// NewTunnelWaitManager 创建隧道等待管理器
func NewTunnelWaitManager() *TunnelWaitManager {
	return &TunnelWaitManager{
		pendingTunnels: make(map[string]chan TunnelConnection),
	}
}

// RegisterPendingTunnel 注册等待建立的隧道
func (m *TunnelWaitManager) RegisterPendingTunnel(tunnelID string) chan TunnelConnection {
	ch := make(chan TunnelConnection, 1)

	m.mu.Lock()
	m.pendingTunnels[tunnelID] = ch
	m.mu.Unlock()

	return ch
}

// UnregisterPendingTunnel 注销等待建立的隧道
func (m *TunnelWaitManager) UnregisterPendingTunnel(tunnelID string) {
	m.mu.Lock()
	delete(m.pendingTunnels, tunnelID)
	m.mu.Unlock()
}

// NotifyTunnelEstablished 通知隧道已建立
func (m *TunnelWaitManager) NotifyTunnelEstablished(tunnelID string, conn TunnelConnection) {
	m.mu.RLock()
	ch, exists := m.pendingTunnels[tunnelID]
	m.mu.RUnlock()

	if !exists {
		corelog.Warnf("TunnelWaitManager: no pending tunnel for ID %s", tunnelID)
		return
	}

	select {
	case ch <- conn:
	default:
		corelog.Warnf("TunnelWaitManager: tunnel channel full for %s", tunnelID)
	}
}

// ============================================================================
// 全局隧道等待管理器（懒加载）
// ============================================================================

var (
	globalTunnelWaitManager     *TunnelWaitManager
	globalTunnelWaitManagerOnce sync.Once
)

// GetGlobalTunnelWaitManager 获取全局隧道等待管理器
func GetGlobalTunnelWaitManager() *TunnelWaitManager {
	globalTunnelWaitManagerOnce.Do(func() {
		globalTunnelWaitManager = NewTunnelWaitManager()
	})
	return globalTunnelWaitManager
}
