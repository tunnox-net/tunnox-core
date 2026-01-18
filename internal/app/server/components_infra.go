package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/factories"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/metrics"
	"tunnox-core/internal/core/node"
	"tunnox-core/internal/core/storage"
)

// ============================================================================
// StorageComponent - 存储组件
// ============================================================================

// StorageComponent 存储组件
type StorageComponent struct {
	*BaseComponent
}

func (c *StorageComponent) Name() string {
	return "Storage"
}

func (c *StorageComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	storageFactory := storage.NewStorageFactory(ctx)
	serverStorage, err := createStorage(storageFactory, deps.Config)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}

	deps.Storage = serverStorage
	deps.IDManager = idgen.NewIDManager(serverStorage, ctx)

	// 创建共享的 Repository
	deps.Repository = repos.NewRepository(serverStorage)

	if deps.Config.Postgres.Enabled {
		pgStorage, err := createPostgresStorage(ctx, deps.Config)
		if err != nil {
			return fmt.Errorf("failed to create postgres storage: %w", err)
		}
		deps.PostgresStorage = pgStorage
	}

	// 确定存储类型用于日志
	storageType := "memory"
	if deps.Config.Storage.Enabled {
		storageType = "remote"
	} else if deps.Config.Redis.Enabled {
		storageType = "redis"
	} else if deps.Config.Persistence.Enabled {
		storageType = "hybrid"
	}
	if deps.Config.Postgres.Enabled {
		storageType += "+postgres"
	}

	corelog.Infof("Storage initialized: type=%s", storageType)
	return nil
}

func (c *StorageComponent) Start() error {
	return nil
}

func (c *StorageComponent) Stop() error {
	return nil
}

// ============================================================================
// MetricsComponent - 指标组件
// ============================================================================

// MetricsComponent 指标组件
type MetricsComponent struct {
	*BaseComponent
}

func (c *MetricsComponent) Name() string {
	return "Metrics"
}

func (c *MetricsComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	metricsFactory := metrics.NewMetricsFactory(ctx)
	// 固定使用 memory 类型的 metrics
	metricsType := metrics.MetricsTypeMemory

	serverMetrics, err := metricsFactory.CreateMetrics(metricsType)
	if err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	metrics.SetGlobalMetrics(serverMetrics)

	// 注册到 dispose 包，用于记录释放计数（避免循环依赖）
	metrics.RegisterDisposeCounter(func(fn func()) {
		dispose.SetIncrementDisposeCountFunc(fn)
	})

	corelog.Infof("Metrics initialized: type=%s", metricsType)
	return nil
}

func (c *MetricsComponent) Start() error {
	return nil
}

func (c *MetricsComponent) Stop() error {
	return nil
}

// ============================================================================
// CloudControlComponent - 云控制组件
// ============================================================================

// CloudControlComponent 云控制组件
type CloudControlComponent struct {
	*BaseComponent
}

func (c *CloudControlComponent) Name() string {
	return "CloudControl"
}

func (c *CloudControlComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.Storage == nil {
		return fmt.Errorf("storage is required")
	}
	if deps.Repository == nil {
		return fmt.Errorf("repository is required")
	}

	cloudControlConfig := managers.DefaultConfig()
	cloudControlConfig.NodeID = deps.NodeID

	var cloudControl *managers.BuiltinCloudControl
	if deps.PostgresStorage != nil {
		cloudControl = factories.NewBuiltinCloudControlWithPostgres(ctx, cloudControlConfig, deps.Storage, deps.PostgresStorage)
		corelog.Infof("CloudControl initialized with PostgreSQL storage")
	} else {
		cloudControl = factories.NewBuiltinCloudControlWithRepo(ctx, cloudControlConfig, deps.Storage, deps.Repository)
		corelog.Infof("CloudControl initialized with shared repository")
	}

	deps.CloudControl = cloudControl
	deps.CloudBuiltin = cloudControl

	return nil
}

func (c *CloudControlComponent) Start() error {
	return nil
}

func (c *CloudControlComponent) Stop() error {
	return nil
}

// ============================================================================
// NodeComponent - 节点组件
// ============================================================================

// NodeComponent 节点组件
type NodeComponent struct {
	*BaseComponent
}

func (c *NodeComponent) Name() string {
	return "Node"
}

func (c *NodeComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.Storage == nil {
		return fmt.Errorf("storage is required")
	}

	// 优先使用环境变量 NODE_ID（用于集群部署）
	envNodeID := os.Getenv("NODE_ID")
	if envNodeID != "" {
		deps.NodeID = envNodeID
		corelog.Infof("Node ID from environment: %s", envNodeID)
	} else {
		// 创建节点ID分配器并分配唯一节点ID
		deps.NodeAllocator = node.NewNodeIDAllocator(deps.Storage)
		allocatedNodeID, err := deps.NodeAllocator.AllocateNodeID(ctx)
		if err != nil {
			return fmt.Errorf("failed to allocate node ID: %w", err)
		}
		deps.NodeID = allocatedNodeID
		corelog.Infof("Node ID allocated: %s", allocatedNodeID)
	}

	// 注册当前节点到 CloudControl
	if deps.CloudBuiltin != nil {
		nodeAddress := c.getNodeAddress(deps)
		if err := c.registerNode(deps, deps.NodeID, nodeAddress); err != nil {
			corelog.Warnf("Failed to register current node: %v", err)
		}
	}

	corelog.Infof("Node initialized: nodeID=%s", deps.NodeID)
	return nil
}

func (c *NodeComponent) getNodeAddress(deps *Dependencies) string {
	// 通过 UDP 探测获取本机出口 IP（与 SessionComponent 使用相同的方法）
	localIP := getLocalOutboundIP()
	if localIP == "" {
		corelog.Warnf("Failed to detect local IP, using Management.Listen as fallback")
		return deps.Config.Management.Listen
	}

	// 提取 Management API 的端口号
	_, port, err := net.SplitHostPort(deps.Config.Management.Listen)
	if err != nil || port == "" {
		port = "9000" // 默认端口
	}

	nodeAddress := fmt.Sprintf("%s:%s", localIP, port)
	corelog.Infof("Node address detected via UDP probe: %s", nodeAddress)
	return nodeAddress
}

func (c *NodeComponent) registerNode(deps *Dependencies, nodeID, address string) error {
	return deps.CloudBuiltin.RegisterNodeDirect(&models.Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID),
		Address:   address,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
}

func (c *NodeComponent) Start() error {
	return nil
}

func (c *NodeComponent) Stop() error {
	return nil
}

type WebhookComponent struct {
	*BaseComponent
}

func (c *WebhookComponent) Name() string {
	return "Webhook"
}

func (c *WebhookComponent) Initialize(ctx context.Context, deps *Dependencies) error {
	if deps.Storage == nil {
		return fmt.Errorf("storage is required")
	}

	deps.WebhookRepo = repos.NewWebhookRepository(deps.Storage)
	deps.WebhookManager = managers.NewWebhookManager(deps.WebhookRepo, ctx)

	if deps.CloudBuiltin != nil {
		deps.CloudBuiltin.SetWebhookNotifier(deps.WebhookManager)
	}

	corelog.Infof("Webhook component initialized")
	return nil
}

func (c *WebhookComponent) Start() error {
	return nil
}

func (c *WebhookComponent) Stop() error {
	return nil
}
