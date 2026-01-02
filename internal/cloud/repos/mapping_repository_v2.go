// Package repos 提供数据访问层实现
package repos

import (
	"context"
	"fmt"
	"time"

	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/repository/index"
	"tunnox-core/internal/core/repository/indexed"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// PortMappingRepositoryV2 使用新存储架构的端口映射 Repository
// =============================================================================

// 编译时接口验证
var _ IPortMappingRepository = (*PortMappingRepositoryV2)(nil)

// PortMappingRepositoryV2 端口映射 Repository（新架构版本）
//
// 特点：
//   - 用户索引：快速 GetUserPortMappings
//   - 客户端索引：快速 GetClientPortMappings（双索引：Listen + Target）
//   - 域名索引：快速 GetPortMappingByDomain
type PortMappingRepositoryV2 struct {
	// baseRepo 底层带索引的 Repository
	baseRepo *indexed.UserIndexedRepository[*models.PortMapping]

	// clientIndexStore 客户端→映射索引存储
	clientIndexStore store.SetStore[string, string]

	// domainIndexStore 域名→映射索引存储（用于 HTTP 映射）
	domainIndexStore store.Store[string, string]

	// globalListStore 全局映射列表存储
	globalListStore store.SetStore[string, string]

	// ctx 操作上下文
	ctx context.Context
}

// PortMappingRepoV2Config 创建 PortMappingRepositoryV2 的配置
type PortMappingRepoV2Config struct {
	// CachedStore 缓存+持久化组合存储
	CachedStore store.CachedPersistentStore[string, *models.PortMapping]

	// UserIndexStore 用户索引存储（Redis SET）
	UserIndexStore store.SetStore[string, string]

	// ClientIndexStore 客户端索引存储（Redis SET）
	ClientIndexStore store.SetStore[string, string]

	// DomainIndexStore 域名索引存储（域名→映射ID）
	DomainIndexStore store.Store[string, string]

	// GlobalListStore 全局列表存储
	GlobalListStore store.SetStore[string, string]

	// Ctx 操作上下文
	Ctx context.Context
}

// NewPortMappingRepositoryV2 创建新版本的 PortMapping Repository
func NewPortMappingRepositoryV2(cfg PortMappingRepoV2Config) *PortMappingRepositoryV2 {
	// 创建用户索引管理器
	userIndexManager := index.NewUserEntityIndexManager[*models.PortMapping](
		cfg.UserIndexStore,
		constants.KeyPrefixIndexUserClients, // 复用用户索引前缀
		func(mapping *models.PortMapping) string {
			return mapping.GetUserID()
		},
	)

	// 创建带索引的 Repository
	baseRepo := indexed.NewUserIndexedRepository[*models.PortMapping](
		cfg.CachedStore,
		userIndexManager,
		constants.KeyPrefixPortMapping,
		"PortMapping",
	)

	return &PortMappingRepositoryV2{
		baseRepo:         baseRepo,
		clientIndexStore: cfg.ClientIndexStore,
		domainIndexStore: cfg.DomainIndexStore,
		globalListStore:  cfg.GlobalListStore,
		ctx:              cfg.Ctx,
	}
}

// =============================================================================
// IPortMappingRepository 接口实现
// =============================================================================

// SavePortMapping 保存端口映射（创建或更新）
func (r *PortMappingRepositoryV2) SavePortMapping(mapping *models.PortMapping) error {
	if mapping == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "mapping is nil")
	}

	// 更新时间戳
	mapping.UpdatedAt = time.Now()

	// 检查是否已存在
	exists, err := r.baseRepo.Exists(r.ctx, mapping.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "check exists failed")
	}

	if exists {
		return r.UpdatePortMapping(mapping)
	}
	return r.CreatePortMapping(mapping)
}

// CreatePortMapping 创建新端口映射
func (r *PortMappingRepositoryV2) CreatePortMapping(mapping *models.PortMapping) error {
	if mapping == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "mapping is nil")
	}

	// 设置时间戳
	now := time.Now()
	mapping.CreatedAt = now
	mapping.UpdatedAt = now

	// 创建（含用户索引更新）
	if err := r.baseRepo.Create(r.ctx, mapping); err != nil {
		if store.IsAlreadyExists(err) {
			return coreerrors.Newf(coreerrors.CodeAlreadyExists, "mapping already exists: %s", mapping.ID)
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "create mapping failed")
	}

	// 更新客户端索引
	r.addClientIndexes(mapping)

	// 更新域名索引
	r.addDomainIndex(mapping)

	// 添加到全局列表
	r.addToGlobalList(mapping)

	return nil
}

