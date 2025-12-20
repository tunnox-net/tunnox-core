package session

import (
	"fmt"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ClientRegistry 客户端注册表
// 负责管理控制连接（Control Connection）的注册、查询和清理
type ClientRegistry struct {
	// 控制连接映射
	connMap     map[string]*ControlConnection // connID -> 控制连接
	clientIDMap map[int64]*ControlConnection  // clientID -> 控制连接（快速查找）
	mu          sync.RWMutex

	// 配置
	maxConnections int

	// 日志
	logger corelog.Logger
}

// ClientRegistryConfig 客户端注册表配置
type ClientRegistryConfig struct {
	MaxConnections int
	Logger         corelog.Logger
}

// NewClientRegistry 创建客户端注册表
func NewClientRegistry(config *ClientRegistryConfig) *ClientRegistry {
	if config == nil {
		config = &ClientRegistryConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &ClientRegistry{
		connMap:        make(map[string]*ControlConnection),
		clientIDMap:    make(map[int64]*ControlConnection),
		maxConnections: config.MaxConnections,
		logger:         logger,
	}
}

// Register 注册控制连接
func (r *ClientRegistry) Register(conn *ControlConnection) error {
	if conn == nil {
		return fmt.Errorf("connection cannot be nil")
	}
	if conn.ConnID == "" {
		return fmt.Errorf("connection ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查连接数限制
	if r.maxConnections > 0 && len(r.connMap) >= r.maxConnections {
		// 尝试清理最旧的连接
		oldestConn := r.findOldestConnectionLocked()
		if oldestConn != nil {
			r.logger.Warnf("ClientRegistry: connection limit reached (%d/%d), removing oldest connection %s",
				len(r.connMap), r.maxConnections, oldestConn.ConnID)
			r.removeConnectionLocked(oldestConn)
		} else {
			return fmt.Errorf("connection limit reached: %d/%d", len(r.connMap), r.maxConnections)
		}
	}

	// 检查是否已存在
	if existing, exists := r.connMap[conn.ConnID]; exists {
		r.logger.Warnf("ClientRegistry: connection %s already exists, replacing", conn.ConnID)
		r.removeConnectionLocked(existing)
	}

	r.connMap[conn.ConnID] = conn

	// 如果已认证，更新 clientIDMap
	if conn.Authenticated && conn.ClientID > 0 {
		r.clientIDMap[conn.ClientID] = conn
	}

	r.logger.Debugf("ClientRegistry: registered connection %s (clientID=%d, authenticated=%v)",
		conn.ConnID, conn.ClientID, conn.Authenticated)

	return nil
}

// UpdateAuth 更新连接的认证信息
func (r *ClientRegistry) UpdateAuth(connID string, clientID int64, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connMap[connID]
	if !exists {
		return fmt.Errorf("connection not found: %s", connID)
	}

	conn.ClientID = clientID
	conn.UserID = userID
	conn.Authenticated = true

	// 更新 clientIDMap
	r.clientIDMap[clientID] = conn

	r.logger.Infof("ClientRegistry: connection authenticated - connID=%s, clientID=%d, userID=%s",
		connID, clientID, userID)

	return nil
}

// GetByConnID 根据连接ID获取控制连接
func (r *ClientRegistry) GetByConnID(connID string) *ControlConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connMap[connID]
}

// GetByClientID 根据客户端ID获取控制连接
func (r *ClientRegistry) GetByClientID(clientID int64) *ControlConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clientIDMap[clientID]
}

// Remove 移除控制连接
func (r *ClientRegistry) Remove(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connMap[connID]
	if !exists {
		return
	}

	r.removeConnectionLocked(conn)
	r.logger.Debugf("ClientRegistry: removed connection %s", connID)
}

// KickOldConnection 踢掉旧的控制连接
func (r *ClientRegistry) KickOldConnection(clientID int64, newConnID string, sendKickFn func(*ControlConnection, string, string)) {
	r.mu.Lock()
	oldConn := r.clientIDMap[clientID]
	r.mu.Unlock()

	if oldConn != nil && oldConn.ConnID != newConnID {
		r.logger.Warnf("ClientRegistry: kicking old connection - clientID=%d, oldConnID=%s, newConnID=%s",
			clientID, oldConn.ConnID, newConnID)

		// 发送 Kick 命令
		if sendKickFn != nil {
			sendKickFn(oldConn, "Another client logged in with the same ID", "DUPLICATE_LOGIN")
		}

		// 关闭旧连接
		if oldConn.Stream != nil {
			oldConn.Stream.Close()
		}

		// 从映射中移除
		r.mu.Lock()
		r.removeConnectionLocked(oldConn)
		r.mu.Unlock()
	}
}

// Count 返回当前连接数
func (r *ClientRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connMap)
}

// List 列出所有控制连接
func (r *ClientRegistry) List() []*ControlConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ControlConnection, 0, len(r.connMap))
	for _, conn := range r.connMap {
		result = append(result, conn)
	}
	return result
}

