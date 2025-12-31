// Package tunnel 提供隧道桥接和路由功能
package tunnel

import (
	"time"

	coreerrors "tunnox-core/internal/core/errors"
)

// ============================================================================
// 连接管理
// ============================================================================

// SetTargetConnection 设置目标端连接（统一接口）
func (b *Bridge) SetTargetConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.targetTunnelConn = conn
	if conn != nil {
		b.targetConn = conn.GetNetConn()
		b.targetStream = conn.GetStream()
		b.targetForwarder = CreateDataForwarder(b.targetConn, b.targetStream)
	}
	b.tunnelConnMu.Unlock()
	close(b.ready)
}

// SetSourceConnection 设置源端连接（统一接口）
func (b *Bridge) SetSourceConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.sourceTunnelConn = conn
	if conn != nil {
		b.sourceConn = conn.GetNetConn()
		b.sourceStream = conn.GetStream()
		b.sourceForwarder = CreateDataForwarder(b.sourceConn, b.sourceStream)
	} else {
		b.sourceForwarder = nil
	}
	b.tunnelConnMu.Unlock()
}

// WaitForTarget 等待目标端连接就绪
func (b *Bridge) WaitForTarget(timeout time.Duration) error {
	select {
	case <-b.ready:
		return nil
	case <-time.After(timeout):
		return coreerrors.New(coreerrors.CodeTimeout, "timeout waiting for target connection")
	case <-b.Ctx().Done():
		return coreerrors.Wrap(b.Ctx().Err(), coreerrors.CodeCancelled, "context cancelled")
	}
}

// IsTargetReady 检查目标端是否就绪
func (b *Bridge) IsTargetReady() bool {
	select {
	case <-b.ready:
		return true
	default:
		return false
	}
}

// NotifyTargetReady 通知目标端就绪（用于跨节点场景）
func (b *Bridge) NotifyTargetReady() {
	select {
	case <-b.ready:
		// 已经关闭，忽略
	default:
		close(b.ready)
	}
}

// ============================================================================
// 跨节点连接管理
// ============================================================================

// SetCrossNodeConnection 设置跨节点连接
func (b *Bridge) SetCrossNodeConnection(conn CrossNodeConnInterface) {
	b.crossNodeConnMu.Lock()
	b.crossNodeConn = conn
	b.crossNodeConnMu.Unlock()
}

// GetCrossNodeConnection 获取跨节点连接
func (b *Bridge) GetCrossNodeConnection() CrossNodeConnInterface {
	b.crossNodeConnMu.RLock()
	defer b.crossNodeConnMu.RUnlock()
	return b.crossNodeConn
}

// ReleaseCrossNodeConnection 释放跨节点连接
// 只清理 Bridge 中的引用，连接的生命周期由数据转发函数管理
func (b *Bridge) ReleaseCrossNodeConnection() {
	b.crossNodeConnMu.Lock()
	b.crossNodeConn = nil // 只清理引用，不关闭连接
	b.crossNodeConnMu.Unlock()
}
