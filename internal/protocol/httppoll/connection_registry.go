package httppoll

import (
	"sync"
)

// ConnectionRegistry HTTP 长轮询连接注册表
// 使用 ConnectionID 作为唯一标识，在连接创建时就注册
type ConnectionRegistry struct {
	mu          sync.RWMutex
	connections map[string]*ServerStreamProcessor
}

// NewConnectionRegistry 创建连接注册表
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		connections: make(map[string]*ServerStreamProcessor),
	}
}

// Register 注册连接
// 如果已存在相同 ConnectionID 的连接，关闭旧的（除非是同一个连接对象）
func (r *ConnectionRegistry) Register(connID string, conn *ServerStreamProcessor) {
	if connID == "" {
		return
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// 如果已存在且不是同一个连接对象，关闭旧的（防止重复连接）
	if oldConn, exists := r.connections[connID]; exists && oldConn != conn {
		// 只有在确实不同时才关闭旧的
		oldConn.Close()
	}
	
	r.connections[connID] = conn
}

// Get 获取连接
// 直接通过 ConnectionID 查找，O(1) 时间复杂度
func (r *ConnectionRegistry) Get(connID string) *ServerStreamProcessor {
	if connID == "" {
		return nil
	}
	
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[connID]
}

// Remove 移除连接
func (r *ConnectionRegistry) Remove(connID string) {
	if connID == "" {
		return
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, connID)
}

// Count 返回连接数量
func (r *ConnectionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connections)
}

// GetOrCreate 获取或创建连接（原子操作）
// 如果连接不存在，使用 createFunc 创建新连接并注册
// 如果连接已存在，返回已存在的连接
func (r *ConnectionRegistry) GetOrCreate(connID string, createFunc func() *ServerStreamProcessor) *ServerStreamProcessor {
	if connID == "" {
		return nil
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// 先检查是否已存在
	if existingConn, exists := r.connections[connID]; exists {
		return existingConn
	}
	
	// 不存在，创建新连接
	newConn := createFunc()
	if newConn == nil {
		return nil
	}
	
	r.connections[connID] = newConn
	return newConn
}

