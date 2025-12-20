package session

import (
corelog "tunnox-core/internal/core/log"
	"net"

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
	oldConn := b.sourceTunnelConn
	b.sourceTunnelConn = conn
	oldForwarder := b.sourceForwarder
	if conn != nil {
		b.sourceConn = conn.GetNetConn()
		b.sourceStream = conn.GetStream()
		connID := "unknown"
		if conn.GetStream() != nil {
			if streamConn, ok := conn.GetStream().(interface{ GetConnectionID() string }); ok {
				connID = streamConn.GetConnectionID()
			}
		}
		corelog.Infof("TunnelBridge[%s]: SetSourceConnection creating forwarder, connID=%s, hasNetConn=%v, hasStream=%v", b.tunnelID, connID, b.sourceConn != nil, b.sourceStream != nil)
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
		corelog.Infof("TunnelBridge[%s]: SetSourceConnection forwarder created, forwarder=%v, connID=%s", b.tunnelID, b.sourceForwarder != nil, connID)
	} else {
		b.sourceForwarder = nil
		corelog.Infof("TunnelBridge[%s]: SetSourceConnection clearing connection", b.tunnelID)
	}
	b.tunnelConnMu.Unlock()
	corelog.Infof("TunnelBridge[%s]: updated sourceConn (unified), mappingID=%s, oldConn=%v, newConn=%v, oldForwarder=%v, newForwarder=%v",
		b.tunnelID, b.mappingID, oldConn != nil, conn != nil, oldForwarder != nil, b.sourceForwarder != nil)
}

// SetSourceConnectionLegacy 设置源端连接（向后兼容）
func (b *TunnelBridge) SetSourceConnectionLegacy(sourceConn net.Conn, sourceStream stream.PackageStreamer) {
	b.sourceConnMu.Lock()
	oldConn := b.sourceConn
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

	corelog.Infof("TunnelBridge[%s]: updated sourceConn (legacy), mappingID=%s, oldConn=%v, newConn=%v, hasForwarder=%v",
		b.tunnelID, b.mappingID, oldConn, sourceConn, b.sourceForwarder != nil)
}

// getSourceConn 获取源端连接（线程安全）
func (b *TunnelBridge) getSourceConn() net.Conn {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

