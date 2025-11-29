package session

import (
	"fmt"
	"io"
	"net"
	"time"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ============================================================================
// Connection 生命周期管理
// ============================================================================

// CreateConnection 创建新连接
func (s *SessionManager) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	// 检查连接数限制
	if s.config != nil && s.config.MaxConnections > 0 {
		s.connLock.RLock()
		currentCount := len(s.connMap)
		s.connLock.RUnlock()
		if currentCount >= s.config.MaxConnections {
			return nil, fmt.Errorf("connection limit reached: %d/%d", currentCount, s.config.MaxConnections)
		}
	}

	// 生成连接ID
	connID, err := s.idManager.GenerateConnectionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate connection ID: %w", err)
	}

	// ✅ 尝试提取原始的net.Conn（用于纯流转发）
	var rawConn net.Conn
	if nc, ok := reader.(net.Conn); ok {
		rawConn = nc
	} else if nc, ok := writer.(net.Conn); ok {
		rawConn = nc
	}

	// 创建流处理器
	streamProcessor, err := s.streamMgr.CreateStream(connID, reader, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// 创建连接对象
	conn := &types.Connection{
		ID:            connID,
		State:         types.StateInitializing,
		Stream:        streamProcessor,
		RawConn:       rawConn, // ✅ 保存原始连接
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}

	// 注册连接
	s.connLock.Lock()
	s.connMap[connID] = conn
	s.connLock.Unlock()

	return conn, nil
}

// AcceptConnection 接受新连接
func (s *SessionManager) AcceptConnection(reader io.Reader, writer io.Writer) (*types.StreamConnection, error) {
	// 创建连接
	conn, err := s.CreateConnection(reader, writer)
	if err != nil {
		return nil, err
	}

	// 更新状态
	if err := s.UpdateConnectionState(conn.ID, types.StateConnected); err != nil {
		return nil, err
	}

	// 转换为 StreamConnection
	streamConn := &types.StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}

	// ✅ 不再预注册控制连接，等待收到 Handshake 包后再注册
	// 这样可以区分控制连接（Handshake）和隧道连接（TunnelOpen with MappingID）

	return streamConn, nil
}

// GetConnection 获取连接
func (s *SessionManager) GetConnection(connID string) (*types.Connection, bool) {
	s.connLock.RLock()
	defer s.connLock.RUnlock()
	conn, exists := s.connMap[connID]
	return conn, exists
}

// getConnectionByConnID 获取连接（内部使用，返回nil如果不存在）
func (s *SessionManager) getConnectionByConnID(connID string) *types.Connection {
	conn, _ := s.GetConnection(connID)
	return conn
}

// ListConnections 列出所有连接
func (s *SessionManager) ListConnections() []*types.Connection {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	connections := make([]*types.Connection, 0, len(s.connMap))
	for _, conn := range s.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// UpdateConnectionState 更新连接状态
func (s *SessionManager) UpdateConnectionState(connID string, state types.ConnectionState) error {
	s.connLock.Lock()
	defer s.connLock.Unlock()

	conn, exists := s.connMap[connID]
	if !exists {
		return fmt.Errorf("connection not found: %s", connID)
	}

	conn.State = state
	return nil
}

// CloseConnection 关闭连接并释放所有资源
func (s *SessionManager) CloseConnection(connectionId string) error {
	// 从连接映射中移除
	s.connLock.Lock()
	conn, exists := s.connMap[connectionId]
	if exists {
		delete(s.connMap, connectionId)
	}
	s.connLock.Unlock()

	// 关闭底层连接
	if conn != nil {
		if conn.RawConn != nil {
			conn.RawConn.Close()
		}
		// 关闭流处理器
		if conn.Stream != nil {
			conn.Stream.Close()
		}
	}

	// 从控制连接映射中移除
	s.RemoveControlConnection(connectionId)

	// 从隧道连接映射中移除
	s.RemoveTunnelConnection(connectionId)

	return nil
}

// GetStreamConnectionInfo 获取流连接信息
func (s *SessionManager) GetStreamConnectionInfo(connectionId string) (*types.StreamConnection, bool) {
	conn, exists := s.GetConnection(connectionId)
	if !exists {
		return nil, false
	}

	streamConn := &types.StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}

	return streamConn, true
}

// GetActiveConnections 获取活跃连接数
// GetActiveConnections 返回当前占用的通道数（控制通道 + 数据隧道）
func (s *SessionManager) GetActiveConnections() int {
	return s.GetActiveChannels()
}

// GetActiveChannels 返回当前占用的通道数（控制通道 + 数据隧道）
func (s *SessionManager) GetActiveChannels() int {
	s.controlConnLock.RLock()
	controlCount := len(s.controlConnMap)
	s.controlConnLock.RUnlock()

	s.tunnelConnLock.RLock()
	tunnelCount := len(s.tunnelConnMap)
	s.tunnelConnLock.RUnlock()

	return controlCount + tunnelCount
}

// GetConnectionStats 获取连接统计信息
func (s *SessionManager) GetConnectionStats() ConnectionStats {
	s.connLock.RLock()
	totalConnections := len(s.connMap)
	s.connLock.RUnlock()

	s.controlConnLock.RLock()
	controlConnections := len(s.controlConnMap)
	s.controlConnLock.RUnlock()

	s.tunnelConnLock.RLock()
	tunnelConnections := len(s.tunnelConnMap)
	s.tunnelConnLock.RUnlock()

	return ConnectionStats{
		TotalConnections:    totalConnections,
		ControlConnections:  controlConnections,
		TunnelConnections:   tunnelConnections,
		MaxConnections:      s.getMaxConnections(),
		MaxControlConnections: s.getMaxControlConnections(),
	}
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	TotalConnections     int
	ControlConnections  int
	TunnelConnections   int
	MaxConnections      int
	MaxControlConnections int
}

