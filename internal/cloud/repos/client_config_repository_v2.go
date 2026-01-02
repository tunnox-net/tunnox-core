// Package repos 提供数据访问层实现
package repos

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/repository/index"
	"tunnox-core/internal/core/repository/indexed"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// ClientConfigRepositoryV2 使用新存储架构的客户端配置 Repository
// =============================================================================

// 编译时接口验证
var _ IClientConfigRepository = (*ClientConfigRepositoryV2)(nil)

// ClientConfigRepositoryV2 客户端配置 Repository（新架构版本）
//
// 相比 V1 版本的改进：
//   - ListUserConfigs 使用索引优化，从 O(n) 降到 O(k)
//   - 支持批量操作和缓存统计
//   - 更好的缓存穿透保护
type ClientConfigRepositoryV2 struct {
	// baseRepo 底层带索引的 Repository
	baseRepo *indexed.UserIndexedRepository[*models.ClientConfig]

	// globalListStore 全局配置列表存储（用于 ListConfigs）
	globalListStore store.SetStore[string, string]

	// ctx 操作上下文
	ctx context.Context
}

// ClientConfigRepoV2Config 创建 ClientConfigRepositoryV2 的配置
type ClientConfigRepoV2Config struct {
	// CachedStore 缓存+持久化组合存储
	CachedStore store.CachedPersistentStore[string, *models.ClientConfig]

	// IndexStore 用户索引存储（Redis SET）
	IndexStore store.SetStore[string, string]

	// GlobalListStore 全局列表存储（用于 ListConfigs）
	GlobalListStore store.SetStore[string, string]

	// Ctx 操作上下文
	Ctx context.Context
}

// NewClientConfigRepositoryV2 创建新版本的 ClientConfig Repository
//
// 使用新的存储架构，支持：
//   - 用户索引：快速 ListUserConfigs
//   - 缓存穿透保护：负缓存
//   - 批量操作：Pipeline 优化
func NewClientConfigRepositoryV2(cfg ClientConfigRepoV2Config) *ClientConfigRepositoryV2 {
	// 创建用户索引管理器
	indexManager := index.NewUserEntityIndexManager[*models.ClientConfig](
		cfg.IndexStore,
		constants.KeyPrefixIndexUserClients,
		func(config *models.ClientConfig) string {
			return config.GetUserID()
		},
	)

	// 创建带索引的 Repository
	baseRepo := indexed.NewUserIndexedRepository[*models.ClientConfig](
		cfg.CachedStore,
		indexManager,
		constants.KeyPrefixPersistClientConfig,
		"ClientConfig",
	)

	return &ClientConfigRepositoryV2{
		baseRepo:        baseRepo,
		globalListStore: cfg.GlobalListStore,
		ctx:             cfg.Ctx,
	}
}

// =============================================================================
// IClientConfigRepository 接口实现
// =============================================================================

// GetConfig 获取客户端配置
func (r *ClientConfigRepositoryV2) GetConfig(clientID int64) (*models.ClientConfig, error) {
	id := fmt.Sprintf("%d", clientID)
	config, err := r.baseRepo.Get(r.ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, coreerrors.Newf(coreerrors.CodeNotFound, "client config not found: %d", clientID)
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "get config failed")
	}
	return config, nil
}

// SaveConfig 保存客户端配置（创建或更新）
func (r *ClientConfigRepositoryV2) SaveConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 检查是否已存在
	exists, err := r.baseRepo.Exists(r.ctx, config.GetID())
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "check exists failed")
	}

	if exists {
		return r.UpdateConfig(config)
	}
	return r.CreateConfig(config)
}

// CreateConfig 创建新的客户端配置
func (r *ClientConfigRepositoryV2) CreateConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	// 设置时间戳
	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	// 创建（含索引更新）
	if err := r.baseRepo.Create(r.ctx, config); err != nil {
		if store.IsAlreadyExists(err) {
			return coreerrors.Newf(coreerrors.CodeAlreadyExists, "config already exists: %d", config.ID)
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "create config failed")
	}

	// 添加到全局列表
	if r.globalListStore != nil {
		_ = r.globalListStore.Add(r.ctx, constants.KeyPrefixPersistClientsList, config.GetID())
	}

	return nil
}

// UpdateConfig 更新客户端配置
func (r *ClientConfigRepositoryV2) UpdateConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 更新（含索引更新）
	if err := r.baseRepo.Update(r.ctx, config); err != nil {
		if store.IsNotFound(err) {
			return coreerrors.Newf(coreerrors.CodeNotFound, "config not found: %d", config.ID)
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "update config failed")
	}

	return nil
}

