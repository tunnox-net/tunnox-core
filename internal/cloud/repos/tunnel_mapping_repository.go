package repos

import (
	"encoding/json"
	"errors"
	"fmt"
	
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/storage"
)

// TunnelMappingRepository 隧道映射仓库
//
// 职责：
//   - 管理TunnelMapping的CRUD操作
//   - 提供按ID、ListenClient、TargetClient查询
//   - 自动处理TTL过期
//   - 维护索引（ListenClient → Mappings, TargetClient → Mappings）
type TunnelMappingRepository struct {
	*Repository
}

// NewTunnelMappingRepository 创建隧道映射仓库
func NewTunnelMappingRepository(repo *Repository) *TunnelMappingRepository {
	return &TunnelMappingRepository{
		Repository: repo,
	}
}

// Create 创建隧道映射
//
// 操作：
//  1. 验证数据完整性
//  2. 存储到Redis（按ID）
//  3. 添加到ListenClient和TargetClient的索引列表
//  4. 设置TTL自动过期
func (r *TunnelMappingRepository) Create(mapping *models.TunnelMapping) error {
	// 1. 验证
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid tunnel mapping: %w", err)
	}
	
	// 2. 序列化
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel mapping: %w", err)
	}
	
	// 3. 存储，设置TTL
	ttl := mapping.Duration
	
	keyByID := constants.KeyPrefixRuntimeTunnelMappingByID + mapping.ID
	if err := r.storage.Set(keyByID, string(data), ttl); err != nil {
		return fmt.Errorf("failed to store tunnel mapping: %w", err)
	}
	
	// 4. 添加到ListenClient的索引列表
	listenIndexKey := constants.KeyPrefixIndexTunnelMappingByListen + fmt.Sprintf("%d", mapping.ListenClientID)
	if err := r.storage.AppendToList(listenIndexKey, mapping.ID); err != nil {
		// 回滚
		_ = r.storage.Delete(keyByID)
		return fmt.Errorf("failed to add to listen client index: %w", err)
	}
	
	// 5. 添加到TargetClient的索引列表
	targetIndexKey := constants.KeyPrefixIndexTunnelMappingByTarget + fmt.Sprintf("%d", mapping.TargetClientID)
	if err := r.storage.AppendToList(targetIndexKey, mapping.ID); err != nil {
		// 回滚
		_ = r.storage.Delete(keyByID)
		_ = r.storage.RemoveFromList(listenIndexKey, mapping.ID)
		return fmt.Errorf("failed to add to target client index: %w", err)
	}
	
	return nil
}

// GetByID 按ID查询
//
// 用于获取映射详情
func (r *TunnelMappingRepository) GetByID(id string) (*models.TunnelMapping, error) {
	key := constants.KeyPrefixRuntimeTunnelMappingByID + id
	
	data, err := r.storage.Get(key)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tunnel mapping: %w", err)
	}
	
	// 类型断言
	dataStr, ok := data.(string)
	if !ok || dataStr == "" {
		return nil, fmt.Errorf("unexpected data type for tunnel mapping")
	}
	
	var mapping models.TunnelMapping
	if err := json.Unmarshal([]byte(dataStr), &mapping); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tunnel mapping: %w", err)
	}
	
	return &mapping, nil
}

// ListByListenClient 查询ListenClient的所有映射（出站映射）
//
// 返回指定ListenClient创建的所有映射（我在访问谁）
func (r *TunnelMappingRepository) ListByListenClient(listenClientID int64) ([]*models.TunnelMapping, error) {
	indexKey := constants.KeyPrefixIndexTunnelMappingByListen + fmt.Sprintf("%d", listenClientID)
	
	// 1. 获取ID列表
	ids, err := r.storage.GetList(indexKey)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return []*models.TunnelMapping{}, nil
		}
		return nil, fmt.Errorf("failed to get listen client index: %w", err)
	}
	
	// 2. 批量查询映射
	mappings := make([]*models.TunnelMapping, 0, len(ids))
	for _, idInterface := range ids {
		idStr, ok := idInterface.(string)
		if !ok {
			continue // 跳过无效的ID
		}
		
		mapping, err := r.GetByID(idStr)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// 映射可能已过期自动删除，从索引中移除
				_ = r.storage.RemoveFromList(indexKey, idStr)
				continue
			}
			return nil, fmt.Errorf("failed to get tunnel mapping %s: %w", idStr, err)
		}
		mappings = append(mappings, mapping)
	}
	
	return mappings, nil
}

// ListByTargetClient 查询TargetClient的所有映射（入站映射）
//
// 返回访问指定TargetClient的所有映射（谁在访问我）
func (r *TunnelMappingRepository) ListByTargetClient(targetClientID int64) ([]*models.TunnelMapping, error) {
	indexKey := constants.KeyPrefixIndexTunnelMappingByTarget + fmt.Sprintf("%d", targetClientID)
	
	// 1. 获取ID列表
	ids, err := r.storage.GetList(indexKey)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return []*models.TunnelMapping{}, nil
		}
		return nil, fmt.Errorf("failed to get target client index: %w", err)
	}
	
	// 2. 批量查询映射
	mappings := make([]*models.TunnelMapping, 0, len(ids))
	for _, idInterface := range ids {
		idStr, ok := idInterface.(string)
		if !ok {
			continue // 跳过无效的ID
		}
		
		mapping, err := r.GetByID(idStr)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// 映射可能已过期自动删除，从索引中移除
				_ = r.storage.RemoveFromList(indexKey, idStr)
				continue
			}
			return nil, fmt.Errorf("failed to get tunnel mapping %s: %w", idStr, err)
		}
		mappings = append(mappings, mapping)
	}
	
	return mappings, nil
}

