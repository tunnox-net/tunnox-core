package tunnel

import (
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/session/connection"
)

// ============================================================================
// TunnelConnectionManager 隧道连接管理器
// 负责隧道连接的注册、认证、查询
// ============================================================================

// ConnectionManagerConfig 隧道连接管理器配置
type ConnectionManagerConfig struct {
	Logger corelog.Logger
}

// ConnectionManager 隧道连接管理器
type ConnectionManager struct {
	config *ConnectionManagerConfig
	logger corelog.Logger

	// 连接存储
	connMap   map[string]*connection.TunnelConnection // connID -> 隧道连接
	tunnelMap map[string]*connection.TunnelConnection // tunnelID -> 隧道连接
	lock      sync.RWMutex
}

// NewConnectionManager 创建隧道连接管理器
func NewConnectionManager(config *ConnectionManagerConfig) *ConnectionManager {
	if config == nil {
		config = &ConnectionManagerConfig{
			Logger: corelog.Default(),
		}
	}
	if config.Logger == nil {
		config.Logger = corelog.Default()
	}

	return &ConnectionManager{
		config:    config,
		logger:    config.Logger,
		connMap:   make(map[string]*connection.TunnelConnection),
		tunnelMap: make(map[string]*connection.TunnelConnection),
	}
}

// ============================================================================
// 注册与移除
// ============================================================================

// Register 注册隧道连接
func (m *ConnectionManager) Register(conn *connection.TunnelConnection) error {
	if conn == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "tunnel connection is nil")
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	m.connMap[conn.ConnID] = conn
	if conn.TunnelID != "" {
		m.tunnelMap[conn.TunnelID] = conn
	}

	m.logger.Debugf("TunnelConnectionManager: registered connection %s, tunnelID=%s",
		conn.ConnID, conn.TunnelID)
	return nil
}

// Remove 移除隧道连接
func (m *ConnectionManager) Remove(connID string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	conn, exists := m.connMap[connID]
	if !exists {
		return
	}

	// 从 tunnelMap 移除
	if conn.TunnelID != "" {
		delete(m.tunnelMap, conn.TunnelID)
	}

	delete(m.connMap, connID)
	m.logger.Debugf("TunnelConnectionManager: removed connection %s", connID)
}

// RemoveByTunnelID 根据隧道ID移除连接
func (m *ConnectionManager) RemoveByTunnelID(tunnelID string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	conn, exists := m.tunnelMap[tunnelID]
	if !exists {
		return
	}

	delete(m.tunnelMap, tunnelID)
	delete(m.connMap, conn.ConnID)
	m.logger.Debugf("TunnelConnectionManager: removed connection for tunnel %s", tunnelID)
}

// ============================================================================
// 认证
// ============================================================================

// UpdateAuth 更新隧道连接认证信息
func (m *ConnectionManager) UpdateAuth(connID string, tunnelID string, mappingID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	conn, exists := m.connMap[connID]
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "tunnel connection not found: %s", connID)
	}

	oldTunnelID := conn.TunnelID

	// 更新认证信息
	conn.TunnelID = tunnelID
	conn.MappingID = mappingID
	conn.Authenticated = true

	// 更新 tunnelMap
	if oldTunnelID != "" && oldTunnelID != tunnelID {
		delete(m.tunnelMap, oldTunnelID)
	}
	if tunnelID != "" {
		m.tunnelMap[tunnelID] = conn
	}

	m.logger.Infof("TunnelConnectionManager: authenticated connection %s, tunnelID=%s, mappingID=%s",
		connID, tunnelID, mappingID)
	return nil
}

// ============================================================================
// 查询
// ============================================================================

// GetByConnID 根据连接ID获取隧道连接
func (m *ConnectionManager) GetByConnID(connID string) *connection.TunnelConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.connMap[connID]
}

// GetByTunnelID 根据隧道ID获取隧道连接
func (m *ConnectionManager) GetByTunnelID(tunnelID string) *connection.TunnelConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.tunnelMap[tunnelID]
}

// Count 获取连接数
func (m *ConnectionManager) Count() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.connMap)
}

// ListAll 列出所有隧道连接
func (m *ConnectionManager) ListAll() []*connection.TunnelConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()

	connections := make([]*connection.TunnelConnection, 0, len(m.connMap))
	for _, conn := range m.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// ListAuthenticated 列出所有已认证的隧道连接
func (m *ConnectionManager) ListAuthenticated() []*connection.TunnelConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()

	connections := make([]*connection.TunnelConnection, 0)
	for _, conn := range m.connMap {
		if conn.Authenticated {
			connections = append(connections, conn)
		}
	}
	return connections
}

// ============================================================================
// 资源清理
// ============================================================================

// Close 关闭管理器
func (m *ConnectionManager) Close() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	connCount := len(m.connMap)
	for connID, conn := range m.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		delete(m.connMap, connID)
	}
	m.tunnelMap = make(map[string]*connection.TunnelConnection)

	m.logger.Infof("TunnelConnectionManager: closed %d connections", connCount)
	return nil
}
