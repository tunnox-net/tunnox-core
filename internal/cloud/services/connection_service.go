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

// ConnectionServiceImpl 连接服务实现
type ConnectionServiceImpl struct {
	*dispose.ServiceBase
	connRepo  *repos.ConnectionRepo
	idManager *idgen.IDManager
}

// NewConnectionService 创建连接服务
func NewConnectionService(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) ConnectionService {
	service := &ConnectionServiceImpl{
		ServiceBase: dispose.NewService("ConnectionService", parentCtx),
		connRepo:    connRepo,
		idManager:   idManager,
	}
	return service
}

// RegisterConnection 注册连接
func (s *ConnectionServiceImpl) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	// 生成连接ID
	connID, err := s.idManager.GenerateConnectionID()
	if err != nil {
		return fmt.Errorf("failed to generate connection ID: %w", err)
	}

	// 设置连接信息
	connInfo.ConnID = connID
	connInfo.MappingID = mappingID
	connInfo.EstablishedAt = time.Now()
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()
	connInfo.Status = "active"

	// 保存连接信息
	if err := s.connRepo.CreateConnection(connInfo); err != nil {
		// 释放已生成的ID
		_ = s.idManager.ReleaseConnectionID(connID)
		return fmt.Errorf("failed to create connection: %w", err)
	}

	// 添加到映射连接列表
	if err := s.connRepo.AddConnectionToMapping(mappingID, connInfo); err != nil {
		utils.Warnf("Failed to add connection to mapping list: %v", err)
	}

	// 添加到客户端连接列表
	if err := s.connRepo.AddConnectionToClient(connInfo.ClientID, connInfo); err != nil {
		utils.Warnf("Failed to add connection to client list: %v", err)
	}

	utils.Infof("Registered connection: %s for mapping: %s", connID, mappingID)
	return nil
}

// UnregisterConnection 注销连接
func (s *ConnectionServiceImpl) UnregisterConnection(connID string) error {
	// 获取连接信息
	connInfo, err := s.connRepo.GetConnection(connID)
	if err != nil {
		return fmt.Errorf("connection %s not found: %w", connID, err)
	}

	// 删除连接
	if err := s.connRepo.DeleteConnection(connID); err != nil {
		return fmt.Errorf("failed to delete connection %s: %w", connID, err)
	}

	// 从映射连接列表中移除
	if err := s.connRepo.RemoveConnectionFromMapping(connInfo.MappingID, connID); err != nil {
		utils.Warnf("Failed to remove connection from mapping list: %v", err)
	}

	// 从客户端连接列表中移除
	if err := s.connRepo.RemoveConnectionFromClient(connInfo.ClientID, connID); err != nil {
		utils.Warnf("Failed to remove connection from client list: %v", err)
	}

	// 释放连接ID
	if err := s.idManager.ReleaseConnectionID(connID); err != nil {
		utils.Warnf("Failed to release connection ID %s: %v", connID, err)
	}

	utils.Infof("Unregistered connection: %s", connID)
	return nil
}

// GetConnections 获取映射的连接
func (s *ConnectionServiceImpl) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	connections, err := s.connRepo.ListConnections(mappingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connections for mapping %s: %w", mappingID, err)
	}
	return connections, nil
}

// GetClientConnections 获取客户端的连接
func (s *ConnectionServiceImpl) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	connections, err := s.connRepo.ListClientConns(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connections for client %d: %w", clientID, err)
	}
	return connections, nil
}

// UpdateConnectionStats 更新连接统计信息
func (s *ConnectionServiceImpl) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	// 获取连接信息
	connInfo, err := s.connRepo.GetConnection(connID)
	if err != nil {
		return fmt.Errorf("connection %s not found: %w", connID, err)
	}

	// 更新统计信息
	connInfo.BytesSent = bytesSent
	connInfo.BytesReceived = bytesReceived
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	// 保存更新
	if err := s.connRepo.UpdateConnection(connInfo); err != nil {
		return fmt.Errorf("failed to update connection stats: %w", err)
	}

	utils.Debugf("Updated connection stats: %s", connID)
	return nil
}