// DeleteConfig 删除客户端配置
func (r *ClientConfigRepositoryV2) DeleteConfig(clientID int64) error {
	id := fmt.Sprintf("%d", clientID)

	// 删除（含索引清理）
	if err := r.baseRepo.Delete(r.ctx, id); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "delete config failed")
	}

	// 从全局列表移除
	if r.globalListStore != nil {
		_ = r.globalListStore.Remove(r.ctx, constants.KeyPrefixPersistClientsList, id)
	}

	return nil
}

// ListConfigs 列出所有客户端配置
func (r *ClientConfigRepositoryV2) ListConfigs() ([]*models.ClientConfig, error) {
	if r.globalListStore == nil {
		return nil, coreerrors.New(coreerrors.CodeStorageError, "global list store not configured")
	}

	// 从全局列表获取所有 ID
	ids, err := r.globalListStore.Members(r.ctx, constants.KeyPrefixPersistClientsList)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "list config ids failed")
	}

	if len(ids) == 0 {
		return []*models.ClientConfig{}, nil
	}

	// 批量获取配置
	configMap, err := r.baseRepo.BatchGet(r.ctx, ids)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "batch get configs failed")
	}

	configs := make([]*models.ClientConfig, 0, len(configMap))
	for _, config := range configMap {
		configs = append(configs, config)
	}

	return configs, nil
}

// ListUserConfigs 列出用户的所有客户端配置
// 这是核心优化方法，使用索引实现 O(k) 查询
func (r *ClientConfigRepositoryV2) ListUserConfigs(userID string) ([]*models.ClientConfig, error) {
	configs, err := r.baseRepo.ListByUser(r.ctx, userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "list user configs failed")
	}
	return configs, nil
}

// AddConfigToList 将配置添加到全局列表
func (r *ClientConfigRepositoryV2) AddConfigToList(config *models.ClientConfig) error {
	if r.globalListStore == nil {
		return nil
	}
	return r.globalListStore.Add(r.ctx, constants.KeyPrefixPersistClientsList, config.GetID())
}

// ExistsConfig 检查配置是否存在
func (r *ClientConfigRepositoryV2) ExistsConfig(clientID int64) (bool, error) {
	id := fmt.Sprintf("%d", clientID)
	return r.baseRepo.Exists(r.ctx, id)
}

// =============================================================================
// 扩展方法
// =============================================================================

// BatchGetConfigs 批量获取配置
func (r *ClientConfigRepositoryV2) BatchGetConfigs(clientIDs []int64) (map[int64]*models.ClientConfig, error) {
	if len(clientIDs) == 0 {
		return map[int64]*models.ClientConfig{}, nil
	}

	ids := make([]string, len(clientIDs))
	for i, clientID := range clientIDs {
		ids[i] = fmt.Sprintf("%d", clientID)
	}

	configMap, err := r.baseRepo.BatchGet(r.ctx, ids)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "batch get configs failed")
	}

	result := make(map[int64]*models.ClientConfig, len(configMap))
	for _, config := range configMap {
		result[config.ID] = config
	}

	return result, nil
}

// CountUserConfigs 统计用户的配置数量
func (r *ClientConfigRepositoryV2) CountUserConfigs(userID string) (int64, error) {
	return r.baseRepo.CountByUser(r.ctx, userID)
}

// InvalidateCache 使指定配置的缓存失效
func (r *ClientConfigRepositoryV2) InvalidateCache(clientID int64) error {
	id := fmt.Sprintf("%d", clientID)
	return r.baseRepo.InvalidateCache(r.ctx, id)
}

// RefreshCache 刷新指定配置的缓存
func (r *ClientConfigRepositoryV2) RefreshCache(clientID int64) error {
	id := fmt.Sprintf("%d", clientID)
	return r.baseRepo.RefreshCache(r.ctx, id)
}

// GetCacheStats 获取缓存统计
func (r *ClientConfigRepositoryV2) GetCacheStats() store.CacheStats {
	return r.baseRepo.GetCacheStats()
}

// GetMetrics 获取监控指标
func (r *ClientConfigRepositoryV2) GetMetrics() *store.RepositoryMetrics {
	return r.baseRepo.GetMetrics()
}

// RebuildUserIndex 重建指定用户的索引
func (r *ClientConfigRepositoryV2) RebuildUserIndex(userID string) error {
	// 获取用户的所有配置
	configs, err := r.ListUserConfigs(userID)
	if err != nil {
		return err
	}

	// 通过索引管理器重建
	indexMgr := r.baseRepo.GetIndexManager()
	return indexMgr.RebuildIndex(r.ctx, configs)
}

// =============================================================================
// 接口验证
// =============================================================================

// 验证 ClientConfig 实现了 UserOwnedEntity 接口
var _ repository.UserOwnedEntity = (*models.ClientConfig)(nil)
