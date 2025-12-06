package server

import (
	"context"
	"fmt"
	"net"
	"time"
	"tunnox-core/api/proto/bridge"
	"tunnox-core/internal/api"
	internalbridge "tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"google.golang.org/grpc"
)

// registerServices 注册所有服务到服务管理器
func (s *Server) registerServices() {
	// 注册云控制服务
	cloudService := NewCloudService("Cloud-Control", s.cloudControl)
	s.serviceManager.RegisterService(cloudService)

	// 注册存储服务
	storageService := NewStorageService("Storage", s.storage)
	s.serviceManager.RegisterService(storageService)

	// 注册协议管理服务
	protocolService := protocol.NewProtocolService("Protocol-Manager", s.protocolMgr)
	s.serviceManager.RegisterService(protocolService)

	// 注册流管理服务
	streamFactory := stream.NewDefaultStreamFactory(s.serviceManager.GetContext())
	streamManager := stream.NewStreamManager(streamFactory, s.serviceManager.GetContext())
	streamService := stream.NewStreamService("Stream-Manager", streamManager)
	s.serviceManager.RegisterService(streamService)

	// 注册 MessageBroker 服务（如果已初始化）
	if s.messageBroker != nil {
		brokerService := NewBrokerService("Message-Broker", s.messageBroker)
		s.serviceManager.RegisterService(brokerService)
	}

	// 注册 BridgeManager 服务（如果已初始化）
	if s.bridgeManager != nil {
		bridgeService := NewBridgeService("Bridge-Manager", s.bridgeManager)
		s.serviceManager.RegisterService(bridgeService)
	}

	// 注册 Management API 服务（如果已初始化）
	if s.apiServer != nil {
		apiService := NewManagementAPIService("Management-API", s.apiServer)
		s.serviceManager.RegisterService(apiService)
	}

}

// createMessageBroker 创建消息代理
func (s *Server) createMessageBroker(ctx context.Context) broker.MessageBroker {
	brokerConfig := &broker.BrokerConfig{
		Type:   broker.BrokerType(s.config.MessageBroker.Type),
		NodeID: s.config.MessageBroker.NodeID,
	}

	// 如果 NodeID 未配置，使用服务器ID
	if brokerConfig.NodeID == "" {
		brokerConfig.NodeID = s.serverID
	}

	// 配置 Redis（如果使用 Redis）
	if brokerConfig.Type == broker.BrokerTypeRedis {
		redisConfig := &broker.RedisBrokerConfig{
			Addrs:       []string{s.config.MessageBroker.Redis.Addr},
			Password:    s.config.MessageBroker.Redis.Password,
			DB:          s.config.MessageBroker.Redis.DB,
			ClusterMode: s.config.MessageBroker.Redis.ClusterMode,
			PoolSize:    s.config.MessageBroker.Redis.PoolSize,
		}

		// 设置默认值
		if redisConfig.PoolSize <= 0 {
			redisConfig.PoolSize = 10
		}

		brokerConfig.Redis = redisConfig
	}

	mb, err := broker.NewMessageBroker(ctx, brokerConfig)
	if err != nil {
		utils.Fatalf("Failed to create message broker: %v", err)
	}

	return mb
}

// createBridgeManager 创建桥接管理器
func (s *Server) createBridgeManager(ctx context.Context) *internalbridge.BridgeManager {
	if s.messageBroker == nil {
		utils.Warn("MessageBroker not initialized, BridgeManager will not be created")
		return nil
	}

	poolConfig := &internalbridge.PoolConfig{
		MinConnsPerNode:     s.config.BridgePool.MinConnsPerNode,
		MaxConnsPerNode:     s.config.BridgePool.MaxConnsPerNode,
		MaxIdleTime:         time.Duration(s.config.BridgePool.MaxIdleTime) * time.Second,
		MaxStreamsPerConn:   s.config.BridgePool.MaxStreamsPerConn,
		DialTimeout:         time.Duration(s.config.BridgePool.DialTimeout) * time.Second,
		HealthCheckInterval: time.Duration(s.config.BridgePool.HealthCheckInterval) * time.Second,
	}

	// 创建简单的节点注册表（实际应该从 Storage 或 Cloud 获取）
	nodeRegistry := NewSimpleNodeRegistry()

	managerConfig := &internalbridge.BridgeManagerConfig{
		NodeID:         s.config.MessageBroker.NodeID,
		PoolConfig:     poolConfig,
		MessageBroker:  s.messageBroker,
		NodeRegistry:   nodeRegistry,
		RequestTimeout: 30 * time.Second,
	}

	manager, err := internalbridge.NewBridgeManager(ctx, managerConfig)
	if err != nil {
		utils.Fatalf("Failed to create bridge manager: %v", err)
	}

	return manager
}

