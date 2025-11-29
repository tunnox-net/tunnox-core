package server

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/api"
	internalbridge "tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/node"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/health"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/security"
	"tunnox-core/internal/utils"

	"google.golang.org/grpc"
)

// Server 服务器结构
type Server struct {
	config          *Config
	serviceManager  *utils.ServiceManager
	protocolMgr     *protocol.ProtocolManager
	serverID        string
	nodeID          string // ✅ 动态分配的节点ID
	storage         storage.Storage
	idManager       *idgen.IDManager
	nodeAllocator   *node.NodeIDAllocator // ✅ 节点ID分配器
	session         *session.SessionManager
	protocolFactory *ProtocolFactory
	cloudControl    managers.CloudControlAPI
	cloudBuiltin    *managers.BuiltinCloudControl
	authHandler     *ServerAuthHandler
	messageBroker   broker.MessageBroker
	bridgeManager   *internalbridge.BridgeManager
	grpcServer      *grpc.Server
	apiServer       *api.ManagementAPIServer

	// 服务
	connCodeService       *services.ConnectionCodeService
	bruteForceProtector   *security.BruteForceProtector
	ipManager             *security.IPManager
	rateLimiter           *security.RateLimiter
	healthManager         *health.HealthManager
	reconnectTokenManager *security.ReconnectTokenManager
	sessionTokenManager   *security.SessionTokenManager
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

	// ✅ 先创建存储（因为CloudControlAPI需要storage）
	storageFactory := storage.NewStorageFactory(parentCtx)
	serverStorage, err := createStorage(storageFactory, &config.Storage)
	if err != nil {
		utils.Fatalf("Failed to create storage: %v", err)
	}

	// ✅ 使用BuiltinCloudControl并传入HybridStorage（替代旧的nil storage）
	cloudControlConfig := managers.DefaultConfig()          // 使用默认配置
	cloudControlConfig.NodeID = config.MessageBroker.NodeID // 设置节点ID

	cloudControl := managers.NewBuiltinCloudControlWithStorage(cloudControlConfig, serverStorage)

	// 创建服务器
	server := &Server{
		config:         config,
		serviceManager: serviceManager,
		cloudControl:   cloudControl,
		cloudBuiltin:   cloudControl,
	}

	server.storage = serverStorage
	server.idManager = idgen.NewIDManager(server.storage, parentCtx)

	// ✅ 创建节点ID分配器并分配唯一节点ID
	server.nodeAllocator = node.NewNodeIDAllocator(server.storage)
	allocatedNodeID, err := server.nodeAllocator.AllocateNodeID(parentCtx)
	if err != nil {
		utils.Fatalf("Failed to allocate node ID: %v", err)
	}
	server.nodeID = allocatedNodeID

	// 创建 SessionManager
	server.session = session.NewSessionManager(server.idManager, parentCtx)

	// 创建 ConnectionCodeService（连接码授权系统）
	repo := repos.NewRepository(serverStorage)
	connCodeRepo := repos.NewConnectionCodeRepository(repo)

	// 创建 PortMappingService（统一使用 PortMapping）
	portMappingRepo := repos.NewPortMappingRepo(repo)
	portMappingService := services.NewPortMappingService(portMappingRepo, server.idManager, nil, parentCtx)

	server.connCodeService = services.NewConnectionCodeService(
		connCodeRepo,
		portMappingService,
		portMappingRepo,
		nil,
		parentCtx,
	)

	// 创建安全组件
	server.bruteForceProtector = security.NewBruteForceProtector(nil, parentCtx)
	server.ipManager = security.NewIPManager(serverStorage, parentCtx)
	server.rateLimiter = security.NewRateLimiter(nil, nil, parentCtx)

	// 创建 HealthManager（健康检查管理）
	server.healthManager = health.NewHealthManager(allocatedNodeID, "1.0.0", parentCtx)
	server.healthManager.SetStatsProvider(server.session)

	// 创建 Token 管理器
	server.reconnectTokenManager = security.NewReconnectTokenManager(nil, serverStorage)
	server.session.SetReconnectTokenManager(server.reconnectTokenManager)
	server.sessionTokenManager = security.NewSessionTokenManager(nil)

	// 创建隧道状态和迁移管理器
	tunnelStateManager := session.NewTunnelStateManager(serverStorage, "")
	server.session.SetTunnelStateManager(tunnelStateManager)
	migrationManager := session.NewTunnelMigrationManager(tunnelStateManager, server.session)
	server.session.SetMigrationManager(migrationManager)

	// 创建并设置 AuthHandler 和 TunnelHandler
	authHandler := NewServerAuthHandler(cloudControl, server.session, server.bruteForceProtector, server.ipManager, server.rateLimiter)
	tunnelHandler := NewServerTunnelHandler(cloudControl, server.connCodeService)
	server.session.SetAuthHandler(authHandler)
	server.session.SetTunnelHandler(tunnelHandler)
	server.authHandler = authHandler

	// 注入 CloudControl 适配器
	cloudControlAdapter := session.NewCloudControlAdapter(cloudControl)
	server.session.SetCloudControl(cloudControlAdapter)

	// 设置 SessionManager 的 NodeID（使用动态分配的节点ID）
	server.session.SetNodeID(server.nodeID)

	// 创建并注册连接码命令处理器
	if err := server.setupConnectionCodeCommands(); err != nil {
		utils.Errorf("Server: failed to setup connection code commands: %v", err)
	}

	// 创建协议工厂和适配器管理器
	server.protocolFactory = NewProtocolFactory(server.session)
	server.protocolMgr = protocol.NewProtocolManager(parentCtx)

	server.serverID, _ = server.idManager.GenerateConnectionID()

	// 初始化 MessageBroker
	server.messageBroker = server.createMessageBroker(parentCtx)

	// 初始化 BridgeConnectionPool 和 BridgeManager
	if config.BridgePool.Enabled {
		server.bridgeManager = server.createBridgeManager(parentCtx)
		server.grpcServer = server.startGRPCServer()
	}

	// 注入 BridgeAdapter 到 SessionManager，用于跨服务器隧道转发
	if server.messageBroker != nil {
		bridgeAdapter := NewBridgeAdapter(server.messageBroker, config.MessageBroker.NodeID)
		server.session.SetBridgeManager(bridgeAdapter)
	}

	// 创建并注入 TunnelRoutingTable，用于跨服务器隧道路由
	if server.storage != nil {
		tunnelRouting := session.NewTunnelRoutingTable(server.storage, 30*time.Second)
		server.session.SetTunnelRoutingTable(tunnelRouting)
	}

	// 初始化 Management API
	if config.ManagementAPI.Enabled {
		server.apiServer = server.createManagementAPI(parentCtx)
		server.apiServer.SetSessionManager(server.session)
	}

	// 注册服务到服务管理器
	server.registerServices()

	return server
}

