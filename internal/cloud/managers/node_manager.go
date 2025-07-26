package managers

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
)

// NodeManager 节点管理器
type NodeManager struct {
	*dispose.ManagerBase
	nodeRepo *repos.NodeRepository
}

// NewNodeManager 创建新的节点管理器
func NewNodeManager(nodeRepo *repos.NodeRepository, parentCtx context.Context) *NodeManager {
	manager := &NodeManager{
		ManagerBase: dispose.NewManager("NodeManager", parentCtx),
		nodeRepo:    nodeRepo,
	}
	return manager
}

// GetNodeServiceInfo 获取节点服务信息
func (nm *NodeManager) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	node, err := nm.nodeRepo.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node not found")
	}

	return &models.NodeServiceInfo{
		NodeID:  node.ID,
		Address: node.Address,
	}, nil
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (nm *NodeManager) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	nodes, err := nm.nodeRepo.ListNodes()
	if err != nil {
		return nil, err
	}

	var nodeInfos []*models.NodeServiceInfo
	for _, node := range nodes {
		nodeInfo := &models.NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		}
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	return nodeInfos, nil
}
