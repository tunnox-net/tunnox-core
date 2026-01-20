package session

import (
	"time"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/security"
)

// ============================================================================
// Handler 设置
// ============================================================================

// SetTunnelHandler 设置隧道处理器
func (s *SessionManager) SetTunnelHandler(handler TunnelHandler) {
	s.tunnelHandler = handler
	corelog.Debug("Tunnel handler configured in SessionManager")
}

// SetAuthHandler 设置认证处理器
func (s *SessionManager) SetAuthHandler(handler AuthHandler) {
	s.authHandler = handler
	corelog.Debug("Auth handler configured in SessionManager")
}

// SetCloudControl 设置CloudControl API
func (s *SessionManager) SetCloudControl(cc CloudControlAPI) {
	s.cloudControl = cc
	corelog.Debugf("CloudControl API configured in SessionManager")
}

// SetBridgeManager 设置BridgeManager（用于跨服务器隧道转发）
func (s *SessionManager) SetBridgeManager(bridgeManager BridgeManager) {
	s.bridgeManager = bridgeManager
	corelog.Infof("SessionManager: BridgeManager configured for cross-server forwarding")

	// 启动跨节点广播订阅
	s.startTunnelOpenBroadcastSubscription()
	s.startConfigPushBroadcastSubscription()
}

// SetReconnectTokenManager 设置ReconnectTokenManager（用于生成重连Token）
func (s *SessionManager) SetReconnectTokenManager(manager *security.ReconnectTokenManager) {
	s.reconnectTokenManager = manager
	corelog.Debugf("SessionManager: ReconnectTokenManager configured")
}

// ============================================================================
// 跨节点组件设置
// ============================================================================

// SetTunnelRoutingTable 设置隧道路由表
func (s *SessionManager) SetTunnelRoutingTable(routingTable *TunnelRoutingTable) {
	s.tunnelRouting = routingTable
	corelog.Infof("SessionManager: TunnelRoutingTable configured")
}

// SetCrossNodePool 设置跨节点连接池
func (s *SessionManager) SetCrossNodePool(pool *CrossNodePool) {
	s.crossNodePool = pool
	corelog.Infof("SessionManager: CrossNodePool configured")
}

// GetCrossNodePool 获取跨节点连接池
func (s *SessionManager) GetCrossNodePool() *CrossNodePool {
	return s.crossNodePool
}

// SetTunnelConnectionManager 设置隧道连接管理器（专用连接模型）
func (s *SessionManager) SetTunnelConnectionManager(mgr *TunnelConnectionManager) {
	s.tunnelConnMgr = mgr
	corelog.Infof("SessionManager: TunnelConnectionManager configured")
}

// GetTunnelConnectionManager 获取隧道连接管理器
func (s *SessionManager) GetTunnelConnectionManager() *TunnelConnectionManager {
	return s.tunnelConnMgr
}

// SetCrossNodeListener 设置跨节点连接监听器
func (s *SessionManager) SetCrossNodeListener(listener *CrossNodeListener) {
	s.crossNodeListener = listener
	corelog.Infof("SessionManager: CrossNodeListener configured")
}

// GetCrossNodeListener 获取跨节点连接监听器
func (s *SessionManager) GetCrossNodeListener() *CrossNodeListener {
	return s.crossNodeListener
}

// SetConnectionStateStore 设置连接状态存储
func (s *SessionManager) SetConnectionStateStore(store *ConnectionStateStore) {
	s.connStateStore = store
	corelog.Infof("SessionManager: ConnectionStateStore configured")
}

// GetConnectionStateStore 获取连接状态存储
func (s *SessionManager) GetConnectionStateStore() *ConnectionStateStore {
	return s.connStateStore
}

// SetNodeID 设置节点ID
func (s *SessionManager) SetNodeID(nodeID string) {
	s.nodeID = nodeID
	corelog.Infof("SessionManager: NodeID set to %s", nodeID)
}

// GetNodeID 获取节点ID
func (s *SessionManager) GetNodeID() string {
	return s.nodeID
}

// ============================================================================
// 新架构组件访问器
// ============================================================================

// GetClientRegistry 获取客户端注册表
func (s *SessionManager) GetClientRegistry() *ClientRegistry {
	return s.clientRegistry
}

// GetTunnelRegistry 获取隧道注册表
func (s *SessionManager) GetTunnelRegistry() *TunnelRegistry {
	return s.tunnelRegistry
}

// GetPacketRouter 获取数据包路由器
func (s *SessionManager) GetPacketRouter() *PacketRouter {
	return s.packetRouter
}

// ============================================================================
// TunnelStateTracker 实现（用于过滤残留帧）
// ============================================================================

// MarkTunnelClosed 标记 tunnel 为已关闭状态
func (s *SessionManager) MarkTunnelClosed(tunnelID string) {
	s.closedTunnelsMu.Lock()
	defer s.closedTunnelsMu.Unlock()
	s.closedTunnels[tunnelID] = time.Now()
	corelog.Debugf("SessionManager: marked tunnel %s as closed", tunnelID)
}

// IsTunnelClosed 检查 tunnel 是否已关闭
func (s *SessionManager) IsTunnelClosed(tunnelID string) bool {
	s.closedTunnelsMu.RLock()
	defer s.closedTunnelsMu.RUnlock()
	_, exists := s.closedTunnels[tunnelID]
	return exists
}
