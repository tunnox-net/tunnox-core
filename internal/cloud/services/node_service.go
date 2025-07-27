package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
)

// NodeServiceImpl 节点服务实现
type NodeServiceImpl struct {
	*dispose.ServiceBase
	baseService *BaseService
	nodeRepo    *repos.NodeRepository
	idManager   *idgen.IDManager
}

// NewNodeService 创建节点服务
func NewNodeService(nodeRepo *repos.NodeRepository, idManager *idgen.IDManager, parentCtx context.Context) NodeService {
	service := &NodeServiceImpl{
		ServiceBase: dispose.NewService("NodeService", parentCtx),
		baseService: NewBaseService(),
		nodeRepo:    nodeRepo,
		idManager:   idManager,
	}
	return service
}

// NodeRegister 节点注册
func (s *NodeServiceImpl) NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error) {
	// 检查节点是否已存在
	existingNode, err := s.nodeRepo.GetNode(req.NodeID)
	if err == nil && existingNode != nil {
		// 节点已存在，更新信息
		existingNode.Address = req.Address
		existingNode.Meta = req.Meta
		s.baseService.SetUpdatedTimestamp(&existingNode.UpdatedAt)

		if err := s.nodeRepo.UpdateNode(existingNode); err != nil {
			return nil, s.baseService.WrapErrorWithID(err, "update existing node", req.NodeID)
		}

		s.baseService.LogUpdated("existing node", req.NodeID)
		return &models.NodeRegisterResponse{
			NodeID:  req.NodeID,
			Success: true,
			Message: "Node updated successfully",
		}, nil
	}

	// 生成新的节点ID
	nodeID, err := s.idManager.GenerateNodeID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate node ID")
	}

	// 创建新节点
	node := &models.Node{
		ID:        nodeID,
		Name:      fmt.Sprintf("Node-%s", nodeID),
		Address:   req.Address,
		Meta:      req.Meta,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 设置时间戳
	s.baseService.SetTimestamps(&node.CreatedAt, &node.UpdatedAt)

	// 保存到存储
	if err := s.nodeRepo.CreateNode(node); err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseString(err, nodeID, s.idManager.ReleaseNodeID, "create node")
	}

	s.baseService.LogCreated("new node", nodeID)
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
		return s.baseService.WrapErrorWithID(err, "get node", req.NodeID)
	}

	// 删除节点
	if err := s.nodeRepo.DeleteNode(req.NodeID); err != nil {
		return s.baseService.WrapErrorWithID(err, "delete node", req.NodeID)
	}

	// 释放节点ID
	if err := s.idManager.ReleaseNodeID(req.NodeID); err != nil {
		s.baseService.LogWarning("release node ID", err, req.NodeID)
	}

	s.baseService.LogDeleted("node", req.NodeID)
	return nil
}

// NodeHeartbeat 节点心跳
func (s *NodeServiceImpl) NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error) {
	// 获取节点信息
	node, err := s.nodeRepo.GetNode(req.NodeID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get node", req.NodeID)
	}

	// 更新节点信息
	node.Address = req.Address
	s.baseService.SetUpdatedTimestamp(&node.UpdatedAt)

	if err := s.nodeRepo.UpdateNode(node); err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "update node", req.NodeID)
	}

	return &models.NodeHeartbeatResponse{
		Success: true,
		Message: "Heartbeat received",
	}, nil
}

// GetNodeServiceInfo 获取节点服务信息
func (s *NodeServiceImpl) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	node, err := s.nodeRepo.GetNode(nodeID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get node", nodeID)
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
		return nil, s.baseService.WrapError(err, "list nodes")
	}

	infos := make([]*models.NodeServiceInfo, len(nodes))
	for i, node := range nodes {
		infos[i] = &models.NodeServiceInfo{
			NodeID:  node.ID,
			Address: node.Address,
		}
	}

	return infos, nil
}
