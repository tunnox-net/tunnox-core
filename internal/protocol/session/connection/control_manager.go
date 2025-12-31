package connection

import (
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// ============================================================================
// ControlConnectionManager 控制连接管理器
// 负责控制连接的注册、认证、查询、踢出
// ============================================================================

// ControlManagerConfig 控制连接管理器配置
type ControlManagerConfig struct {
	MaxConnections int
	Logger         corelog.Logger
}

// ControlManager 控制连接管理器
type ControlManager struct {
	config *ControlManagerConfig
	logger corelog.Logger

	// 连接存储
	connMap      map[string]*ControlConnection  // connID -> 控制连接
	clientIDMap  map[int64]*ControlConnection   // clientID -> 控制连接
	lock         sync.RWMutex
}

// NewControlManager 创建控制连接管理器
func NewControlManager(config *ControlManagerConfig) *ControlManager {
	if config == nil {
		config = &ControlManagerConfig{
			MaxConnections: 5000,
			Logger:         corelog.Default(),
		}
	}
	if config.Logger == nil {
		config.Logger = corelog.Default()
	}

	return &ControlManager{
		config:      config,
		logger:      config.Logger,
		connMap:     make(map[string]*ControlConnection),
		clientIDMap: make(map[int64]*ControlConnection),
	}
}

// ============================================================================
// 注册与移除
// ============================================================================

// Register 注册控制连接
func (m *ControlManager) Register(conn *ControlConnection) error {
	if conn == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "connection is nil")
	}

	// 检查连接数限制
	if m.config.MaxConnections > 0 {
		m.lock.RLock()
		currentCount := len(m.connMap)
		m.lock.RUnlock()

		if currentCount >= m.config.MaxConnections {
			return coreerrors.Newf(coreerrors.CodeQuotaExceeded,
				"control connection limit reached: %d/%d", currentCount, m.config.MaxConnections)
		}
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	// 如果已有相同 clientID 的连接，踢掉旧连接
	if conn.ClientID > 0 {
		if oldConn, exists := m.clientIDMap[conn.ClientID]; exists && oldConn.ConnID != conn.ConnID {
			m.logger.Warnf("ControlManager: kicking old connection for client %d, old=%s new=%s",
				conn.ClientID, oldConn.ConnID, conn.ConnID)
			delete(m.connMap, oldConn.ConnID)
			if oldConn.Stream != nil {
				go oldConn.Stream.Close() // 异步关闭避免死锁
			}
		}
		m.clientIDMap[conn.ClientID] = conn
	}

	m.connMap[conn.ConnID] = conn
	m.logger.Debugf("ControlManager: registered connection %s for client %d", conn.ConnID, conn.ClientID)
	return nil
}

// Remove 移除控制连接
func (m *ControlManager) Remove(connID string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	conn, exists := m.connMap[connID]
	if !exists {
		return
	}

	// 从 clientIDMap 移除
	if conn.ClientID > 0 {
		if existingConn, ok := m.clientIDMap[conn.ClientID]; ok && existingConn.ConnID == connID {
			delete(m.clientIDMap, conn.ClientID)
		}
	}

	delete(m.connMap, connID)
	m.logger.Debugf("ControlManager: removed connection %s", connID)
}

// RemoveByClientID 根据客户端ID移除连接
func (m *ControlManager) RemoveByClientID(clientID int64) {
	m.lock.Lock()
	defer m.lock.Unlock()

	conn, exists := m.clientIDMap[clientID]
	if !exists {
		return
	}

	delete(m.clientIDMap, clientID)
	delete(m.connMap, conn.ConnID)
	m.logger.Debugf("ControlManager: removed connection for client %d", clientID)
}

// ============================================================================
// 认证
// ============================================================================

// UpdateAuth 更新控制连接认证信息
func (m *ControlManager) UpdateAuth(connID string, clientID int64, userID string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	conn, exists := m.connMap[connID]
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "control connection not found: %s", connID)
	}

	oldClientID := conn.ClientID

	// 更新认证信息
	conn.ClientID = clientID
	conn.UserID = userID
	conn.Authenticated = true

	// 更新 clientIDMap
	if oldClientID > 0 && oldClientID != clientID {
		delete(m.clientIDMap, oldClientID)
	}
	if clientID > 0 {
		// 踢掉同一 clientID 的旧连接
		if oldConn, exists := m.clientIDMap[clientID]; exists && oldConn.ConnID != connID {
			m.logger.Warnf("ControlManager: kicking old connection for client %d during auth", clientID)
			delete(m.connMap, oldConn.ConnID)
			if oldConn.Stream != nil {
				go oldConn.Stream.Close()
			}
		}
		m.clientIDMap[clientID] = conn
	}

	m.logger.Infof("ControlManager: authenticated connection %s for client %d, user %s",
		connID, clientID, userID)
	return nil
}

// ============================================================================
// 查询
// ============================================================================

// GetByConnID 根据连接ID获取控制连接
func (m *ControlManager) GetByConnID(connID string) *ControlConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.connMap[connID]
}

// GetByClientID 根据客户端ID获取控制连接
func (m *ControlManager) GetByClientID(clientID int64) *ControlConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.clientIDMap[clientID]
}

// Count 获取连接数
func (m *ControlManager) Count() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.connMap)
}

// ListAll 列出所有控制连接
func (m *ControlManager) ListAll() []*ControlConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()

	connections := make([]*ControlConnection, 0, len(m.connMap))
	for _, conn := range m.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// ListAuthenticated 列出所有已认证的控制连接
func (m *ControlManager) ListAuthenticated() []*ControlConnection {
	m.lock.RLock()
	defer m.lock.RUnlock()

	connections := make([]*ControlConnection, 0)
	for _, conn := range m.connMap {
		if conn.Authenticated {
			connections = append(connections, conn)
		}
	}
	return connections
}

// ============================================================================
// 清理
// ============================================================================

// CleanupStale 清理过期的控制连接
func (m *ControlManager) CleanupStale(timeout time.Duration) int {
	var staleConnIDs []string

	m.lock.RLock()
	for connID, conn := range m.connMap {
		if conn.IsStale(timeout) {
			staleConnIDs = append(staleConnIDs, connID)
		}
	}
	m.lock.RUnlock()

	for _, connID := range staleConnIDs {
		m.Remove(connID)
	}

	if len(staleConnIDs) > 0 {
		m.logger.Infof("ControlManager: cleaned up %d stale connections", len(staleConnIDs))
	}

	return len(staleConnIDs)
}

// ============================================================================
// 资源清理
// ============================================================================

// Close 关闭管理器
func (m *ControlManager) Close() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	connCount := len(m.connMap)
	for connID, conn := range m.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		delete(m.connMap, connID)
	}
	m.clientIDMap = make(map[int64]*ControlConnection)

	m.logger.Infof("ControlManager: closed %d connections", connCount)
	return nil
}
