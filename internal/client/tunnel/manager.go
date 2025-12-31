package tunnel

import (
	"context"
	"fmt"
	"sync"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// TunnelManager 隧道管理器接口
type TunnelManager interface {
	// 生命周期
	Ctx() context.Context
	Close() error

	// 注册和注销
	RegisterTunnel(tunnel *Tunnel) error
	UnregisterTunnel(tunnelID string) bool

	// 查询
	GetTunnel(tunnelID string) *Tunnel
	ListTunnels() []*Tunnel
	CountTunnels() int

	// 关闭
	CloseTunnel(tunnelID string, reason CloseReason) error
	CloseAll()

	// 通知处理（实现 client.NotificationHandler 接口）
	OnSystemMessage(title, message, level string)
	OnQuotaWarning(quotaType string, usagePercent float64, message string)
	OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string)
	OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64)
	OnTunnelOpened(tunnelID, mappingID string, peerClientID int64)
	OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool)
	OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string)
	OnGenericNotification(notification *packet.ClientNotification)
}

// DefaultTunnelManager 默认隧道管理器实现
type DefaultTunnelManager struct {
	*dispose.ManagerBase

	tunnels sync.Map // map[string]*Tunnel
	role    TunnelRole
}

// NewTunnelManager 创建隧道管理器
func NewTunnelManager(parentCtx context.Context, role TunnelRole) *DefaultTunnelManager {
	m := &DefaultTunnelManager{
		ManagerBase: dispose.NewManager(fmt.Sprintf("TunnelManager-%s", role), parentCtx),
		role:        role,
	}

	corelog.Infof("TunnelManager[%s]: created", role)

	return m
}

// RegisterTunnel 注册隧道
func (m *DefaultTunnelManager) RegisterTunnel(tunnel *Tunnel) error {
	if tunnel == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "tunnel is nil")
	}

	// 检查是否已存在
	if _, exists := m.tunnels.LoadOrStore(tunnel.id, tunnel); exists {
		corelog.Warnf("TunnelManager[%s]: tunnel %s already exists", m.role, tunnel.id)
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "tunnel already exists: %s", tunnel.id)
	}

	corelog.Debugf("TunnelManager[%s]: registered tunnel %s", m.role, tunnel.id)
	return nil
}

// UnregisterTunnel 注销隧道
func (m *DefaultTunnelManager) UnregisterTunnel(tunnelID string) bool {
	_, deleted := m.tunnels.LoadAndDelete(tunnelID)
	if deleted {
		corelog.Debugf("TunnelManager[%s]: unregistered tunnel %s", m.role, tunnelID)
	}
	return deleted
}

// GetTunnel 获取隧道
func (m *DefaultTunnelManager) GetTunnel(tunnelID string) *Tunnel {
	value, ok := m.tunnels.Load(tunnelID)
	if !ok {
		return nil
	}
	return value.(*Tunnel)
}

// ListTunnels 列出所有隧道
func (m *DefaultTunnelManager) ListTunnels() []*Tunnel {
	tunnels := make([]*Tunnel, 0)
	m.tunnels.Range(func(key, value interface{}) bool {
		tunnels = append(tunnels, value.(*Tunnel))
		return true
	})
	return tunnels
}

