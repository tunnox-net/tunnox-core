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

// connectionService 连接服务实现
type connectionService struct {
	*dispose.ServiceBase
	baseService *BaseService
	connRepo    *repos.ConnectionRepo
	idManager   *idgen.IDManager
}

// NewConnectionService 创建连接服务
func NewConnectionService(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) ConnectionService {
	service := &connectionService{
		ServiceBase: dispose.NewService("ConnectionService", parentCtx),
		baseService: NewBaseService(),
		connRepo:    connRepo,
		idManager:   idManager,
	}
	return service
}

// RegisterConnection 注册连接
func (s *connectionService) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error {
	// 生成连接ID
	connID, err := s.idManager.GenerateConnectionID()
	if err != nil {
		return s.baseService.WrapError(err, "generate connection ID")
	}

	// 设置连接信息
	connInfo.ConnID = connID
	connInfo.MappingID = mappingID
	connInfo.EstablishedAt = time.Now()
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()
	connInfo.Status = "active"

	// 保存到存储（CreateConnection 内部会自动添加到映射和客户端连接列表）
	if err := s.connRepo.CreateConnection(connInfo); err != nil {
		return s.baseService.HandleErrorWithIDReleaseString(err, connID, s.idManager.ReleaseConnectionID, "create connection")
	}

	s.baseService.LogCreated("connection", fmt.Sprintf("%s for mapping: %s", connID, mappingID))
	return nil
}

// UnregisterConnection 注销连接
func (s *connectionService) UnregisterConnection(connID string) error {
	// 删除连接（DeleteConnection 内部会自动从映射和客户端列表中移除）
	if err := s.connRepo.DeleteConnection(connID); err != nil {
		return s.baseService.WrapErrorWithID(err, "delete connection", connID)
	}

	// 释放连接ID
	if err := s.idManager.ReleaseConnectionID(connID); err != nil {
		s.baseService.LogWarning("release connection ID", err, connID)
	}

	s.baseService.LogDeleted("connection", connID)
	return nil
}

// GetConnections 获取映射的连接列表
func (s *connectionService) GetConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	connections, err := s.connRepo.ListMappingConns(mappingID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get connections for mapping", mappingID)
	}
	return connections, nil
}

// GetClientConnections 获取客户端的连接列表
func (s *connectionService) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error) {
	connections, err := s.connRepo.ListClientConns(clientID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithInt64ID(err, "get connections for client", clientID)
	}
	return connections, nil
}

// UpdateConnectionStats 更新连接统计信息
func (s *connectionService) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error {
	connInfo, err := s.connRepo.GetConnection(connID)
	if err != nil {
		return s.baseService.WrapErrorWithID(err, "get connection", connID)
	}

	connInfo.BytesSent = bytesSent
	connInfo.BytesReceived = bytesReceived
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	if err := s.connRepo.UpdateConnection(connInfo); err != nil {
		return s.baseService.WrapError(err, "update connection stats")
	}

	return nil
}
