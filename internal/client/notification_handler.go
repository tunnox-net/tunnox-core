// Package client notification_handler.go
// 通知处理 facade - 向后兼容层
// 实际实现已移至 internal/client/notify 子包
package client

import (
	"tunnox-core/internal/client/notify"
	"tunnox-core/internal/packet"
)

// NotificationHandler 通知处理回调接口
// Deprecated: 请使用 notify.Handler
type NotificationHandler = notify.Handler

// DefaultNotificationHandler 默认通知处理器
// Deprecated: 请使用 notify.DefaultHandler
type DefaultNotificationHandler = notify.DefaultHandler

// NotificationDispatcher 通知分发器
// Deprecated: 请使用 notify.Dispatcher
type NotificationDispatcher = notify.Dispatcher

// NewNotificationDispatcher 创建通知分发器
// Deprecated: 请使用 notify.NewDispatcher
func NewNotificationDispatcher() *NotificationDispatcher {
	return notify.NewDispatcher()
}

// notifyHandlerAdapter 适配器，将旧接口转换为新接口
type notifyHandlerAdapter struct {
	handler notify.Handler
}

func (a *notifyHandlerAdapter) OnSystemMessage(title, message, level string) {
	a.handler.OnSystemMessage(title, message, level)
}

func (a *notifyHandlerAdapter) OnQuotaWarning(quotaType string, usagePercent float64, message string) {
	a.handler.OnQuotaWarning(quotaType, usagePercent, message)
}

func (a *notifyHandlerAdapter) OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string) {
	a.handler.OnMappingEvent(eventType, mappingID, status, message)
}

func (a *notifyHandlerAdapter) OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64) {
	a.handler.OnTunnelClosed(tunnelID, mappingID, reason, bytesSent, bytesRecv, durationMs)
}

func (a *notifyHandlerAdapter) OnTunnelOpened(tunnelID, mappingID string, peerClientID int64) {
	a.handler.OnTunnelOpened(tunnelID, mappingID, peerClientID)
}

func (a *notifyHandlerAdapter) OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool) {
	a.handler.OnTunnelError(tunnelID, mappingID, errorCode, errorMessage, recoverable)
}

func (a *notifyHandlerAdapter) OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string) {
	a.handler.OnCustomNotification(senderClientID, action, data, raw)
}

func (a *notifyHandlerAdapter) OnGenericNotification(notification *packet.ClientNotification) {
	a.handler.OnGenericNotification(notification)
}
