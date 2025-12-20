package managers

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
)

// NodeRegister 注册节点
func (c *CloudControl) NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error) {
	// 生成节点ID，确保不重复
	var nodeID string
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := c.idManager.GenerateNodeID()
		if err != nil {
			return nil, fmt.Errorf("generate node ID failed: %w", err)
		}

		// 检查节点是否已存在
		existingNode, err := c.nodeRepo.GetNode(generatedID)
		if err != nil {
			// 节点不存在，可以使用这个ID
			nodeID = generatedID
			break
		}

		if existingNode != nil {
			// 节点已存在，释放ID并重试
			_ = c.idManager.ReleaseNodeID(generatedID)
			continue
		}

		nodeID = generatedID
		break
	}

	if nodeID == "" {
		return nil, fmt.Errorf("failed to generate unique node ID after %d attempts", constants.DefaultMaxAttempts)
	}

	now := time.Now()
	node := &models.Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID),
		Address:   req.Address,
		Meta:      req.Meta,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := c.nodeRepo.CreateNode(node); err != nil {
		// 如果保存失败，释放节点ID
		_ = c.idManager.ReleaseNodeID(nodeID)
		return nil, fmt.Errorf("save node failed: %w", err)
	}

	return &models.NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: "Node registered successfully",
	}, nil
}

// NodeUnregister 注销节点
func (c *CloudControl) NodeUnregister(req *models.NodeUnregisterRequest) error {
	// 获取节点信息，用于释放ID
	node, err := c.nodeRepo.GetNode(req.NodeID)
	if err == nil && node != nil {
		// 释放节点ID
		_ = c.idManager.ReleaseNodeID(req.NodeID)
	}
	return c.nodeRepo.DeleteNode(req.NodeID)
}

// NodeHeartbeat 节点心跳
func (c *CloudControl) NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error) {
	// 更新节点心跳时间
	node, err := c.nodeRepo.GetNode(req.NodeID)
	if err != nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	if node == nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: "Node not found",
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()
	if err := c.nodeRepo.UpdateNode(node); err != nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: "Failed to update node",
		}, nil
	}

	return &models.NodeHeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

// GetNodeServiceInfo 获取节点服务信息
func (c *CloudControl) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	return c.nodeManager.GetNodeServiceInfo(nodeID)
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (c *CloudControl) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	return c.nodeManager.GetAllNodeServiceInfo()
}
