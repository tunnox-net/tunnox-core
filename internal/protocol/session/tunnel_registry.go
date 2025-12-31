package session

import (
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// TunnelRegistry 隧道注册表
// 负责管理隧道连接（Tunnel Connection）的注册、查询和清理
type TunnelRegistry struct {
	// 隧道连接映射
	connMap   map[string]*TunnelConnection // connID -> 隧道连接
	tunnelMap map[string]*TunnelConnection // tunnelID -> 隧道连接
	mu        sync.RWMutex

	// 日志
	logger corelog.Logger
}

// TunnelRegistryConfig 隧道注册表配置
type TunnelRegistryConfig struct {
	Logger corelog.Logger
}

// NewTunnelRegistry 创建隧道注册表
func NewTunnelRegistry(config *TunnelRegistryConfig) *TunnelRegistry {
	if config == nil {
		config = &TunnelRegistryConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &TunnelRegistry{
		connMap:   make(map[string]*TunnelConnection),
		tunnelMap: make(map[string]*TunnelConnection),
		logger:    logger,
	}
}

// Register 注册隧道连接
func (r *TunnelRegistry) Register(conn *TunnelConnection) error {
	if conn == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "connection cannot be nil")
	}
	if conn.ConnID == "" {
		return coreerrors.New(coreerrors.CodeInvalidParam, "connection ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.connMap[conn.ConnID] = conn
	if conn.TunnelID != "" {
		r.tunnelMap[conn.TunnelID] = conn
	}

	r.logger.Debugf("TunnelRegistry: registered connection %s (tunnelID=%s, mappingID=%s)",
		conn.ConnID, conn.TunnelID, conn.MappingID)

	return nil
}

// UpdateAuth 更新隧道连接的认证信息
func (r *TunnelRegistry) UpdateAuth(connID string, tunnelID string, mappingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connMap[connID]
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "tunnel connection not found: %s", connID)
	}

	conn.TunnelID = tunnelID
	conn.MappingID = mappingID
	conn.Authenticated = true

	// 更新 tunnelMap
	r.tunnelMap[tunnelID] = conn

	r.logger.Infof("TunnelRegistry: connection authenticated - connID=%s, tunnelID=%s, mappingID=%s",
		connID, tunnelID, mappingID)

	return nil
}

// GetByConnID 根据连接ID获取隧道连接
func (r *TunnelRegistry) GetByConnID(connID string) *TunnelConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connMap[connID]
}

// GetByTunnelID 根据隧道ID获取隧道连接
func (r *TunnelRegistry) GetByTunnelID(tunnelID string) *TunnelConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tunnelMap[tunnelID]
}

// Remove 移除隧道连接
func (r *TunnelRegistry) Remove(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connMap[connID]
	if !exists {
		return
	}

	// 从 tunnelMap 移除
	if conn.TunnelID != "" {
		delete(r.tunnelMap, conn.TunnelID)
	}

	// 从 connMap 移除
	delete(r.connMap, connID)

	r.logger.Debugf("TunnelRegistry: removed connection %s (tunnelID=%s)", connID, conn.TunnelID)
}

// Count 返回当前连接数
func (r *TunnelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connMap)
}

// List 列出所有隧道连接
func (r *TunnelRegistry) List() []*TunnelConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*TunnelConnection, 0, len(r.connMap))
	for _, conn := range r.connMap {
		result = append(result, conn)
	}
	return result
}

// Close 关闭所有连接
func (r *TunnelRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conn := range r.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
	}

	r.connMap = make(map[string]*TunnelConnection)
	r.tunnelMap = make(map[string]*TunnelConnection)

	r.logger.Info("TunnelRegistry: closed all connections")
}
