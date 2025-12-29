package client

import (
	corelog "tunnox-core/internal/core/log"
)

// TargetNotificationHandler 目标端通知处理器
// 处理来自 listenClient 的隧道关闭通知
// 通过嵌入 DefaultNotificationHandler 复用默认实现，只覆写需要特殊处理的方法
type TargetNotificationHandler struct {
	DefaultNotificationHandler // 嵌入默认处理器
	tunnelManager              *TargetTunnelManager
}

// NewTargetNotificationHandler 创建目标端通知处理器
func NewTargetNotificationHandler(tunnelManager *TargetTunnelManager) *TargetNotificationHandler {
	return &TargetNotificationHandler{
		tunnelManager: tunnelManager,
	}
}

// OnTunnelClosed 处理隧道关闭通知（覆写：核心处理逻辑）
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
