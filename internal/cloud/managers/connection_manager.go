package managers

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
)

// ConnectionManager 连接管理器
type ConnectionManager struct {
	*dispose.ManagerBase
	connRepo  *repos.ConnectionRepo
	idManager *idgen.IDManager
}

// NewConnectionManager 创建新的连接管理器
func NewConnectionManager(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) *ConnectionManager {
	manager := &ConnectionManager{
		ManagerBase: dispose.NewManager("ConnectionManager", parentCtx),
		connRepo:    connRepo,
		idManager:   idManager,
	}
	return manager
}

// RegisterConnection 注册连接
func (cm *ConnectionManager) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	// 如果连接ID为空，则生成新的连接ID
	if connInfo.ConnID == "" {
		connID, err := cm.idManager.GenerateConnectionID()
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