// Update 更新映射
//
// 用于撤销、更新统计等操作后更新状态
func (r *TunnelMappingRepository) Update(mapping *models.TunnelMapping) error {
	// 1. 验证
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid tunnel mapping: %w", err)
	}
	
	// 2. 序列化
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel mapping: %w", err)
	}
	
	
	// 3. 计算剩余TTL
	ttl := mapping.TimeRemaining()
	if ttl <= 0 {
		// 已过期，直接删除
		return r.Delete(mapping.ID)
	}
	
	
	// 4. 更新
	keyByID := constants.KeyPrefixRuntimeTunnelMappingByID + mapping.ID
	if err := r.storage.Set(keyByID, string(data), ttl); err != nil {
		return fmt.Errorf("failed to update tunnel mapping: %w", err)
	}
	
	return nil
}

// UpdateUsage 更新使用统计
//
// 批量更新：使用次数、最后使用时间、流量统计
// 为性能优化，使用单独方法避免整个对象的序列化
func (r *TunnelMappingRepository) UpdateUsage(id string) error {
	// 获取当前映射
	mapping, err := r.GetByID(id)
	if err != nil {
		return err
	}
	
	// 更新使用统计
	mapping.RecordUsage()
	
	// 保存
	return r.Update(mapping)
}

// UpdateTraffic 更新流量统计
//
// 用于隧道连接关闭时更新流量统计
func (r *TunnelMappingRepository) UpdateTraffic(id string, bytesSent, bytesReceived int64) error {
	// 获取当前映射
	mapping, err := r.GetByID(id)
	if err != nil {
		return err
	}
	
	// 更新流量统计
	mapping.UpdateTraffic(bytesSent, bytesReceived)
	
	// 保存
	return r.Update(mapping)
}

// Delete 删除映射
//
// 删除所有相关数据：按ID存储、ListenClient索引、TargetClient索引
func (r *TunnelMappingRepository) Delete(id string) error {
	
	// 1. 先获取映射，以便获取ListenClientID和TargetClientID
	mapping, err := r.GetByID(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // 已删除，视为成功
		}
		return fmt.Errorf("failed to get tunnel mapping for deletion: %w", err)
	}
	
	// 2. 删除按ID存储
	keyByID := constants.KeyPrefixRuntimeTunnelMappingByID + mapping.ID
	if err := r.storage.Delete(keyByID); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete tunnel mapping: %w", err)
	}
	
	// 3. 从ListenClient的索引列表中移除
	listenIndexKey := constants.KeyPrefixIndexTunnelMappingByListen + fmt.Sprintf("%d", mapping.ListenClientID)
	if err := r.storage.RemoveFromList(listenIndexKey, mapping.ID); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return fmt.Errorf("failed to remove from listen client index: %w", err)
	}
	
	// 4. 从TargetClient的索引列表中移除
	targetIndexKey := constants.KeyPrefixIndexTunnelMappingByTarget + fmt.Sprintf("%d", mapping.TargetClientID)
	if err := r.storage.RemoveFromList(targetIndexKey, mapping.ID); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return fmt.Errorf("failed to remove from target client index: %w", err)
	}
	
	return nil
}

// CountByListenClient 统计ListenClient的映射数量
//
// 包括所有状态的映射（活跃、已撤销、已过期）
func (r *TunnelMappingRepository) CountByListenClient(listenClientID int64) (int, error) {
	mappings, err := r.ListByListenClient(listenClientID)
	if err != nil {
		return 0, err
	}
	return len(mappings), nil
}

// CountByTargetClient 统计TargetClient的映射数量
//
// 包括所有状态的映射（活跃、已撤销、已过期）
func (r *TunnelMappingRepository) CountByTargetClient(targetClientID int64) (int, error) {
	mappings, err := r.ListByTargetClient(targetClientID)
	if err != nil {
		return 0, err
	}
	return len(mappings), nil
}

// CountActiveByListenClient 统计ListenClient的活跃映射数量
//
// 只统计有效的映射
func (r *TunnelMappingRepository) CountActiveByListenClient(listenClientID int64) (int, error) {
	mappings, err := r.ListByListenClient(listenClientID)
	if err != nil {
		return 0, err
	}
	
	count := 0
	for _, mapping := range mappings {
		if mapping.IsValid() {
			count++
		}
	}
	
	return count, nil
}

// CountActiveByTargetClient 统计TargetClient的活跃映射数量
//
// 只统计有效的映射
func (r *TunnelMappingRepository) CountActiveByTargetClient(targetClientID int64) (int, error) {
	mappings, err := r.ListByTargetClient(targetClientID)
	if err != nil {
		return 0, err
	}
	
	count := 0
	for _, mapping := range mappings {
		if mapping.IsValid() {
			count++
		}
	}
	
	return count, nil
}