// CountTunnels 统计隧道数量
func (m *DefaultTunnelManager) CountTunnels() int {
	count := 0
	m.tunnels.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// CloseTunnel 关闭指定隧道
func (m *DefaultTunnelManager) CloseTunnel(tunnelID string, reason CloseReason) error {
	tunnel := m.GetTunnel(tunnelID)
	if tunnel == nil {
		return coreerrors.Newf(coreerrors.CodeNotFound, "tunnel not found: %s", tunnelID)
	}

	return tunnel.Close(reason, nil)
}

// CloseAll 关闭所有隧道
func (m *DefaultTunnelManager) CloseAll() {
	corelog.Infof("TunnelManager[%s]: closing all tunnels", m.role)

	m.tunnels.Range(func(key, value interface{}) bool {
		tunnel := value.(*Tunnel)
		tunnel.Close(CloseReasonContextCanceled, nil)
		return true
	})
}

// OnPeerClosed 接收对端关闭通知
func (m *DefaultTunnelManager) OnPeerClosed(tunnelID string, reason string, stats *TunnelStats) {
	corelog.Infof("TunnelManager[%s]: received peer close notification for %s, reason=%s",
		m.role, tunnelID, reason)

	// 查找隧道
	tunnel := m.GetTunnel(tunnelID)
	if tunnel == nil {
		corelog.Warnf("TunnelManager[%s]: tunnel %s not found for peer close", m.role, tunnelID)
		return
	}

	// 通知隧道对端已关闭
	tunnel.NotifyPeerClosed(reason, stats)
}

// Close 关闭管理器
func (m *DefaultTunnelManager) Close() error {
	corelog.Infof("TunnelManager[%s]: closing", m.role)

	// 关闭所有隧道
	m.CloseAll()

	// 关闭 dispose
	return m.ManagerBase.Close()
}

// ========== 实现 NotificationHandler 接口 ==========

// OnSystemMessage 系统消息通知
func (m *DefaultTunnelManager) OnSystemMessage(title, message, level string) {
	corelog.Debugf("TunnelManager[%s]: system message - %s: %s (level=%s)", m.role, title, message, level)
}

// OnQuotaWarning 配额预警通知
func (m *DefaultTunnelManager) OnQuotaWarning(quotaType string, usagePercent float64, message string) {
	corelog.Warnf("TunnelManager[%s]: quota warning - %s: %.1f%% - %s", m.role, quotaType, usagePercent, message)
}

// OnMappingEvent 映射事件通知
func (m *DefaultTunnelManager) OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string) {
	corelog.Infof("TunnelManager[%s]: mapping event %s - mappingID=%s, status=%s, message=%s",
		m.role, eventType.String(), mappingID, status, message)
}

// OnTunnelClosed 隧道关闭通知（关键方法）
func (m *DefaultTunnelManager) OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64) {
	corelog.Infof("TunnelManager[%s]: received tunnel close notification - tunnelID=%s, mappingID=%s, reason=%s",
		m.role, tunnelID, mappingID, reason)

	// 构造统计信息
	stats := &TunnelStats{
		BytesSent:  bytesSent,
		BytesRecv:  bytesRecv,
		DurationMs: durationMs,
	}

	// 调用 OnPeerClosed 触发实际的关闭逻辑
	m.OnPeerClosed(tunnelID, reason, stats)
}

// OnTunnelOpened 隧道打开通知
func (m *DefaultTunnelManager) OnTunnelOpened(tunnelID, mappingID string, peerClientID int64) {
	corelog.Infof("TunnelManager[%s]: tunnel opened - tunnelID=%s, mappingID=%s, peerClientID=%d",
		m.role, tunnelID, mappingID, peerClientID)
}

// OnTunnelError 隧道错误通知
func (m *DefaultTunnelManager) OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool) {
	if recoverable {
		corelog.Warnf("TunnelManager[%s]: tunnel error (recoverable) - tunnelID=%s, mappingID=%s, code=%s, message=%s",
			m.role, tunnelID, mappingID, errorCode, errorMessage)
	} else {
		corelog.Errorf("TunnelManager[%s]: tunnel error (fatal) - tunnelID=%s, mappingID=%s, code=%s, message=%s",
			m.role, tunnelID, mappingID, errorCode, errorMessage)

		// 如果是致命错误，尝试关闭该 tunnel
		if tunnel := m.GetTunnel(tunnelID); tunnel != nil {
			tunnel.Close(CloseReasonError, coreerrors.Newf(coreerrors.CodeInternalError, "%s: %s", errorCode, errorMessage))
		}
	}
}

// OnCustomNotification 自定义通知
func (m *DefaultTunnelManager) OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string) {
	corelog.Debugf("TunnelManager[%s]: custom notification from client %d - action=%s",
		m.role, senderClientID, action)
}

// OnGenericNotification 通用通知
func (m *DefaultTunnelManager) OnGenericNotification(notification *packet.ClientNotification) {
	corelog.Debugf("TunnelManager[%s]: generic notification - id=%s, type=%s",
		m.role, notification.NotifyID, notification.Type.String())
}