// startGRPCServer 启动 gRPC 服务器
func (s *Server) startGRPCServer() *grpc.Server {
	// 从配置中获取 gRPC 服务器地址
	grpcServerConfig := s.config.BridgePool.GRPCServer

	// 检查端口是否配置（如果未配置则不启动）
	if grpcServerConfig.Port == 0 {
		utils.Warn("gRPC server port not configured, skipping gRPC server startup")
		return nil
	}

	port := grpcServerConfig.Port
	host := grpcServerConfig.Addr
	if host == "" {
		host = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		utils.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	grpcServer := grpc.NewServer()
	// 使用 serviceManager 的 context 作为父 context，确保能接收退出信号
	bridgeServer := internalbridge.NewGRPCBridgeServer(s.serviceManager.GetContext(), s.config.MessageBroker.NodeID, s.bridgeManager)
	bridge.RegisterBridgeServiceServer(grpcServer, bridgeServer)

	// 在后台启动 gRPC 服务器
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			utils.Errorf("gRPC server error: %v", err)
		}
	}()

	return grpcServer
}

// setupProtocolAdapters 设置协议适配器
func (s *Server) setupProtocolAdapters() error {
	// 获取启用的协议配置
	enabledProtocols := s.getEnabledProtocols()
	if len(enabledProtocols) == 0 {
		utils.Warn("No protocols enabled")
		return nil
	}

	// 创建并注册所有启用的协议适配器
	registeredProtocols := make([]string, 0, len(enabledProtocols))

	for protocolName, config := range enabledProtocols {
		// 创建适配器
		adapter, err := s.protocolFactory.CreateAdapter(protocolName, s.serviceManager.GetContext())
		if err != nil {
			return fmt.Errorf("failed to create %s adapter: %v", protocolName, err)
		}

		// 配置监听地址
		addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
		adapter.SetAddr(addr)

		// 注册到管理器
		s.protocolMgr.Register(adapter)
		registeredProtocols = append(registeredProtocols, protocolName)
	}

	return nil
}

// getEnabledProtocols 获取启用的协议配置
func (s *Server) getEnabledProtocols() map[string]ProtocolConfig {
	enabled := make(map[string]ProtocolConfig)

	for name, config := range s.config.Server.Protocols {
		if config.Enabled {
			enabled[name] = config
		}
	}

	return enabled
}

// createManagementAPI 创建 Management API 服务器
func (s *Server) createManagementAPI(ctx context.Context) *api.ManagementAPIServer {
	// 使用强类型配置
	apiConfig := &api.APIConfig{
		Enabled:    s.config.ManagementAPI.Enabled,
		ListenAddr: s.config.ManagementAPI.ListenAddr,
		Auth: api.AuthConfig{
			Type:   s.config.ManagementAPI.Auth.Type,
			Secret: s.config.ManagementAPI.Auth.Token, // Token 映射到 Secret
		},
		CORS: api.CORSConfig{
			Enabled:        s.config.ManagementAPI.CORS.Enabled,
			AllowedOrigins: s.config.ManagementAPI.CORS.AllowedOrigins,
		},
		RateLimit: api.RateLimitConfig{
			Enabled: s.config.ManagementAPI.RateLimit.Enabled,
		},
		PProf: api.PProfConfig{
			Enabled:     s.config.ManagementAPI.PProf.Enabled,
			DataDir:     s.config.ManagementAPI.PProf.DataDir,
			Retention:   s.config.ManagementAPI.PProf.Retention,
			AutoCapture: s.config.ManagementAPI.PProf.AutoCapture,
		},
	}

	apiServer := api.NewManagementAPIServer(ctx, apiConfig, s.cloudControl, s.connCodeService, s.healthManager)
	return apiServer
}
