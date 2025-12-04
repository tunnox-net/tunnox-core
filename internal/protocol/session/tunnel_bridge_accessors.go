package session

// GetTunnelID 获取隧道ID（实现 TunnelBridgeAccessor 接口）
func (b *TunnelBridge) GetTunnelID() string {
	if b == nil {
		return ""
	}
	return b.tunnelID
}

// GetSourceConnectionID 获取源连接ID（实现 TunnelBridgeAccessor 接口）
func (b *TunnelBridge) GetSourceConnectionID() string {
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

// GetTargetConnectionID 获取目标连接ID（实现 TunnelBridgeAccessor 接口）
func (b *TunnelBridge) GetTargetConnectionID() string {
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

// GetMappingID 获取映射ID（实现 TunnelBridgeAccessor 接口）
func (b *TunnelBridge) GetMappingID() string {
	if b == nil {
		return ""
	}
	return b.mappingID
}

// GetClientID 获取客户端ID（实现 TunnelBridgeAccessor 接口）
func (b *TunnelBridge) GetClientID() int64 {
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

// IsActive 检查桥接是否活跃（实现 TunnelBridgeAccessor 接口）
func (b *TunnelBridge) IsActive() bool {
	if b == nil {
		return false
	}
	return !b.IsClosed()
}