// CleanupStale 清理过期的连接
// 返回清理的连接数量
func (r *ClientRegistry) CleanupStale(timeout time.Duration, closeFn func(string) error) int {
	// 1. 收集超时连接（避免长时间持锁）
	var staleConns []*ControlConnection

	r.mu.RLock()
	for _, conn := range r.connMap {
		if conn.IsStale(timeout) {
			staleConns = append(staleConns, conn)
		}
	}
	r.mu.RUnlock()

	if len(staleConns) == 0 {
		return 0
	}

	// 2. 清理超时连接
	for _, conn := range staleConns {
		idleDuration := time.Since(conn.LastActiveAt)
		r.logger.Warnf("ClientRegistry: removing stale connection - connID=%s, clientID=%d, idle=%v",
			conn.ConnID, conn.ClientID, idleDuration)

		// 关闭连接
		if closeFn != nil {
			if err := closeFn(conn.ConnID); err != nil {
				r.logger.Errorf("ClientRegistry: failed to close stale connection %s: %v", conn.ConnID, err)
			}
		}

		// 从映射中移除
		r.mu.Lock()
		r.removeConnectionLocked(conn)
		r.mu.Unlock()
	}

	return len(staleConns)
}

// Close 关闭所有连接
func (r *ClientRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, conn := range r.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
	}

	r.connMap = make(map[string]*ControlConnection)
	r.clientIDMap = make(map[int64]*ControlConnection)

	r.logger.Info("ClientRegistry: closed all connections")
}

// removeConnectionLocked 移除连接（需要在持有锁的情况下调用）
func (r *ClientRegistry) removeConnectionLocked(conn *ControlConnection) {
	if conn == nil {
		return
	}

	// 关闭流
	if conn.Stream != nil {
		conn.Stream.Close()
	}

	// 从 clientIDMap 移除（只有在映射确实指向这个连接时才移除）
	if conn.Authenticated && conn.ClientID > 0 {
		if existingConn, exists := r.clientIDMap[conn.ClientID]; exists && existingConn.ConnID == conn.ConnID {
			delete(r.clientIDMap, conn.ClientID)
		}
	}

	// 从 connMap 移除
	delete(r.connMap, conn.ConnID)
}

// findOldestConnectionLocked 查找最旧的连接（需要在持有锁的情况下调用）
func (r *ClientRegistry) findOldestConnectionLocked() *ControlConnection {
	var oldestConn *ControlConnection
	var oldestTime time.Time

	for _, conn := range r.connMap {
		if oldestConn == nil || conn.CreatedAt.Before(oldestTime) {
			oldestConn = conn
			oldestTime = conn.CreatedAt
		}
	}

	return oldestConn
}

// SendKickCommand 发送踢下线命令
func SendKickCommand(conn *ControlConnection, reason, code string) {
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
		corelog.Warnf("ClientRegistry: failed to send kick command to %s: %v", conn.ConnID, err)
	} else {
		corelog.Infof("ClientRegistry: sent kick command to client %d (connID=%s): %s", conn.ClientID, conn.ConnID, reason)
	}
}
