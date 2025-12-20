package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"tunnox-core/api/proto/bridge"
	internalbridge "tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/httpservice/modules/domainproxy"
	"tunnox-core/internal/httpservice/modules/httppoll"
	"tunnox-core/internal/httpservice/modules/management"
	"tunnox-core/internal/protocol"

	"google.golang.org/grpc"
)

// ============================================================================
// ProtocolComponent - 协议组件
// ============================================================================

// ProtocolComponent 协议组件
type ProtocolComponent struct {
	*BaseComponent
}

func (c *ProtocolComponent) Name() string {
	return "Protocol"
}

func (c *ProtocolComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.SessionMgr == nil {
		return fmt.Errorf("SessionManager is required")
	}

	deps.ProtocolFactory = NewProtocolFactory(deps.SessionMgr)
	deps.ProtocolMgr = protocol.NewProtocolManager(ctx)

	corelog.Infof("Protocol components initialized")
	return nil
}

func (c *ProtocolComponent) Start() error {
	return nil
}

func (c *ProtocolComponent) Stop() error {
	return nil
}

// ============================================================================
// MessageBrokerComponent - 消息代理组件
// ============================================================================

// MessageBrokerComponent 消息代理组件
type MessageBrokerComponent struct {
	*BaseComponent
}

func (c *MessageBrokerComponent) Name() string {
	return "MessageBroker"
}

func (c *MessageBrokerComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	brokerConfig := &broker.BrokerConfig{
		Type:   broker.BrokerType(deps.Config.MessageBroker.Type),
		NodeID: deps.Config.MessageBroker.NodeID,
	}

	// 如果 NodeID 未配置，使用节点ID
	if brokerConfig.NodeID == "" {
		brokerConfig.NodeID = deps.NodeID
	}

	// 配置 Redis（如果使用 Redis）
	if brokerConfig.Type == broker.BrokerTypeRedis {
		redisConfig := &broker.RedisBrokerConfig{
			Addrs:       []string{deps.Config.MessageBroker.Redis.Addr},
			Password:    deps.Config.MessageBroker.Redis.Password,
			DB:          deps.Config.MessageBroker.Redis.DB,
			ClusterMode: deps.Config.MessageBroker.Redis.ClusterMode,
			PoolSize:    deps.Config.MessageBroker.Redis.PoolSize,
		}

		if redisConfig.PoolSize <= 0 {
			redisConfig.PoolSize = 10
		}

		brokerConfig.Redis = redisConfig
	}

	mb, err := broker.NewMessageBroker(ctx, brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to create message broker: %w", err)
	}

	deps.MessageBroker = mb

	// 注入 BridgeAdapter 到 SessionManager
	if deps.SessionMgr != nil && mb != nil {
		bridgeAdapter := NewBridgeAdapter(ctx, mb, deps.Config.MessageBroker.NodeID)
		deps.SessionMgr.SetBridgeManager(bridgeAdapter)
	}

	corelog.Infof("MessageBroker initialized: type=%s", brokerConfig.Type)
	return nil
}

func (c *MessageBrokerComponent) Start() error {
	return nil
}

func (c *MessageBrokerComponent) Stop() error {
	return nil
}

// ============================================================================
// BridgeComponent - 桥接组件
// ============================================================================

// BridgeComponent 桥接组件
type BridgeComponent struct {
	*BaseComponent
	grpcServer *grpc.Server
}

func (c *BridgeComponent) Name() string {
	return "Bridge"
}

func (c *BridgeComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	// 如果未启用桥接池，跳过
	if !deps.Config.BridgePool.Enabled {
		corelog.Debugf("BridgePool not enabled, skipping")
		return nil
	}

	if deps.MessageBroker == nil {
		corelog.Warnf("MessageBroker not initialized, BridgeManager will not be created")
		return nil
	}

	// 创建桥接管理器
	poolConfig := &internalbridge.PoolConfig{
		MinConnsPerNode:     deps.Config.BridgePool.MinConnsPerNode,
		MaxConnsPerNode:     deps.Config.BridgePool.MaxConnsPerNode,
		MaxIdleTime:         time.Duration(deps.Config.BridgePool.MaxIdleTime) * time.Second,
		MaxStreamsPerConn:   deps.Config.BridgePool.MaxStreamsPerConn,
		DialTimeout:         time.Duration(deps.Config.BridgePool.DialTimeout) * time.Second,
		HealthCheckInterval: time.Duration(deps.Config.BridgePool.HealthCheckInterval) * time.Second,
	}

	nodeRegistry := NewSimpleNodeRegistry()

	managerConfig := &internalbridge.BridgeManagerConfig{
		NodeID:         deps.Config.MessageBroker.NodeID,
		PoolConfig:     poolConfig,
		MessageBroker:  deps.MessageBroker,
		NodeRegistry:   nodeRegistry,
		RequestTimeout: 30 * time.Second,
	}

	manager, err := internalbridge.NewBridgeManager(ctx, managerConfig)
	if err != nil {
		return fmt.Errorf("failed to create bridge manager: %w", err)
	}

	deps.BridgeManager = manager

	// 启动 gRPC 服务器
	grpcServer, err := c.startGRPCServer(ctx, deps)
	if err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	deps.GRPCServer = grpcServer
	c.grpcServer = grpcServer

	corelog.Infof("BridgeManager initialized")
	return nil
}

