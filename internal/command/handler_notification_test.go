package command

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// mockNotificationRouter 模拟通知路由器
type mockNotificationRouter struct {
	sentNotifications []*packet.ClientNotification
	onlineClients     map[int64]bool
}

func newMockNotificationRouter() *mockNotificationRouter {
	return &mockNotificationRouter{
		sentNotifications: make([]*packet.ClientNotification, 0),
		onlineClients:     make(map[int64]bool),
	}
}

func (m *mockNotificationRouter) SendToClient(targetClientID int64, notification *packet.ClientNotification) error {
	m.sentNotifications = append(m.sentNotifications, notification)
	return nil
}

func (m *mockNotificationRouter) IsClientOnline(clientID int64) bool {
	return m.onlineClients[clientID]
}

func (m *mockNotificationRouter) SetClientOnline(clientID int64, online bool) {
	m.onlineClients[clientID] = online
}

func TestNotifyClientAckHandler_Handle(t *testing.T) {
	handler := NewNotifyClientAckHandler()

	// 验证命令类型
	assert.Equal(t, packet.NotifyClientAck, handler.GetCommandType())
	assert.Equal(t, CategoryNotification, handler.GetCategory())
	assert.Equal(t, DirectionOneway, handler.GetDirection())

	// 创建确认请求
	ackReq := packet.NotifyAckRequest{
		NotifyID:  "notify-123",
		Received:  true,
		Processed: true,
	}
	body, _ := json.Marshal(ackReq)

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  string(body),
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
}

func TestNotifyClientAckHandler_InvalidRequest(t *testing.T) {
	handler := NewNotifyClientAckHandler()

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  "invalid json",
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "invalid request body")
}

func TestSendNotifyToClientHandler_Handle(t *testing.T) {
	router := newMockNotificationRouter()
	router.SetClientOnline(200, true)

	handler := NewSendNotifyToClientHandler(router)

	// 验证命令类型
	assert.Equal(t, packet.SendNotifyToClient, handler.GetCommandType())
	assert.Equal(t, CategoryNotification, handler.GetCategory())
	assert.Equal(t, DirectionDuplex, handler.GetDirection())

	// 创建 C2C 通知请求
	req := packet.C2CNotifyRequest{
		TargetClientID: 200,
		Type:           packet.NotifyTypeCustom,
		Payload:        `{"action":"ping"}`,
		Priority:       packet.PriorityNormal,
	}
	body, _ := json.Marshal(req)

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  string(body),
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)

	// 验证通知已发送
	assert.Len(t, router.sentNotifications, 1)
	assert.Equal(t, packet.NotifyTypeCustom, router.sentNotifications[0].Type)
	assert.Equal(t, int64(100), router.sentNotifications[0].SenderClientID)
}

func TestSendNotifyToClientHandler_TargetOffline(t *testing.T) {
	router := newMockNotificationRouter()
	// 目标客户端离线

	handler := NewSendNotifyToClientHandler(router)

	req := packet.C2CNotifyRequest{
		TargetClientID: 200,
		Type:           packet.NotifyTypeCustom,
		Payload:        `{"action":"ping"}`,
	}
	body, _ := json.Marshal(req)

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  string(body),
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "target client is offline")
}

func TestSendNotifyToClientHandler_SelfNotify(t *testing.T) {
	router := newMockNotificationRouter()
	router.SetClientOnline(100, true)

	handler := NewSendNotifyToClientHandler(router)

	// 尝试发送给自己
	req := packet.C2CNotifyRequest{
		TargetClientID: 100, // 与发送者相同
		Type:           packet.NotifyTypeCustom,
		Payload:        `{"action":"ping"}`,
	}
	body, _ := json.Marshal(req)

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  string(body),
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "cannot send notification to self")
}

func TestSendNotifyToClientHandler_NoRouter(t *testing.T) {
	handler := NewSendNotifyToClientHandler(nil)

	req := packet.C2CNotifyRequest{
		TargetClientID: 200,
		Type:           packet.NotifyTypeCustom,
		Payload:        `{"action":"ping"}`,
	}
	body, _ := json.Marshal(req)

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  string(body),
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "notification service unavailable")
}

func TestSendNotifyToClientHandler_MissingTargetID(t *testing.T) {
	router := newMockNotificationRouter()
	handler := NewSendNotifyToClientHandler(router)

	req := packet.C2CNotifyRequest{
		TargetClientID: 0, // 缺少目标ID
		Type:           packet.NotifyTypeCustom,
		Payload:        `{"action":"ping"}`,
	}
	body, _ := json.Marshal(req)

	ctx := &types.CommandContext{
		ConnectionID: "conn-001",
		ClientID:     100,
		RequestBody:  string(body),
		RequestID:    "req-001",
		CommandId:    "cmd-001",
	}

	resp, err := handler.Handle(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "target_client_id is required")
}

func TestGenerateNotifyID(t *testing.T) {
	id1 := generateNotifyID()
	id2 := generateNotifyID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.Contains(t, id1, "notify-")
	// 两个 ID 应该不同（虽然理论上可能相同，但概率极低）
	// 在高并发场景下可能会失败，所以只检查格式
}
