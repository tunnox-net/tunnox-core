package cloud

import (
	"fmt"
	"tunnox-core/internal/utils"
)

// NodeManager 节点管理服务
type NodeManager struct {
	nodeRepo *NodeRepository
	utils.Dispose
}

// NewNodeManager 创建节点管理服务
func NewNodeManager(nodeRepo *NodeRepository) *NodeManager {
	manager := &NodeManager{
		nodeRepo: nodeRepo,
	}
	manager.SetCtx(nil, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (nm *NodeManager) onClose() {
	utils.Infof("Node manager resources cleaned up")
}

// GetNodeServiceInfo 获取节点服务信息
func (nm *NodeManager) GetNodeServiceInfo(nodeID string) (*NodeServiceInfo, error) {
	node, err := nm.nodeRepo.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node not found")
	}

	return &NodeServiceInfo{
		NodeID:  node.ID,
		Address: node.Address,
	}, nil
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (nm *NodeManager) GetAllNodeServiceInfo() ([]*NodeServiceInfo, error) {
	nodes, err := nm.nodeRepo.ListNodes()
	if err != nil {
		return nil, err
	}

	var nodeInfos []*NodeServiceInfo
	for _, node := range nodes {
		nodeInfo := &NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		}
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	return nodeInfos, nil
}
