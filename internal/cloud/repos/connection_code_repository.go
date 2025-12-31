package repos

import (
	"encoding/json"
	"errors"
	"fmt"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
)

// 编译时接口断言，确保 ConnectionCodeRepository 实现了 IConnectionCodeRepository 接口
var _ IConnectionCodeRepository = (*ConnectionCodeRepository)(nil)

// ConnectionCodeRepository 连接码仓库
//
// 职责：
//   - 管理TunnelConnectionCode的CRUD操作
//   - 提供按Code、ID、TargetClient查询
//   - 自动处理TTL过期
//   - 维护索引（TargetClient → ConnectionCodes）
type ConnectionCodeRepository struct {
	*Repository
}

// NewConnectionCodeRepository 创建连接码仓库
func NewConnectionCodeRepository(repo *Repository) *ConnectionCodeRepository {
	return &ConnectionCodeRepository{
		Repository: repo,
	}
}

// Create 创建连接码
//
// 操作：
//  1. 验证数据完整性
//  2. 存储到两个位置（按Code和按ID）
//  3. 添加到TargetClient的索引列表
//  4. 设置TTL自动过期
func (r *ConnectionCodeRepository) Create(code *models.TunnelConnectionCode) error {
	// 1. 验证
	if err := code.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid connection code")
	}

	// 2. 序列化
	data, err := json.Marshal(code)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal connection code")
	}

	// 3. 存储到两个位置，设置TTL
	ttl := code.ActivationTTL

	// 3.1 按Code存储（用于快速激活）
	keyByCode := constants.KeyPrefixRuntimeConnectionCodeByCode + code.Code
	if err := r.storage.Set(keyByCode, string(data), ttl); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to store connection code by code")
	}

	// 3.2 按ID存储（用于管理）
	keyByID := constants.KeyPrefixRuntimeConnectionCodeByID + code.ID
	if err := r.storage.Set(keyByID, string(data), ttl); err != nil {
		// 回滚：删除按Code的存储（忽略删除错误，主流程已失败）
		_ = r.storage.Delete(keyByCode)
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to store connection code by ID")
	}

	// 4. 添加到TargetClient的索引列表
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}
	indexKey := constants.KeyPrefixIndexConnectionCodeByTarget + fmt.Sprintf("%d", code.TargetClientID)
	if err := listStore.AppendToList(indexKey, code.ID); err != nil {
		// 回滚：删除已存储的数据（忽略删除错误，主流程已失败）
		_ = r.storage.Delete(keyByCode)
		_ = r.storage.Delete(keyByID)
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to add to target client index")
	}

	return nil
}

// GetByCode 按连接码查询
//
// 用于激活流程，返回连接码详情
func (r *ConnectionCodeRepository) GetByCode(code string) (*models.TunnelConnectionCode, error) {
	key := constants.KeyPrefixRuntimeConnectionCodeByCode + code

	data, err := r.storage.Get(key)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get connection code by code")
	}

	// 类型断言
	dataStr, ok := data.(string)
	if !ok || dataStr == "" {
		return nil, coreerrors.New(coreerrors.CodeInvalidData, "unexpected data type for connection code")
	}

	var connCode models.TunnelConnectionCode
	if err := json.Unmarshal([]byte(dataStr), &connCode); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal connection code")
	}

	return &connCode, nil
}

// GetByID 按ID查询
//
// 用于管理流程，返回连接码详情
func (r *ConnectionCodeRepository) GetByID(id string) (*models.TunnelConnectionCode, error) {
	key := constants.KeyPrefixRuntimeConnectionCodeByID + id

	data, err := r.storage.Get(key)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get connection code by ID")
	}

	// 类型断言
	dataStr, ok := data.(string)
	if !ok || dataStr == "" {
		return nil, coreerrors.New(coreerrors.CodeInvalidData, "unexpected data type for connection code")
	}

	var connCode models.TunnelConnectionCode
	if err := json.Unmarshal([]byte(dataStr), &connCode); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal connection code")
	}

	return &connCode, nil
}

