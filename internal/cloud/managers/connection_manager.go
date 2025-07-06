package managers

import (
	"fmt"
	"time"
	"tunnox-core/internal/cloud/distributed"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/utils"
)

// ConnectionManager 连接管理服务
type ConnectionManager struct {
	connRepo *repos.ConnectionRepo
	idGen    *distributed.DistributedIDGenerator
	utils.Dispose
}

// NewConnectionManager 创建连接管理服务
func NewConnectionManager(connRepo *repos.ConnectionRepo, idGen *distributed.DistributedIDGenerator) *ConnectionManager {
	manager := &ConnectionManager{
		connRepo: connRepo,
		idGen:    idGen,
	}
	manager.SetCtx(nil, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (cm *ConnectionManager) onClose() {
	utils.Infof("Connection manager resources cleaned up")
}

// RegisterConnection 注册连接
func (cm *ConnectionManager) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	// 如果连接ID为空，则生成新的连接ID
	if connInfo.ConnID == "" {
		connID, err := cm.idGen.GenerateMappingID(cm.Ctx())
		if err != nil {
			return fmt.Errorf("generate connection ID failed: %w", err)
		}
		connInfo.ConnID = connID
	}

	connInfo.MappingID = mappingID
	connInfo.EstablishedAt = time.Now()
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	// 创建连接（CreateConnection会自动添加到映射连接列表）
	return cm.connRepo.CreateConnection(connInfo)
}

// UnregisterConnection 注销连接
func (cm *ConnectionManager) UnregisterConnection(connID string) error {
	return cm.connRepo.DeleteConnection(connID)
}

// GetConnections 获取映射的所有连接
func (cm *ConnectionManager) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	return cm.connRepo.ListConnections(mappingID)
}

// GetClientConnections 获取客户端的所有连接
func (cm *ConnectionManager) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	return cm.connRepo.ListClientConns(clientID)
}

// UpdateConnectionStats 更新连接统计信息
func (cm *ConnectionManager) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	return cm.connRepo.UpdateStats(connID, bytesSent, bytesReceived)
}
