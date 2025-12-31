package client

import (
	"encoding/json"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// SendTunnelCloseNotify 发送隧道关闭通知给对端客户端
// 实现 mapping.ClientInterface 接口
func (c *TunnoxClient) SendTunnelCloseNotify(targetClientID int64, tunnelID, mappingID, reason string) error {
	if !c.IsConnected() {
		corelog.Warnf("Client: cannot send tunnel close notify, not connected")
		return coreerrors.New(coreerrors.CodeConnectionError, "not connected")
	}

	if targetClientID <= 0 {
		corelog.Warnf("Client: invalid targetClientID for tunnel close notify: %d", targetClientID)
		return coreerrors.New(coreerrors.CodeInvalidParam, "invalid target client ID")
	}

	// 构造 TunnelClosedPayload
	payload := &packet.TunnelClosedPayload{
		TunnelID:  tunnelID,
		MappingID: mappingID,
		Reason:    reason,
		ClosedAt:  time.Now().UnixMilli(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal tunnel closed payload")
	}

	// 构造 C2CNotifyRequest
	c2cReq := &packet.C2CNotifyRequest{
		TargetClientID: targetClientID,
		Type:           packet.NotifyTypeTunnelClosed,
		Payload:        string(payloadBytes),
		Priority:       packet.PriorityHigh, // 高优先级，尽快送达
	}

	// 发送命令（不等待响应，异步发送）
	go func() {
		select {
		case <-c.Ctx().Done():
			return
		default:
			_, err := c.sendCommandAndWaitResponse(&CommandRequest{
				CommandType: packet.SendNotifyToClient,
				RequestBody: c2cReq,
			})
			if err != nil {
				corelog.Warnf("Client: failed to send tunnel close notify to client %d for tunnel %s: %v",
					targetClientID, tunnelID, err)
			} else {
				corelog.Debugf("Client: sent tunnel close notify to client %d for tunnel %s",
					targetClientID, tunnelID)
			}
		}
	}()

	return nil
}