func (s *SessionManager) getMaxConnections() int {
	if s.config != nil {
		return s.config.MaxConnections
	}
	return 0
}

func (s *SessionManager) getMaxControlConnections() int {
	if s.config != nil {
		return s.config.MaxControlConnections
	}
	return 0
}

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
				utils.Warnf("Control connection limit reached (%d/%d), removing oldest connection %s",
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
		utils.Warnf("Control connection %s already exists, replacing", conn.ConnID)
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

	utils.Infof("Control connection authenticated: connID=%s, clientID=%d, userID=%s", connID, clientID, userID)
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

// GetControlConnectionInterface 根据 ClientID 获取指令连接（返回interface{}用于API）
func (s *SessionManager) GetControlConnectionInterface(clientID int64) interface{} {
	return s.GetControlConnectionByClientID(clientID)
}

// KickOldControlConnection 踢掉旧的指令连接
func (s *SessionManager) KickOldControlConnection(clientID int64, newConnID string) {
	s.controlConnLock.Lock()
	oldConn := s.clientIDIndexMap[clientID]
	s.controlConnLock.Unlock()

	if oldConn != nil && oldConn.ConnID != newConnID {
		utils.Warnf("Kicking old control connection: clientID=%d, oldConnID=%s, newConnID=%s",
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
		utils.Warnf("Failed to send kick command to %s: %v", conn.ConnID, err)
	} else {
		utils.Infof("Sent kick command to client %d (connID=%s): %s", conn.ClientID, conn.ConnID, reason)
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
			} else {
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


// ============================================================================
// Tunnel Connection 管理
// ============================================================================

// RegisterTunnelConnection 注册映射连接
func (s *SessionManager) RegisterTunnelConnection(conn *TunnelConnection) {
	s.tunnelConnLock.Lock()
	defer s.tunnelConnLock.Unlock()

	s.tunnelConnMap[conn.ConnID] = conn
	if conn.TunnelID != "" {
		s.tunnelIDMap[conn.TunnelID] = conn
	}

}

// UpdateTunnelConnectionAuth 更新映射连接的认证信息
func (s *SessionManager) UpdateTunnelConnectionAuth(connID string, tunnelID string, mappingID string) error {
	s.tunnelConnLock.Lock()
	defer s.tunnelConnLock.Unlock()

	conn, exists := s.tunnelConnMap[connID]
	if !exists {
		return fmt.Errorf("tunnel connection not found: %s", connID)
	}

	conn.TunnelID = tunnelID
	conn.MappingID = mappingID
	conn.Authenticated = true

	// 更新 tunnelIDMap
	s.tunnelIDMap[tunnelID] = conn

	utils.Infof("Tunnel connection authenticated: connID=%s, tunnelID=%s, mappingID=%s", connID, tunnelID, mappingID)
	return nil
}

// GetTunnelConnectionByTunnelID 根据 TunnelID 获取映射连接
func (s *SessionManager) GetTunnelConnectionByTunnelID(tunnelID string) *TunnelConnection {
	s.tunnelConnLock.RLock()
	defer s.tunnelConnLock.RUnlock()

	return s.tunnelIDMap[tunnelID]
}

// GetTunnelConnectionByConnID 根据 ConnID 获取映射连接
func (s *SessionManager) GetTunnelConnectionByConnID(connID string) *TunnelConnection {
	s.tunnelConnLock.RLock()
	defer s.tunnelConnLock.RUnlock()

	return s.tunnelConnMap[connID]
}

// RemoveTunnelConnection 移除映射连接
func (s *SessionManager) RemoveTunnelConnection(connID string) {
	s.tunnelConnLock.Lock()
	defer s.tunnelConnLock.Unlock()

	conn, exists := s.tunnelConnMap[connID]
	if exists {
		// 从 tunnelIDMap 移除
		if conn.TunnelID != "" {
			delete(s.tunnelIDMap, conn.TunnelID)
		}
		// 从 tunnelConnMap 移除
		delete(s.tunnelConnMap, connID)
		utils.Debugf("Removed tunnel connection: connID=%s, tunnelID=%s", connID, conn.TunnelID)
	}
}

// ============================================================================
// 连接清理（心跳超时检测）
// ============================================================================

// startConnectionCleanup 启动连接清理协程
// 定期检查并清理超时未发送心跳的控制连接
func (s *SessionManager) startConnectionCleanup() {
	if s.config == nil {
		utils.Warnf("SessionManager: config is nil, cleanup disabled")
		return
	}

	go func() {
		ticker := time.NewTicker(s.config.CleanupInterval)
		defer ticker.Stop()

		utils.Infof("SessionManager: connection cleanup started (interval=%v, timeout=%v)",
			s.config.CleanupInterval, s.config.HeartbeatTimeout)

		for {
			select {
			case <-ticker.C:
				if cleaned := s.cleanupStaleConnections(); cleaned > 0 {
					utils.Infof("SessionManager: cleaned up %d stale connections", cleaned)
				}

			case <-s.Ctx().Done():
				utils.Infof("SessionManager: connection cleanup stopped")
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
		utils.Warnf("SessionManager: removing stale connection - connID=%s, clientID=%d, idle=%v",
			conn.ConnID, conn.ClientID, idleDuration)

		// 更新CloudControl状态（标记客户端离线）
		if s.cloudControl != nil && conn.ClientID > 0 {
			// CloudControl会在DisconnectClient中清理状态
		}

		// 关闭连接（会自动从映射中移除）
		if err := s.CloseConnection(conn.ConnID); err != nil {
			utils.Errorf("SessionManager: failed to close stale connection %s: %v", conn.ConnID, err)
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
