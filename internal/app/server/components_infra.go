package server

import (
	"context"
	"fmt"
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
	serverStorage, err := createStorage(storageFactory, &deps.Config.Storage)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}

	deps.Storage = serverStorage
	deps.IDManager = idgen.NewIDManager(serverStorage, ctx)

	// 创建共享的 Repository
	deps.Repository = repos.NewRepository(serverStorage)

	corelog.Infof("Storage initialized: type=%s", deps.Config.Storage.Type)
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
	metricsType := metrics.MetricsType(deps.Config.Metrics.Type)
	if metricsType == "" {
		metricsType = metrics.MetricsTypeMemory
	}

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
	cloudControlConfig.NodeID = deps.Config.MessageBroker.NodeID

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

	// 创建节点ID分配器并分配唯一节点ID
	deps.NodeAllocator = node.NewNodeIDAllocator(deps.Storage)
	allocatedNodeID, err := deps.NodeAllocator.AllocateNodeID(ctx)
	if err != nil {
		return fmt.Errorf("failed to allocate node ID: %w", err)
	}
	deps.NodeID = allocatedNodeID

	// 注册当前节点到 CloudControl
	if deps.CloudBuiltin != nil {
		nodeAddress := c.getNodeAddress(deps)
		if err := c.registerNode(deps, allocatedNodeID, nodeAddress); err != nil {
			corelog.Warnf("Failed to register current node: %v", err)
		}
	}

	corelog.Infof("Node initialized: nodeID=%s", allocatedNodeID)
	return nil
}

func (c *NodeComponent) getNodeAddress(deps *Dependencies) string {
	nodeAddress := fmt.Sprintf("%s:%d", deps.Config.Server.Host, deps.Config.Server.Port)

	// 尝试从 RemoteStorage 获取真实的外部地址
	if deps.Storage != nil {
		if hybrid, ok := deps.Storage.(*storage.HybridStorage); ok {
			if remoteStorage := hybrid.GetRemoteStorage(); remoteStorage != nil {
				corelog.Infof("RemoteStorage found, trying to get client address...")
				if clientAddr, err := remoteStorage.GetClientAddress(); err == nil && clientAddr != "" {
					nodeAddress = fmt.Sprintf("%s:%d", clientAddr, deps.Config.Server.Port)
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
