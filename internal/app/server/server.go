package server

import (
	"context"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/node"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/health"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/security"
	"tunnox-core/internal/utils"
)

// Server 服务器结构
type Server struct {
	config          *Config
	serviceManager  *utils.ServiceManager
	protocolMgr     *protocol.ProtocolManager
	serverID        string
	nodeID          string
	storage         storage.Storage
	idManager       *idgen.IDManager
	nodeAllocator   *node.NodeIDAllocator
	session         *session.SessionManager
	protocolFactory *ProtocolFactory
	cloudControl    managers.CloudControlAPI
	cloudBuiltin    *managers.BuiltinCloudControl
	authHandler     *ServerAuthHandler
	messageBroker   broker.MessageBroker
	httpService     *httpservice.HTTPService

	// 服务
	connCodeService       *services.ConnectionCodeService
	bruteForceProtector   *security.BruteForceProtector
	ipManager             *security.IPManager
	rateLimiter           *security.RateLimiter
	healthManager         *health.HealthManager
	reconnectTokenManager *security.ReconnectTokenManager
	sessionTokenManager   *security.SessionTokenManager

	// 仓库
	httpDomainRepo repos.IHTTPDomainMappingRepository
}

// New 创建新服务器（使用 Builder 模式）
func New(config *Config, parentCtx context.Context) *Server {
	server, err := NewServerBuilder(config).
		WithDefaults().
		Build(parentCtx)

	if err != nil {
		corelog.Fatalf("Failed to create server: %v", err)
	}

	return server
}

// NewWithError 创建新服务器，返回错误而不是 Fatal
// 用于测试和需要错误处理的场景
func NewWithError(config *Config, parentCtx context.Context) (*Server, error) {
	return NewServerBuilder(config).
		WithDefaults().
		Build(parentCtx)
}

// createServiceManager 创建服务管理器
func (s *Server) createServiceManager(parentCtx context.Context) {
	serviceConfig := utils.DefaultServiceConfig()
	serviceConfig.EnableSignalHandling = true
	serviceConfig.GracefulShutdownTimeout = 30 * time.Second
	serviceConfig.ResourceDisposeTimeout = 10 * time.Second

	s.serviceManager = utils.NewServiceManager(serviceConfig)

	// 生成服务器ID
	if s.idManager != nil {
		var err error
		s.serverID, err = s.idManager.GenerateConnectionID()
		if err != nil {
			corelog.Warnf("Failed to generate server ID, using empty ID: %v", err)
		}
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 设置协议适配器
	if err := s.setupProtocolAdapters(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to setup protocol adapters")
	}

	// 设置连接码命令处理器
	if err := s.setupConnectionCodeCommands(); err != nil {
		corelog.Errorf("Server: failed to setup connection code commands: %v", err)
	}

	// 使用服务管理器启动所有服务
	if err := s.serviceManager.StartAllServices(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to start services")
	}

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	corelog.Info(constants.MsgShuttingDownServer)

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
			corelog.Warnf("Failed to save tunnel states (non-fatal): %v", err)
		}
	}

	// 等待活跃隧道完成传输（最多30秒）
	if s.session != nil {
		s.session.WaitForTunnelsToComplete(30)
	}

	// 使用服务管理器停止所有服务
	if err := s.serviceManager.StopAllServices(); err != nil {
		corelog.Errorf("Failed to stop services: %v", err)
	}

	return nil
}

// Run 运行服务器（使用ServiceManager的优雅关闭）
func (s *Server) Run() error {
	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		corelog.Default().Errorf("Failed to setup protocol adapters: %v", err)
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to setup protocol adapters")
	}

	// 设置连接码命令处理器
	if err := s.setupConnectionCodeCommands(); err != nil {
		corelog.Errorf("Server: failed to setup connection code commands: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.Run()
}

// RunWithContext 使用指定上下文运行服务器
func (s *Server) RunWithContext(ctx context.Context) error {
	// 设置协议适配器（但不启动服务）
	if err := s.setupProtocolAdapters(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to setup protocol adapters")
	}

	// 设置连接码命令处理器
	if err := s.setupConnectionCodeCommands(); err != nil {
		corelog.Errorf("Server: failed to setup connection code commands: %v", err)
	}

	// 使用服务管理器运行（包含信号处理和优雅关闭）
	return s.serviceManager.RunWithContext(ctx)
}

// registerCurrentNode 注册当前节点到 CloudControl
func (s *Server) registerCurrentNode(nodeID, address string) error {
	now := time.Now()
	nodeModel := &models.Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID),
		Address:   address,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if s.cloudBuiltin != nil {
		return s.cloudBuiltin.RegisterNodeDirect(nodeModel)
	}
	return coreerrors.New(coreerrors.CodeNotConfigured, "cloudBuiltin not initialized")
}

// getRemoteStorage 获取 RemoteStorage 实例（如果存在）
func (s *Server) getRemoteStorage() *storage.RemoteStorage {
	if s.storage == nil {
		return nil
	}
	if hybrid, ok := s.storage.(*storage.HybridStorage); ok {
		return hybrid.GetRemoteStorage()
	}
	return nil
}
