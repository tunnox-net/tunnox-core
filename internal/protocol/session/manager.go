package session

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/security"
	"tunnox-core/internal/stream"
)

// SessionConfig SessionManager配置
type SessionConfig struct {
	// HeartbeatTimeout 心跳超时时间（超过此时间未收到心跳则认为连接失效）
	HeartbeatTimeout time.Duration

	// CleanupInterval 清理检查间隔（定期扫描并清理过期连接）
	CleanupInterval time.Duration

	// MaxConnections 最大连接数限制（0表示无限制）
	MaxConnections int

	// MaxControlConnections 最大控制连接数限制（0表示无限制）
	MaxControlConnections int
}

// DefaultSessionConfig 返回默认配置
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		HeartbeatTimeout:      60 * time.Second, // 缩短为60秒（负载均衡器后面连接多，需要更积极清理）
		CleanupInterval:       15 * time.Second, // 缩短为15秒（更频繁检查）
		MaxConnections:        10000,            // 最大10000个连接
		MaxControlConnections: 5000,             // 最大5000个控制连接
	}
}

// SessionManager 会话管理器（双连接模型）
//
// 职责说明：
// 本文件仅包含核心协调逻辑，具体实现已按功能拆分：
//
//   - client_registry.go: 客户端注册表（控制连接管理）
//   - tunnel_registry.go: 隧道注册表（隧道连接管理）
//   - packet_router.go: 数据包路由
//   - connection_lifecycle.go: 连接生命周期管理
//   - command_integration.go: Command 集成
//   - packet_handler.go: 数据包处理
//   - event_handlers.go: 事件处理
type SessionManager struct {
	// ============================================================================
	// 新架构组件（推荐使用）
	// ============================================================================

	// 客户端注册表（控制连接管理）
	clientRegistry *ClientRegistry

	// 隧道注册表（隧道连接管理）
	tunnelRegistry *TunnelRegistry

	// 数据包路由器
	packetRouter *PacketRouter

	// ============================================================================
	// 旧架构字段（保持向后兼容，逐步迁移）
	// ============================================================================

	// 基础连接映射（所有连接）
	connMap  map[string]*types.Connection
	connLock sync.RWMutex

	// 指令连接（Control Connection）- 已迁移到 clientRegistry
	controlConnMap   map[string]*ControlConnection // connID -> 指令连接
	clientIDIndexMap map[int64]*ControlConnection  // clientID -> 指令连接（快速查找）
	controlConnLock  sync.RWMutex

	// 映射连接（Tunnel Connection）- 已迁移到 tunnelRegistry
	tunnelConnMap  map[string]*TunnelConnection // connID -> 映射连接
	tunnelIDMap    map[string]*TunnelConnection // tunnelID -> 映射连接
	tunnelConnLock sync.RWMutex

	idManager     *idgen.IDManager
	streamMgr     *stream.StreamManager
	streamFactory stream.StreamFactory

	// 日志接口（支持依赖注入）
	logger corelog.Logger

	// 事件驱动架构
	eventBus        events.EventBus
	responseManager *ResponseManager

	// Command集成
	commandRegistry types.CommandRegistry
	commandExecutor types.CommandExecutor

	// 隧道和认证处理器
	tunnelHandler TunnelHandler
	authHandler   AuthHandler

	// 隧道桥接管理
	tunnelBridges map[string]*TunnelBridge // tunnelID -> bridge
	bridgeLock    sync.RWMutex

	// CloudControl API（用于查询映射配置）
	cloudControl CloudControlAPI

	// BridgeManager（用于跨服务器隧道转发）
	bridgeManager BridgeManager

	// TunnelRoutingTable（用于跨服务器隧道路由）
	tunnelRouting *TunnelRoutingTable

	// 节点ID（用于跨服务器识别）
	nodeID string

	// 配置
	config *SessionConfig

	// ReconnectTokenManager（用于生成重连Token）
	reconnectTokenManager *security.ReconnectTokenManager
	sessionTokenManager   *security.SessionTokenManager

	// ✨ Phase 2: 隧道迁移支持
	tunnelStateManager *TunnelStateManager
	migrationManager   *TunnelMigrationManager

	dispose.Dispose
}

// NewSessionManager 创建新的会话管理器（双连接模型）
func NewSessionManager(idManager *idgen.IDManager, parentCtx context.Context) *SessionManager {
	return NewSessionManagerWithConfig(idManager, parentCtx, DefaultSessionConfig())
}

