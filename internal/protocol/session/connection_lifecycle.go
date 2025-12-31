package session

import (
	"io"
	"net"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
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
			return nil, coreerrors.Newf(coreerrors.CodeQuotaExceeded, "connection limit reached: %d/%d", currentCount, s.config.MaxConnections)
		}
	}

	// ✅ 对于支持自定义 connectionID 的连接，使用其 connectionID 而不是生成新的
	// 这样可以确保 connMap 中的连接ID和协议特定注册表中的一致
	var connID string
	var err error
	if connIDProvider, ok := reader.(interface{ GetConnectionID() string }); ok {
		connID = connIDProvider.GetConnectionID()
	} else if connIDProvider, ok := writer.(interface{ GetConnectionID() string }); ok {
		connID = connIDProvider.GetConnectionID()
	}

	// 如果没有从连接获取到 connectionID，则生成新的
	if connID == "" {
		connID, err = s.idManager.GenerateConnectionID()
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate connection ID")
		}
		corelog.Infof("CreateConnection: generated new connectionID=%s", connID)
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create stream")
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
		return coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", connID)
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

	// ✅ 清理 Redis 中的连接状态记录
	if s.connStateStore != nil {
		if err := s.connStateStore.UnregisterConnection(s.Ctx(), connectionId); err != nil {
			corelog.Warnf("Failed to unregister connection state from Redis: %v", err)
		}
	}

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
	// ✅ 优先使用 registry（新架构）
	controlCount := s.clientRegistry.Count()
	tunnelCount := s.tunnelRegistry.Count()

	return controlCount + tunnelCount
}

// GetConnectionStats 获取连接统计信息
func (s *SessionManager) GetConnectionStats() ConnectionStats {
	// ✅ 优先使用 registry（新架构）
	controlConnections := s.clientRegistry.Count()
	tunnelConnections := s.tunnelRegistry.Count()

	// ⚠️ 总连接数仍从旧map获取（待子阶段4.6处理）
	s.connLock.RLock()
	totalConnections := len(s.connMap)
	s.connLock.RUnlock()

	return ConnectionStats{
		TotalConnections:      totalConnections,
		ControlConnections:    controlConnections,
		TunnelConnections:     tunnelConnections,
		MaxConnections:        s.getMaxConnections(),
		MaxControlConnections: s.getMaxControlConnections(),
	}
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	TotalConnections      int
	ControlConnections    int
	TunnelConnections     int
	MaxConnections        int
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
// Tunnel Connection 管理
// ============================================================================

// RegisterTunnelConnection 注册映射连接
func (s *SessionManager) RegisterTunnelConnection(conn *TunnelConnection) {
	// 委托给 tunnelRegistry
	if err := s.tunnelRegistry.Register(conn); err != nil {
		corelog.Errorf("Failed to register tunnel connection: %v", err)
	}
}

// UpdateTunnelConnectionAuth 更新映射连接的认证信息
func (s *SessionManager) UpdateTunnelConnectionAuth(connID string, tunnelID string, mappingID string) error {
	// 委托给 tunnelRegistry
	return s.tunnelRegistry.UpdateAuth(connID, tunnelID, mappingID)
}

// GetTunnelConnectionByTunnelID 根据 TunnelID 获取映射连接
func (s *SessionManager) GetTunnelConnectionByTunnelID(tunnelID string) *TunnelConnection {
	return s.tunnelRegistry.GetByTunnelID(tunnelID)
}

// GetTunnelConnectionByConnID 根据 ConnID 获取映射连接
func (s *SessionManager) GetTunnelConnectionByConnID(connID string) *TunnelConnection {
	return s.tunnelRegistry.GetByConnID(connID)
}

// RemoveTunnelConnection 移除映射连接
func (s *SessionManager) RemoveTunnelConnection(connID string) {
	s.tunnelRegistry.Remove(connID)
}
