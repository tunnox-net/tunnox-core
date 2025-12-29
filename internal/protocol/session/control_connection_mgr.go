package session

import (
	"fmt"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ============================================================================
// Control Connection 管理
// ============================================================================

// RegisterControlConnection 注册指令连接
func (s *SessionManager) RegisterControlConnection(conn *ControlConnection) {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	// 检查控制连接数限制
	if s.config != nil && s.config.MaxControlConnections > 0 {
		currentCount := len(s.controlConnMap)
		if currentCount >= s.config.MaxControlConnections {
			// 尝试清理最旧的连接
			oldestConn := s.findOldestControlConnectionLocked()
			if oldestConn != nil {
				corelog.Warnf("Control connection limit reached (%d/%d), removing oldest connection %s",
					currentCount, s.config.MaxControlConnections, oldestConn.ConnID)
				if oldestConn.Stream != nil {
					oldestConn.Stream.Close()
				}
				delete(s.controlConnMap, oldestConn.ConnID)
				if oldestConn.ClientID > 0 {
					delete(s.clientIDIndexMap, oldestConn.ClientID)
				}
			}
		}
	}

	// 检查是否已存在
	if existing, exists := s.controlConnMap[conn.ConnID]; exists {
		corelog.Warnf("Control connection %s already exists, replacing", conn.ConnID)
		// 关闭旧连接
		if existing.Stream != nil {
			existing.Stream.Close()
		}
	}

	s.controlConnMap[conn.ConnID] = conn
	// 如果已认证，更新 clientIDIndexMap
	if conn.Authenticated && conn.ClientID > 0 {
		s.clientIDIndexMap[conn.ClientID] = conn
	}
}

// UpdateControlConnectionAuth 更新指令连接的认证信息
func (s *SessionManager) UpdateControlConnectionAuth(connID string, clientID int64, userID string) error {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	conn, exists := s.controlConnMap[connID]
	if !exists {
		return fmt.Errorf("control connection not found: %s", connID)
	}

	conn.ClientID = clientID
	conn.UserID = userID
	conn.Authenticated = true

	// 更新 clientIDIndexMap
	s.clientIDIndexMap[clientID] = conn

	corelog.Infof("Control connection authenticated: connID=%s, clientID=%d, userID=%s", connID, clientID, userID)
	return nil
}

// GetControlConnection 根据 ConnectionID 获取指令连接
func (s *SessionManager) GetControlConnection(connID string) *ControlConnection {
	s.controlConnLock.RLock()
	defer s.controlConnLock.RUnlock()

	return s.controlConnMap[connID]
}

// GetControlConnectionByClientID 根据 ClientID 获取指令连接（类型安全版本）
func (s *SessionManager) GetControlConnectionByClientID(clientID int64) *ControlConnection {
	s.controlConnLock.RLock()
	defer s.controlConnLock.RUnlock()

	return s.clientIDIndexMap[clientID]
}

// GetControlConnectionInterface 根据 ClientID 获取指令连接（返回接口用于API）
func (s *SessionManager) GetControlConnectionInterface(clientID int64) ControlConnectionInterface {
	return s.GetControlConnectionByClientID(clientID)
}

// KickOldControlConnection 踢掉旧的指令连接
func (s *SessionManager) KickOldControlConnection(clientID int64, newConnID string) {
	s.controlConnLock.Lock()
	oldConn := s.clientIDIndexMap[clientID]
	s.controlConnLock.Unlock()

	if oldConn != nil && oldConn.ConnID != newConnID {
		corelog.Warnf("Kicking old control connection: clientID=%d, oldConnID=%s, newConnID=%s",
			clientID, oldConn.ConnID, newConnID)

		// 发送 Kick 命令
		s.sendKickCommand(oldConn, "Another client logged in with the same ID", "DUPLICATE_LOGIN")

		// 关闭旧连接
		go func() {
			_ = s.CloseConnection(oldConn.ConnID)
		}()

		// 从映射中移除（必须同时清理controlConnMap和clientIDIndexMap）
		s.controlConnLock.Lock()
		delete(s.controlConnMap, oldConn.ConnID)
		delete(s.clientIDIndexMap, clientID) // ✅ 修复：同时清理clientIDIndexMap
		s.controlConnLock.Unlock()
	}
}

// sendKickCommand 发送踢下线命令
func (s *SessionManager) sendKickCommand(conn *ControlConnection, reason, code string) {
	if conn == nil || conn.Stream == nil {
		return
	}

	kickBody := fmt.Sprintf(`{"reason":"%s","code":"%s"}`, reason, code)

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
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	conn, exists := s.controlConnMap[connID]
	if exists {
		// ✅ 只有在 clientIDIndexMap 中的映射确实指向这个连接时，才从 clientIDIndexMap 移除
		// 这样可以避免误删真正的控制连接（当隧道连接被错误注册为控制连接时）
		if conn.Authenticated && conn.ClientID > 0 {
			if existingConn, exists := s.clientIDIndexMap[conn.ClientID]; exists && existingConn.ConnID == connID {
				delete(s.clientIDIndexMap, conn.ClientID)
			}
		}
		// 从 controlConnMap 移除
		delete(s.controlConnMap, connID)
	}
}

// getControlConnectionByConnID 根据连接ID获取控制连接（内部使用）
func (s *SessionManager) getControlConnectionByConnID(connID string) *ControlConnection {
	s.controlConnLock.RLock()
	defer s.controlConnLock.RUnlock()

	return s.controlConnMap[connID]
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

	// 1. 收集超时连接（避免长时间持锁）
	var staleConns []*ControlConnection

	s.controlConnLock.RLock()
	for _, conn := range s.controlConnMap {
		if conn.IsStale(s.config.HeartbeatTimeout) {
			staleConns = append(staleConns, conn)
		}
	}
	s.controlConnLock.RUnlock()

	// 2. 清理超时连接
	if len(staleConns) == 0 {
		return 0
	}

	for _, conn := range staleConns {
		idleDuration := time.Since(conn.LastActiveAt)
		corelog.Warnf("SessionManager: removing stale connection - connID=%s, clientID=%d, idle=%v",
			conn.ConnID, conn.ClientID, idleDuration)

		// 更新CloudControl状态（标记客户端离线）
		if s.cloudControl != nil && conn.ClientID > 0 {
			// CloudControl会在DisconnectClient中清理状态
		}

		// 关闭连接（会自动从映射中移除）
		if err := s.CloseConnection(conn.ConnID); err != nil {
			corelog.Errorf("SessionManager: failed to close stale connection %s: %v", conn.ConnID, err)
		}
	}

	return len(staleConns)
}

// findOldestControlConnectionLocked 查找最旧的控制连接（需要在持有锁的情况下调用）
func (s *SessionManager) findOldestControlConnectionLocked() *ControlConnection {
	var oldestConn *ControlConnection
	var oldestTime time.Time

	for _, conn := range s.controlConnMap {
		if oldestConn == nil || conn.CreatedAt.Before(oldestTime) {
			oldestConn = conn
			oldestTime = conn.CreatedAt
		}
	}

	return oldestConn
}
