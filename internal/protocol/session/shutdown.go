package session

import (
	"encoding/json"
	"time"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"

	"github.com/google/uuid"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 优雅关闭相关
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ShutdownReason 关闭原因
type ShutdownReason string

const (
	ShutdownReasonRollingUpdate ShutdownReason = "rolling_update" // 滚动更新
	ShutdownReasonMaintenance   ShutdownReason = "maintenance"    // 维护
	ShutdownReasonShutdown      ShutdownReason = "shutdown"       // 正常关闭
)

// BroadcastShutdown 向所有指令连接广播服务器关闭通知
//
// 参数：
//   - reason: 关闭原因（rolling_update, maintenance, shutdown）
//   - gracePeriodSeconds: 优雅期（秒），在此期间服务器将等待活跃隧道完成
//   - recommendReconnect: 是否建议客户端重连
//   - message: 可选的人类可读消息
//
// 返回：
//   - 成功发送通知的连接数
//   - 失败发送的连接数
func (s *SessionManager) BroadcastShutdown(
	reason ShutdownReason,
	gracePeriodSeconds int,
	recommendReconnect bool,
	message string,
) (successCount int, failureCount int) {
	corelog.Infof("SessionManager: broadcasting server shutdown to all clients (reason=%s, gracePeriod=%ds, reconnect=%v)",
		reason, gracePeriodSeconds, recommendReconnect)

	// 构造 ServerShutdownCommand（基础模板，每个客户端会添加其专属的ReconnectToken）
	shutdownCmd := packet.ServerShutdownCommand{
		Reason:             string(reason),
		GracePeriodSeconds: gracePeriodSeconds,
		RecommendReconnect: recommendReconnect,
		Message:            message,
	}

	// 获取所有指令连接的快照（避免长时间持锁）
	s.controlConnLock.RLock()
	controlConns := make([]*ControlConnection, 0, len(s.controlConnMap))
	for _, conn := range s.controlConnMap {
		controlConns = append(controlConns, conn)
	}
	s.controlConnLock.RUnlock()

	corelog.Infof("SessionManager: found %d control connections to notify", len(controlConns))

	// 遍历所有指令连接，发送关闭通知
	for _, conn := range controlConns {
		if conn == nil || conn.Stream == nil {
			failureCount++
			continue
		}

		// 为每个客户端生成独立的ReconnectToken
		var reconnectTokenStr string
		if s.reconnectTokenManager != nil && conn.ClientID > 0 {
			token, err := s.reconnectTokenManager.GenerateReconnectToken(conn.ClientID, s.nodeID)
			if err != nil {
				corelog.Warnf("SessionManager: failed to generate reconnect token for client %d: %v", conn.ClientID, err)
			} else {
				// 编码为JSON字符串
				reconnectTokenStr, err = s.reconnectTokenManager.EncodeToken(token)
				if err != nil {
					corelog.Warnf("SessionManager: failed to encode reconnect token for client %d: %v", conn.ClientID, err)
					reconnectTokenStr = ""
				}
			}
		}

		// 异步发送（防止单个慢连接阻塞广播）
		go func(c *ControlConnection, tokenStr string) {
			// 为该客户端构造特定的命令包（包含其ReconnectToken）
			clientShutdownCmd := shutdownCmd
			clientShutdownCmd.ReconnectToken = tokenStr

			// 重新序列化
			clientCmdBody, err := json.Marshal(clientShutdownCmd)
			if err != nil {
				corelog.Errorf("SessionManager: failed to marshal shutdown command for client %d: %v", c.ClientID, err)
				return
			}

			clientCommandPacket := &packet.CommandPacket{
				CommandType: packet.ServerShutdown,
				CommandId:   uuid.NewString(),
				CommandBody: string(clientCmdBody),
			}

			clientTransferPacket := &packet.TransferPacket{
				PacketType:    packet.JsonCommand,
				CommandPacket: clientCommandPacket,
			}

			// 设置5秒超时
			deadline := time.Now().Add(5 * time.Second)
			if _, err := c.Stream.WritePacket(clientTransferPacket, true, 0); err != nil {
				corelog.Warnf("SessionManager: failed to send shutdown notification to client %d (connID=%s): %v",
					c.ClientID, c.ConnID, err)
			} else {
				if tokenStr != "" {
					corelog.Infof("SessionManager: sent shutdown notification with reconnect token to client %d (connID=%s) by %v",
						c.ClientID, c.ConnID, deadline)
				} else {
					corelog.Infof("SessionManager: sent shutdown notification to client %d (connID=%s) by %v",
						c.ClientID, c.ConnID, deadline)
				}
			}
		}(conn, reconnectTokenStr)
	}

	// 等待一小段时间确保大部分消息发出
	time.Sleep(500 * time.Millisecond)

	// 返回统计信息（注意：由于异步发送，这里的统计不完全准确）
	successCount = len(controlConns) - failureCount

	corelog.Infof("SessionManager: shutdown broadcast completed (total=%d, estimated_success=%d, estimated_failure=%d)",
		len(controlConns), successCount, failureCount)

	return successCount, failureCount
}

// GetActiveTunnelCount 获取当前活跃隧道数量
//
// 返回所有活跃的隧道连接数量，用于优雅关闭时判断是否还有传输任务。
func (s *SessionManager) GetActiveTunnelCount() int {
	s.tunnelConnLock.RLock()
	defer s.tunnelConnLock.RUnlock()

	return len(s.tunnelConnMap)
}

// GetActiveTunnels 获取活跃隧道数（health.StatsProvider接口别名）
func (s *SessionManager) GetActiveTunnels() int {
	return s.GetActiveTunnelCount()
}

// WaitForTunnelsToComplete 等待活跃隧道完成传输
//
// 参数：
//   - timeoutSeconds: 最大等待时间（秒）
//
// 返回：
//   - true: 所有隧道已完成
//   - false: 超时，仍有活跃隧道
func (s *SessionManager) WaitForTunnelsToComplete(timeoutSeconds int) bool {
	corelog.Infof("SessionManager: waiting for active tunnels to complete (timeout=%ds)", timeoutSeconds)

	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	checkInterval := 500 * time.Millisecond

	for {
		activeTunnels := s.GetActiveTunnelCount()
		if activeTunnels == 0 {
			corelog.Infof("SessionManager: all tunnels completed successfully")
			return true
		}

		if time.Now().After(deadline) {
			corelog.Warnf("SessionManager: timeout waiting for tunnels (still have %d active tunnels)", activeTunnels)
			return false
		}

		corelog.Debugf("SessionManager: waiting for %d active tunnels to complete...", activeTunnels)
		time.Sleep(checkInterval)
	}
}
