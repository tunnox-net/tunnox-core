package notify

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/packet"
)

// testHandler 测试用通知处理器
type testHandler struct {
	mu             sync.Mutex
	systemMessages []struct{ title, message, level string }
	quotaWarnings  []struct{ quotaType, message string }
	mappingEvents  []struct {
		eventType                  packet.NotificationType
		mappingID, status, message string
	}
	tunnelClosedEvents []struct{ tunnelID, mappingID, reason string }
	tunnelOpenedEvents []struct {
		tunnelID, mappingID string
		peerClientID        int64
	}
	tunnelErrorEvents []struct {
		tunnelID, errorCode, errorMessage string
		recoverable                       bool
	}
	customNotifications []struct {
		senderClientID int64
		action         string
		data           map[string]string
	}
	genericNotifications []*packet.ClientNotification
}

func newTestHandler() *testHandler {
	return &testHandler{}
}

func (h *testHandler) OnSystemMessage(title, message, level string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.systemMessages = append(h.systemMessages, struct{ title, message, level string }{title, message, level})
}

func (h *testHandler) OnQuotaWarning(quotaType string, usagePercent float64, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.quotaWarnings = append(h.quotaWarnings, struct{ quotaType, message string }{quotaType, message})
}

func (h *testHandler) OnMappingEvent(eventType packet.NotificationType, mappingID, status, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mappingEvents = append(h.mappingEvents, struct {
		eventType                  packet.NotificationType
		mappingID, status, message string
	}{eventType, mappingID, status, message})
}

func (h *testHandler) OnTunnelClosed(tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tunnelClosedEvents = append(h.tunnelClosedEvents, struct{ tunnelID, mappingID, reason string }{tunnelID, mappingID, reason})
}

func (h *testHandler) OnTunnelOpened(tunnelID, mappingID string, peerClientID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tunnelOpenedEvents = append(h.tunnelOpenedEvents, struct {
		tunnelID, mappingID string
		peerClientID        int64
	}{tunnelID, mappingID, peerClientID})
}

func (h *testHandler) OnTunnelError(tunnelID, mappingID, errorCode, errorMessage string, recoverable bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tunnelErrorEvents = append(h.tunnelErrorEvents, struct {
		tunnelID, errorCode, errorMessage string
		recoverable                       bool
	}{tunnelID, errorCode, errorMessage, recoverable})
}

func (h *testHandler) OnCustomNotification(senderClientID int64, action string, data map[string]string, raw string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.customNotifications = append(h.customNotifications, struct {
		senderClientID int64
		action         string
		data           map[string]string
	}{senderClientID, action, data})
}

func (h *testHandler) OnGenericNotification(notification *packet.ClientNotification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.genericNotifications = append(h.genericNotifications, notification)
}

func TestDispatcher_SystemMessage(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)

	payload := packet.SystemMessagePayload{
		Title:   "Test Title",
		Message: "Test Message",
		Level:   "info",
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeSystemMessage, string(payloadBytes))
	dispatcher.Dispatch(notification)

	require.Len(t, handler.systemMessages, 1)
	assert.Equal(t, "Test Title", handler.systemMessages[0].title)
	assert.Equal(t, "Test Message", handler.systemMessages[0].message)
	assert.Equal(t, "info", handler.systemMessages[0].level)
}

func TestDispatcher_TunnelClosed(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)

	payload := packet.TunnelClosedPayload{
		TunnelID:  "tunnel-123",
		MappingID: "mapping-456",
		Reason:    "normal",
		BytesSent: 1024,
		BytesRecv: 2048,
		Duration:  5000,
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeTunnelClosed, string(payloadBytes))
	dispatcher.Dispatch(notification)

	require.Len(t, handler.tunnelClosedEvents, 1)
	assert.Equal(t, "tunnel-123", handler.tunnelClosedEvents[0].tunnelID)
	assert.Equal(t, "mapping-456", handler.tunnelClosedEvents[0].mappingID)
	assert.Equal(t, "normal", handler.tunnelClosedEvents[0].reason)
}