// Start 启动服务器
func (s *Server) Start() error {
	// 设置协议适配器
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器启动所有服务
	if err := s.serviceManager.StartAllServices(); err != nil {
		return fmt.Errorf("failed to start services: %v", err)
	}

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	utils.Info(constants.MsgShuttingDownServer)

	// 标记健康状态为draining（通知负载均衡器不再路由新请求）
	if s.healthManager != nil {
		s.healthManager.MarkDraining()
	}

	// 广播关闭通知给所有连接的客户端
	if s.session != nil {
		s.session.BroadcastShutdown(
			session.ShutdownReasonShutdown,
			30,
			true,
			"Server is shutting down gracefully",
		)
	}

	// 保存活跃隧道状态（用于隧道迁移）
	if s.session != nil {
		if err := s.session.SaveActiveTunnelStates(); err != nil {
			utils.Warnf("Failed to save tunnel states (non-fatal): %v", err)
		}
	}

	// 等待活跃隧道完成传输（最多30秒）
	if s.session != nil {
		s.session.WaitForTunnelsToComplete(30)
	}

	// 停止 gRPC 服务器
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// 使用服务管理器停止所有服务
	if err := s.serviceManager.StopAllServices(); err != nil {
		utils.Errorf("Failed to stop services: %v", err)
	}

	return nil
}

// Run 运行服务器（使用ServiceManager的优雅关闭）
func (s *Server) Run() error {
	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.Run()
}

// RunWithContext 使用指定上下文运行服务器
func (s *Server) RunWithContext(ctx context.Context) error {
	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return fmt.Errorf("failed to setup protocol adapters: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.RunWithContext(ctx)
}
