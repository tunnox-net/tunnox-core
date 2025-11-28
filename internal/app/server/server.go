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
	utils.Infof("✅ Server: allocated node ID: %s", server.nodeID)

	// 创建 SessionManager
	server.session = session.NewSessionManager(server.idManager, parentCtx)

	// ✅ 创建 ConnectionCodeService（连接码授权系统）
	repo := repos.NewRepository(serverStorage)
	connCodeRepo := repos.NewConnectionCodeRepository(repo)
	mappingRepo := repos.NewTunnelMappingRepository(repo)
	server.connCodeService = services.NewConnectionCodeService(
		connCodeRepo,
		mappingRepo,
		nil, // 使用默认配置
		parentCtx,
	)
	// ConnectionCodeService 使用 dispose 体系管理，不需要注册到 ServiceManager

	// ✅ 创建 BruteForceProtector（暴力破解防护）
	server.bruteForceProtector = security.NewBruteForceProtector(nil, parentCtx)
	utils.Infof("Server: BruteForceProtector initialized with default config")

	// ✅ 创建 IPManager（IP黑白名单管理）
	server.ipManager = security.NewIPManager(serverStorage, parentCtx)
	utils.Infof("Server: IPManager initialized")

	// ✅ 创建 RateLimiter（速率限制器）
	server.rateLimiter = security.NewRateLimiter(nil, nil, parentCtx)
	utils.Infof("Server: RateLimiter initialized with default config")

	// ✅ 创建 HealthManager（健康检查管理）
	server.healthManager = health.NewHealthManager(allocatedNodeID, "1.0.0", parentCtx)
	// 设置SessionManager为StatsProvider（提供连接和隧道统计）
	server.healthManager.SetStatsProvider(server.session)
	utils.Infof("Server: HealthManager initialized (node=%s)", allocatedNodeID)

	// ✅ 创建 ReconnectTokenManager（重连Token管理）
	server.reconnectTokenManager = security.NewReconnectTokenManager(nil, serverStorage)
	server.session.SetReconnectTokenManager(server.reconnectTokenManager)
	utils.Infof("Server: ReconnectTokenManager initialized")

	// ✅ 创建 SessionTokenManager（会话Token管理）
	server.sessionTokenManager = security.NewSessionTokenManager(nil)
	utils.Infof("Server: SessionTokenManager initialized")

	// ✨ Phase 2: 创建 TunnelStateManager（隧道状态管理）
	tunnelStateManager := session.NewTunnelStateManager(serverStorage, "")  // 空字符串使用默认密钥
	server.session.SetTunnelStateManager(tunnelStateManager)
	utils.Infof("Server: TunnelStateManager initialized")

	// ✨ Phase 2: 创建 MigrationManager（隧道迁移管理）
	migrationManager := session.NewTunnelMigrationManager(tunnelStateManager, server.session)
	server.session.SetMigrationManager(migrationManager)
	utils.Infof("Server: MigrationManager initialized")

	// 创建并设置 AuthHandler 和 TunnelHandler
	// 注意：先创建handler，稍后设置NodeID
	authHandler := NewServerAuthHandler(cloudControl, server.session, server.bruteForceProtector, server.ipManager, server.rateLimiter) // ⭐ 注入安全组件
	tunnelHandler := NewServerTunnelHandler(cloudControl, server.connCodeService) // ⭐ 注入 connCodeService
	server.session.SetAuthHandler(authHandler)
	server.session.SetTunnelHandler(tunnelHandler)

	// ✅ 注入CloudControl，用于查询映射配置（使用适配器）
	cloudControlAdapter := session.NewCloudControlAdapter(cloudControl)
	server.session.SetCloudControl(cloudControlAdapter)

	// ✅ 设置SessionManager的NodeID（使用动态分配的节点ID）
	server.session.SetNodeID(server.nodeID)
	utils.Infof("Server: SessionManager NodeID set to %s (dynamically allocated)", server.nodeID)

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

	// ✅ 注入BridgeAdapter到SessionManager，用于跨服务器隧道转发
	if server.messageBroker != nil {
		bridgeAdapter := NewBridgeAdapter(server.messageBroker, config.MessageBroker.NodeID)
		server.session.SetBridgeManager(bridgeAdapter)
		utils.Infof("Server: BridgeAdapter injected into SessionManager for cross-server tunnel forwarding")
	}

	// ✅ 创建并注入TunnelRoutingTable，用于跨服务器隧道路由
	if server.storage != nil {
		tunnelRouting := session.NewTunnelRoutingTable(server.storage, 30*time.Second)
		server.session.SetTunnelRoutingTable(tunnelRouting)
		utils.Infof("Server: TunnelRoutingTable created and injected")
	}

	// ✅ 设置节点ID
	server.session.SetNodeID(config.MessageBroker.NodeID)

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

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 阶段0: 标记健康状态为draining（通知负载均衡器不再路由新请求）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	if s.healthManager != nil {
		s.healthManager.MarkDraining()
		utils.Info("Server health status marked as 'draining'")
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 阶段1: 广播服务器关闭通知给所有客户端
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	if s.session != nil {
		utils.Info("Broadcasting shutdown notification to all connected clients...")
		successCount, failureCount := s.session.BroadcastShutdown(
			session.ShutdownReasonShutdown,
			30, // 30秒优雅期
			true, // 建议重连
			"Server is shutting down gracefully",
		)
		utils.Infof("Shutdown broadcast completed: success=%d, failure=%d", successCount, failureCount)
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 阶段2: 保存活跃隧道状态（Phase 2: 隧道迁移支持）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	if s.session != nil {
		utils.Info("Saving active tunnel states for migration...")
		if err := s.session.SaveActiveTunnelStates(); err != nil {
			utils.Warnf("Failed to save tunnel states (non-fatal): %v", err)
			// 不阻塞关闭流程，继续执行
		} else {
			utils.Info("Active tunnel states saved successfully")
		}
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 阶段3: 等待活跃隧道完成传输（最多30秒）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	if s.session != nil {
		utils.Info("Waiting for active tunnels to complete...")
		allCompleted := s.session.WaitForTunnelsToComplete(30)
		if allCompleted {
			utils.Info("All active tunnels completed successfully")
		} else {
			activeTunnels := s.session.GetActiveTunnelCount()
			utils.Warnf("Timeout waiting for tunnels (still have %d active tunnels), proceeding with shutdown", activeTunnels)
		}
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 阶段4: 停止 gRPC 服务器和其他服务
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
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