// UpdatePortMapping 更新端口映射
func (r *PortMappingRepositoryV2) UpdatePortMapping(mapping *models.PortMapping) error {
	if mapping == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "mapping is nil")
	}

	// 获取旧映射（用于索引更新）
	oldMapping, err := r.GetPortMapping(mapping.ID)
	if err != nil {
		return err
	}

	// 更新时间戳
	mapping.UpdatedAt = time.Now()

	// 更新（含用户索引更新）
	if err := r.baseRepo.Update(r.ctx, mapping); err != nil {
		if store.IsNotFound(err) {
			return coreerrors.Newf(coreerrors.CodeNotFound, "mapping not found: %s", mapping.ID)
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "update mapping failed")
	}

	// 更新客户端索引（如果客户端ID变化）
	if oldMapping.ListenClientID != mapping.ListenClientID ||
		oldMapping.TargetClientID != mapping.TargetClientID {
		r.removeClientIndexes(oldMapping)
		r.addClientIndexes(mapping)
	}

	// 更新域名索引（如果域名变化）
	if oldMapping.FullDomain() != mapping.FullDomain() {
		r.removeDomainIndex(oldMapping)
		r.addDomainIndex(mapping)
	}

	return nil
}

// GetPortMapping 获取端口映射
func (r *PortMappingRepositoryV2) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	mapping, err := r.baseRepo.Get(r.ctx, mappingID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, coreerrors.Newf(coreerrors.CodeNotFound, "mapping not found: %s", mappingID)
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "get mapping failed")
	}
	return mapping, nil
}

// GetPortMappingByDomain 通过域名查找 HTTP 映射
func (r *PortMappingRepositoryV2) GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) {
	if r.domainIndexStore == nil {
		// 降级到扫描方式
		return r.getPortMappingByDomainScan(fullDomain)
	}

	// 从域名索引获取映射ID
	domainKey := r.buildDomainIndexKey(fullDomain)
	mappingID, err := r.domainIndexStore.Get(r.ctx, domainKey)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, coreerrors.Newf(coreerrors.CodeNotFound, "mapping not found for domain: %s", fullDomain)
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "get domain index failed")
	}

	return r.GetPortMapping(mappingID)
}

// getPortMappingByDomainScan 扫描方式查找域名映射（降级方案）
func (r *PortMappingRepositoryV2) getPortMappingByDomainScan(fullDomain string) (*models.PortMapping, error) {
	allMappings, err := r.ListAllMappings()
	if err != nil {
		return nil, err
	}

	for _, mapping := range allMappings {
		if mapping.Protocol == models.ProtocolHTTP && mapping.FullDomain() == fullDomain {
			return mapping, nil
		}
	}

	return nil, coreerrors.Newf(coreerrors.CodeNotFound, "mapping not found for domain: %s", fullDomain)
}

// DeletePortMapping 删除端口映射
func (r *PortMappingRepositoryV2) DeletePortMapping(mappingID string) error {
	// 获取映射（用于清理索引）
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return nil // 不存在则视为删除成功
	}

	// 删除客户端索引
	r.removeClientIndexes(mapping)

	// 删除域名索引
	r.removeDomainIndex(mapping)

	// 从全局列表移除
	r.removeFromGlobalList(mapping)

	// 删除映射（含用户索引清理）
	if err := r.baseRepo.Delete(r.ctx, mappingID); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "delete mapping failed")
	}

	return nil
}

// UpdatePortMappingStatus 更新端口映射状态
func (r *PortMappingRepositoryV2) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	mapping.Status = status
	mapping.UpdatedAt = time.Now()

	return r.UpdatePortMapping(mapping)
}

// UpdatePortMappingStats 更新端口映射统计
func (r *PortMappingRepositoryV2) UpdatePortMappingStats(mappingID string, trafficStats *stats.TrafficStats) error {
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	if trafficStats != nil {
		mapping.TrafficStats = *trafficStats
	}
	mapping.UpdatedAt = time.Now()

	return r.UpdatePortMapping(mapping)
}

