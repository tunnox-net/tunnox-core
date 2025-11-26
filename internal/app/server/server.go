package server

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/api"
	internalbridge "tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"

	"google.golang.org/grpc"
)

// Server 服务器结构
type Server struct {
	config          *Config
	serviceManager  *utils.ServiceManager
	protocolMgr     *protocol.ProtocolManager
	serverID        string
	storage         storage.Storage
	idManager       *idgen.IDManager
	session         *session.SessionManager
	protocolFactory *ProtocolFactory
	cloudControl    managers.CloudControlAPI
	cloudBuiltin    *managers.BuiltinCloudControl
	messageBroker   broker.MessageBroker
	bridgeManager   *internalbridge.BridgeManager
	grpcServer      *grpc.Server
	apiServer       *api.ManagementAPIServer
}

// New 创建新服务器
func New(config *Config, parentCtx context.Context) *Server {
	// 初始化日志
	if err := utils.InitLogger(&config.Log); err != nil {
		utils.Fatalf("Failed to initialize logger: %v", err)
	}

	// 创建服务管理器
	serviceConfig := utils.DefaultServiceConfig()
	serviceConfig.EnableSignalHandling = true
	serviceConfig.GracefulShutdownTimeout = 30 * time.Second
	serviceConfig.ResourceDisposeTimeout = 10 * time.Second

	serviceManager := utils.NewServiceManager(serviceConfig)

	// 创建云控制器
	cloudControl := managers.NewBuiltinCloudControl(nil)

	// 创建服务器
	server := &Server{
		config:         config,
		serviceManager: serviceManager,
		cloudControl:   cloudControl,
		cloudBuiltin:   cloudControl,
	}

	// 创建存储和ID管理器
	server.storage = storage.NewMemoryStorage(parentCtx)
	server.idManager = idgen.NewIDManager(server.storage, parentCtx)

	// 创建 SessionManager
	server.session = session.NewSessionManager(server.idManager, parentCtx)

	// 创建并设置 AuthHandler 和 TunnelHandler
	authHandler := NewServerAuthHandler(cloudControl)
	tunnelHandler := NewServerTunnelHandler(cloudControl)
	server.session.SetAuthHandler(authHandler)
	server.session.SetTunnelHandler(tunnelHandler)
	
	// ✅ 注入CloudControl，用于查询映射配置（使用适配器）
	cloudControlAdapter := session.NewCloudControlAdapter(cloudControl)
	server.session.SetCloudControl(cloudControlAdapter)

	// 创建协议工厂
	server.protocolFactory = NewProtocolFactory(server.session)

	// 创建协议适配器管理器
	server.protocolMgr = protocol.NewProtocolManager(parentCtx)

	server.serverID, _ = server.idManager.GenerateConnectionID()

	// 初始化 MessageBroker
	server.messageBroker = server.createMessageBroker(parentCtx)

	// 初始化 BridgeConnectionPool 和 BridgeManager
	if config.BridgePool.Enabled {
		server.bridgeManager = server.createBridgeManager(parentCtx)
		server.grpcServer = server.startGRPCServer()
	}

	// 初始化 Management API
	if config.ManagementAPI.Enabled {
		server.apiServer = server.createManagementAPI(parentCtx)
		// ✅ 注入SessionManager，用于推送配置给客户端
		server.apiServer.SetSessionManager(server.session)
		utils.Infof("Server: SessionManager injected into Management API")
	}

	// 注册服务到服务管理器
	server.registerServices()

	return server
}

// Start 启动服务器
func (s *Server) Start() error {
	utils.Info(constants.MsgStartingServer)

	// 设置协议适配器
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器启动所有服务
	if err := s.serviceManager.StartAllServices(); err != nil {
		return fmt.Errorf("failed to start services: %v", err)
	}

	utils.Info(constants.MsgServerStarted)
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info(constants.MsgShuttingDownServer)

	// 停止 gRPC 服务器
	if s.grpcServer != nil {
		utils.Info("Stopping gRPC server...")
		s.grpcServer.GracefulStop()
	}

	// 使用服务管理器停止所有服务
	if err := s.serviceManager.StopAllServices(); err != nil {
		utils.Errorf("Failed to stop services: %v", err)
	}

	utils.Info(constants.MsgServerShutdownCompleted)
	return nil
}

// Run 运行服务器（使用ServiceManager的优雅关闭）
func (s *Server) Run() error {
	utils.Info("Starting Tunnox Core with ServiceManager...")

	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.Run()
}

// RunWithContext 使用指定上下文运行服务器
func (s *Server) RunWithContext(ctx context.Context) error {
	utils.Info("Starting Tunnox Core with ServiceManager...")

	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.RunWithContext(ctx)
}

