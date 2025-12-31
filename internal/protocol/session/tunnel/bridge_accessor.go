// Package tunnel 提供隧道桥接和路由功能
package tunnel

import "net"

// ============================================================================
// 访问器方法 (实现 BridgeAccessor 接口)
// ============================================================================

// GetTunnelID 获取隧道ID
func (b *Bridge) GetTunnelID() string {
	if b == nil {
		return ""
	}
	return b.tunnelID
}

// GetSourceConnectionID 获取源连接ID
func (b *Bridge) GetSourceConnectionID() string {
	if b == nil {
		return ""
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	if b.sourceTunnelConn != nil {
		return b.sourceTunnelConn.GetConnectionID()
	}
	return ""
}

// GetTargetConnectionID 获取目标连接ID
func (b *Bridge) GetTargetConnectionID() string {
	if b == nil {
		return ""
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	if b.targetTunnelConn != nil {
		return b.targetTunnelConn.GetConnectionID()
	}
	return ""
}

// GetMappingID 获取映射ID
func (b *Bridge) GetMappingID() string {
	if b == nil {
		return ""
	}
	return b.mappingID
}

// GetClientID 获取客户端ID
func (b *Bridge) GetClientID() int64 {
	if b == nil {
		return 0
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	if b.sourceTunnelConn != nil {
		return b.sourceTunnelConn.GetClientID()
	}
	return 0
}

// IsActive 检查桥接是否活跃
func (b *Bridge) IsActive() bool {
	if b == nil {
		return false
	}
	return !b.IsClosed()
}

// GetSourceTunnelConn 获取源端隧道连接
func (b *Bridge) GetSourceTunnelConn() TunnelConnectionInterface {
	if b == nil {
		return nil
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	return b.sourceTunnelConn
}

// GetTargetTunnelConn 获取目标端隧道连接
func (b *Bridge) GetTargetTunnelConn() TunnelConnectionInterface {
	if b == nil {
		return nil
	}
	b.tunnelConnMu.RLock()
	defer b.tunnelConnMu.RUnlock()
	return b.targetTunnelConn
}

// GetSourceNetConn 获取源端网络连接
func (b *Bridge) GetSourceNetConn() net.Conn {
	if b == nil {
		return nil
	}
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

// GetTargetNetConn 获取目标端网络连接
func (b *Bridge) GetTargetNetConn() net.Conn {
	if b == nil {
		return nil
	}
	return b.targetConn
}

// GetSourceConn 获取源端连接（线程安全）
func (b *Bridge) GetSourceConn() net.Conn {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

// GetSourceForwarder 获取源端数据转发器（线程安全）
func (b *Bridge) GetSourceForwarder() DataForwarder {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceForwarder
}

// GetTargetForwarder 获取目标端数据转发器
func (b *Bridge) GetTargetForwarder() DataForwarder {
	return b.targetForwarder
}
