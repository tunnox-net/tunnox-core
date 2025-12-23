package server

import (
	"context"
	"fmt"
	"time"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/health"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/security"
)

// ============================================================================
// SessionComponent - 会话管理组件
// ============================================================================

// SessionComponent 会话管理组件
type SessionComponent struct {
	*BaseComponent
}

func (c *SessionComponent) Name() string {
	return "Session"
}

func (c *SessionComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.IDManager == nil {
		return fmt.Errorf("IDManager is required")
	}

	deps.SessionMgr = session.NewSessionManager(deps.IDManager, ctx)

	corelog.Infof("SessionManager initialized")
	return nil
}

func (c *SessionComponent) Start() error {
	return nil
}

func (c *SessionComponent) Stop() error {
	return nil
}

// ============================================================================
// SecurityComponent - 安全组件
// ============================================================================

// SecurityComponent 安全组件
type SecurityComponent struct {
	*BaseComponent
}

func (c *SecurityComponent) Name() string {
	return "Security"
}

func (c *SecurityComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	// 创建安全组件
	deps.BruteForceProtector = security.NewBruteForceProtector(nil, ctx)
	deps.IPManager = security.NewIPManager(deps.Storage, ctx)
	deps.RateLimiter = security.NewRateLimiter(nil, nil, ctx)

	// 创建 Token 管理器
	deps.ReconnectTokenManager = security.NewReconnectTokenManager(nil, deps.Storage)
	deps.SessionTokenManager = security.NewSessionTokenManager(nil)

	// 注入到 SessionManager
	if deps.SessionMgr != nil {
		deps.SessionMgr.SetReconnectTokenManager(deps.ReconnectTokenManager)
	}

	corelog.Infof("Security components initialized")
	return nil
}

func (c *SecurityComponent) Start() error {
	return nil
}

func (c *SecurityComponent) Stop() error {
	return nil
}

// ============================================================================
// HealthComponent - 健康检查组件
// ============================================================================

// HealthComponent 健康检查组件
type HealthComponent struct {
	*BaseComponent
}

func (c *HealthComponent) Name() string {
	return "Health"
}

func (c *HealthComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	deps.HealthManager = health.NewHealthManager(deps.NodeID, "1.0.0", ctx)

	// 设置统计信息提供者
	if deps.SessionMgr != nil {
		deps.HealthManager.SetStatsProvider(deps.SessionMgr)
	}

	corelog.Infof("HealthManager initialized")
	return nil
}

func (c *HealthComponent) Start() error {
	return nil
}

func (c *HealthComponent) Stop() error {
	return nil
}

// ============================================================================
// HandlersComponent - 处理器组件
// ============================================================================

// HandlersComponent 处理器组件（AuthHandler、TunnelHandler 等）
type HandlersComponent struct {
	*BaseComponent
}

func (c *HandlersComponent) Name() string {
	return "Handlers"
}

