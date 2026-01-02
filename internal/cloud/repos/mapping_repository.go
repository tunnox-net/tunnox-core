package repos

import (
	"encoding/json"
	"fmt"
	"time"

	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils/random"
)

// 编译时接口断言，确保 PortMappingRepo 实现了 IPortMappingRepository 接口
var _ IPortMappingRepository = (*PortMappingRepo)(nil)

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

// GetPortMappingByDomain 通过域名查找 HTTP 映射
func (r *PortMappingRepo) GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) {
	// 从全局映射列表中查找
	allMappings, err := r.ListAllMappings()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list mappings")
	}

	for _, mapping := range allMappings {
		if mapping.Protocol == models.ProtocolHTTP {
			if mapping.FullDomain() == fullDomain {
				return mapping, nil
			}
		}
	}

	return nil, coreerrors.Newf(coreerrors.CodeNotFound, "mapping not found for domain: %s", fullDomain)
}

// DeletePortMapping 删除端口映射
// 同时清理所有相关索引（client_mappings, mapping_list）
func (r *PortMappingRepo) DeletePortMapping(mappingID string) error {
	// 先获取 mapping 信息，用于清理索引
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	// 先清理索引（在删除主 key 之前，因为需要 mapping 对象）

	// 清理 ListenClientID 的索引
	if mapping.ListenClientID != 0 {
		clientKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, random.Int64ToString(mapping.ListenClientID))
		r.RemoveFromList(mapping, clientKey)
	}

	// 清理 TargetClientID 的索引（如果不同于 ListenClientID）
	if mapping.TargetClientID != 0 && mapping.TargetClientID != mapping.ListenClientID {
		clientKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, random.Int64ToString(mapping.TargetClientID))
		r.RemoveFromList(mapping, clientKey)
	}

	// 清理全局映射列表
	r.RemoveFromList(mapping, constants.KeyPrefixMappingList)

	// 最后删除主 key
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
//
// 查询流程：
// 1. 先从缓存索引查询（tunnox:client_mappings:{clientID}）
// 2. 如果索引为空，从持久存储按字段查询（QueryByField）
// 3. 去重并返回结果
//
// ✅ 由于映射会同时添加到 ListenClientID 和 TargetClientID 的索引，
// 同一个映射可能出现在两个索引中，需要去重
func (r *PortMappingRepo) GetClientPortMappings(clientID string) ([]*models.PortMapping, error) {
	// 1. 先从缓存索引查询
	indexKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	allMappings, err := r.List(indexKey)
	if err == nil && len(allMappings) > 0 {
		// 索引查询成功，去重并返回
		mappingMap := make(map[string]*models.PortMapping)
		for _, m := range allMappings {
			if m != nil && m.ID != "" {
				mappingMap[m.ID] = m
			}
		}
		result := make([]*models.PortMapping, 0, len(mappingMap))
		for _, m := range mappingMap {
			result = append(result, m)
		}
		return result, nil
	}

	// 2. 索引查询失败或为空，从持久存储按字段查询
	stor := r.GetStorage()
	if hybridStorage, ok := stor.(interface {
		GetPersistentStorage() storage.PersistentStorage
	}); ok {
		persistent := hybridStorage.GetPersistentStorage()
		if persistent != nil {
			// 查询 ListenClientID 匹配的映射
			clientIDInt64 := int64(0)
			if id, err := random.StringToInt64(clientID); err == nil {
				clientIDInt64 = id
			}

			mappingMap := make(map[string]*models.PortMapping)

			// 查询 listen_client_id 匹配的映射
			jsonStrs, err := persistent.QueryByField(constants.KeyPrefixPortMapping+":", "listen_client_id", clientIDInt64)
			if err == nil {
				for _, jsonStr := range jsonStrs {
					var mapping models.PortMapping
					if err := json.Unmarshal([]byte(jsonStr), &mapping); err == nil && mapping.ID != "" {
						mappingMap[mapping.ID] = &mapping
					}
				}
			}

			// 查询 target_client_id 匹配的映射
			jsonStrs, err = persistent.QueryByField(constants.KeyPrefixPortMapping+":", "target_client_id", clientIDInt64)
			if err == nil {
				for _, jsonStr := range jsonStrs {
					var mapping models.PortMapping
					if err := json.Unmarshal([]byte(jsonStr), &mapping); err == nil && mapping.ID != "" {
						mappingMap[mapping.ID] = &mapping
					}
				}
			}

			// 转换为列表
			if len(mappingMap) > 0 {
				result := make([]*models.PortMapping, 0, len(mappingMap))
				for _, m := range mappingMap {
					result = append(result, m)
				}
				return result, nil
			}
		}
	}

	// 3. 持久存储查询也失败，返回空列表
	return []*models.PortMapping{}, nil
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
