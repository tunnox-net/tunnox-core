package managers

import (
	"context"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
)

// ConnectionManager 连接管理器
// 通过 ConnectionService 接口访问连接数据，遵循 Manager -> Service -> Repository 架构
type ConnectionManager struct {
	*dispose.ManagerBase
	connService services.ConnectionService
}

// NewConnectionManager 创建新的连接管理器
func NewConnectionManager(connService services.ConnectionService, parentCtx context.Context) *ConnectionManager {
	manager := &ConnectionManager{
		ManagerBase: dispose.NewManager("ConnectionManager", parentCtx),
		connService: connService,
	}
	return manager
}

// RegisterConnection 注册连接
func (cm *ConnectionManager) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	return cm.connService.RegisterConnection(mappingID, connInfo)
}

// UnregisterConnection 注销连接
func (cm *ConnectionManager) UnregisterConnection(connID string) error {
	return cm.connService.UnregisterConnection(connID)
}

// GetConnections 获取映射的所有连接
func (cm *ConnectionManager) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	return cm.connService.GetConnections(mappingID)
}

// GetClientConnections 获取客户端的所有连接
func (cm *ConnectionManager) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	return cm.connService.GetClientConnections(clientID)
}

// UpdateConnectionStats 更新连接统计信息
func (cm *ConnectionManager) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	return cm.connService.UpdateConnectionStats(connID, bytesSent, bytesReceived)
}
