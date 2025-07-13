package managers

import (
	"fmt"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/utils"
)

// NodeManager 节点管理服务
type NodeManager struct {
	nodeRepo *repos.NodeRepository
	utils.Dispose
}

// NewNodeManager 创建节点管理服务
func NewNodeManager(nodeRepo *repos.NodeRepository) *NodeManager {
	manager := &NodeManager{
		nodeRepo: nodeRepo,
	}
	manager.SetCtx(nil, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (nm *NodeManager) onClose() error {
	utils.Infof("Node manager resources cleaned up")
	// 清理节点缓存和连接信息
	// 这里可以添加清理节点资源的逻辑
	return nil
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