func (c *HandlersComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.CloudControl == nil {
		return fmt.Errorf("CloudControl is required")
	}
	if deps.SessionMgr == nil {
		return fmt.Errorf("SessionManager is required")
	}

	// 创建 ConnectionCodeService
	connCodeRepo := repos.NewConnectionCodeRepository(deps.Repository)
	portMappingRepo := repos.NewPortMappingRepo(deps.Repository)
	portMappingService := services.NewPortMappingService(portMappingRepo, deps.IDManager, nil, ctx)

	deps.ConnCodeService = services.NewConnectionCodeService(
		connCodeRepo,
		portMappingService,
		portMappingRepo,
		nil,
		ctx,
	)

	// 创建 AuthHandler 和 TunnelHandler
	deps.AuthHandler = NewServerAuthHandler(
		deps.CloudControl,
		deps.SessionMgr,
		deps.BruteForceProtector,
		deps.IPManager,
		deps.RateLimiter,
	)
	deps.TunnelHandler = NewServerTunnelHandler(deps.CloudControl, deps.ConnCodeService)

	// 注入到 SessionManager
	deps.SessionMgr.SetAuthHandler(deps.AuthHandler)
	deps.SessionMgr.SetTunnelHandler(deps.TunnelHandler)

	// 注入 CloudControl 适配器
	cloudControlAdapter := session.NewCloudControlAdapter(deps.CloudBuiltin)
	deps.SessionMgr.SetCloudControl(cloudControlAdapter)

	// 设置 NodeID
	deps.SessionMgr.SetNodeID(deps.NodeID)

	// 创建隧道状态和迁移管理器
	tunnelStateManager := session.NewTunnelStateManager(deps.Storage, "")
	deps.SessionMgr.SetTunnelStateManager(tunnelStateManager)
	migrationManager := session.NewTunnelMigrationManager(tunnelStateManager, deps.SessionMgr)
	deps.SessionMgr.SetMigrationManager(migrationManager)

	// 创建并注入 TunnelRoutingTable
	if deps.Storage != nil {
		tunnelRouting := session.NewTunnelRoutingTable(deps.Storage, 30*time.Second)
		deps.SessionMgr.SetTunnelRoutingTable(tunnelRouting)

		// 注册节点地址到 Redis（用于跨节点转发）
		// 从 RemoteStorage 获取 Pod IP（通过连接 storage 服务获取本机地址）
		nodeHost := deps.NodeID
		if hybrid, ok := deps.Storage.(*storage.HybridStorage); ok {
			if remoteStorage := hybrid.GetRemoteStorage(); remoteStorage != nil {
				if clientAddr, err := remoteStorage.GetClientAddress(); err == nil && clientAddr != "" {
					// storage 服务返回的是纯 IP 地址（不含端口）
					nodeHost = clientAddr
					corelog.Infof("Got Pod IP from RemoteStorage: %s", nodeHost)
				} else if err != nil {
					corelog.Warnf("Failed to get Pod IP from RemoteStorage: %v", err)
				} else {
					corelog.Warnf("RemoteStorage.GetClientAddress() returned empty address")
				}
			} else {
				corelog.Warnf("HybridStorage.GetRemoteStorage() returned nil, using nodeID as host")
			}
		} else {
			corelog.Warnf("Storage is not HybridStorage (type=%T), using nodeID as host", deps.Storage)
		}
		nodeAddr := fmt.Sprintf("%s:50052", nodeHost)
		if err := tunnelRouting.RegisterNodeAddress(deps.NodeID, nodeAddr); err != nil {
			corelog.Warnf("Failed to register node address: %v", err)
		} else {
			corelog.Infof("Registered node address: %s -> %s", deps.NodeID, nodeAddr)
		}

		// ✅ 创建并注入 ConnectionStateStore（用于跨节点客户端位置查询）
		connStateStore := session.NewConnectionStateStore(deps.Storage, deps.NodeID, 5*time.Minute)
		deps.SessionMgr.SetConnectionStateStore(connStateStore)
		corelog.Infof("ConnectionStateStore initialized for node %s", deps.NodeID)

		// ✅ 创建并注入 CrossNodePool（跨节点连接池）
		crossNodePoolConfig := session.DefaultCrossNodePoolConfig()
		crossNodePool := session.NewCrossNodePool(ctx, deps.Storage, deps.NodeID, crossNodePoolConfig)
		deps.SessionMgr.SetCrossNodePool(crossNodePool)
		corelog.Infof("CrossNodePool initialized for node %s", deps.NodeID)

		// ✅ 创建并启动 CrossNodeListener（跨节点连接监听器）
		crossNodeListener := session.NewCrossNodeListener(deps.SessionMgr, 50052)
		if err := crossNodeListener.Start(ctx); err != nil {
			corelog.Warnf("Failed to start CrossNodeListener: %v", err)
		} else {
			deps.SessionMgr.SetCrossNodeListener(crossNodeListener)
			corelog.Infof("CrossNodeListener started on port 50052 for node %s", deps.NodeID)
		}
	}

	corelog.Infof("Handlers initialized")
	return nil
}

func (c *HandlersComponent) Start() error {
	return nil
}

func (c *HandlersComponent) Stop() error {
	return nil
}
