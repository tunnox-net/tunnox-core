package session

import (
	"context"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/protocol/session/core"
	"tunnox-core/internal/security"
	"tunnox-core/internal/stream"
)

// 会话管理器默认配置常量
const (
	// DefaultHeartbeatTimeout 默认心跳超时时间
	// 缩短为60秒，负载均衡器后面连接多，需要更积极清理
	DefaultHeartbeatTimeout = 60 * time.Second

	// DefaultCleanupInterval 默认清理检查间隔
	// 缩短为15秒，更频繁检查过期连接
	DefaultCleanupInterval = 15 * time.Second

	// DefaultMaxConnections 默认最大连接数限制
	DefaultMaxConnections = 10000

	// DefaultMaxControlConnections 默认最大控制连接数限制
	DefaultMaxControlConnections = 5000
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
		HeartbeatTimeout:      DefaultHeartbeatTimeout,
		CleanupInterval:       DefaultCleanupInterval,
		MaxConnections:        DefaultMaxConnections,
		MaxControlConnections: DefaultMaxControlConnections,
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

	// 基础连接映射（所有连接的原始映射）
	connMap  map[string]*types.Connection
	connLock sync.RWMutex

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

	// CrossNodePool（跨节点连接池）
	crossNodePool *CrossNodePool

	// CrossNodeListener（跨节点连接监听器）
	crossNodeListener *CrossNodeListener

	// ConnectionStateStore（用于跨节点连接状态查询）
	connStateStore *ConnectionStateStore

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

	// 已关闭的 tunnel 跟踪（用于过滤残留帧）
	closedTunnels   map[string]time.Time // tunnelID -> 关闭时间
	closedTunnelsMu sync.RWMutex

	*dispose.ManagerBase
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

		// 基础连接映射
		connMap: make(map[string]*types.Connection),

		// 隧道桥接
		tunnelBridges: make(map[string]*TunnelBridge),

		// 已关闭的 tunnel 跟踪
		closedTunnels: make(map[string]time.Time),

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

		// Manager 级组件使用 ManagerBase
		ManagerBase: dispose.NewManager("SessionManager", parentCtx),
	}

	// 添加资源清理回调
	session.AddCleanHandler(session.onClose)

	// 启动连接清理协程
	session.startConnectionCleanup()

	// 注意：跨服务器订阅将在SetBridgeManager()之后启动

	return session
}

// CloudControlAPI 定义CloudControl接口
// 已迁移到 core 子包，这里保留别名以保持向后兼容
type CloudControlAPI = core.CloudControlAPI

// ============================================================================
// Handler 设置、跨节点组件设置、组件访问器已移至 manager_ops.go
// ============================================================================

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
	controlConnCount := 0
	tunnelConnCount := 0
	if s.clientRegistry != nil {
		controlConnCount = s.clientRegistry.Count()
		s.clientRegistry.Close()
	}
	if s.tunnelRegistry != nil {
		tunnelConnCount = s.tunnelRegistry.Count()
		s.tunnelRegistry.Close()
	}

	// 关闭基础连接映射
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

	// 关闭跨节点连接监听器
	if s.crossNodeListener != nil {
		if err := s.crossNodeListener.Stop(); err != nil {
			corelog.Warnf("Failed to stop CrossNodeListener: %v", err)
		}
		corelog.Info("CrossNodeListener stopped")
	}

	// 关闭跨节点连接池
	if s.crossNodePool != nil {
		s.crossNodePool.Close()
		corelog.Info("CrossNodePool closed")
	}

	corelog.Info("Session manager resources cleanup completed")
	return nil
}
