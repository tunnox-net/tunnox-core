package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/command"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils/random"
)

// NotificationService 通知服务
// 负责向客户端发送通知，实现 command.NotificationRouter 接口
type NotificationService struct {
	*dispose.ServiceBase

	registry *ClientRegistry // 客户端注册表
	mu       sync.RWMutex
}

// NewNotificationService 创建通知服务
func NewNotificationService(parentCtx context.Context, registry *ClientRegistry) *NotificationService {
	ns := &NotificationService{
		ServiceBase: dispose.NewService("NotificationService", parentCtx),
		registry:    registry,
	}
	return ns
}

// SendToClient 发送通知到目标客户端
// 实现 command.NotificationRouter 接口
func (ns *NotificationService) SendToClient(targetClientID int64, notification *packet.ClientNotification) error {
	if ns.IsClosed() {
		return coreerrors.New(coreerrors.CodeServiceClosed, "notification service is closed")
	}

	if notification == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "notification cannot be nil")
	}

	// 检查通知是否已过期
	if notification.IsExpired() {
		corelog.Warnf("NotificationService: notification %s has expired, skipping", notification.NotifyID)
		return coreerrors.New(coreerrors.CodeExpired, "notification has expired")
	}

	// 获取目标客户端的控制连接
	conn := ns.registry.GetByClientID(targetClientID)
	if conn == nil {
		return coreerrors.Newf(coreerrors.CodeClientOffline, "client %d not found or offline", targetClientID)
	}

	// 序列化通知
	notifyBody, err := json.Marshal(notification)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal notification")
	}

	// 创建通知数据包
	notifyPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.NotifyClient,
			CommandBody: string(notifyBody),
		},
	}

	// 发送通知
	if _, err := conn.Stream.WritePacket(notifyPkt, true, 0); err != nil {
		corelog.Errorf("NotificationService: failed to send notification %s to client %d: %v",
			notification.NotifyID, targetClientID, err)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send notification")
	}

	corelog.Debugf("NotificationService: sent notification %s (type=%s) to client %d",
		notification.NotifyID, notification.Type.String(), targetClientID)

	return nil
}

// IsClientOnline 检查客户端是否在线
// 实现 command.NotificationRouter 接口
func (ns *NotificationService) IsClientOnline(clientID int64) bool {
	if ns.IsClosed() {
		return false
	}

	conn := ns.registry.GetByClientID(clientID)
	return conn != nil && conn.Authenticated
}

// BroadcastToAll 广播通知到所有在线客户端
func (ns *NotificationService) BroadcastToAll(notification *packet.ClientNotification) (successCount int, failCount int) {
	if ns.IsClosed() || notification == nil {
		return 0, 0
	}

	// 检查通知是否已过期
	if notification.IsExpired() {
		corelog.Warnf("NotificationService: notification %s has expired, skipping broadcast", notification.NotifyID)
		return 0, 0
	}

	// 序列化通知
	notifyBody, err := json.Marshal(notification)
	if err != nil {
		corelog.Errorf("NotificationService: failed to marshal notification for broadcast: %v", err)
		return 0, 0
	}

	// 创建通知数据包
	notifyPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.NotifyClient,
			CommandBody: string(notifyBody),
		},
	}

	// 遍历所有已认证的连接
	connections := ns.registry.ListAuthenticated()
	for _, conn := range connections {
		if _, err := conn.Stream.WritePacket(notifyPkt, true, 0); err != nil {
			corelog.Warnf("NotificationService: failed to broadcast to client %d: %v", conn.ClientID, err)
			failCount++
		} else {
			successCount++
		}
	}

	corelog.Infof("NotificationService: broadcast notification %s to %d clients (%d failed)",
		notification.NotifyID, successCount, failCount)

	return successCount, failCount
}

// SendSystemMessage 发送系统消息
func (ns *NotificationService) SendSystemMessage(targetClientID int64, title, message, level string) error {
	payload := &packet.SystemMessagePayload{
		Title:   title,
		Message: message,
		Level:   level,
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeSystemMessage, string(payloadBytes))
	notification.NotifyID = ns.generateNotifyID()

	return ns.SendToClient(targetClientID, notification)
}

// SendTunnelClosedNotification 发送隧道关闭通知
func (ns *NotificationService) SendTunnelClosedNotification(targetClientID int64, tunnelID, mappingID, reason string, bytesSent, bytesRecv, durationMs int64) error {
	payload := &packet.TunnelClosedPayload{
		TunnelID:  tunnelID,
		MappingID: mappingID,
		Reason:    reason,
		BytesSent: bytesSent,
		BytesRecv: bytesRecv,
		Duration:  durationMs,
		ClosedAt:  time.Now().UnixMilli(),
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(packet.NotifyTypeTunnelClosed, string(payloadBytes)).
		WithPriority(packet.PriorityHigh)
	notification.NotifyID = ns.generateNotifyID()

	return ns.SendToClient(targetClientID, notification)
}

// SendMappingEvent 发送映射事件通知
func (ns *NotificationService) SendMappingEvent(targetClientID int64, eventType packet.NotificationType, mappingID, protocol string, sourcePort, targetPort int, targetHost, status, message string) error {
	payload := &packet.MappingEventPayload{
		MappingID:  mappingID,
		Protocol:   protocol,
		SourcePort: sourcePort,
		TargetHost: targetHost,
		TargetPort: targetPort,
		Status:     status,
		Message:    message,
	}
	payloadBytes, _ := json.Marshal(payload)

	notification := packet.NewNotification(eventType, string(payloadBytes))
	notification.NotifyID = ns.generateNotifyID()

	return ns.SendToClient(targetClientID, notification)
}

// generateNotifyID 生成通知ID
func (ns *NotificationService) generateNotifyID() string {
	randomPart, _ := random.Int(100000, 999999)
	return fmt.Sprintf("notify-%d-%d", time.Now().UnixMilli(), randomPart)
}

// 确保实现 command.NotificationRouter 接口
var _ command.NotificationRouter = (*NotificationService)(nil)
