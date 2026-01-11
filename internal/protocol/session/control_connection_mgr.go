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
	// 先获取连接信息，用于后续调用 DisconnectClient
	conn := s.clientRegistry.GetByConnID(connID)
	var clientID int64
	var authenticated bool
	if conn != nil {
		clientID = conn.ClientID
		authenticated = conn.Authenticated
	}

	// 从注册表移除
	s.clientRegistry.Remove(connID)

	// 如果是已认证的控制连接，通知云控层（触发 webhook）
	// 使用 DisconnectClientIfMatch 避免多节点竞争：只有当 Redis 中的状态与当前节点/连接匹配时才删除
	if authenticated && clientID > 0 && s.cloudControl != nil {
		disconnected, err := s.cloudControl.DisconnectClientIfMatch(clientID, s.nodeID, connID)
		if err != nil {
			corelog.Warnf("RemoveControlConnection: failed to disconnect client %d: %v", clientID, err)
		} else if disconnected {
			corelog.Infof("RemoveControlConnection: client %d disconnected from node %s", clientID, s.nodeID)
		} else {
			corelog.Infof("RemoveControlConnection: client %d skipped (already reconnected to another node)", clientID)
		}
	}
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

	// 委托给 clientRegistry，使用带 clientID 信息的回调
	return s.clientRegistry.CleanupStale(s.config.HeartbeatTimeout, func(connID string, clientID int64, authenticated bool) error {
		// 先通知云控层（触发 webhook）
		// 使用 DisconnectClientIfMatch 避免多节点竞争：只有当 Redis 中的状态与当前节点/连接匹配时才删除
		if authenticated && clientID > 0 && s.cloudControl != nil {
			disconnected, err := s.cloudControl.DisconnectClientIfMatch(clientID, s.nodeID, connID)
			if err != nil {
				corelog.Warnf("cleanupStaleConnections: failed to disconnect client %d: %v", clientID, err)
			} else if disconnected {
				corelog.Infof("cleanupStaleConnections: client %d disconnected from node %s", clientID, s.nodeID)
			} else {
				corelog.Infof("cleanupStaleConnections: client %d skipped (already reconnected to another node)", clientID)
			}
		}
		// 然后关闭连接（注意：连接已从 registry 移除，这里只清理其他资源）
		return s.CloseConnection(connID)
	})
}
