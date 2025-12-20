package managers

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/distributed"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
)

// CloudControl 基础云控实现，所有存储操作通过 Storage 接口
// 业务逻辑、资源管理、定时清理等通用逻辑全部在这里实现
// 子类只需注入不同的 Storage 实现

type CloudControl struct {
	*dispose.ResourceBase
	config            *ControlConfig
	storage           storage.Storage
	idManager         *idgen.IDManager
	userRepo          *repos.UserRepository
	clientRepo        *repos.ClientRepository
	mappingRepo       *repos.PortMappingRepo
	nodeRepo          *repos.NodeRepository
	connRepo          *repos.ConnectionRepo
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
}

func NewCloudControl(config *ControlConfig, storage storage.Storage) *CloudControl {
	ctx := context.Background()
	repo := repos.NewRepository(storage)

	// 使用锁工厂创建分布式锁
	lockFactory := distributed.NewLockFactory(storage)
	owner := fmt.Sprintf("cloud_control_%d", time.Now().UnixNano())
	lock := lockFactory.CreateDefaultLock(owner)

	// 创建仓库实例
	userRepo := repos.NewUserRepository(repo)
	clientRepo := repos.NewClientRepository(repo)
	mappingRepo := repos.NewPortMappingRepo(repo)
	nodeRepo := repos.NewNodeRepository(repo)
	connRepo := repos.NewConnectionRepo(repo)

	// 创建ID管理器
	idManager := idgen.NewIDManager(storage, ctx)

	base := &CloudControl{
		ResourceBase:      dispose.NewResourceBase("CloudControl"),
		config:            config,
		storage:           storage,
		idManager:         idManager,
		userRepo:          userRepo,
		clientRepo:        clientRepo,
		mappingRepo:       mappingRepo,
		nodeRepo:          nodeRepo,
		connRepo:          connRepo,
		jwtManager:        NewJWTManager(config, repo, ctx),
		configManager:     NewConfigManager(storage, config, ctx),
		cleanupManager:    NewCleanupManager(storage, lock, ctx),
		statsManager:      NewStatsManager(userRepo, clientRepo, mappingRepo, nodeRepo, storage, ctx),
		anonymousManager:  NewAnonymousManager(clientRepo, mappingRepo, idManager, ctx),
		nodeManager:       NewNodeManager(nodeRepo, ctx),
		searchManager:     NewSearchManager(userRepo, clientRepo, mappingRepo, ctx),
		connectionManager: NewConnectionManager(connRepo, idManager, ctx),
		lock:              lock,
		cleanupTicker:     time.NewTicker(60 * time.Second), // 默认清理间隔
		done:              make(chan bool),
	}
	base.Initialize(ctx)
	return base
}

// handleErrorWithIDRelease 处理需要释放ID的错误
// 这是一个通用的错误处理模式，用于在操作失败时自动释放已分配的ID
func (c *CloudControl) handleErrorWithIDRelease(err error, id int64, releaseFunc func(int64) error, message string) error {
	if err == nil {
		return nil
	}

	// 释放ID
	if releaseFunc != nil {
		_ = releaseFunc(id)
	}

	return fmt.Errorf("%s: %w", message, err)
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

	// 调用 ResourceBase 的清理逻辑
	result := c.ResourceBase.Dispose.Close()
	if result.HasErrors() {
		return result
	}
	return nil
}

// SetNotifier 设置通知器
func (c *CloudControl) SetNotifier(notifier interface{}) {
	if c.anonymousManager != nil {
		c.anonymousManager.SetNotifier(notifier)
	}
}
