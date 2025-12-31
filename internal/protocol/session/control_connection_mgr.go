package session

import (
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ============================================================================
// Control Connection 管理
// ============================================================================

// RegisterControlConnection 注册指令连接
func (s *SessionManager) RegisterControlConnection(conn *ControlConnection) {
	// 委托给 clientRegistry
	if err := s.clientRegistry.Register(conn); err != nil {
		corelog.Errorf("Failed to register control connection: %v", err)
	}
}

// UpdateControlConnectionAuth 更新指令连接的认证信息
func (s *SessionManager) UpdateControlConnectionAuth(connID string, clientID int64, userID string) error {
	// 委托给 clientRegistry
	return s.clientRegistry.UpdateAuth(connID, clientID, userID)
}

// GetControlConnection 根据 ConnectionID 获取指令连接
func (s *SessionManager) GetControlConnection(connID string) *ControlConnection {
	return s.clientRegistry.GetByConnID(connID)
}

// GetControlConnectionByClientID 根据 ClientID 获取指令连接（类型安全版本）
func (s *SessionManager) GetControlConnectionByClientID(clientID int64) *ControlConnection {
	return s.clientRegistry.GetByClientID(clientID)
}

// GetControlConnectionInterface 根据 ClientID 获取指令连接（返回接口用于API）
func (s *SessionManager) GetControlConnectionInterface(clientID int64) ControlConnectionInterface {
	return s.GetControlConnectionByClientID(clientID)
}

// KickOldControlConnection 踢掉旧的指令连接
func (s *SessionManager) KickOldControlConnection(clientID int64, newConnID string) {
	s.clientRegistry.KickOldConnection(clientID, newConnID, s.sendKickCommand)
}

// sendKickCommand 发送踢下线命令
func (s *SessionManager) sendKickCommand(conn *ControlConnection, reason, code string) {
	if conn == nil || conn.Stream == nil {
		return
	}

	kickBody := `{"reason":"` + reason + `","code":"` + code + `"}`

	kickPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.KickClient,
			CommandBody: kickBody,
		},
	}

	if _, err := conn.Stream.WritePacket(kickPkt, true, 0); err != nil {
		corelog.Warnf("Failed to send kick command to %s: %v", conn.ConnID, err)
	} else {
		corelog.Infof("Sent kick command to client %d (connID=%s): %s", conn.ClientID, conn.ConnID, reason)
	}
}

// RemoveControlConnection 移除指令连接
func (s *SessionManager) RemoveControlConnection(connID string) {
	s.clientRegistry.Remove(connID)
}

// getControlConnectionByConnID 根据连接ID获取控制连接（内部使用）
func (s *SessionManager) getControlConnectionByConnID(connID string) *ControlConnection {
	return s.GetControlConnection(connID)
}

// GetClientIDByConnectionID 根据连接ID获取客户端ID（用于命令上下文填充）
func (s *SessionManager) GetClientIDByConnectionID(connID string) int64 {
	conn := s.getControlConnectionByConnID(connID)
	if conn != nil {
		return conn.ClientID
	}
	return 0
}

// ============================================================================
// 连接清理（心跳超时检测）
// ============================================================================

// startConnectionCleanup 启动连接清理协程
// 定期检查并清理超时未发送心跳的控制连接
func (s *SessionManager) startConnectionCleanup() {
	if s.config == nil {
		corelog.Warnf("SessionManager: config is nil, cleanup disabled")
		return
	}

	go func() {
		ticker := time.NewTicker(s.config.CleanupInterval)
		defer ticker.Stop()

		corelog.Infof("SessionManager: connection cleanup started (interval=%v, timeout=%v)",
			s.config.CleanupInterval, s.config.HeartbeatTimeout)

		for {
			select {
			case <-ticker.C:
				if cleaned := s.cleanupStaleConnections(); cleaned > 0 {
					corelog.Infof("SessionManager: cleaned up %d stale connections", cleaned)
				}

			case <-s.Ctx().Done():
				corelog.Infof("SessionManager: connection cleanup stopped")
				return
			}
		}
	}()
}

// cleanupStaleConnections 清理过期的控制连接
// 返回清理的连接数量
func (s *SessionManager) cleanupStaleConnections() int {
	if s.config == nil {
		return 0
	}

	// 委托给 clientRegistry
	return s.clientRegistry.CleanupStale(s.config.HeartbeatTimeout, s.CloseConnection)
}
