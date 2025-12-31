package session

import (
	"fmt"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
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

// Remove 移除控制连接（并关闭 stream）
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

// Unregister 从映射中移除连接但不关闭 stream
// 用于隧道连接场景：连接从控制连接映射移除后仍需要继续使用
func (r *ClientRegistry) Unregister(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connMap[connID]
	if !exists {
		return
	}

	// 从 clientIDMap 移除（只有在映射确实指向这个连接时才移除）
	if conn.Authenticated && conn.ClientID > 0 {
		if existingConn, exists := r.clientIDMap[conn.ClientID]; exists && existingConn.ConnID == connID {
			delete(r.clientIDMap, conn.ClientID)
		}
	}

	// 从 connMap 移除（但不关闭 stream）
	delete(r.connMap, connID)
	r.logger.Debugf("ClientRegistry: unregistered connection %s (stream kept open)", connID)
}

// kickConnectionInfo 用于在锁释放后执行 I/O 操作所需的连接信息
type kickConnectionInfo struct {
	connID   string
	clientID int64
	stream   stream.PackageStreamer
}

// KickOldConnection 踢掉旧的控制连接
// 采用"先移除后操作"模式：在持锁期间完成验证和映射移除，锁释放后再执行 I/O 操作
func (r *ClientRegistry) KickOldConnection(clientID int64, newConnID string, sendKickFn func(*ControlConnection, string, string)) {
	var connInfo *kickConnectionInfo
	var oldConnForCallback *ControlConnection

	// 1. 持锁期间：验证、记录信息、从映射移除
	r.mu.Lock()
	oldConn := r.clientIDMap[clientID]
	if oldConn != nil && oldConn.ConnID != newConnID {
		// 记录连接信息用于后续 I/O 操作
		connInfo = &kickConnectionInfo{
			connID:   oldConn.ConnID,
			clientID: oldConn.ClientID,
			stream:   oldConn.Stream,
		}
		// 保存引用用于回调（回调可能需要完整的连接信息）
		oldConnForCallback = oldConn

		r.logger.Warnf("ClientRegistry: kicking old connection - clientID=%d, oldConnID=%s, newConnID=%s",
			clientID, oldConn.ConnID, newConnID)

		// 从映射中移除（但不关闭 stream，稍后在锁外关闭）
		if oldConn.Authenticated && oldConn.ClientID > 0 {
			if existingConn, exists := r.clientIDMap[oldConn.ClientID]; exists && existingConn.ConnID == oldConn.ConnID {
				delete(r.clientIDMap, oldConn.ClientID)
			}
		}
		delete(r.connMap, oldConn.ConnID)
	}
	r.mu.Unlock()

	// 2. 锁释放后：执行 I/O 操作（发送 Kick 命令、关闭 Stream）
	if connInfo != nil {
		// 发送 Kick 命令
		if sendKickFn != nil && oldConnForCallback != nil {
			sendKickFn(oldConnForCallback, "Another client logged in with the same ID", "DUPLICATE_LOGIN")
		}

		// 关闭旧连接的 stream
		if connInfo.stream != nil {
			connInfo.stream.Close()
		}
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

// staleConnectionInfo 用于在锁释放后执行清理操作所需的连接信息
type staleConnectionInfo struct {
	connID       string
	clientID     int64
	idleDuration time.Duration
	stream       stream.PackageStreamer
}

// CleanupStale 清理过期的连接
// 采用"先移除后操作"模式：在持锁期间完成检查和映射移除，锁释放后再执行 I/O 操作
// 返回清理的连接数量
func (r *ClientRegistry) CleanupStale(timeout time.Duration, closeFn func(string) error) int {
	var staleInfos []staleConnectionInfo

	// 1. 持锁期间：检查、记录信息、从映射移除
	r.mu.Lock()
	for _, conn := range r.connMap {
		if conn.IsStale(timeout) {
			// 记录连接信息用于后续 I/O 操作
			staleInfos = append(staleInfos, staleConnectionInfo{
				connID:       conn.ConnID,
				clientID:     conn.ClientID,
				idleDuration: time.Since(conn.LastActiveAt),
				stream:       conn.Stream,
			})

			// 从映射中移除（但不关闭 stream，稍后在锁外关闭）
			if conn.Authenticated && conn.ClientID > 0 {
				if existingConn, exists := r.clientIDMap[conn.ClientID]; exists && existingConn.ConnID == conn.ConnID {
					delete(r.clientIDMap, conn.ClientID)
				}
			}
			delete(r.connMap, conn.ConnID)
		}
	}
	r.mu.Unlock()

	if len(staleInfos) == 0 {
		return 0
	}

	// 2. 锁释放后：执行 I/O 操作（关闭连接）
	for _, info := range staleInfos {
		r.logger.Warnf("ClientRegistry: removing stale connection - connID=%s, clientID=%d, idle=%v",
			info.connID, info.clientID, info.idleDuration)

		// 调用外部关闭函数
		if closeFn != nil {
			if err := closeFn(info.connID); err != nil {
				r.logger.Errorf("ClientRegistry: failed to close stale connection %s: %v", info.connID, err)
			}
		}

		// 关闭 stream
		if info.stream != nil {
			info.stream.Close()
		}
	}

	return len(staleInfos)
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

// ListAuthenticated 列出所有已认证的连接
func (r *ClientRegistry) ListAuthenticated() []*ControlConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ControlConnection
	for _, conn := range r.connMap {
		if conn.Authenticated {
			result = append(result, conn)
		}
	}
	return result
}
