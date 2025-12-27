package client

import (
	"context"
	"sync"

	corelog "tunnox-core/internal/core/log"
)

// TargetTunnelManager 目标端隧道管理器
// 用于管理 targetClient 的活跃隧道连接，支持根据 tunnelID 关闭隧道
type TargetTunnelManager struct {
	tunnels map[string]context.CancelFunc // tunnelID -> cancel function
	mu      sync.RWMutex
}

// NewTargetTunnelManager 创建隧道管理器
func NewTargetTunnelManager() *TargetTunnelManager {
	return &TargetTunnelManager{
		tunnels: make(map[string]context.CancelFunc),
	}
}

// RegisterTunnel 注册隧道
// 返回一个带取消功能的 context，当收到关闭通知时会被取消
func (m *TargetTunnelManager) RegisterTunnel(tunnelID string, parentCtx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parentCtx)

	m.mu.Lock()
	// 如果已存在，先取消旧的
	if oldCancel, exists := m.tunnels[tunnelID]; exists {
		oldCancel()
	}
	m.tunnels[tunnelID] = cancel
	m.mu.Unlock()

	corelog.Debugf("TargetTunnelManager: registered tunnel %s, total=%d", tunnelID, m.Count())

	return ctx, cancel
}

// UnregisterTunnel 注销隧道（隧道正常结束时调用）
func (m *TargetTunnelManager) UnregisterTunnel(tunnelID string) {
	m.mu.Lock()
	delete(m.tunnels, tunnelID)
	m.mu.Unlock()

	corelog.Debugf("TargetTunnelManager: unregistered tunnel %s, total=%d", tunnelID, m.Count())
}

// CloseTunnel 关闭指定隧道（收到关闭通知时调用）
func (m *TargetTunnelManager) CloseTunnel(tunnelID string) bool {
	m.mu.Lock()
	cancel, exists := m.tunnels[tunnelID]
	if exists {
		delete(m.tunnels, tunnelID)
	}
	m.mu.Unlock()

	if exists && cancel != nil {
		cancel()
		corelog.Infof("TargetTunnelManager: closed tunnel %s by notification", tunnelID)
		return true
	}

	corelog.Debugf("TargetTunnelManager: tunnel %s not found, may already closed", tunnelID)
	return false
}

// CloseAllTunnels 关闭所有隧道
func (m *TargetTunnelManager) CloseAllTunnels() {
	m.mu.Lock()
	tunnels := make(map[string]context.CancelFunc, len(m.tunnels))
	for k, v := range m.tunnels {
		tunnels[k] = v
	}
	m.tunnels = make(map[string]context.CancelFunc)
	m.mu.Unlock()

	for tunnelID, cancel := range tunnels {
		if cancel != nil {
			cancel()
		}
		corelog.Debugf("TargetTunnelManager: closed tunnel %s during cleanup", tunnelID)
	}

	corelog.Infof("TargetTunnelManager: closed all %d tunnels", len(tunnels))
}

// Count 返回当前活跃隧道数量
func (m *TargetTunnelManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tunnels)
}

// HasTunnel 检查隧道是否存在
func (m *TargetTunnelManager) HasTunnel(tunnelID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.tunnels[tunnelID]
	return exists
}
