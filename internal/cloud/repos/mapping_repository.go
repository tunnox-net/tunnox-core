package repos

import (
	"fmt"
	"time"

	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/constants"
)

// PortMappingRepo 端口映射数据访问
type PortMappingRepo struct {
	*GenericRepositoryImpl[*models.PortMapping]
}

// NewPortMappingRepo 创建端口映射数据访问层
func NewPortMappingRepo(repo *Repository) *PortMappingRepo {
	genericRepo := NewGenericRepository[*models.PortMapping](repo, func(mapping *models.PortMapping) (string, error) {
		return mapping.ID, nil
	})
	return &PortMappingRepo{GenericRepositoryImpl: genericRepo}
}

// SavePortMapping 保存端口映射（创建或更新）
func (r *PortMappingRepo) SavePortMapping(mapping *models.PortMapping) error {
	if err := r.Save(mapping, constants.KeyPrefixPortMapping, constants2.DefaultMappingDataTTL); err != nil {
		return err
	}
	// 将映射添加到全局映射列表中
	return r.AddMappingToList(mapping)
}

// CreatePortMapping 创建新端口映射（仅创建，不允许覆盖）
func (r *PortMappingRepo) CreatePortMapping(mapping *models.PortMapping) error {
	if err := r.Create(mapping, constants.KeyPrefixPortMapping, constants2.DefaultMappingDataTTL); err != nil {
		return err
	}
	// 将映射添加到全局映射列表中
	return r.AddMappingToList(mapping)
}

// UpdatePortMapping 更新端口映射（仅更新，不允许创建）
func (r *PortMappingRepo) UpdatePortMapping(mapping *models.PortMapping) error {
	return r.Update(mapping, constants.KeyPrefixPortMapping, constants2.DefaultMappingDataTTL)
}

// GetPortMapping 获取端口映射
func (r *PortMappingRepo) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return r.Get(mappingID, constants.KeyPrefixPortMapping)
}

// DeletePortMapping 删除端口映射
func (r *PortMappingRepo) DeletePortMapping(mappingID string) error {
	return r.Delete(mappingID, constants.KeyPrefixPortMapping)
}

// UpdatePortMappingStatus 更新端口映射状态
func (r *PortMappingRepo) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	mapping.Status = status
	mapping.UpdatedAt = time.Now()

	return r.UpdatePortMapping(mapping)
}

// UpdatePortMappingStats 更新端口映射统计
func (r *PortMappingRepo) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	if stats != nil {
		mapping.TrafficStats = *stats
	}
	mapping.UpdatedAt = time.Now()

	return r.UpdatePortMapping(mapping)
}

// GetUserPortMappings 列出用户的端口映射
func (r *PortMappingRepo) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, userID)
	return r.List(key)
}

// GetClientPortMappings 列出客户端的端口映射
func (r *PortMappingRepo) GetClientPortMappings(clientID string) ([]*models.PortMapping, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	return r.List(key)
}

// AddMappingToUser 添加映射到用户
func (r *PortMappingRepo) AddMappingToUser(userID string, mapping *models.PortMapping) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, userID)
	return r.AddToList(mapping, key)
}

// AddMappingToClient 添加映射到客户端
func (r *PortMappingRepo) AddMappingToClient(clientID string, mapping *models.PortMapping) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	return r.AddToList(mapping, key)
}

// ListAllMappings 列出所有端口映射
func (r *PortMappingRepo) ListAllMappings() ([]*models.PortMapping, error) {
	return r.List(constants.KeyPrefixMappingList)
}

// AddMappingToList 添加映射到全局映射列表
func (r *PortMappingRepo) AddMappingToList(mapping *models.PortMapping) error {
	return r.AddToList(mapping, constants.KeyPrefixMappingList)
}

