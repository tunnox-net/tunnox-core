package managers

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/distributed"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
)

// CloudControl 基础云控实现，所有存储操作通过 Storage 接口
// 业务逻辑、资源管理、定时清理等通用逻辑全部在这里实现
// 子类只需注入不同的 Storage 实现
//
// 架构说明：CloudControl 通过 Service 层访问数据，遵循 Manager -> Service -> Repository 架构

type CloudControl struct {
	*dispose.ManagerBase
	config            *ControlConfig
	storage           storage.Storage
	idManager         *idgen.IDManager
	jwtManager        *JWTManager
	configManager     *ConfigManager
	cleanupManager    *CleanupManager
	statsManager      *StatsManager
	anonymousManager  *AnonymousManager
	nodeManager       *NodeManager
	searchManager     *SearchManager
	connectionManager *ConnectionManager
	lock              distributed.DistributedLock
	cleanupTicker     *time.Ticker
	done              chan bool

	// Service 层引用，用于 auth_manager 等需要直接访问 Service 的场景
	userService        services.UserService
	clientService      services.ClientService
	nodeService        services.NodeService
	portMappingService services.PortMappingService
}

// CloudControlDeps 云控依赖项，包含所有需要的 Service 实例
type CloudControlDeps struct {
	UserService        services.UserService
	ClientService      services.ClientService
	PortMappingService services.PortMappingService
	NodeService        services.NodeService
	ConnectionService  services.ConnectionService
	AnonymousService   services.AnonymousService
	StatsService       services.StatsService
}

// NewCloudControl 创建新的云控实例
// 参数 deps 包含所有需要的 Service 实例，遵循依赖注入原则
func NewCloudControl(parentCtx context.Context, config *ControlConfig, storage storage.Storage, deps *CloudControlDeps) *CloudControl {
	// 使用锁工厂创建分布式锁
	lockFactory := distributed.NewLockFactory(storage)
	owner := fmt.Sprintf("cloud_control_%d", time.Now().UnixNano())
	lock := lockFactory.CreateDefaultLock(owner)

	// 创建ID管理器
	idManager := idgen.NewIDManager(storage, parentCtx)

	// 创建 StatsManager（需要 Services）
	statsManager := NewStatsManager(
		deps.UserService,
		deps.ClientService,
		deps.PortMappingService,
		deps.StatsService,
		storage,
		parentCtx,
	)

	base := &CloudControl{
		ManagerBase:       dispose.NewManager("CloudControl", parentCtx),
		config:            config,
		storage:           storage,
		idManager:         idManager,
		jwtManager:        NewJWTManager(config, storage, parentCtx),
		configManager:     NewConfigManager(storage, config, parentCtx),
		cleanupManager:    NewCleanupManager(storage, lock, parentCtx),
		statsManager:      statsManager,
		anonymousManager:  NewAnonymousManager(deps.AnonymousService, parentCtx),
		nodeManager:       NewNodeManager(deps.NodeService, parentCtx),
		searchManager:     NewSearchManager(deps.UserService, deps.ClientService, deps.PortMappingService, parentCtx),
		connectionManager: NewConnectionManager(deps.ConnectionService, parentCtx),
		lock:              lock,
		cleanupTicker:     time.NewTicker(60 * time.Second), // 默认清理间隔
		done:              make(chan bool),
		// Service 层引用
		userService:        deps.UserService,
		clientService:      deps.ClientService,
		nodeService:        deps.NodeService,
		portMappingService: deps.PortMappingService,
	}
	return base
}

// handleErrorWithIDRelease 处理需要释放ID的错误
// 这是一个通用的错误处理模式，用于在操作失败时自动释放已分配的ID
func (c *CloudControl) handleErrorWithIDRelease(err error, id int64, releaseFunc func(int64) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID（忽略释放错误，主流程已失败）
	if releaseFunc != nil {
		_ = releaseFunc(id)
	}

	return coreerrors.Wrap(err, coreerrors.CodeInternal, message)
}

// Close 实现 CloudControlAPI 接口的 Close 方法
func (c *CloudControl) Close() error {
	// 停止清理定时器
	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}

	// 安全关闭 done 通道
	select {
	case <-c.done:
		// 通道已经关闭，不需要再次关闭
	default:
		close(c.done)
	}

	// 调用 ManagerBase 的清理逻辑
	return c.ManagerBase.Close()
}

// SetNotifier 设置通知器
// 实现 NotifierAware 接口
func (c *CloudControl) SetNotifier(notifier ClientNotifier) {
	if c.anonymousManager != nil {
		c.anonymousManager.SetNotifier(notifier)
	}
}
