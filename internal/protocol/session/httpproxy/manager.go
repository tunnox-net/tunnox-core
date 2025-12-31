// Package httpproxy HTTP 代理功能
// 实现 Server 端到 Client 端的 HTTP 代理请求转发
package httpproxy

import (
	"context"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/httptypes"
)

// Manager HTTP 代理管理器
// 管理 HTTP 代理请求的发送和响应等待
type Manager struct {
	// 等待响应的请求
	pendingRequests map[string]chan *httptypes.HTTPProxyResponse
	pendingMu       sync.RWMutex

	// 默认超时
	defaultTimeout time.Duration
}

// NewManager 创建 HTTP 代理管理器
func NewManager() *Manager {
	return &Manager{
		pendingRequests: make(map[string]chan *httptypes.HTTPProxyResponse),
		defaultTimeout:  30 * time.Second,
	}
}

// RegisterPendingRequest 注册等待响应的请求
func (m *Manager) RegisterPendingRequest(requestID string) chan *httptypes.HTTPProxyResponse {
	ch := make(chan *httptypes.HTTPProxyResponse, 1)

	m.pendingMu.Lock()
	m.pendingRequests[requestID] = ch
	m.pendingMu.Unlock()

	return ch
}

// UnregisterPendingRequest 注销等待响应的请求
func (m *Manager) UnregisterPendingRequest(requestID string) {
	m.pendingMu.Lock()
	delete(m.pendingRequests, requestID)
	m.pendingMu.Unlock()
}

// HandleResponse 处理 HTTP 代理响应
func (m *Manager) HandleResponse(resp *httptypes.HTTPProxyResponse) {
	m.pendingMu.RLock()
	ch, exists := m.pendingRequests[resp.RequestID]
	m.pendingMu.RUnlock()

	if !exists {
		corelog.Warnf("HTTPProxyManager: no pending request for ID %s", resp.RequestID)
		return
	}

	select {
	case ch <- resp:
	default:
		corelog.Warnf("HTTPProxyManager: response channel full for request %s", resp.RequestID)
	}
}

// WaitForResponse 等待响应
func (m *Manager) WaitForResponse(
	ctx context.Context,
	requestID string,
	timeout time.Duration,
) (*httptypes.HTTPProxyResponse, error) {
	ch := m.RegisterPendingRequest(requestID)
	defer m.UnregisterPendingRequest(requestID)

	if timeout == 0 {
		timeout = m.defaultTimeout
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return nil, coreerrors.New(coreerrors.CodeTimeout, "HTTP proxy request timeout")
	case <-ctx.Done():
		return nil, coreerrors.New(coreerrors.CodeTimeout, "context cancelled")
	}
}

// ============================================================================
// 全局管理器（懒加载）
// ============================================================================

var (
	globalManager     *Manager
	globalManagerOnce sync.Once
)

// GetGlobalManager 获取全局 HTTP 代理管理器
func GetGlobalManager() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = NewManager()
	})
	return globalManager
}