func (c *BridgeComponent) startGRPCServer(ctx context.Context, deps *Dependencies) (*grpc.Server, error) {
	grpcServerConfig := deps.Config.BridgePool.GRPCServer

	if grpcServerConfig.Port == 0 {
		corelog.Warnf("gRPC server port not configured, skipping gRPC server startup")
		return nil, nil
	}

	port := grpcServerConfig.Port
	host := grpcServerConfig.Addr
	if host == "" {
		host = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	bridgeServer := internalbridge.NewGRPCBridgeServer(ctx, deps.Config.MessageBroker.NodeID, deps.BridgeManager)
	bridge.RegisterBridgeServiceServer(grpcServer, bridgeServer)

	// 在后台启动 gRPC 服务器
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			corelog.Errorf("gRPC server error: %v", err)
		}
	}()

	corelog.Infof("gRPC server started on %s", addr)
	return grpcServer, nil
}

func (c *BridgeComponent) Start() error {
	return nil
}

func (c *BridgeComponent) Stop() error {
	if c.grpcServer != nil {
		c.grpcServer.GracefulStop()
	}
	return nil
}

// ============================================================================
// HTTPServiceComponent - HTTP 服务组件（替代旧的 ManagementAPIComponent）
// ============================================================================

// HTTPServiceComponent HTTP 服务组件
type HTTPServiceComponent struct {
	*BaseComponent
}

func (c *HTTPServiceComponent) Name() string {
	return "HTTPService"
}

func (c *HTTPServiceComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if !deps.Config.ManagementAPI.Enabled {
		corelog.Debugf("HTTPService not enabled, skipping")
		return nil
	}

	// 构建 HTTP 服务配置
	httpConfig := &httpservice.HTTPServiceConfig{
		Enabled:    deps.Config.ManagementAPI.Enabled,
		ListenAddr: deps.Config.ManagementAPI.ListenAddr,
		CORS: httpservice.CORSConfig{
			Enabled:        deps.Config.ManagementAPI.CORS.Enabled,
			AllowedOrigins: deps.Config.ManagementAPI.CORS.AllowedOrigins,
		},
		RateLimit: httpservice.RateLimitConfig{
			Enabled: deps.Config.ManagementAPI.RateLimit.Enabled,
		},
		Modules: httpservice.ModulesConfig{
			ManagementAPI: httpservice.ManagementAPIModuleConfig{
				Enabled: true,
				Auth: httpservice.AuthConfig{
					Type:   deps.Config.ManagementAPI.Auth.Type,
					Secret: deps.Config.ManagementAPI.Auth.Token,
				},
				PProf: httpservice.PProfConfig{
					Enabled:     deps.Config.ManagementAPI.PProf.Enabled,
					DataDir:     deps.Config.ManagementAPI.PProf.DataDir,
					Retention:   deps.Config.ManagementAPI.PProf.Retention,
					AutoCapture: deps.Config.ManagementAPI.PProf.AutoCapture,
				},
			},
			HTTPPoll: httpservice.HTTPPollModuleConfig{
				Enabled:        true,
				MaxRequestSize: 1048576, // 1MB
				DefaultTimeout: 30,
				MaxTimeout:     60,
			},
			DomainProxy: httpservice.DomainProxyModuleConfig{
				Enabled:              true,
				BaseDomains:          []string{},
				DefaultScheme:        "http",
				CommandModeThreshold: 1048576, // 1MB
				RequestTimeout:       30 * time.Second,
			},
		},
	}

	// 创建 HTTP 服务
	httpSvc := httpservice.NewHTTPService(ctx, httpConfig, deps.CloudControl, deps.Storage, deps.HealthManager)

	// 创建并注册 Management API 模块
	mgmtConfig := &httpConfig.Modules.ManagementAPI
	mgmtModule := management.NewManagementModule(ctx, mgmtConfig, deps.CloudControl, deps.ConnCodeService, deps.HealthManager)
	httpSvc.RegisterModule(mgmtModule)

	// 创建并注册 HTTPPoll 模块
	httpPollConfig := &httpConfig.Modules.HTTPPoll
	httpPollModule := httppoll.NewHTTPPollModule(ctx, httpPollConfig)
	httpSvc.RegisterModule(httpPollModule)

	// 创建并注册 Domain Proxy 模块
	domainProxyConfig := &httpConfig.Modules.DomainProxy
	domainProxyModule := domainproxy.NewDomainProxyModule(ctx, domainProxyConfig)
	httpSvc.RegisterModule(domainProxyModule)

	// 设置会话管理器（如果可用）
	if deps.SessionMgr != nil {
		httpSvc.SetSessionManager(NewSessionManagerAdapter(deps.SessionMgr))
	}

	deps.HTTPService = httpSvc

	corelog.Infof("HTTPService initialized: addr=%s", httpConfig.ListenAddr)
	return nil
}

func (c *HTTPServiceComponent) Start() error {
	return nil
}

func (c *HTTPServiceComponent) Stop() error {
	return nil
}
