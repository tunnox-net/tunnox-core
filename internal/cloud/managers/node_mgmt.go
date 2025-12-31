package managers

import (
	coreerrors "tunnox-core/internal/core/errors"

	"tunnox-core/internal/cloud/models"
)

// NodeRegister 注册节点
// 注意：此方法委托给 NodeService 处理，遵循 Manager -> Service -> Repository 架构
func (c *CloudControl) NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error) {
	if c.nodeService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "nodeService not initialized")
	}
	return c.nodeService.NodeRegister(req)
}

// NodeUnregister 注销节点
func (c *CloudControl) NodeUnregister(req *models.NodeUnregisterRequest) error {
	if c.nodeService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "nodeService not initialized")
	}
	return c.nodeService.NodeUnregister(req)
}

// NodeHeartbeat 节点心跳
func (c *CloudControl) NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error) {
	if c.nodeService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "nodeService not initialized")
	}
	return c.nodeService.NodeHeartbeat(req)
}

// GetNodeServiceInfo 获取节点服务信息
func (c *CloudControl) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error) {
	if c.nodeManager != nil {
		return c.nodeManager.GetNodeServiceInfo(nodeID)
	}
	// 降级：直接调用 nodeService
	if c.nodeService != nil {
		return c.nodeService.GetNodeServiceInfo(nodeID)
	}
	return nil, coreerrors.New(coreerrors.CodeNotConfigured, "nodeManager and nodeService both not initialized")
}

// GetAllNodeServiceInfo 获取所有节点服务信息
func (c *CloudControl) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error) {
	if c.nodeManager != nil {
		return c.nodeManager.GetAllNodeServiceInfo()
	}
	// 降级：直接调用 nodeService
	if c.nodeService != nil {
		return c.nodeService.GetAllNodeServiceInfo()
	}
	return nil, coreerrors.New(coreerrors.CodeNotConfigured, "nodeManager and nodeService both not initialized")
}
