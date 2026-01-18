package connection

import (
	"context"
	"net"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/stream"
)

// ============================================================================
// ConnectionManager 连接管理器
// 负责基础连接的创建、存储、清理
// ============================================================================

// ManagerConfig 连接管理器配置
type ManagerConfig struct {
	MaxConnections   int
	HeartbeatTimeout time.Duration
	CleanupInterval  time.Duration
	Logger           corelog.Logger
}

// DefaultManagerConfig 默认配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		MaxConnections:   10000,
		HeartbeatTimeout: 60 * time.Second,
		CleanupInterval:  15 * time.Second,
		Logger:           corelog.Default(),
	}
}

// Manager 连接管理器
type Manager struct {
	config *ManagerConfig
	logger corelog.Logger

	// 父 context - 用于派生清理 goroutine 的 context
	parentCtx context.Context

	// 连接存储
	connMap  map[string]*types.Connection
	connLock sync.RWMutex

	// 清理控制
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupWg     sync.WaitGroup
}

// NewManager 创建连接管理器
// parentCtx 用于派生清理 goroutine 的 context，遵循 dispose 模式
func NewManager(parentCtx context.Context, config *ManagerConfig) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}
	if config.Logger == nil {
		config.Logger = corelog.Default()
	}

	return &Manager{
		config:    config,
		logger:    config.Logger,
		parentCtx: parentCtx,
		connMap:   make(map[string]*types.Connection),
	}
}

// ============================================================================
// 连接生命周期
// ============================================================================

// CreateConnection 创建并注册连接
func (m *Manager) CreateConnection(connID string, s stream.PackageStreamer, rawConn net.Conn) error {
	// 检查连接数限制
	if m.config.MaxConnections > 0 {
		m.connLock.RLock()
		currentCount := len(m.connMap)
		m.connLock.RUnlock()

		if currentCount >= m.config.MaxConnections {
			return coreerrors.Newf(coreerrors.CodeQuotaExceeded,
				"connection limit reached: %d/%d", currentCount, m.config.MaxConnections)
		}
	}

	conn := &types.Connection{
		ID:            connID,
		State:         types.StateConnected,
		Stream:        s,
		RawConn:       rawConn,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}

	m.connLock.Lock()
	m.connMap[connID] = conn
	m.connLock.Unlock()

	m.logger.Debugf("ConnectionManager: created connection %s", connID)
	return nil
}

// GetConnection 获取连接
func (m *Manager) GetConnection(connID string) (*types.Connection, bool) {
	m.connLock.RLock()
	defer m.connLock.RUnlock()
	conn, exists := m.connMap[connID]
	return conn, exists
}

// CloseConnection 关闭并移除连接
func (m *Manager) CloseConnection(connID string) error {
	m.connLock.Lock()
	conn, exists := m.connMap[connID]
	if exists {
		delete(m.connMap, connID)
	}
	m.connLock.Unlock()

	if conn != nil {
		if conn.RawConn != nil {
			conn.RawConn.Close()
		}
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		m.logger.Debugf("ConnectionManager: closed connection %s", connID)
	}

	return nil
}

// UpdateConnectionState 更新连接状态
func (m *Manager) UpdateConnectionState(connID string, state types.ConnectionState) error {
	m.connLock.Lock()
	defer m.connLock.Unlock()

	conn, exists := m.connMap[connID]
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", connID)
	}

	conn.State = state
	conn.UpdatedAt = time.Now()
	return nil
}

// ListConnections 列出所有连接
func (m *Manager) ListConnections() []*types.Connection {
	m.connLock.RLock()
	defer m.connLock.RUnlock()

	connections := make([]*types.Connection, 0, len(m.connMap))
	for _, conn := range m.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// GetConnectionCount 获取连接数
func (m *Manager) GetConnectionCount() int {
	m.connLock.RLock()
	defer m.connLock.RUnlock()
	return len(m.connMap)
}

// ============================================================================
// 连接清理
// ============================================================================

// StartCleanup 启动清理协程
func (m *Manager) StartCleanup() {
	if m.cleanupCancel != nil {
		return // 已经在运行
	}

	// 从父 context 派生，确保父组件关闭时清理 goroutine 也会被取消
	m.cleanupCtx, m.cleanupCancel = context.WithCancel(m.parentCtx)
	m.cleanupWg.Add(1)

	go func() {
		defer m.cleanupWg.Done()
		m.cleanupLoop()
	}()

	m.logger.Info("ConnectionManager: cleanup started")
}

// StopCleanup 停止清理协程
func (m *Manager) StopCleanup() {
	if m.cleanupCancel != nil {
		m.cleanupCancel()
		m.cleanupWg.Wait()
		m.cleanupCancel = nil
		m.logger.Info("ConnectionManager: cleanup stopped")
	}
}

// cleanupLoop 清理循环
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.cleanupCtx.Done():
			return
		case <-ticker.C:
			m.cleanupStaleConnections()
		}
	}
}

// cleanupStaleConnections 清理过期连接
func (m *Manager) cleanupStaleConnections() {
	now := time.Now()
	var staleConnIDs []string

	m.connLock.RLock()
	for connID, conn := range m.connMap {
		if now.Sub(conn.LastHeartbeat) > m.config.HeartbeatTimeout {
			staleConnIDs = append(staleConnIDs, connID)
		}
	}
	m.connLock.RUnlock()

	for _, connID := range staleConnIDs {
		m.logger.Warnf("ConnectionManager: closing stale connection %s", connID)
		_ = m.CloseConnection(connID)
	}

	if len(staleConnIDs) > 0 {
		m.logger.Infof("ConnectionManager: cleaned up %d stale connections", len(staleConnIDs))
	}
}

// ============================================================================
// 资源清理
// ============================================================================

// Close 关闭管理器
func (m *Manager) Close() error {
	m.StopCleanup()

	m.connLock.Lock()
	connCount := len(m.connMap)
	for connID, conn := range m.connMap {
		if conn.RawConn != nil {
			conn.RawConn.Close()
		}
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		delete(m.connMap, connID)
	}
	m.connLock.Unlock()

	m.logger.Infof("ConnectionManager: closed %d connections", connCount)
	return nil
}
