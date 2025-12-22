package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"
	corelog "tunnox-core/internal/core/log"

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

	// 确定存储类型用于日志
	storageType := "memory"
	if deps.Config.Storage.Enabled {
		storageType = "remote"
	} else if deps.Config.Redis.Enabled {
		storageType = "redis"
	} else if deps.Config.Persistence.Enabled {
		storageType = "hybrid"
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

	cloudControlConfig := managers.DefaultConfig()
	cloudControlConfig.NodeID = deps.NodeID // 使用运行时分配的 NodeID

	cloudControl := managers.NewBuiltinCloudControlWithStorage(cloudControlConfig, deps.Storage)

	deps.CloudControl = cloudControl
	deps.CloudBuiltin = cloudControl

	corelog.Infof("CloudControl initialized")
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
	// 使用 Management API 的监听地址作为节点地址
	nodeAddress := deps.Config.Management.Listen

	// 尝试从 RemoteStorage 获取真实的外部地址
	if deps.Storage != nil {
		if hybrid, ok := deps.Storage.(*storage.HybridStorage); ok {
			if remoteStorage := hybrid.GetRemoteStorage(); remoteStorage != nil {
				corelog.Infof("RemoteStorage found, trying to get client address...")
				if clientAddr, err := remoteStorage.GetClientAddress(); err == nil && clientAddr != "" {
					// 提取端口号
					_, port, _ := net.SplitHostPort(deps.Config.Management.Listen)
					if port != "" {
						nodeAddress = fmt.Sprintf("%s:%s", clientAddr, port)
					} else {
						nodeAddress = clientAddr
					}
					corelog.Infof("Node external address detected: %s", nodeAddress)
				} else if err != nil {
					corelog.Warnf("Failed to get client address from RemoteStorage: %v", err)
				}
			}
		}
	}

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
