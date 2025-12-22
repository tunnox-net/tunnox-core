package session

import (
	"fmt"
	"net"
	"time"

	"tunnox-core/internal/stream"
)

// SetTargetConnection 设置目标端连接（统一接口）
func (b *TunnelBridge) SetTargetConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.targetTunnelConn = conn
	if conn != nil {
		b.targetConn = conn.GetNetConn()
		b.targetStream = conn.GetStream()
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}
	b.tunnelConnMu.Unlock()
	close(b.ready)
}

// SetTargetConnectionLegacy 设置目标端连接（向后兼容）
func (b *TunnelBridge) SetTargetConnectionLegacy(targetConn net.Conn, targetStream stream.PackageStreamer) {
	b.targetConn = targetConn
	b.targetStream = targetStream
	b.targetForwarder = createDataForwarder(targetConn, targetStream)

	// 创建统一接口
	if targetConn != nil || targetStream != nil {
		connID := ""
		if targetConn != nil {
			connID = targetConn.RemoteAddr().String()
		}
		clientID := extractClientID(targetStream, targetConn)
		b.tunnelConnMu.Lock()
		b.targetTunnelConn = CreateTunnelConnection(
			connID,
			targetConn,
			targetStream,
			clientID,
			b.mappingID,
			b.tunnelID,
		)
		b.tunnelConnMu.Unlock()
	}

	close(b.ready)
}

// SetSourceConnection 设置源端连接（统一接口）
func (b *TunnelBridge) SetSourceConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.sourceTunnelConn = conn
	if conn != nil {
		b.sourceConn = conn.GetNetConn()
		b.sourceStream = conn.GetStream()
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
	} else {
		b.sourceForwarder = nil
	}
	b.tunnelConnMu.Unlock()
}

// SetSourceConnectionLegacy 设置源端连接（向后兼容）
func (b *TunnelBridge) SetSourceConnectionLegacy(sourceConn net.Conn, sourceStream stream.PackageStreamer) {
	b.sourceConnMu.Lock()
	b.sourceConn = sourceConn
	b.sourceForwarder = createDataForwarder(sourceConn, sourceStream)
	b.sourceConnMu.Unlock()
	if sourceStream != nil {
		b.sourceStream = sourceStream
	}

	// 创建统一接口
	if sourceConn != nil || sourceStream != nil {
		connID := ""
		if sourceConn != nil {
			connID = sourceConn.RemoteAddr().String()
		}
		clientID := extractClientID(sourceStream, sourceConn)
		b.tunnelConnMu.Lock()
		b.sourceTunnelConn = CreateTunnelConnection(
			connID,
			sourceConn,
			sourceStream,
			clientID,
			b.mappingID,
			b.tunnelID,
		)
		b.tunnelConnMu.Unlock()
	}
}

// getSourceConn 获取源端连接（线程安全）
func (b *TunnelBridge) getSourceConn() net.Conn {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

// getSourceForwarder 获取源端数据转发器（线程安全）
func (b *TunnelBridge) getSourceForwarder() DataForwarder {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceForwarder
}

// WaitForTarget 等待目标端连接就绪
func (b *TunnelBridge) WaitForTarget(timeout time.Duration) error {
	select {
	case <-b.ready:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for target connection")
	case <-b.Ctx().Done():
		return b.Ctx().Err()
	}
}

// IsTargetReady 检查目标端是否就绪
func (b *TunnelBridge) IsTargetReady() bool {
	select {
	case <-b.ready:
		return true
	default:
		return false
	}
}

// NotifyTargetReady 通知目标端就绪（用于跨节点场景）
func (b *TunnelBridge) NotifyTargetReady() {
	select {
	case <-b.ready:
		// 已经关闭，忽略
	default:
		close(b.ready)
	}
}

// SetCrossNodeConnection 设置跨节点连接
func (b *TunnelBridge) SetCrossNodeConnection(conn *CrossNodeConn) {
	b.crossNodeConnMu.Lock()
	b.crossNodeConn = conn
	b.crossNodeConnMu.Unlock()
}

// GetCrossNodeConnection 获取跨节点连接
func (b *TunnelBridge) GetCrossNodeConnection() *CrossNodeConn {
	b.crossNodeConnMu.RLock()
	defer b.crossNodeConnMu.RUnlock()
	return b.crossNodeConn
}

// ReleaseCrossNodeConnection 释放跨节点连接（归还到池）
func (b *TunnelBridge) ReleaseCrossNodeConnection() {
	b.crossNodeConnMu.Lock()
	conn := b.crossNodeConn
	b.crossNodeConn = nil
	b.crossNodeConnMu.Unlock()

	if conn != nil {
		conn.Release()
	}
}
