package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/utils"
)

// NodeServiceImpl 节点服务实现
type NodeServiceImpl struct {
	*dispose.ResourceBase
	nodeRepo  *repos.NodeRepository
	idManager *idgen.IDManager
}

// NewNodeService 创建节点服务
func NewNodeService(nodeRepo *repos.NodeRepository, idManager *idgen.IDManager, parentCtx context.Context) NodeService {
	service := &NodeServiceImpl{
		ResourceBase: dispose.NewResourceBase("NodeService"),
		nodeRepo:     nodeRepo,
		idManager:    idManager,
	}
	service.Initialize(parentCtx)
	return service
}

// NodeRegister 节点注册
func (s *NodeServiceImpl) NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error) {
	var nodeID string
	var err error

	// 如果请求中没有提供节点ID，生成一个新的
	if req.NodeID == "" {
		nodeID, err = s.idManager.GenerateNodeID()
		if err != nil {
			return &models.NodeRegisterResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to generate node ID: %v", err),
			}, nil
		}
	} else {
		nodeID = req.NodeID
	}

	// 检查节点是否已存在
	existingNode, err := s.nodeRepo.GetNode(nodeID)
	if err == nil && existingNode != nil {
		// 节点已存在，更新信息
		existingNode.Address = req.Address
		existingNode.UpdatedAt = time.Now()
		if req.Meta != nil {
			existingNode.Meta = req.Meta
		}

		if err := s.nodeRepo.UpdateNode(existingNode); err != nil {
			return &models.NodeRegisterResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to update existing node: %v", err),
			}, nil
		}

		utils.Infof("Updated existing node: %s", nodeID)
	} else {
		// 创建新节点
		node := &models.Node{
			ID:        nodeID,
			Name:      fmt.Sprintf("Node-%s", nodeID),
			Address:   req.Address,
			Meta:      req.Meta,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.nodeRepo.CreateNode(node); err != nil {
			// 释放已生成的ID
			if req.NodeID == "" {
				_ = s.idManager.ReleaseNodeID(nodeID)
			}
			return &models.NodeRegisterResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to create node: %v", err),
			}, nil
		}

		utils.Infof("Created new node: %s", nodeID)
	}

	return &models.NodeRegisterResponse{
		NodeID:  nodeID,
		Success: true,
		Message: "Node registered successfully",
	}, nil
}

// NodeUnregister 节点注销
func (s *NodeServiceImpl) NodeUnregister(req *models.NodeUnregisterRequest) error {
	// 检查节点是否存在
	_, err := s.nodeRepo.GetNode(req.NodeID)
	if err != nil {
		return fmt.Errorf("node %s not found: %w", req.NodeID, err)
	}

	// 删除节点
	if err := s.nodeRepo.DeleteNode(req.NodeID); err != nil {
		return fmt.Errorf("failed to delete node %s: %w", req.NodeID, err)
	}

	// 释放节点ID
	if err := s.idManager.ReleaseNodeID(req.NodeID); err != nil {
		utils.Warnf("Failed to release node ID %s: %v", req.NodeID, err)
	}

	utils.Infof("Unregistered node: %s", req.NodeID)
	return nil
}

// NodeHeartbeat 节点心跳
func (s *NodeServiceImpl) NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error) {
	// 检查节点是否存在
	node, err := s.nodeRepo.GetNode(req.NodeID)
	if err != nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: fmt.Sprintf("Node %s not found", req.NodeID),
		}, nil
	}

	// 更新节点信息
	node.Address = req.Address
	node.UpdatedAt = time.Now()
	if req.Version != "" {
		if node.Meta == nil {
			node.Meta = make(map[string]string)
		}
		node.Meta["version"] = req.Version
	}

	if err := s.nodeRepo.UpdateNode(node); err != nil {
		return &models.NodeHeartbeatResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update node: %v", err),
		}, nil
	}

	utils.Debugf("Node heartbeat: %s", req.NodeID)

	return &models.NodeHeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

// GetNodeServiceInfo 获取节点服务信息
func (s *NodeServiceImpl) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	node, err := s.nodeRepo.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node %s not found: %w", nodeID, err)
	}

	return &models.NodeServiceInfo{
		NodeID:  node.ID,
		Address: node.Address,
	}, nil
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (s *NodeServiceImpl) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	nodes, err := s.nodeRepo.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	infos := make([]*models.NodeServiceInfo, 0, len(nodes))
	for _, node := range nodes {
		infos = append(infos, &models.NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		})
	}

	return infos, nil
}
