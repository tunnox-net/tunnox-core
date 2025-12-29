// Package notify 通知处理模块
// 处理来自服务器的各种通知消息
package notify

import (
	"encoding/json"
	"sync"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// Handler 通知处理回调接口
type Handler interface {
	// OnSystemMessage 系统消息
	OnSystemMessage(title, message, level string)

	// OnQuotaWarning 配额预警
	OnQuotaWarning(quotaType string, usagePercent float64, message string)

	// OnMappingEvent 映射事件
	OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string)

	// OnTunnelClosed 隧道关闭
	OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64)

	// OnTunnelOpened 隧道打开
	OnTunnelOpened(tunnelID, mappingID string, peerClientID int64)

	// OnTunnelError 隧道错误
	OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool)

	// OnCustomNotification 自定义通知（C2C）
	OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string)

	// OnGenericNotification 通用通知（未处理的类型）
	OnGenericNotification(notification *packet.ClientNotification)
}

// DefaultHandler 默认通知处理器（仅记录日志）
type DefaultHandler struct{}

func (h *DefaultHandler) OnSystemMessage(title, message, level string) {
	corelog.Infof("Client: [NOTIFY] System message - %s: %s (level=%s)", title, message, level)
}

func (h *DefaultHandler) OnQuotaWarning(quotaType string, usagePercent float64, message string) {
	corelog.Warnf("Client: [NOTIFY] Quota warning - %s: %.1f%% used - %s", quotaType, usagePercent, message)
}

func (h *DefaultHandler) OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string) {
	corelog.Infof("Client: [NOTIFY] Mapping event %s - mappingID=%s, status=%s, message=%s",
		eventType.String(), mappingID, status, message)
}

func (h *DefaultHandler) OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64) {
	corelog.Infof("Client: [NOTIFY] Tunnel closed - tunnelID=%s, mappingID=%s, reason=%s, sent=%d, recv=%d, duration=%dms",
		tunnelID, mappingID, reason, bytesSent, bytesRecv, durationMs)
}

func (h *DefaultHandler) OnTunnelOpened(tunnelID, mappingID string, peerClientID int64) {
	corelog.Infof("Client: [NOTIFY] Tunnel opened - tunnelID=%s, mappingID=%s, peerClientID=%d",
		tunnelID, mappingID, peerClientID)
}

func (h *DefaultHandler) OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool) {
	if recoverable {
		corelog.Warnf("Client: [NOTIFY] Tunnel error (recoverable) - tunnelID=%s, code=%s, message=%s",
			tunnelID, errorCode, errorMessage)
	} else {
		corelog.Errorf("Client: [NOTIFY] Tunnel error (fatal) - tunnelID=%s, code=%s, message=%s",
			tunnelID, errorCode, errorMessage)
	}
}

func (h *DefaultHandler) OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string) {
	corelog.Infof("Client: [NOTIFY] Custom notification from client %d - action=%s, data=%v",
		senderClientID, action, data)
}

func (h *DefaultHandler) OnGenericNotification(notification *packet.ClientNotification) {
	corelog.Debugf("Client: [NOTIFY] Generic notification - id=%s, type=%s, priority=%v",
		notification.NotifyID, notification.Type.String(), notification.Priority)
}

// Dispatcher 通知分发器
type Dispatcher struct {
	handlers []Handler
	mu       sync.RWMutex
}

// NewDispatcher 创建通知分发器
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make([]Handler, 0),
	}
}

// AddHandler 添加通知处理器
func (d *Dispatcher) AddHandler(handler Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = append(d.handlers, handler)
}

// RemoveHandler 移除通知处理器
func (d *Dispatcher) RemoveHandler(handler Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, h := range d.handlers {
		if h == handler {
			d.handlers = append(d.handlers[:i], d.handlers[i+1:]...)
			return
		}
	}
}

// getHandlers 获取处理器列表的副本
func (d *Dispatcher) getHandlers() []Handler {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]Handler, len(d.handlers))
	copy(result, d.handlers)
	return result
}

// Dispatch 分发通知
func (d *Dispatcher) Dispatch(notification *packet.ClientNotification) {
	if notification == nil {
		return
	}

	handlers := d.getHandlers()
	if len(handlers) == 0 {
		corelog.Debugf("Client: no notification handlers registered, ignoring notification %s", notification.NotifyID)
		return
	}

	// 根据通知类型分发
	switch notification.Type {
	case packet.NotifyTypeSystemMessage:
		d.dispatchSystemMessage(notification, handlers)

	case packet.NotifyTypeQuotaWarning, packet.NotifyTypeQuotaExhausted:
		d.dispatchQuotaWarning(notification, handlers)

	case packet.NotifyTypeMappingCreated, packet.NotifyTypeMappingDeleted,
		packet.NotifyTypeMappingUpdated, packet.NotifyTypeMappingExpired,
		packet.NotifyTypeMappingActivated:
		d.dispatchMappingEvent(notification, handlers)

	case packet.NotifyTypeTunnelClosed:
		d.dispatchTunnelClosed(notification, handlers)

	case packet.NotifyTypeTunnelOpened:
		d.dispatchTunnelOpened(notification, handlers)

	case packet.NotifyTypeTunnelError:
		d.dispatchTunnelError(notification, handlers)

	case packet.NotifyTypeCustom:
		d.dispatchCustomNotification(notification, handlers)

	default:
		// 通用处理
		for _, h := range handlers {
			h.OnGenericNotification(notification)
		}
	}
}

func (d *Dispatcher) dispatchSystemMessage(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.SystemMessagePayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse SystemMessagePayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnSystemMessage(payload.Title, payload.Message, payload.Level)
	}
}

func (d *Dispatcher) dispatchQuotaWarning(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.QuotaWarningPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse QuotaWarningPayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnQuotaWarning(payload.QuotaType, payload.UsagePercent, payload.Message)
	}
}

func (d *Dispatcher) dispatchMappingEvent(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.MappingEventPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse MappingEventPayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnMappingEvent(notification.Type, payload.MappingID, payload.Status, payload.Message)
	}
}

func (d *Dispatcher) dispatchTunnelClosed(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.TunnelClosedPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse TunnelClosedPayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnTunnelClosed(payload.TunnelID, payload.MappingID, payload.Reason,
			payload.BytesSent, payload.BytesRecv, payload.Duration)
	}
}

func (d *Dispatcher) dispatchTunnelOpened(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.TunnelOpenedPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse TunnelOpenedPayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnTunnelOpened(payload.TunnelID, payload.MappingID, payload.PeerClientID)
	}
}

func (d *Dispatcher) dispatchTunnelError(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.TunnelErrorPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse TunnelErrorPayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnTunnelError(payload.TunnelID, payload.MappingID, payload.ErrorCode, payload.ErrorMessage, payload.Recoverable)
	}
}

func (d *Dispatcher) dispatchCustomNotification(notification *packet.ClientNotification, handlers []Handler) {
	var payload packet.CustomPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		corelog.Warnf("Client: failed to parse CustomPayload: %v", err)
		return
	}
	for _, h := range handlers {
		h.OnCustomNotification(notification.SenderClientID, payload.Action, payload.Data, payload.Raw)
	}
}
