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

	utils.Debugf("Connection created: %s", connID)
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

	// 预注册控制连接，优先使用写端元信息
	enforcedProtocol := conn.Protocol
	if enforcedProtocol == "" {
		enforcedProtocol = resolveProtocol(writer, reader)
		if enforcedProtocol == "" {
			enforcedProtocol = "tcp"
		}
	}
	remoteAddr := resolveRemoteAddr(writer, reader)
	controlConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
	s.RegisterControlConnection(controlConn)

	utils.Debugf("Connection accepted: %s", conn.ID)
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
	utils.Debugf("Connection state updated: %s -> %v", connID, state)
	return nil
}

// CloseConnection 关闭连接
func (s *SessionManager) CloseConnection(connectionId string) error {
	// 从连接映射中移除
	s.connLock.Lock()
	conn, exists := s.connMap[connectionId]
	if exists {
		delete(s.connMap, connectionId)
	}
	s.connLock.Unlock()

	// 从流管理器中注销（如果有的话）
	// Note: 流管理器可能没有 UnregisterStream 方法

	// 关闭流处理器
	if conn != nil && conn.Stream != nil {
		conn.Stream.Close()
	}

	// 从控制连接映射中移除
	s.RemoveControlConnection(connectionId)

	// 从隧道连接映射中移除
	s.RemoveTunnelConnection(connectionId)

	utils.Infof("Closed connection: %s", connectionId)
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

// ============================================================================
// Control Connection 管理
// ============================================================================

// RegisterControlConnection 注册指令连接
func (s *SessionManager) RegisterControlConnection(conn *ControlConnection) {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	s.controlConnMap[conn.ConnID] = conn
	// 如果已认证，更新 clientIDIndexMap
	if conn.Authenticated && conn.ClientID > 0 {
		s.clientIDIndexMap[conn.ClientID] = conn
	}

	utils.Debugf("Registered control connection: %s", conn.ConnID)
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
		delete(s.clientIDIndexMap, clientID)  // ✅ 修复：同时清理clientIDIndexMap
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

	if _, err := conn.Stream.WritePacket(kickPkt, false, 0); err != nil {
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
		// 从 clientIDIndexMap 移除
		if conn.Authenticated && conn.ClientID > 0 {
			delete(s.clientIDIndexMap, conn.ClientID)
		}
		// 从 controlConnMap 移除
		delete(s.controlConnMap, connID)
		utils.Debugf("Removed control connection: %s", connID)
	}
}

// getControlConnectionByConnID 根据连接ID获取控制连接（内部使用）
func (s *SessionManager) getControlConnectionByConnID(connID string) *ControlConnection {
	s.controlConnLock.RLock()
	defer s.controlConnLock.RUnlock()

	return s.controlConnMap[connID]
}

func resolveRemoteAddr(primary interface{}, secondary interface{}) net.Addr {
	type remote interface {
		RemoteAddr() net.Addr
	}
	if conn, ok := primary.(remote); ok && conn.RemoteAddr() != nil {
		return conn.RemoteAddr()
	}
	if conn, ok := secondary.(remote); ok {
		return conn.RemoteAddr()
	}
	return nil
}

func resolveProtocol(primary interface{}, secondary interface{}) string {
	type local interface {
		LocalAddr() net.Addr
	}
	if conn, ok := primary.(local); ok && conn.LocalAddr() != nil {
		return conn.LocalAddr().Network()
	}
	if conn, ok := secondary.(local); ok && conn.LocalAddr() != nil {
		return conn.LocalAddr().Network()
	}
	return ""
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

	utils.Debugf("Registered tunnel connection: connID=%s, tunnelID=%s", conn.ConnID, conn.TunnelID)
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
