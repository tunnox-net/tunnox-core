package server

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"fmt"

	"tunnox-core/internal/utils"
)

// ============================================================================
// ServerBuilder - 服务器构建器
// ============================================================================

// ServerBuilder 服务器构建器
// 使用 Builder 模式组装服务器，支持自定义组件组合
type ServerBuilder struct {
	config     *Config
	components []Component
	deps       *Dependencies
	parentCtx  context.Context
}

// NewServerBuilder 创建服务器构建器
func NewServerBuilder(config *Config) *ServerBuilder {
	return &ServerBuilder{
		config:     config,
		components: make([]Component, 0),
		deps:       &Dependencies{Config: config},
	}
}

// With 添加组件
func (b *ServerBuilder) With(c Component) *ServerBuilder {
	b.components = append(b.components, c)
	return b
}

// WithDefaults 添加默认组件（按依赖顺序）
// 这是生产环境使用的标准组件组合
func (b *ServerBuilder) WithDefaults() *ServerBuilder {
	return b.
		With(&StorageComponent{}).
		With(&MetricsComponent{}).
		With(&CloudControlComponent{}).
		With(&NodeComponent{}).
		With(&SessionComponent{}).
		With(&SecurityComponent{}).
		With(&HealthComponent{}).
		With(&HandlersComponent{}).
		With(&ProtocolComponent{}).
		With(&MessageBrokerComponent{}).
		With(&BridgeComponent{}).
		With(&ManagementAPIComponent{})
}

// Build 构建服务器
// 按顺序初始化所有组件，任何组件失败都会返回错误
func (b *ServerBuilder) Build(parentCtx context.Context) (*Server, error) {
	b.parentCtx = parentCtx

	// 初始化日志（在组件初始化之前）
	if err := utils.InitLogger(&b.config.Log); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// 按顺序初始化所有组件
	for _, c := range b.components {
		corelog.Debugf("Initializing component: %s", c.Name())

		if err := c.Initialize(parentCtx, b.deps); err != nil {
			return nil, NewComponentError(c.Name(), err)
		}

		corelog.Debugf("Component initialized: %s", c.Name())
	}

	// 创建服务器实例
	server := &Server{
		config:                b.config,
		storage:               b.deps.Storage,
		idManager:             b.deps.IDManager,
		cloudControl:          b.deps.CloudControl,
		cloudBuiltin:          b.deps.CloudBuiltin,
		nodeID:                b.deps.NodeID,
		nodeAllocator:         b.deps.NodeAllocator,
		session:               b.deps.SessionMgr,
		bruteForceProtector:   b.deps.BruteForceProtector,
		ipManager:             b.deps.IPManager,
		rateLimiter:           b.deps.RateLimiter,
		reconnectTokenManager: b.deps.ReconnectTokenManager,
		sessionTokenManager:   b.deps.SessionTokenManager,
		connCodeService:       b.deps.ConnCodeService,
		healthManager:         b.deps.HealthManager,
		protocolMgr:           b.deps.ProtocolMgr,
		protocolFactory:       b.deps.ProtocolFactory,
		messageBroker:         b.deps.MessageBroker,
		bridgeManager:         b.deps.BridgeManager,
		grpcServer:            b.deps.GRPCServer,
		apiServer:             b.deps.APIServer,
		authHandler:           b.deps.AuthHandler,
	}

	// 创建服务管理器并注册服务
	server.createServiceManager(parentCtx)
	server.registerServices()

	return server, nil
}

// BuildForTesting 构建测试用服务器
// 可以替换特定组件为 Mock 实现
func (b *ServerBuilder) BuildForTesting(parentCtx context.Context) (*Server, error) {
	// 测试时可以使用简化的组件组合
	return b.Build(parentCtx)
}