// NewSessionManagerWithConfig 使用指定配置创建会话管理器
func NewSessionManagerWithConfig(idManager *idgen.IDManager, parentCtx context.Context, config *SessionConfig) *SessionManager {
	if config == nil {
		config = DefaultSessionConfig()
	}

	// 创建默认流工厂
	streamFactory := stream.NewDefaultStreamFactory(parentCtx)

	// 创建流管理器
	streamMgr := stream.NewStreamManager(streamFactory, parentCtx)

	// 默认使用全局 Logger
	logger := corelog.Default()

	// 创建新架构组件
	clientRegistry := NewClientRegistry(&ClientRegistryConfig{
		MaxConnections: config.MaxControlConnections,
		Logger:         logger,
	})

	tunnelRegistry := NewTunnelRegistry(&TunnelRegistryConfig{
		Logger: logger,
	})

	packetRouter := NewPacketRouter(&PacketRouterConfig{
		Logger: logger,
	})

	session := &SessionManager{
		// 新架构组件
		clientRegistry: clientRegistry,
		tunnelRegistry: tunnelRegistry,
		packetRouter:   packetRouter,

		// 基础连接（保持向后兼容）
		connMap: make(map[string]*types.Connection),

		// 指令连接（保持向后兼容，同时使用 clientRegistry）
		controlConnMap:   make(map[string]*ControlConnection),
		clientIDIndexMap: make(map[int64]*ControlConnection),

		// 映射连接（保持向后兼容，同时使用 tunnelRegistry）
		tunnelConnMap: make(map[string]*TunnelConnection),
		tunnelIDMap:   make(map[string]*TunnelConnection),

		// 隧道桥接
		tunnelBridges: make(map[string]*TunnelBridge),

		idManager:     idManager,
		streamMgr:     streamMgr,
		streamFactory: streamFactory,
		config:        config,
		logger:        logger,

		// 事件驱动架构将在后续设置
		eventBus:        nil,
		responseManager: nil,
		tunnelHandler:   nil,
		authHandler:     nil,
	}

	// 设置资源清理回调
	session.SetCtx(parentCtx, session.onClose)

	// 启动连接清理协程
	session.startConnectionCleanup()

	// 注意：跨服务器订阅将在SetBridgeManager()之后启动

	return session
}

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

// CloudControlAPI 定义CloudControl接口
// ✅ 统一返回 *models.PortMapping，不再使用 interface{}
type CloudControlAPI interface {
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	UpdatePortMappingStats(mappingID string, stats interface{}) error
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)
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
// Stream 管理
// ============================================================================

// GetStreamManager 获取流管理器
func (s *SessionManager) GetStreamManager() *stream.StreamManager {
	return s.streamMgr
}

// GetStreamFactory 获取流工厂
func (s *SessionManager) GetStreamFactory() stream.StreamFactory {
	return s.streamFactory
}

// ============================================================================
// 资源清理
// ============================================================================

// onClose 资源清理回调
func (s *SessionManager) onClose() error {
	corelog.Infof("Cleaning up session manager resources...")

	// 取消事件订阅
	if s.eventBus != nil {
		if err := s.eventBus.Unsubscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
			corelog.Warnf("Failed to unsubscribe from DisconnectRequest events: %v", err)
		}
		corelog.Infof("Unsubscribed from disconnect request events")
	}

	// 关闭新架构组件
	if s.clientRegistry != nil {
		s.clientRegistry.Close()
	}
	if s.tunnelRegistry != nil {
		s.tunnelRegistry.Close()
	}

	// 关闭所有连接（旧架构，保持向后兼容）
	s.connLock.Lock()
	connCount := len(s.connMap)
	for _, conn := range s.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		if conn.RawConn != nil {
			conn.RawConn.Close()
		}
	}
	s.connMap = make(map[string]*types.Connection)
	s.connLock.Unlock()

	// 关闭所有控制连接（旧架构）
	s.controlConnLock.Lock()
	controlConnCount := len(s.controlConnMap)
	for _, conn := range s.controlConnMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
	}
	s.controlConnMap = make(map[string]*ControlConnection)
	s.clientIDIndexMap = make(map[int64]*ControlConnection)
	s.controlConnLock.Unlock()

	// 关闭所有隧道连接（旧架构）
	s.tunnelConnLock.Lock()
	tunnelConnCount := len(s.tunnelConnMap)
	for _, conn := range s.tunnelConnMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
	}
	s.tunnelConnMap = make(map[string]*TunnelConnection)
	s.tunnelIDMap = make(map[string]*TunnelConnection)
	s.tunnelConnLock.Unlock()

	corelog.Infof("SessionManager: closed %d connections, %d control connections, %d tunnel connections",
		connCount, controlConnCount, tunnelConnCount)

	// 关闭流管理器
	if s.streamMgr != nil {
		corelog.Debug("Stream manager resources cleaned up")
	}

	// 关闭事件总线
	if s.eventBus != nil {
		if err := s.eventBus.Close(); err != nil {
			corelog.Errorf("Failed to close event bus: %v", err)
		}
		corelog.Info("Event bus resources cleaned up")
	}

	corelog.Info("Session manager resources cleanup completed")
	return nil
}

// SetTunnelRoutingTable 设置隧道路由表
func (s *SessionManager) SetTunnelRoutingTable(routingTable *TunnelRoutingTable) {
	s.tunnelRouting = routingTable
	corelog.Infof("SessionManager: TunnelRoutingTable configured")
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
