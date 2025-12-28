package command

import (
	"encoding/json"
	"fmt"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// NotifyClientAck 处理器（客户端 -> 服务端：确认收到通知）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// NotifyClientAckHandler 通知确认处理器
type NotifyClientAckHandler struct {
	*BaseHandler
}

// NewNotifyClientAckHandler 创建通知确认处理器
func NewNotifyClientAckHandler() *NotifyClientAckHandler {
	return &NotifyClientAckHandler{
		BaseHandler: NewBaseHandler(
			packet.NotifyClientAck,
			CategoryNotification,
			DirectionOneway,
			"notify_client_ack",
			"客户端确认收到通知",
		),
	}
}

// Handle 处理通知确认
func (h *NotifyClientAckHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	var ackReq packet.NotifyAckRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &ackReq); err != nil {
		corelog.Warnf("Failed to parse NotifyAckRequest: %v", err)
		return &CommandResponse{
			Success:   false,
			Error:     fmt.Sprintf("invalid request body: %v", err),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	corelog.Debugf("Received notification ack from client %d for notify_id: %s, received: %v, processed: %v",
		ctx.ClientID, ackReq.NotifyID, ackReq.Received, ackReq.Processed)

	// 通知确认目前仅记录日志，后续可扩展为：更新通知状态、触发后续流程等

	resp := &packet.NotifyAckResponse{
		Success: true,
	}
	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SendNotifyToClient 处理器（客户端A -> 服务端 -> 客户端B：C2C通知）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// NotificationRouter 通知路由接口（由外部实现，用于发送通知到目标客户端）
type NotificationRouter interface {
	// SendToClient 发送通知到目标客户端
	SendToClient(targetClientID int64, notification *packet.ClientNotification) error

	// IsClientOnline 检查客户端是否在线
	IsClientOnline(clientID int64) bool
}

// SendNotifyToClientHandler C2C通知处理器
type SendNotifyToClientHandler struct {
	*BaseHandler
	router NotificationRouter
}

// NewSendNotifyToClientHandler 创建C2C通知处理器
func NewSendNotifyToClientHandler(router NotificationRouter) *SendNotifyToClientHandler {
	return &SendNotifyToClientHandler{
		BaseHandler: NewBaseHandler(
			packet.SendNotifyToClient,
			CategoryNotification,
			DirectionDuplex,
			"send_notify_to_client",
			"C2C通知（客户端到客户端）",
		),
		router: router,
	}
}

// SetRouter 设置通知路由器
func (h *SendNotifyToClientHandler) SetRouter(router NotificationRouter) {
	h.router = router
}

// Handle 处理C2C通知请求
func (h *SendNotifyToClientHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	var req packet.C2CNotifyRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		corelog.Warnf("Failed to parse C2CNotifyRequest: %v", err)
		return &CommandResponse{
			Success:   false,
			Error:     fmt.Sprintf("invalid request body: %v", err),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 验证请求
	if req.TargetClientID == 0 {
		return &CommandResponse{
			Success:   false,
			Error:     "target_client_id is required",
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 不允许发送给自己
	if req.TargetClientID == ctx.ClientID {
		return &CommandResponse{
			Success:   false,
			Error:     "cannot send notification to self",
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 检查路由器是否可用
	if h.router == nil {
		corelog.Errorf("Notification router not configured")
		return &CommandResponse{
			Success:   false,
			Error:     "notification service unavailable",
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 检查目标客户端是否在线
	if !h.router.IsClientOnline(req.TargetClientID) {
		return &CommandResponse{
			Success:   false,
			Error:     "target client is offline",
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 生成通知ID
	notifyID := generateNotifyID()

	// 创建通知
	notification := packet.NewNotification(req.Type, req.Payload).
		WithSender(ctx.ClientID).
		WithPriority(req.Priority)

	notification.NotifyID = notifyID
	if req.ExpireAt > 0 {
		notification.ExpireAt = req.ExpireAt
	}
	if req.RequireAck {
		notification.RequireAck = true
	}

	// 发送通知到目标客户端
	if err := h.router.SendToClient(req.TargetClientID, notification); err != nil {
		corelog.Errorf("Failed to send notification to client %d: %v", req.TargetClientID, err)
		return &CommandResponse{
			Success:   false,
			Error:     fmt.Sprintf("failed to deliver notification: %v", err),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	corelog.Debugf("C2C notification sent from client %d to client %d, notify_id: %s",
		ctx.ClientID, req.TargetClientID, notifyID)

	resp := &packet.C2CNotifyResponse{
		Success:  true,
		NotifyID: notifyID,
	}
	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// generateNotifyID 生成通知ID
func generateNotifyID() string {
	randomPart, _ := utils.GenerateRandomInt(100000, 999999)
	return fmt.Sprintf("notify-%d-%d", time.Now().UnixMilli(), randomPart)
}
