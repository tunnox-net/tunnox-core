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
	"tunnox-core/internal/httpservice/modules/management"
	"tunnox-core/internal/httpservice/modules/websocket"
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
	// 根据配置决定使用哪种消息代理
	brokerType := "memory"
	var redisConfig *broker.RedisBrokerConfig

	if deps.Config.Redis.Enabled {
		brokerType = "redis"
		redisConfig = &broker.RedisBrokerConfig{
			Addrs:       []string{deps.Config.Redis.Addr},
			Password:    deps.Config.Redis.Password,
			DB:          deps.Config.Redis.DB,
			ClusterMode: false,
			PoolSize:    10,
		}
	}

	brokerConfig := &broker.BrokerConfig{
		Type:   broker.BrokerType(brokerType),
		NodeID: deps.NodeID,
		Redis:  redisConfig,
	}

	mb, err := broker.NewMessageBroker(ctx, brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to create message broker: %w", err)
	}

	deps.MessageBroker = mb

	// 注入 BridgeAdapter 到 SessionManager
	if deps.SessionMgr != nil && mb != nil {
		bridgeAdapter := NewBridgeAdapter(ctx, mb, deps.NodeID)
		deps.SessionMgr.SetBridgeManager(bridgeAdapter)
		deps.BridgeAdapter = bridgeAdapter // 保存到 deps 供后续组件使用
	}

	corelog.Infof("MessageBroker initialized: type=%s", brokerType)
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
	// Bridge Manager 只在集群模式下需要（即启用了 Redis）
	if !deps.Config.Redis.Enabled {
		corelog.Debug("Redis not enabled, BridgeManager will not be created")
		return nil
	}

	if deps.MessageBroker == nil {
		corelog.Warnf("MessageBroker not initialized, BridgeManager will not be created")
		return nil
	}

	// 使用默认的桥接池配置
	poolConfig := &internalbridge.PoolConfig{
		MinConnsPerNode:     2,
		MaxConnsPerNode:     10,
		MaxIdleTime:         300 * time.Second,
		MaxStreamsPerConn:   100,
		DialTimeout:         10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}

	nodeRegistry := NewSimpleNodeRegistry()

	managerConfig := &internalbridge.BridgeManagerConfig{
		NodeID:         deps.NodeID,
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

	// 将 BridgeManager 设置到 BridgeAdapter（用于跨节点转发）
	if deps.BridgeAdapter != nil {
		deps.BridgeAdapter.SetBridgeManager(manager)
	}

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
	// 使用默认端口 50051
	port := 50051
	host := "0.0.0.0"
	addr := fmt.Sprintf("%s:%d", host, port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	bridgeServer := internalbridge.NewGRPCBridgeServer(ctx, deps.NodeID, deps.BridgeManager)
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
	// HTTP 服务始终启用

	// 检查协议是否在 server.protocols 中启用
	websocketEnabled := false
	if wsConfig, exists := deps.Config.Server.Protocols["websocket"]; exists {
		websocketEnabled = wsConfig.Enabled
	}

	// 构建 HTTP 服务配置
	httpConfig := &httpservice.HTTPServiceConfig{
		Enabled:    true,
		ListenAddr: deps.Config.Management.Listen,
		CORS: httpservice.CORSConfig{
			Enabled:        false,
			AllowedOrigins: []string{},
		},
		RateLimit: httpservice.RateLimitConfig{
			Enabled: false,
		},
		Modules: httpservice.ModulesConfig{
			ManagementAPI: httpservice.ManagementAPIModuleConfig{
				Enabled: true,
				Auth: httpservice.AuthConfig{
					Type:   deps.Config.Management.Auth.Type,
					Secret: deps.Config.Management.Auth.Token,
				},
				PProf: httpservice.PProfConfig{
					Enabled:     deps.Config.Management.PProf.Enabled,
					DataDir:     deps.Config.Management.PProf.DataDir,
					Retention:   deps.Config.Management.PProf.Retention,
					AutoCapture: deps.Config.Management.PProf.AutoCapture,
				},
			},
			WebSocket: httpservice.WebSocketModuleConfig{
				Enabled: websocketEnabled,
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

	// 创建并注册 WebSocket 模块（如果启用）
	if websocketEnabled {
		wsConfig := &httpConfig.Modules.WebSocket
		wsModule := websocket.NewWebSocketModule(ctx, wsConfig)
		if deps.SessionMgr != nil {
			wsModule.SetSession(deps.SessionMgr)
		}
		httpSvc.RegisterModule(wsModule)
	}

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
