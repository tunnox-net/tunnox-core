package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// 编译时接口断言，确保 ConnectionRepo 实现了 IConnectionRepository 接口
var _ IConnectionRepository = (*ConnectionRepo)(nil)

// ConnectionRepo 连接数据访问
type ConnectionRepo struct {
	*GenericRepositoryImpl[*models.ConnectionInfo]
	dispose.Dispose
}

// NewConnectionRepo 创建连接数据访问层
func NewConnectionRepo(parentCtx context.Context, repo *Repository) *ConnectionRepo {
	genericRepo := NewGenericRepository[*models.ConnectionInfo](repo, func(connInfo *models.ConnectionInfo) (string, error) {
		return connInfo.ConnID, nil
	})
	cr := &ConnectionRepo{GenericRepositoryImpl: genericRepo}
	cr.Dispose.SetCtx(parentCtx, cr.onClose)
	return cr
}

// getEntityID 获取连接ID
func (r *ConnectionRepo) getEntityID(connInfo *models.ConnectionInfo) (string, error) {
	return connInfo.ConnID, nil
}

func (cr *ConnectionRepo) onClose() error {
	if cr.GenericRepositoryImpl != nil {
		return cr.GenericRepositoryImpl.Repository.Dispose.Close()
	}
	return nil
}

// SaveConnection 保存连接信息（创建或更新）
func (r *ConnectionRepo) SaveConnection(connInfo *models.ConnectionInfo) error {
	if err := r.Save(connInfo, constants.KeyPrefixConnection, constants2.DefaultConnectionTTL); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "save connection failed")
	}
	// 添加到映射和客户端的连接列表
	if err := r.AddConnectionToMapping(connInfo.MappingID, connInfo); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "add connection to mapping failed")
	}
	if err := r.AddConnectionToClient(connInfo.ClientID, connInfo); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "add connection to client failed")
	}
	return nil
}

// CreateConnection 创建新连接（仅创建，不允许覆盖）
func (r *ConnectionRepo) CreateConnection(connInfo *models.ConnectionInfo) error {
	if err := r.Create(connInfo, constants.KeyPrefixConnection, constants2.DefaultConnectionTTL); err != nil {
		return err
	}
	// 添加到映射和客户端的连接列表
	if err := r.AddConnectionToMapping(connInfo.MappingID, connInfo); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "add connection to mapping failed")
	}
	if err := r.AddConnectionToClient(connInfo.ClientID, connInfo); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "add connection to client failed")
	}
	return nil
}

// UpdateConnection 更新连接（仅更新，不允许创建）
func (r *ConnectionRepo) UpdateConnection(connInfo *models.ConnectionInfo) error {
	// 检查连接是否存在
	_, err := r.GetConnection(connInfo.ConnID)
	if err != nil {
		return coreerrors.Newf(coreerrors.CodeNotFound, "connection with ID %s does not exist", connInfo.ConnID)
	}

	// 只更新主连接记录，不重新添加到列表中
	data, err := json.Marshal(connInfo)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "marshal connection failed")
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixConnection, connInfo.ConnID)
	if err := r.storage.Set(key, string(data), constants2.DefaultConnectionTTL); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "update connection failed")
	}

	return nil
}

// GetConnection 获取连接信息
func (r *ConnectionRepo) GetConnection(connID string) (*models.ConnectionInfo, error) {
	return r.Get(connID, constants.KeyPrefixConnection)
}

// DeleteConnection 删除连接
func (r *ConnectionRepo) DeleteConnection(connID string) error {
	// 获取连接信息以便从列表中移除
	connInfo, err := r.GetConnection(connID)
	if err != nil {
		return err
	}

	// 从映射和客户端列表中移除
	if err := r.RemoveConnectionFromMapping(connInfo.MappingID, connInfo); err != nil {
		corelog.Warnf("Failed to remove connection from mapping list: %v", err)
	}
	if err := r.RemoveConnectionFromClient(connInfo.ClientID, connInfo); err != nil {
		corelog.Warnf("Failed to remove connection from client list: %v", err)
	}

	// 删除主连接记录
	return r.Delete(connID, constants.KeyPrefixConnection)
}

// ListMappingConns 列出映射的连接
func (r *ConnectionRepo) ListMappingConns(mappingID string) ([]*models.ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.List(key)
}

// ListClientConns 列出客户端的连接
func (r *ConnectionRepo) ListClientConns(clientID int64) ([]*models.ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.List(key)
}

// AddConnectionToMapping 添加连接到映射列表
func (r *ConnectionRepo) AddConnectionToMapping(mappingID string, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.AddToList(connInfo, key)
}

// AddConnectionToClient 添加连接到客户端
func (r *ConnectionRepo) AddConnectionToClient(clientID int64, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.AddToList(connInfo, key)
}

// RemoveConnectionFromMapping 从映射连接列表中移除连接
func (r *ConnectionRepo) RemoveConnectionFromMapping(mappingID string, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.RemoveFromList(connInfo, key)
}

// RemoveConnectionFromClient 从客户端连接列表中移除连接
func (r *ConnectionRepo) RemoveConnectionFromClient(clientID int64, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.RemoveFromList(connInfo, key)
}

// UpdateStats 更新连接统计
func (r *ConnectionRepo) UpdateStats(connID string, bytesSent, bytesReceived int64) error {
	connInfo, err := r.GetConnection(connID)
	if err != nil {
		return err
	}

	connInfo.BytesSent = bytesSent
	connInfo.BytesReceived = bytesReceived
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	return r.UpdateConnection(connInfo)
}
