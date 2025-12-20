package session

import (
	corelog "tunnox-core/internal/core/log"
)

// GetClientID 获取客户端 ID
func (c *ServerHTTPLongPollingConn) GetClientID() int64 {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	return c.clientID
}

// GetMappingID 获取映射ID（隧道连接才有，指令通道返回空字符串）
func (c *ServerHTTPLongPollingConn) GetMappingID() string {
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	return c.mappingID
}

// SetMappingID 设置映射ID（隧道连接才有）
func (c *ServerHTTPLongPollingConn) SetMappingID(mappingID string) {
	c.closeMu.Lock()
	c.mappingID = mappingID
	clientID := c.clientID
	c.closeMu.Unlock()

	corelog.Infof("HTTP long polling: [SetMappingID] setting mappingID=%s, clientID=%d, connID=%s",
		mappingID, clientID, c.GetConnectionID())
}

// SetStreamMode 切换到流模式（隧道建立后调用）
func (c *ServerHTTPLongPollingConn) SetStreamMode(streamMode bool) {
	c.streamMu.Lock()
	oldMode := c.streamMode
	c.streamMode = streamMode
	c.streamMu.Unlock()

	corelog.Infof("HTTP long polling: [SetStreamMode] switching stream mode from %v to %v, clientID=%d, mappingID=%s",
		oldMode, streamMode, c.GetClientID(), c.GetMappingID())
}

// IsStreamMode 检查是否处于流模式
func (c *ServerHTTPLongPollingConn) IsStreamMode() bool {
	c.streamMu.RLock()
	defer c.streamMu.RUnlock()
	return c.streamMode
}

// ShouldKeepInConnMap 判断是否应该保留在 connMap 中
// HTTP 长轮询连接需要保留，因为读取循环还在运行
func (c *ServerHTTPLongPollingConn) ShouldKeepInConnMap() bool {
	return true
}

// CanCreateTemporaryControlConn 判断是否可以创建临时控制连接
// HTTP 长轮询隧道连接可能没有注册为控制连接，可以创建临时控制连接
func (c *ServerHTTPLongPollingConn) CanCreateTemporaryControlConn() bool {
	return true
}

// SetConnectionID 设置连接 ID（唯一标识，在创建时就确定）
func (c *ServerHTTPLongPollingConn) SetConnectionID(connID string) {
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	c.connectionID = connID
}

// GetConnectionID 获取连接 ID
func (c *ServerHTTPLongPollingConn) GetConnectionID() string {
	c.connectionMu.RLock()
	defer c.connectionMu.RUnlock()
	return c.connectionID
}

// OnHandshakeComplete 握手完成回调（统一接口）
// 当握手成功且 clientID > 0 时，自动调用此方法
func (c *ServerHTTPLongPollingConn) OnHandshakeComplete(clientID int64) {
	c.UpdateClientID(clientID)
}

// UpdateClientID 更新客户端 ID（握手后调用）
// 注意：ConnectionID 不变，只更新 clientID
func (c *ServerHTTPLongPollingConn) UpdateClientID(newClientID int64) {
	c.closeMu.Lock()
	oldClientID := c.clientID
	c.clientID = newClientID
	c.closeMu.Unlock()

	corelog.Infof("HTTP long polling: [UpdateClientID] updated clientID from %d to %d, connID=%s",
		oldClientID, newClientID, c.GetConnectionID())
}
