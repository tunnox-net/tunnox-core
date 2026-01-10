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
// 同时清理所有相关索引（client_mappings, mapping_list, user_mappings）
//
// 即使主数据不存在，也会尝试清理索引，以处理数据不一致的情况
func (r *PortMappingRepo) DeletePortMapping(mappingID string) error {
	// 先获取 mapping 信息，用于清理索引
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		// 主数据不存在，尝试从索引中清理残留数据
		// 这种情况可能发生在：主数据被删除但索引未清理，或数据不一致
		return r.cleanupOrphanedMappingFromIndexes(mappingID)
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

	// 清理用户映射索引
	if mapping.UserID != "" {
		userKey := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, mapping.UserID)
		r.RemoveFromList(mapping, userKey)
	}

	// 最后删除主 key
	return r.Delete(mappingID, constants.KeyPrefixPortMapping)
}

// cleanupOrphanedMappingFromIndexes 清理孤立的 mapping 索引
// 当主数据不存在但索引中可能存在残留时调用
//
// 搜索策略：依次从全局映射列表、所有用户映射列表中查找残留数据
func (r *PortMappingRepo) cleanupOrphanedMappingFromIndexes(mappingID string) error {
	listStore, ok := r.GetStorage().(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}

	// 辅助函数：从列表中查找并清理指定 mapping
	findAndCleanup := func(listKey string) *models.PortMapping {
		listData, err := listStore.GetList(listKey)
		if err != nil {
			return nil
		}
		for _, item := range listData {
			if itemStr, ok := item.(string); ok {
				var mapping models.PortMapping
				if json.Unmarshal([]byte(itemStr), &mapping) == nil && mapping.ID == mappingID {
					return &mapping
				}
			}
		}
		return nil
	}

	// 1. 先从全局映射列表查找
	if mapping := findAndCleanup(constants.KeyPrefixMappingList); mapping != nil {
		r.cleanupMappingIndexes(mapping)
		return nil
	}

	// 2. 从所有用户映射列表查找（遍历所有用户索引）
	// 获取所有以 user_mappings: 开头的 key
	if keyLister, ok := r.GetStorage().(interface{ Keys(prefix string) ([]string, error) }); ok {
		keys, err := keyLister.Keys(constants.KeyPrefixUserMappings + ":")
		if err == nil {
			for _, key := range keys {
				if mapping := findAndCleanup(key); mapping != nil {
					r.cleanupMappingIndexes(mapping)
					return nil
				}
			}
		}
	}

	// 3. 从所有客户端映射列表查找
	if keyLister, ok := r.GetStorage().(interface{ Keys(prefix string) ([]string, error) }); ok {
		keys, err := keyLister.Keys(constants.KeyPrefixClientMappings + ":")
		if err == nil {
			for _, key := range keys {
				if mapping := findAndCleanup(key); mapping != nil {
					r.cleanupMappingIndexes(mapping)
					return nil
				}
			}
		}
	}

	// 未找到任何残留数据，返回原始的 not found 错误
	return coreerrors.Newf(coreerrors.CodeNotFound, "mapping %s not found", mappingID)
}

// cleanupMappingIndexes 清理 mapping 的所有索引（内部方法）
func (r *PortMappingRepo) cleanupMappingIndexes(mapping *models.PortMapping) {
	// 清理全局映射列表
	r.RemoveFromList(mapping, constants.KeyPrefixMappingList)

	// 清理 ListenClientID 索引
	if mapping.ListenClientID != 0 {
		clientKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, random.Int64ToString(mapping.ListenClientID))
		r.RemoveFromList(mapping, clientKey)
	}

	// 清理 TargetClientID 索引
	if mapping.TargetClientID != 0 && mapping.TargetClientID != mapping.ListenClientID {
		clientKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, random.Int64ToString(mapping.TargetClientID))
		r.RemoveFromList(mapping, clientKey)
	}

	// 清理用户映射索引
	if mapping.UserID != "" {
		userKey := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, mapping.UserID)
		r.RemoveFromList(mapping, userKey)
	}
}

// CleanupMappingIndexesByData 根据已知的 mapping 数据清理索引
// 当主数据不存在但我们有完整的 mapping 信息时使用
// 注意：这个方法通过遍历列表按 ID 匹配删除，而不是完整对象匹配
func (r *PortMappingRepo) CleanupMappingIndexesByData(mapping *models.PortMapping) error {
	if mapping == nil || mapping.ID == "" {
		return coreerrors.New(coreerrors.CodeNotFound, "mapping or mapping ID is empty")
	}
	r.cleanupMappingIndexesByID(mapping.ID, mapping.UserID, mapping.ListenClientID, mapping.TargetClientID)
	return nil
}

// cleanupMappingIndexesByID 通过 ID 匹配清理索引
// 与 cleanupMappingIndexes 不同，这个方法遍历列表并按 ID 匹配删除，
// 适用于只有部分 mapping 信息的场景
func (r *PortMappingRepo) cleanupMappingIndexesByID(mappingID, userID string, listenClientID, targetClientID int64) {
	// 辅助函数：从列表中按 ID 删除 mapping
	removeByID := func(listKey string) {
		listStore, ok := r.GetStorage().(storage.ListStore)
		if !ok {
			return
		}
		listData, err := listStore.GetList(listKey)
		if err != nil {
			return
		}

		// 过滤掉匹配的 mapping
		newList := make([]interface{}, 0, len(listData))
		for _, item := range listData {
			if itemStr, ok := item.(string); ok {
				var m models.PortMapping
				if json.Unmarshal([]byte(itemStr), &m) == nil && m.ID == mappingID {
					// 找到匹配项，跳过不添加
					continue
				}
			}
			newList = append(newList, item)
		}

		// 如果有变化，重写列表
		if len(newList) < len(listData) {
			// 使用 Set 而不是 SetList，因为需要保持 TTL
			if setter, ok := r.GetStorage().(interface{ Set(key string, value interface{}, ttl time.Duration) error }); ok {
				setter.Set(listKey, newList, 0) // TTL 0 表示不过期
			}
		}
	}

	// 清理全局映射列表
	removeByID(constants.KeyPrefixMappingList)

	// 清理 ListenClientID 索引
	if listenClientID != 0 {
		clientKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, random.Int64ToString(listenClientID))
		removeByID(clientKey)
	}

	// 清理 TargetClientID 索引
	if targetClientID != 0 && targetClientID != listenClientID {
		clientKey := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, random.Int64ToString(targetClientID))
		removeByID(clientKey)
	}

	// 清理用户映射索引
	if userID != "" {
		userKey := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, userID)
		removeByID(userKey)
	}
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
