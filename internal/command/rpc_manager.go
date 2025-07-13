package command

import (
	"sync"
	"time"
)

// RPCManager RPC管理器，用于管理双工命令的请求-响应
type RPCManager struct {
	pendingRequests map[string]chan *CommandResponse
	timeout         time.Duration
	mu              sync.RWMutex
}

// NewRPCManager 创建新的RPC管理器
func NewRPCManager() *RPCManager {
	return &RPCManager{
		pendingRequests: make(map[string]chan *CommandResponse),
		timeout:         30 * time.Second,
	}
}

// RegisterRequest 注册请求
func (rm *RPCManager) RegisterRequest(requestID string, responseChan chan *CommandResponse) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.pendingRequests[requestID] = responseChan
}

// UnregisterRequest 注销请求
func (rm *RPCManager) UnregisterRequest(requestID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.pendingRequests, requestID)
}

// GetRequest 获取请求
func (rm *RPCManager) GetRequest(requestID string) (chan *CommandResponse, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	responseChan, exists := rm.pendingRequests[requestID]
	return responseChan, exists
}

// SetTimeout 设置超时时间
func (rm *RPCManager) SetTimeout(timeout time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.timeout = timeout
}

// GetTimeout 获取超时时间
func (rm *RPCManager) GetTimeout() time.Duration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.timeout
}

// GetPendingRequestCount 获取待处理请求数量
func (rm *RPCManager) GetPendingRequestCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.pendingRequests)
}
