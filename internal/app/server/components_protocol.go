package server

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"fmt"
	"net"
	"time"

	"tunnox-core/api/proto/bridge"
	"tunnox-core/internal/api"
	internalbridge "tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/health"
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
// ManagementAPIComponent - 管理 API 组件
// ============================================================================

// ManagementAPIComponent 管理 API 组件
type ManagementAPIComponent struct {
	*BaseComponent
}

func (c *ManagementAPIComponent) Name() string {
	return "ManagementAPI"
}

func (c *ManagementAPIComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if !deps.Config.ManagementAPI.Enabled {
		corelog.Debugf("ManagementAPI not enabled, skipping")
		return nil
	}

	// 使用强类型配置
	apiConfig := &api.APIConfig{
		Enabled:    deps.Config.ManagementAPI.Enabled,
		ListenAddr: deps.Config.ManagementAPI.ListenAddr,
		Auth: api.AuthConfig{
			Type:   deps.Config.ManagementAPI.Auth.Type,
			Secret: deps.Config.ManagementAPI.Auth.Token,
		},
		CORS: api.CORSConfig{
			Enabled:        deps.Config.ManagementAPI.CORS.Enabled,
			AllowedOrigins: deps.Config.ManagementAPI.CORS.AllowedOrigins,
		},
		RateLimit: api.RateLimitConfig{
			Enabled: deps.Config.ManagementAPI.RateLimit.Enabled,
		},
		PProf: api.PProfConfig{
			Enabled:     deps.Config.ManagementAPI.PProf.Enabled,
			DataDir:     deps.Config.ManagementAPI.PProf.DataDir,
			Retention:   deps.Config.ManagementAPI.PProf.Retention,
			AutoCapture: deps.Config.ManagementAPI.PProf.AutoCapture,
		},
	}

	apiServer := api.NewManagementAPIServer(ctx, apiConfig, deps.CloudControl, deps.ConnCodeService, deps.HealthManager)

	// 设置 SessionManager
	if deps.SessionMgr != nil {
		apiServer.SetSessionManager(api.AdaptSessionManager(deps.SessionMgr))
	}

	// 注册健康检查器
	c.setupHealthCheckers(apiServer, deps)

	deps.APIServer = apiServer

	corelog.Infof("ManagementAPI initialized: addr=%s", apiConfig.ListenAddr)
	return nil
}

func (c *ManagementAPIComponent) setupHealthCheckers(apiServer *api.ManagementAPIServer, deps *Dependencies) {
	var storageChecker health.StorageChecker
	var brokerChecker health.BrokerChecker
	var sessionManagerChecker health.SessionManagerChecker

	// 注册存储检查器
	if deps.Storage != nil {
		storageChecker = health.NewStorageAdapter(deps.Storage)
	}

	// 注册消息代理检查器
	if deps.MessageBroker != nil {
		if pingBroker, ok := deps.MessageBroker.(interface {
			Ping(ctx context.Context) error
		}); ok {
			brokerChecker = &brokerPingAdapter{broker: pingBroker}
		}
	}

	// 注册协议子系统检查器
	if deps.SessionMgr != nil {
		sessionManagerChecker = deps.SessionMgr
	}

	apiServer.SetHealthCheckers(storageChecker, brokerChecker, sessionManagerChecker)
}

func (c *ManagementAPIComponent) Start() error {
	return nil
}

func (c *ManagementAPIComponent) Stop() error {
	return nil
}
