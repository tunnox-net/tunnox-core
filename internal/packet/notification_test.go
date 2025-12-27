package packet

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationType_String(t *testing.T) {
	tests := []struct {
		name     string
		notType  NotificationType
		expected string
	}{
		{"SystemMessage", NotifyTypeSystemMessage, "SystemMessage"},
		{"QuotaWarning", NotifyTypeQuotaWarning, "QuotaWarning"},
		{"MappingCreated", NotifyTypeMappingCreated, "MappingCreated"},
		{"TunnelClosed", NotifyTypeTunnelClosed, "TunnelClosed"},
		{"Custom", NotifyTypeCustom, "Custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.notType.String())
		})
	}
}

func TestNotificationType_Categories(t *testing.T) {
	// 系统通知
	assert.True(t, NotifyTypeSystemMessage.IsSystemNotification())
	assert.False(t, NotifyTypeSystemMessage.IsMappingNotification())
	assert.False(t, NotifyTypeSystemMessage.IsTunnelNotification())

	// 映射通知
	assert.False(t, NotifyTypeMappingCreated.IsSystemNotification())
	assert.True(t, NotifyTypeMappingCreated.IsMappingNotification())
	assert.False(t, NotifyTypeMappingCreated.IsTunnelNotification())

	// 隧道通知
	assert.False(t, NotifyTypeTunnelClosed.IsSystemNotification())
	assert.False(t, NotifyTypeTunnelClosed.IsMappingNotification())
	assert.True(t, NotifyTypeTunnelClosed.IsTunnelNotification())

	// 自定义通知
	assert.True(t, NotifyTypeCustom.IsCustomNotification())
}

func TestNewNotification(t *testing.T) {
	payload := `{"message":"test"}`
	notification := NewNotification(NotifyTypeSystemMessage, payload)

	assert.NotNil(t, notification)
	assert.Equal(t, NotifyTypeSystemMessage, notification.Type)
	assert.Equal(t, payload, notification.Payload)
	assert.Equal(t, PriorityNormal, notification.Priority)
	assert.True(t, notification.Timestamp > 0)
}

func TestClientNotification_WithMethods(t *testing.T) {
	notification := NewNotification(NotifyTypeSystemMessage, "test")

	// 测试链式调用
	notification.WithPriority(PriorityHigh).
		WithSender(123).
		WithAckRequired()

	assert.Equal(t, PriorityHigh, notification.Priority)
	assert.Equal(t, int64(123), notification.SenderClientID)
	assert.True(t, notification.RequireAck)
}

func TestClientNotification_WithExpiry(t *testing.T) {
	notification := NewNotification(NotifyTypeSystemMessage, "test")
	expireTime := time.Now().Add(1 * time.Hour)

	notification.WithExpiry(expireTime)

	assert.Equal(t, expireTime.UnixMilli(), notification.ExpireAt)
}

func TestClientNotification_IsExpired(t *testing.T) {
	// 未设置过期时间
	notification := NewNotification(NotifyTypeSystemMessage, "test")
	assert.False(t, notification.IsExpired())

	// 设置未来的过期时间
	notification.ExpireAt = time.Now().Add(1 * time.Hour).UnixMilli()
	assert.False(t, notification.IsExpired())

	// 设置过去的过期时间
	notification.ExpireAt = time.Now().Add(-1 * time.Hour).UnixMilli()
	assert.True(t, notification.IsExpired())
}

func TestSystemMessagePayload_Serialization(t *testing.T) {
	payload := SystemMessagePayload{
		Title:   "Test Title",
		Message: "Test Message",
		Level:   "info",
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded SystemMessagePayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload.Title, decoded.Title)
	assert.Equal(t, payload.Message, decoded.Message)
	assert.Equal(t, payload.Level, decoded.Level)
}

func TestTunnelClosedPayload_Serialization(t *testing.T) {
	payload := TunnelClosedPayload{
		TunnelID:  "tunnel-123",
		MappingID: "mapping-456",
		Reason:    "normal",
		BytesSent: 1024,
		BytesRecv: 2048,
		Duration:  5000,
		ClosedAt:  time.Now().UnixMilli(),
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded TunnelClosedPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload.TunnelID, decoded.TunnelID)
	assert.Equal(t, payload.MappingID, decoded.MappingID)
	assert.Equal(t, payload.Reason, decoded.Reason)
	assert.Equal(t, payload.BytesSent, decoded.BytesSent)
	assert.Equal(t, payload.BytesRecv, decoded.BytesRecv)
	assert.Equal(t, payload.Duration, decoded.Duration)
}

func TestC2CNotifyRequest_Serialization(t *testing.T) {
	req := C2CNotifyRequest{
		TargetClientID: 456,
		Type:           NotifyTypeCustom,
		Payload:        `{"action":"ping"}`,
		Priority:       PriorityHigh,
		RequireAck:     true,
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded C2CNotifyRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.TargetClientID, decoded.TargetClientID)
	assert.Equal(t, req.Type, decoded.Type)
	assert.Equal(t, req.Payload, decoded.Payload)
	assert.Equal(t, req.Priority, decoded.Priority)
	assert.Equal(t, req.RequireAck, decoded.RequireAck)
}

func TestNotifyAckRequest_Serialization(t *testing.T) {
	req := NotifyAckRequest{
		NotifyID:  "notify-123",
		Received:  true,
		Processed: true,
		Error:     "",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded NotifyAckRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.NotifyID, decoded.NotifyID)
	assert.Equal(t, req.Received, decoded.Received)
	assert.Equal(t, req.Processed, decoded.Processed)
}

func TestClientNotification_FullSerialization(t *testing.T) {
	// 创建完整的通知
	systemPayload := SystemMessagePayload{
		Title:   "Server Maintenance",
		Message: "The server will restart in 5 minutes",
		Level:   "warning",
	}
	payloadBytes, _ := json.Marshal(systemPayload)

	notification := &ClientNotification{
		NotifyID:       "notify-001",
		Type:           NotifyTypeSystemMessage,
		Timestamp:      time.Now().UnixMilli(),
		Payload:        string(payloadBytes),
		SenderClientID: 0,
		Priority:       PriorityHigh,
		ExpireAt:       time.Now().Add(5 * time.Minute).UnixMilli(),
		RequireAck:     true,
	}

	// 序列化
	data, err := json.Marshal(notification)
	require.NoError(t, err)

	// 反序列化
	var decoded ClientNotification
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, notification.NotifyID, decoded.NotifyID)
	assert.Equal(t, notification.Type, decoded.Type)
	assert.Equal(t, notification.Priority, decoded.Priority)
	assert.Equal(t, notification.RequireAck, decoded.RequireAck)

	// 解析 payload
	var decodedPayload SystemMessagePayload
	err = json.Unmarshal([]byte(decoded.Payload), &decodedPayload)
	require.NoError(t, err)
	assert.Equal(t, systemPayload.Title, decodedPayload.Title)
	assert.Equal(t, systemPayload.Message, decodedPayload.Message)
}