func TestDispatcher_CustomNotification(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)

	payload := packet.CustomPayload{
		Action: "ping",
		Data:   map[string]string{"key": "value"},
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeCustom, string(payloadBytes)).
		WithSender(100)
	dispatcher.Dispatch(notification)

	require.Len(t, handler.customNotifications, 1)
	assert.Equal(t, int64(100), handler.customNotifications[0].senderClientID)
	assert.Equal(t, "ping", handler.customNotifications[0].action)
	assert.Equal(t, "value", handler.customNotifications[0].data["key"])
}

func TestDispatcher_MultipleHandlers(t *testing.T) {
	dispatcher := NewDispatcher()
	handler1 := newTestHandler()
	handler2 := newTestHandler()
	dispatcher.AddHandler(handler1)
	dispatcher.AddHandler(handler2)

	payload := packet.SystemMessagePayload{
		Title:   "Broadcast",
		Message: "Test",
		Level:   "info",
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeSystemMessage, string(payloadBytes))
	dispatcher.Dispatch(notification)

	// 两个处理器都应该收到通知
	assert.Len(t, handler1.systemMessages, 1)
	assert.Len(t, handler2.systemMessages, 1)
}

func TestDispatcher_RemoveHandler(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)
	dispatcher.RemoveHandler(handler)

	payload := packet.SystemMessagePayload{
		Title:   "Test",
		Message: "Test",
		Level:   "info",
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeSystemMessage, string(payloadBytes))
	dispatcher.Dispatch(notification)

	// 处理器已移除，不应收到通知
	assert.Len(t, handler.systemMessages, 0)
}

func TestDispatcher_NilNotification(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)

	// 不应该 panic
	dispatcher.Dispatch(nil)
}

func TestDispatcher_NoHandlers(t *testing.T) {
	dispatcher := NewDispatcher()

	notification := packet.NewNotification(packet.NotifyTypeSystemMessage, "{}")

	// 不应该 panic
	dispatcher.Dispatch(notification)
}

func TestDispatcher_MappingEvent(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)

	payload := packet.MappingEventPayload{
		MappingID: "mapping-001",
		Protocol:  "tcp",
		Status:    "active",
		Message:   "Mapping created",
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeMappingCreated, string(payloadBytes))
	dispatcher.Dispatch(notification)

	require.Len(t, handler.mappingEvents, 1)
	assert.Equal(t, packet.NotifyTypeMappingCreated, handler.mappingEvents[0].eventType)
	assert.Equal(t, "mapping-001", handler.mappingEvents[0].mappingID)
	assert.Equal(t, "active", handler.mappingEvents[0].status)
}

func TestDispatcher_QuotaWarning(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := newTestHandler()
	dispatcher.AddHandler(handler)

	payload := packet.QuotaWarningPayload{
		QuotaType:    "bandwidth",
		UsagePercent: 85.5,
		Message:      "Approaching limit",
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeQuotaWarning, string(payloadBytes))
	dispatcher.Dispatch(notification)

	require.Len(t, handler.quotaWarnings, 1)
	assert.Equal(t, "bandwidth", handler.quotaWarnings[0].quotaType)
	assert.Equal(t, "Approaching limit", handler.quotaWarnings[0].message)
}

func TestDefaultHandler(t *testing.T) {
	handler := &DefaultHandler{}

	// 验证所有方法都不会 panic
	handler.OnSystemMessage("Title", "Message", "info")
	handler.OnQuotaWarning("bandwidth", 85.5, "Warning")
	handler.OnMappingEvent(packet.NotifyTypeMappingCreated, "mapping-001", "active", "Created")
	handler.OnTunnelClosed("tunnel-001", "mapping-001", "normal", 1024, 2048, 5000)
	handler.OnTunnelOpened("tunnel-001", "mapping-001", 123)
	handler.OnTunnelError("tunnel-001", "mapping-001", "ERR001", "Connection failed", true)
	handler.OnCustomNotification(100, "ping", nil, "")
	handler.OnGenericNotification(packet.NewNotification(packet.NotifyTypeSystemMessage, "{}"))
}