// GetUserPortMappings 列出用户的端口映射
func (r *PortMappingRepositoryV2) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	mappings, err := r.baseRepo.ListByUser(r.ctx, userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "list user mappings failed")
	}
	return mappings, nil
}

// GetClientPortMappings 列出客户端的端口映射
// 包括 ListenClientID 和 TargetClientID 匹配的映射
func (r *PortMappingRepositoryV2) GetClientPortMappings(clientID string) ([]*models.PortMapping, error) {
	if r.clientIndexStore == nil {
		// 降级到扫描方式
		return r.getClientPortMappingsScan(clientID)
	}

	// 从客户端索引获取映射ID列表
	indexKey := r.buildClientIndexKey(clientID)
	mappingIDs, err := r.clientIndexStore.Members(r.ctx, indexKey)
	if err != nil {
		if store.IsNotFound(err) {
			return []*models.PortMapping{}, nil
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "get client index failed")
	}

	if len(mappingIDs) == 0 {
		return []*models.PortMapping{}, nil
	}

	// 批量获取映射
	mappingMap, err := r.baseRepo.BatchGet(r.ctx, mappingIDs)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "batch get mappings failed")
	}

	mappings := make([]*models.PortMapping, 0, len(mappingMap))
	for _, mapping := range mappingMap {
		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

// getClientPortMappingsScan 扫描方式查找客户端映射（降级方案）
func (r *PortMappingRepositoryV2) getClientPortMappingsScan(clientID string) ([]*models.PortMapping, error) {
	allMappings, err := r.ListAllMappings()
	if err != nil {
		return nil, err
	}

	var clientIDInt64 int64
	fmt.Sscanf(clientID, "%d", &clientIDInt64)

	result := make([]*models.PortMapping, 0)
	for _, mapping := range allMappings {
		if mapping.ListenClientID == clientIDInt64 || mapping.TargetClientID == clientIDInt64 {
			result = append(result, mapping)
		}
	}

	return result, nil
}

// AddMappingToUser 添加映射到用户
func (r *PortMappingRepositoryV2) AddMappingToUser(userID string, mapping *models.PortMapping) error {
	// 用户索引由 baseRepo 自动管理
	return nil
}

// AddMappingToClient 添加映射到客户端
func (r *PortMappingRepositoryV2) AddMappingToClient(clientID string, mapping *models.PortMapping) error {
	if r.clientIndexStore == nil {
		return nil
	}
	indexKey := r.buildClientIndexKey(clientID)
	return r.clientIndexStore.Add(r.ctx, indexKey, mapping.ID)
}

// ListAllMappings 列出所有端口映射
func (r *PortMappingRepositoryV2) ListAllMappings() ([]*models.PortMapping, error) {
	if r.globalListStore == nil {
		return nil, coreerrors.New(coreerrors.CodeStorageError, "global list store not configured")
	}

	// 从全局列表获取所有 ID
	ids, err := r.globalListStore.Members(r.ctx, constants.KeyPrefixMappingList)
	if err != nil {
		if store.IsNotFound(err) {
			return []*models.PortMapping{}, nil
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "list mapping ids failed")
	}

	if len(ids) == 0 {
		return []*models.PortMapping{}, nil
	}

	// 批量获取映射
	mappingMap, err := r.baseRepo.BatchGet(r.ctx, ids)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "batch get mappings failed")
	}

	mappings := make([]*models.PortMapping, 0, len(mappingMap))
	for _, mapping := range mappingMap {
		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

// AddMappingToList 添加映射到全局映射列表
func (r *PortMappingRepositoryV2) AddMappingToList(mapping *models.PortMapping) error {
	return r.addToGlobalList(mapping)
}

// =============================================================================
// 索引管理辅助方法
// =============================================================================

// buildClientIndexKey 构建客户端索引键
func (r *PortMappingRepositoryV2) buildClientIndexKey(clientID string) string {
	return fmt.Sprintf("%s%s", constants.KeyPrefixIndexClientMappings, clientID)
}

// buildDomainIndexKey 构建域名索引键
func (r *PortMappingRepositoryV2) buildDomainIndexKey(domain string) string {
	return fmt.Sprintf("tunnox:index:domain:mapping:%s", domain)
}

// addClientIndexes 添加客户端索引
func (r *PortMappingRepositoryV2) addClientIndexes(mapping *models.PortMapping) {
	if r.clientIndexStore == nil {
		return
	}

	if mapping.ListenClientID != 0 {
		key := r.buildClientIndexKey(fmt.Sprintf("%d", mapping.ListenClientID))
		_ = r.clientIndexStore.Add(r.ctx, key, mapping.ID)
	}

	if mapping.TargetClientID != 0 && mapping.TargetClientID != mapping.ListenClientID {
		key := r.buildClientIndexKey(fmt.Sprintf("%d", mapping.TargetClientID))
		_ = r.clientIndexStore.Add(r.ctx, key, mapping.ID)
	}
}

// removeClientIndexes 移除客户端索引
func (r *PortMappingRepositoryV2) removeClientIndexes(mapping *models.PortMapping) {
	if r.clientIndexStore == nil {
		return
	}

	if mapping.ListenClientID != 0 {
		key := r.buildClientIndexKey(fmt.Sprintf("%d", mapping.ListenClientID))
		_ = r.clientIndexStore.Remove(r.ctx, key, mapping.ID)
	}

	if mapping.TargetClientID != 0 && mapping.TargetClientID != mapping.ListenClientID {
		key := r.buildClientIndexKey(fmt.Sprintf("%d", mapping.TargetClientID))
		_ = r.clientIndexStore.Remove(r.ctx, key, mapping.ID)
	}
}

// addDomainIndex 添加域名索引
func (r *PortMappingRepositoryV2) addDomainIndex(mapping *models.PortMapping) {
	if r.domainIndexStore == nil || mapping.Protocol != models.ProtocolHTTP {
		return
	}

	domain := mapping.FullDomain()
	if domain == "" {
		return
	}

	key := r.buildDomainIndexKey(domain)

	// 使用 SetWithTTL 如果支持，否则普通 Set
	if ttlStore, ok := r.domainIndexStore.(store.TTLStore[string, string]); ok {
		_ = ttlStore.SetWithTTL(r.ctx, key, mapping.ID, constants2.DefaultMappingDataTTL)
	} else {
		_ = r.domainIndexStore.Set(r.ctx, key, mapping.ID)
	}
}

// removeDomainIndex 移除域名索引
func (r *PortMappingRepositoryV2) removeDomainIndex(mapping *models.PortMapping) {
	if r.domainIndexStore == nil || mapping.Protocol != models.ProtocolHTTP {
		return
	}

	domain := mapping.FullDomain()
	if domain == "" {
		return
	}

	key := r.buildDomainIndexKey(domain)
	_ = r.domainIndexStore.Delete(r.ctx, key)
}

// addToGlobalList 添加到全局列表
func (r *PortMappingRepositoryV2) addToGlobalList(mapping *models.PortMapping) error {
	if r.globalListStore == nil {
		return nil
	}
	return r.globalListStore.Add(r.ctx, constants.KeyPrefixMappingList, mapping.ID)
}

// removeFromGlobalList 从全局列表移除
func (r *PortMappingRepositoryV2) removeFromGlobalList(mapping *models.PortMapping) {
	if r.globalListStore == nil {
		return
	}
	_ = r.globalListStore.Remove(r.ctx, constants.KeyPrefixMappingList, mapping.ID)
}

// =============================================================================
// 扩展方法
// =============================================================================

// BatchGetMappings 批量获取映射
func (r *PortMappingRepositoryV2) BatchGetMappings(mappingIDs []string) (map[string]*models.PortMapping, error) {
	if len(mappingIDs) == 0 {
		return map[string]*models.PortMapping{}, nil
	}

	return r.baseRepo.BatchGet(r.ctx, mappingIDs)
}

// CountUserMappings 统计用户的映射数量
func (r *PortMappingRepositoryV2) CountUserMappings(userID string) (int64, error) {
	return r.baseRepo.CountByUser(r.ctx, userID)
}

// GetCacheStats 获取缓存统计
func (r *PortMappingRepositoryV2) GetCacheStats() store.CacheStats {
	return r.baseRepo.GetCacheStats()
}

// GetMetrics 获取监控指标
func (r *PortMappingRepositoryV2) GetMetrics() *store.RepositoryMetrics {
	return r.baseRepo.GetMetrics()
}

// =============================================================================
// 接口验证
// =============================================================================

// 验证 PortMapping 实现了 UserOwnedEntity 接口
var _ repository.UserOwnedEntity = (*models.PortMapping)(nil)
