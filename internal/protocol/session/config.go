package session

import (
	"context"
	"errors"
	"time"

	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/security"
)

// ============================================================================
// SessionManagerConfig - 必需依赖配置
// ============================================================================

// SessionManagerConfig SessionManager 必需依赖配置
// 所有必需依赖通过此结构体传入，避免遗漏
type SessionManagerConfig struct {
	// IDManager ID管理器（必需）
	IDManager *idgen.IDManager

	// Logger 日志接口（可选，默认使用全局 Logger）
	Logger corelog.Logger

	// HeartbeatTimeout 心跳超时时间（可选，默认60秒）
	HeartbeatTimeout time.Duration

	// CleanupInterval 清理检查间隔（可选，默认15秒）
	CleanupInterval time.Duration

	// MaxConnections 最大连接数限制（可选，默认10000）
	MaxConnections int

	// MaxControlConnections 最大控制连接数限制（可选，默认5000）
	MaxControlConnections int
}

// Validate 验证配置
func (c *SessionManagerConfig) Validate() error {
	if c.IDManager == nil {
		return errors.New("IDManager is required")
	}
	return nil
}

// ApplyDefaults 应用默认值
func (c *SessionManagerConfig) ApplyDefaults() {
	if c.Logger == nil {
		c.Logger = corelog.Default()
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = 60 * time.Second
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = 15 * time.Second
	}
	if c.MaxConnections <= 0 {
		c.MaxConnections = 10000
	}
	if c.MaxControlConnections <= 0 {
		c.MaxControlConnections = 5000
	}
}

// ============================================================================
// SessionManagerOption - 可选依赖（函数式选项）
// ============================================================================

// SessionManagerOption SessionManager 可选配置函数
type SessionManagerOption func(*SessionManager)

// WithAuthHandler 设置认证处理器
func WithAuthHandler(handler AuthHandler) SessionManagerOption {
	return func(s *SessionManager) {
		s.authHandler = handler
	}
}

// WithTunnelHandler 设置隧道处理器
func WithTunnelHandler(handler TunnelHandler) SessionManagerOption {
	return func(s *SessionManager) {
		s.tunnelHandler = handler
	}
}

// WithCloudControl 设置云控制接口
func WithCloudControl(cc CloudControlAPI) SessionManagerOption {
	return func(s *SessionManager) {
		s.cloudControl = cc
	}
}

// WithBridgeManager 设置桥接管理器
func WithBridgeManager(bm BridgeManager) SessionManagerOption {
	return func(s *SessionManager) {
		s.bridgeManager = bm
		// 启动跨节点广播订阅
		s.startTunnelOpenBroadcastSubscription()
		s.startConfigPushBroadcastSubscription()
	}
}

// WithNodeID 设置节点ID
func WithNodeID(nodeID string) SessionManagerOption {
	return func(s *SessionManager) {
		s.nodeID = nodeID
	}
}

// WithTunnelRoutingTable 设置隧道路由表
func WithTunnelRoutingTable(rt *TunnelRoutingTable) SessionManagerOption {
	return func(s *SessionManager) {
		s.tunnelRouting = rt
	}
}

// WithReconnectTokenManager 设置重连Token管理器
func WithReconnectTokenManager(m *security.ReconnectTokenManager) SessionManagerOption {
	return func(s *SessionManager) {
		s.reconnectTokenManager = m
	}
}

// WithSessionTokenManager 设置会话Token管理器
func WithSessionTokenManager(m *security.SessionTokenManager) SessionManagerOption {
	return func(s *SessionManager) {
		s.sessionTokenManager = m
	}
}

// WithTunnelStateManager 设置隧道状态管理器
func WithTunnelStateManager(m *TunnelStateManager) SessionManagerOption {
	return func(s *SessionManager) {
		s.tunnelStateManager = m
	}
}

// WithMigrationManager 设置迁移管理器
func WithMigrationManager(m *TunnelMigrationManager) SessionManagerOption {
	return func(s *SessionManager) {
		s.migrationManager = m
	}
}

// WithEventBus 设置事件总线
func WithEventBus(eb events.EventBus) SessionManagerOption {
	return func(s *SessionManager) {
		s.eventBus = eb
		// 订阅断开连接事件
		if eb != nil {
			_ = eb.Subscribe("DisconnectRequest", s.handleDisconnectRequestEvent)
		}
	}
}

// WithCommandRegistry 设置命令注册表
func WithCommandRegistry(cr types.CommandRegistry) SessionManagerOption {
	return func(s *SessionManager) {
		s.commandRegistry = cr
	}
}

// WithCommandExecutor 设置命令执行器
func WithCommandExecutor(ce types.CommandExecutor) SessionManagerOption {
	return func(s *SessionManager) {
		s.commandExecutor = ce
	}
}

// ============================================================================
// NewSessionManagerV2 - 新版构造函数（推荐使用）
// ============================================================================

// NewSessionManagerV2 创建 SessionManager（推荐使用）
// 使用 Config + Options 模式，确保必需依赖不会遗漏
//
// 示例:
//
//	sm, err := NewSessionManagerV2(ctx, &SessionManagerConfig{
//	    IDManager: idManager,
//	},
//	    WithAuthHandler(authHandler),
//	    WithTunnelHandler(tunnelHandler),
//	    WithNodeID("node-1"),
//	)
func NewSessionManagerV2(
	ctx context.Context,
	config *SessionManagerConfig,
	opts ...SessionManagerOption,
) (*SessionManager, error) {
	// 验证配置
	if config == nil {
		return nil, errors.New("config is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 应用默认值
	config.ApplyDefaults()

	// 转换为旧版配置（复用现有逻辑）
	sessionConfig := &SessionConfig{
		HeartbeatTimeout:      config.HeartbeatTimeout,
		CleanupInterval:       config.CleanupInterval,
		MaxConnections:        config.MaxConnections,
		MaxControlConnections: config.MaxControlConnections,
	}

	// 创建 SessionManager
	sm := NewSessionManagerWithConfig(config.IDManager, ctx, sessionConfig)

	// 设置 Logger
	sm.logger = config.Logger

	// 应用可选配置
	for _, opt := range opts {
		opt(sm)
	}

	return sm, nil
}
