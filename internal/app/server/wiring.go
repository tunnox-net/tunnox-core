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
	"tunnox-core/internal/stream"

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

	// 注册 HTTP 服务（如果已初始化）
	if s.httpService != nil {
		httpServiceAdapter := NewHTTPServiceAdapter("HTTP-Service", s.httpService)
		s.serviceManager.RegisterService(httpServiceAdapter)
	}

}

// createMessageBroker 创建消息代理
func (s *Server) createMessageBroker(ctx context.Context) broker.MessageBroker {
	// 根据配置决定使用哪种消息代理
	brokerType := "memory"
	var redisConfig *broker.RedisBrokerConfig

	if s.config.Redis.Enabled {
		brokerType = "redis"
		redisConfig = &broker.RedisBrokerConfig{
			Addrs:       []string{s.config.Redis.Addr},
			Password:    s.config.Redis.Password,
			DB:          s.config.Redis.DB,
			ClusterMode: false,
			PoolSize:    10,
		}
	}

	brokerConfig := &broker.BrokerConfig{
		Type:   broker.BrokerType(brokerType),
		NodeID: s.nodeID,
		Redis:  redisConfig,
	}

	mb, err := broker.NewMessageBroker(ctx, brokerConfig)
	if err != nil {
		corelog.Fatalf("Failed to create message broker: %v", err)
	}

	return mb
}

// createBridgeManager 创建桥接管理器
func (s *Server) createBridgeManager(ctx context.Context) *internalbridge.BridgeManager {
	// Bridge Manager 只在集群模式下需要（即启用了 Redis）
	if !s.config.Redis.Enabled {
		corelog.Debug("Redis not enabled, BridgeManager will not be created")
		return nil
	}

	if s.messageBroker == nil {
		corelog.Warn("MessageBroker not initialized, BridgeManager will not be created")
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

	// 创建简单的节点注册表（实际应该从 Storage 或 Cloud 获取）
	nodeRegistry := NewSimpleNodeRegistry()

	managerConfig := &internalbridge.BridgeManagerConfig{
		NodeID:         s.nodeID,
		PoolConfig:     poolConfig,
		MessageBroker:  s.messageBroker,
		NodeRegistry:   nodeRegistry,
		RequestTimeout: 30 * time.Second,
	}

	manager, err := internalbridge.NewBridgeManager(ctx, managerConfig)
	if err != nil {
		corelog.Fatalf("Failed to create bridge manager: %v", err)
	}

	return manager
}

// startGRPCServer 启动 gRPC 服务器
func (s *Server) startGRPCServer() *grpc.Server {
	// gRPC 服务器只在集群模式下需要（即启用了 Redis）
	if !s.config.Redis.Enabled {
		corelog.Debug("Redis not enabled, skipping gRPC server startup")
		return nil
	}

	if s.bridgeManager == nil {
		corelog.Warn("BridgeManager not initialized, skipping gRPC server startup")
		return nil
	}

	// 使用默认端口 50051
	port := 50051
	host := "0.0.0.0"
	addr := fmt.Sprintf("%s:%d", host, port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		corelog.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	grpcServer := grpc.NewServer()
	// 使用 serviceManager 的 context 作为父 context，确保能接收退出信号
	bridgeServer := internalbridge.NewGRPCBridgeServer(s.serviceManager.GetContext(), s.nodeID, s.bridgeManager)
	bridge.RegisterBridgeServiceServer(grpcServer, bridgeServer)

	// 在后台启动 gRPC 服务器
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			corelog.Errorf("gRPC server error: %v", err)
		}
	}()

	return grpcServer
}

// setupProtocolAdapters 设置协议适配器
func (s *Server) setupProtocolAdapters() error {
	// 获取启用的协议配置
	enabledProtocols := s.getEnabledProtocols()
	if len(enabledProtocols) == 0 {
		corelog.Warn("No protocols enabled")
		return nil
	}

	// 创建并注册所有启用的协议适配器
	registeredProtocols := make([]string, 0, len(enabledProtocols))

	for protocolName, config := range enabledProtocols {
		// 跳过通过 HTTP 服务提供的协议（它们不需要独立的协议适配器）
		// 或已废弃的协议
		if protocolName == "websocket" || protocolName == "httppoll" {
			corelog.Debugf("Skipping %s protocol adapter (provided by HTTP service or deprecated)", protocolName)
			continue
		}

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

// 创建统一 HTTP 服务
func (s *Server) createHTTPService(ctx context.Context) *httpservice.HTTPService {
	// 检查协议是否在 server.protocols 中启用
	websocketEnabled := false
	if wsConfig, exists := s.config.Server.Protocols["websocket"]; exists {
		websocketEnabled = wsConfig.Enabled
	}

	// 构建 HTTP 服务配置
	httpConfig := &httpservice.HTTPServiceConfig{
		Enabled:    true, // HTTP 服务始终启用
		ListenAddr: s.config.Management.Listen,
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
					Type:   s.config.Management.Auth.Type,
					Secret: s.config.Management.Auth.Token,
				},
				PProf: httpservice.PProfConfig{
					Enabled:     s.config.Management.PProf.Enabled,
					DataDir:     s.config.Management.PProf.DataDir,
					Retention:   s.config.Management.PProf.Retention,
					AutoCapture: s.config.Management.PProf.AutoCapture,
				},
			},
			WebSocket: httpservice.WebSocketModuleConfig{
				Enabled: websocketEnabled,
			},
			DomainProxy: httpservice.DomainProxyModuleConfig{
				Enabled:              true,
				BaseDomains:          []string{}, // 可从配置读取
				DefaultScheme:        "http",
				CommandModeThreshold: 1048576, // 1MB
				RequestTimeout:       30 * time.Second,
			},
		},
	}

	// 创建 HTTP 服务
	httpSvc := httpservice.NewHTTPService(ctx, httpConfig, s.cloudControl, s.storage, s.healthManager)

	// 创建并注册 Management API 模块
	mgmtConfig := &httpConfig.Modules.ManagementAPI
	mgmtModule := management.NewManagementModule(ctx, mgmtConfig, s.cloudControl, s.connCodeService, s.healthManager)
	httpSvc.RegisterModule(mgmtModule)

	// 创建并注册 WebSocket 模块（如果启用）
	if websocketEnabled {
		wsConfig := &httpConfig.Modules.WebSocket
		wsModule := websocket.NewWebSocketModule(ctx, wsConfig)
		if s.session != nil {
			wsModule.SetSession(s.session)
		}
		httpSvc.RegisterModule(wsModule)
	}

	// 创建并注册 Domain Proxy 模块
	domainProxyConfig := &httpConfig.Modules.DomainProxy
	domainProxyModule := domainproxy.NewDomainProxyModule(ctx, domainProxyConfig)
	httpSvc.RegisterModule(domainProxyModule)

	// 设置会话管理器（如果可用）
	if s.session != nil {
		httpSvc.SetSessionManager(NewSessionManagerAdapter(s.session))
	}

	return httpSvc
}
