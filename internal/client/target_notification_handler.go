package client

import (
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// TargetNotificationHandler 目标端通知处理器
// 处理来自 listenClient 的隧道关闭通知
type TargetNotificationHandler struct {
	tunnelManager *TargetTunnelManager
}

// NewTargetNotificationHandler 创建目标端通知处理器
func NewTargetNotificationHandler(tunnelManager *TargetTunnelManager) *TargetNotificationHandler {
	return &TargetNotificationHandler{
		tunnelManager: tunnelManager,
	}
}

func (h *TargetNotificationHandler) OnSystemMessage(title, message, level string) {
	corelog.Infof("Client: [NOTIFY] System message - %s: %s (level=%s)", title, message, level)
}

func (h *TargetNotificationHandler) OnQuotaWarning(quotaType string, usagePercent float64, message string) {
	corelog.Warnf("Client: [NOTIFY] Quota warning - %s: %.1f%% used - %s", quotaType, usagePercent, message)
}

func (h *TargetNotificationHandler) OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string) {
	corelog.Infof("Client: [NOTIFY] Mapping event %s - mappingID=%s, status=%s, message=%s",
		eventType.String(), mappingID, status, message)
}

// OnTunnelClosed 处理隧道关闭通知（核心处理逻辑）
func (h *TargetNotificationHandler) OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64) {
	corelog.Infof("Client: [NOTIFY] Tunnel closed - tunnelID=%s, mappingID=%s, reason=%s, sent=%d, recv=%d, duration=%dms",
		tunnelID, mappingID, reason, bytesSent, bytesRecv, durationMs)

	// 关闭对应的隧道连接
	if h.tunnelManager != nil {
		if h.tunnelManager.CloseTunnel(tunnelID) {
			corelog.Infof("Client: [NOTIFY] Successfully closed tunnel %s by peer notification", tunnelID)
		}
	}
}

func (h *TargetNotificationHandler) OnTunnelOpened(tunnelID, mappingID string, peerClientID int64) {
	corelog.Infof("Client: [NOTIFY] Tunnel opened - tunnelID=%s, mappingID=%s, peerClientID=%d",
		tunnelID, mappingID, peerClientID)
}

func (h *TargetNotificationHandler) OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool) {
	if recoverable {
		corelog.Warnf("Client: [NOTIFY] Tunnel error (recoverable) - tunnelID=%s, code=%s, message=%s",
			tunnelID, errorCode, errorMessage)
	} else {
		corelog.Errorf("Client: [NOTIFY] Tunnel error (fatal) - tunnelID=%s, code=%s, message=%s",
			tunnelID, errorCode, errorMessage)
	}
}

func (h *TargetNotificationHandler) OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string) {
	corelog.Infof("Client: [NOTIFY] Custom notification from client %d - action=%s, data=%v",
		senderClientID, action, data)
}

func (h *TargetNotificationHandler) OnGenericNotification(notification *packet.ClientNotification) {
	corelog.Debugf("Client: [NOTIFY] Generic notification - id=%s, type=%s, priority=%v",
		notification.NotifyID, notification.Type.String(), notification.Priority)
}
