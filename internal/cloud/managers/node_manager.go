package managers

import (
	"context"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
)

// NodeManager 节点管理器
// 通过 NodeService 接口访问节点数据，遵循 Manager -> Service -> Repository 架构
type NodeManager struct {
	*dispose.ManagerBase
	nodeService services.NodeService
}

// NewNodeManager 创建新的节点管理器
func NewNodeManager(nodeService services.NodeService, parentCtx context.Context) *NodeManager {
	manager := &NodeManager{
		ManagerBase: dispose.NewManager("NodeManager", parentCtx),
		nodeService: nodeService,
	}
	return manager
}

// GetNodeServiceInfo 获取节点服务信息
func (nm *NodeManager) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	return nm.nodeService.GetNodeServiceInfo(nodeID)
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (nm *NodeManager) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	return nm.nodeService.GetAllNodeServiceInfo()
}