// ListByTargetClient 查询TargetClient的所有连接码
//
// 返回指定TargetClient生成的所有连接码（包括活跃、已使用、已过期）
func (r *ConnectionCodeRepository) ListByTargetClient(targetClientID int64) ([]*models.TunnelConnectionCode, error) {
	indexKey := constants.KeyPrefixIndexConnectionCodeByTarget + fmt.Sprintf("%d", targetClientID)

	// 1. 获取ID列表
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}
	ids, err := listStore.GetList(indexKey)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return []*models.TunnelConnectionCode{}, nil
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get target client index")
	}

	// 2. 批量查询连接码
	codes := make([]*models.TunnelConnectionCode, 0, len(ids))
	for _, idInterface := range ids {
		idStr, ok := idInterface.(string)
		if !ok {
			continue // 跳过无效的ID
		}

		code, err := r.GetByID(idStr)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// 连接码可能已过期自动删除，从索引中移除（忽略移除错误，不影响主流程）
				_ = listStore.RemoveFromList(indexKey, idStr)
				continue
			}
			return nil, coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to get connection code %s", idStr)
		}
		codes = append(codes, code)
	}

	return codes, nil
}

// Update 更新连接码
//
// 用于激活、撤销等操作后更新状态
func (r *ConnectionCodeRepository) Update(code *models.TunnelConnectionCode) error {
	// 1. 验证
	if err := code.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid connection code")
	}

	// 2. 序列化
	data, err := json.Marshal(code)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal connection code")
	}

	// 3. 计算剩余TTL
	ttl := code.TimeRemaining()
	if ttl <= 0 {
		// 已过期，直接删除
		return r.Delete(code.ID)
	}

	// 4. 更新两个位置
	keyByCode := constants.KeyPrefixRuntimeConnectionCodeByCode + code.Code
	if err := r.storage.Set(keyByCode, string(data), ttl); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update connection code by code")
	}

	keyByID := constants.KeyPrefixRuntimeConnectionCodeByID + code.ID
	if err := r.storage.Set(keyByID, string(data), ttl); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update connection code by ID")
	}

	return nil
}

// Delete 删除连接码
//
// 删除所有相关数据：按Code存储、按ID存储、索引
func (r *ConnectionCodeRepository) Delete(id string) error {
	// 1. 先获取连接码，以便获取Code和TargetClientID
	code, err := r.GetByID(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // 已删除，视为成功
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get connection code for deletion")
	}

	// 2. 删除按Code存储
	keyByCode := constants.KeyPrefixRuntimeConnectionCodeByCode + code.Code
	if err := r.storage.Delete(keyByCode); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete connection code by code")
	}

	// 3. 删除按ID存储
	keyByID := constants.KeyPrefixRuntimeConnectionCodeByID + code.ID
	if err := r.storage.Delete(keyByID); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete connection code by ID")
	}

	// 4. 从TargetClient的索引列表中移除
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}
	indexKey := constants.KeyPrefixIndexConnectionCodeByTarget + fmt.Sprintf("%d", code.TargetClientID)
	if err := listStore.RemoveFromList(indexKey, code.ID); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to remove from target client index")
	}

	return nil
}

// CountByTargetClient 统计TargetClient的连接码数量
//
// 包括所有状态的连接码（活跃、已使用、已过期）
func (r *ConnectionCodeRepository) CountByTargetClient(targetClientID int64) (int, error) {
	codes, err := r.ListByTargetClient(targetClientID)
	if err != nil {
		return 0, err
	}
	return len(codes), nil
}

// CountActiveByTargetClient 统计TargetClient的活跃连接码数量
//
// 只统计可用于激活的连接码
func (r *ConnectionCodeRepository) CountActiveByTargetClient(targetClientID int64) (int, error) {
	codes, err := r.ListByTargetClient(targetClientID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, code := range codes {
		if code.IsValidForActivation() {
			count++
		}
	}

	return count, nil
}
