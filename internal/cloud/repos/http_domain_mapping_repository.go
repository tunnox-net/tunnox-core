package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
)

// 编译时接口断言
var _ IHTTPDomainMappingRepository = (*HTTPDomainMappingRepository)(nil)

// HTTPDomainMappingRepository HTTP 域名映射仓库，实现 IHTTPDomainMappingRepository 接口
type HTTPDomainMappingRepository struct {
	*Repository
	baseDomains []string // 支持的基础域名列表
}

// NewHTTPDomainMappingRepository 创建 HTTP 域名映射仓库
func NewHTTPDomainMappingRepository(repo *Repository, baseDomains []string) *HTTPDomainMappingRepository {
	// 如果未提供基础域名，使用默认值
	if len(baseDomains) == 0 {
		baseDomains = []string{"tunnox.net"}
	}

	return &HTTPDomainMappingRepository{
		Repository:  repo,
		baseDomains: baseDomains,
	}
}

// CheckSubdomainAvailable 检查子域名是否可用（true=可用，false=已占用）
func (r *HTTPDomainMappingRepository) CheckSubdomainAvailable(ctx context.Context, subdomain string, baseDomain string) (bool, error) {
	fullDomain := subdomain + "." + baseDomain
	indexKey := HTTPDomainIndexKey(fullDomain)
	exists, err := r.storage.Exists(indexKey)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check subdomain availability")
	}
	return !exists, nil
}

// CreateMapping 创建域名映射（原子操作，确保域名唯一性）
func (r *HTTPDomainMappingRepository) CreateMapping(ctx context.Context, clientID int64, subdomain, baseDomain, targetHost string, targetPort int) (*HTTPDomainMapping, error) {
	// 1. 验证基础域名是否在支持列表中
	if !r.isBaseDomainSupported(baseDomain) {
		return nil, coreerrors.Newf(coreerrors.CodeInvalidParam, "base domain %s is not supported", baseDomain)
	}

	// 2. 构造完整域名
	fullDomain := subdomain + "." + baseDomain

	// 3. 生成唯一映射 ID
	mappingID, err := r.generateMappingID(ctx)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to generate mapping ID")
	}

	// 4. 创建映射对象
	now := time.Now().Unix()
	mapping := &HTTPDomainMapping{
		ID:         mappingID,
		Subdomain:  subdomain,
		BaseDomain: baseDomain,
		FullDomain: fullDomain,
		ClientID:   clientID,
		TargetHost: targetHost,
		TargetPort: targetPort,
		Status:     HTTPDomainMappingStatusActive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// 5. 验证映射数据完整性
	if err := mapping.Validate(); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid mapping data")
	}

	// 6. 使用 SetNX 原子操作设置域名索引，确保域名唯一性
	casStore, ok := r.storage.(storage.CASStore)
	if !ok {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support CAS operations")
	}

	indexKey := HTTPDomainIndexKey(fullDomain)
	success, err := casStore.SetNX(indexKey, mappingID, 0)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to set domain index")
	}
	if !success {
		// 域名已被占用
		return nil, coreerrors.Newf(coreerrors.CodeAlreadyExists, "domain %s is already in use", fullDomain)
	}

	// 7. 序列化映射数据
	data, err := json.Marshal(mapping)
	if err != nil {
		// 回滚：删除域名索引
		_ = r.storage.Delete(indexKey)
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal mapping")
	}

	// 8. 存储映射数据
	mappingKey := HTTPDomainMappingKey(mappingID)
	if err := r.storage.Set(mappingKey, string(data), 0); err != nil {
		// 回滚：删除域名索引
		_ = r.storage.Delete(indexKey)
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to store mapping data")
	}

	// 9. 添加到客户端映射列表
	if err := r.addToClientMappingList(ctx, clientID, mappingID); err != nil {
		// 回滚：删除映射数据和域名索引
		_ = r.storage.Delete(mappingKey)
		_ = r.storage.Delete(indexKey)
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to add to client mapping list")
	}

	// 10. 添加到全局映射列表
	if err := r.addToGlobalMappingList(ctx, mappingID); err != nil {
		// 不回滚，全局列表只是辅助索引
		// 记录警告日志但不返回错误
	}

	return mapping, nil
}

// GetMapping 获取映射详情
func (r *HTTPDomainMappingRepository) GetMapping(ctx context.Context, mappingID string) (*HTTPDomainMapping, error) {
	mappingKey := HTTPDomainMappingKey(mappingID)

	data, err := r.storage.Get(mappingKey)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return nil, coreerrors.Newf(coreerrors.CodeMappingNotFound, "mapping %s not found", mappingID)
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get mapping")
	}

	dataStr, ok := data.(string)
	if !ok || dataStr == "" {
		return nil, coreerrors.New(coreerrors.CodeInvalidData, "unexpected data type for mapping")
	}

	var mapping HTTPDomainMapping
	if err := json.Unmarshal([]byte(dataStr), &mapping); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal mapping")
	}

	return &mapping, nil
}

// GetMappingsByClientID 获取客户端的所有映射
func (r *HTTPDomainMappingRepository) GetMappingsByClientID(ctx context.Context, clientID int64) ([]*HTTPDomainMapping, error) {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}

	clientKey := HTTPDomainClientKey(clientID)
	ids, err := listStore.GetList(clientKey)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return []*HTTPDomainMapping{}, nil
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client mapping list")
	}

	// 批量获取映射数据
	mappings := make([]*HTTPDomainMapping, 0, len(ids))
	for _, idInterface := range ids {
		idStr, ok := idInterface.(string)
		if !ok {
			continue // 跳过无效的 ID
		}

		mapping, err := r.GetMapping(ctx, idStr)
		if err != nil {
			if coreerrors.IsCode(err, coreerrors.CodeMappingNotFound) {
				// 映射可能已被删除，从客户端列表中移除
				_ = listStore.RemoveFromList(clientKey, idStr)
				continue
			}
			return nil, coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to get mapping %s", idStr)
		}
		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

// UpdateMapping 更新映射（只允许更新 TargetHost/TargetPort/Description/Status）
func (r *HTTPDomainMappingRepository) UpdateMapping(ctx context.Context, mapping *HTTPDomainMapping) error {
	// 1. 获取现有映射，验证存在性
	existingMapping, err := r.GetMapping(ctx, mapping.ID)
	if err != nil {
		return err
	}

	// 2. 验证不可变字段未被修改
	if mapping.Subdomain != existingMapping.Subdomain ||
		mapping.BaseDomain != existingMapping.BaseDomain ||
		mapping.FullDomain != existingMapping.FullDomain ||
		mapping.ClientID != existingMapping.ClientID {
		return coreerrors.New(coreerrors.CodeInvalidRequest, "cannot modify subdomain, base_domain, full_domain, or client_id")
	}

	// 3. 更新时间戳
	mapping.UpdatedAt = time.Now().Unix()

	// 4. 验证映射数据
	if err := mapping.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid mapping data")
	}

	// 5. 序列化并存储
	data, err := json.Marshal(mapping)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal mapping")
	}

	mappingKey := HTTPDomainMappingKey(mapping.ID)
	if err := r.storage.Set(mappingKey, string(data), 0); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update mapping")
	}

	return nil
}

// DeleteMapping 删除映射（级联删除映射数据、域名索引、客户端列表引用）
func (r *HTTPDomainMappingRepository) DeleteMapping(ctx context.Context, mappingID string, clientID int64) error {
	// 1. 获取映射，验证存在性和权限
	mapping, err := r.GetMapping(ctx, mappingID)
	if err != nil {
		if coreerrors.IsCode(err, coreerrors.CodeMappingNotFound) {
			return nil // 已删除，视为成功
		}
		return err
	}

	// 2. 验证权限（映射必须属于指定客户端）
	if mapping.ClientID != clientID {
		return coreerrors.Newf(coreerrors.CodeForbidden, "mapping %s does not belong to client %d", mappingID, clientID)
	}

	// 3. 删除域名索引
	indexKey := HTTPDomainIndexKey(mapping.FullDomain)
	if err := r.storage.Delete(indexKey); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete domain index")
	}

	// 4. 删除映射数据
	mappingKey := HTTPDomainMappingKey(mappingID)
	if err := r.storage.Delete(mappingKey); err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete mapping data")
	}

	// 5. 从客户端映射列表移除
	if err := r.removeFromClientMappingList(ctx, clientID, mappingID); err != nil {
		// 不阻塞删除操作，仅记录警告
	}

	// 6. 从全局映射列表移除
	if err := r.removeFromGlobalMappingList(ctx, mappingID); err != nil {
		// 不阻塞删除操作，仅记录警告
	}

	return nil
}

// LookupByDomain 根据完整域名查找映射（O(1) 查找）
func (r *HTTPDomainMappingRepository) LookupByDomain(ctx context.Context, fullDomain string) (*HTTPDomainMapping, error) {
	// 1. 从域名索引获取 mappingID
	indexKey := HTTPDomainIndexKey(fullDomain)
	data, err := r.storage.Get(indexKey)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return nil, coreerrors.Newf(coreerrors.CodeMappingNotFound, "domain %s not found", fullDomain)
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to lookup domain index")
	}

	mappingID, ok := data.(string)
	if !ok || mappingID == "" {
		return nil, coreerrors.New(coreerrors.CodeInvalidData, "unexpected data type for domain index")
	}

	// 2. 用 mappingID 获取完整映射数据
	return r.GetMapping(ctx, mappingID)
}

// GetBaseDomains 获取所有支持的基础域名
func (r *HTTPDomainMappingRepository) GetBaseDomains() []string {
	// 返回副本，避免外部修改
	result := make([]string, len(r.baseDomains))
	copy(result, r.baseDomains)
	return result
}

// ListAllMappings 列出所有映射
func (r *HTTPDomainMappingRepository) ListAllMappings(ctx context.Context) ([]*HTTPDomainMapping, error) {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}

	ids, err := listStore.GetList(KeyHTTPDomainMappingList)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return []*HTTPDomainMapping{}, nil
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get all mappings list")
	}

	// 批量获取映射数据
	mappings := make([]*HTTPDomainMapping, 0, len(ids))
	for _, idInterface := range ids {
		idStr, ok := idInterface.(string)
		if !ok {
			continue // 跳过无效的 ID
		}

		mapping, err := r.GetMapping(ctx, idStr)
		if err != nil {
			if coreerrors.IsCode(err, coreerrors.CodeMappingNotFound) {
				// 映射可能已被删除，从全局列表中移除
				_ = listStore.RemoveFromList(KeyHTTPDomainMappingList, idStr)
				continue
			}
			// 跳过其他错误，继续处理剩余映射
			continue
		}
		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

// CleanupExpiredMappings 清理过期的映射，返回清理数量
func (r *HTTPDomainMappingRepository) CleanupExpiredMappings(ctx context.Context) (int, error) {
	mappings, err := r.ListAllMappings(ctx)
	if err != nil {
		return 0, err
	}

	cleanedCount := 0
	for _, mapping := range mappings {
		if mapping.IsExpired() {
			// 删除过期映射
			if err := r.DeleteMapping(ctx, mapping.ID, mapping.ClientID); err != nil {
				// 记录错误但继续清理其他映射
				continue
			}
			cleanedCount++
		}
	}

	return cleanedCount, nil
}

// isBaseDomainSupported 检查基础域名是否在支持列表中
func (r *HTTPDomainMappingRepository) isBaseDomainSupported(baseDomain string) bool {
	for _, bd := range r.baseDomains {
		if bd == baseDomain {
			return true
		}
	}
	return false
}

// generateMappingID 生成唯一的映射 ID（格式：hdm_{数字}）
func (r *HTTPDomainMappingRepository) generateMappingID(ctx context.Context) (string, error) {
	counterStore, ok := r.storage.(storage.CounterStore)
	if !ok {
		return "", coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support counter operations")
	}

	nextID, err := counterStore.Incr(KeyHTTPDomainNextID)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to generate mapping ID")
	}

	return fmt.Sprintf("hdm_%d", nextID), nil
}

// addToClientMappingList 添加映射 ID 到客户端映射列表
func (r *HTTPDomainMappingRepository) addToClientMappingList(ctx context.Context, clientID int64, mappingID string) error {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}

	clientKey := HTTPDomainClientKey(clientID)
	return listStore.AppendToList(clientKey, mappingID)
}

// removeFromClientMappingList 从客户端映射列表移除映射 ID
func (r *HTTPDomainMappingRepository) removeFromClientMappingList(ctx context.Context, clientID int64, mappingID string) error {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}

	clientKey := HTTPDomainClientKey(clientID)
	return listStore.RemoveFromList(clientKey, mappingID)
}

// addToGlobalMappingList 添加映射 ID 到全局映射列表
func (r *HTTPDomainMappingRepository) addToGlobalMappingList(ctx context.Context, mappingID string) error {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}

	return listStore.AppendToList(KeyHTTPDomainMappingList, mappingID)
}

// removeFromGlobalMappingList 从全局映射列表移除映射 ID
func (r *HTTPDomainMappingRepository) removeFromGlobalMappingList(ctx context.Context, mappingID string) error {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}

	return listStore.RemoveFromList(KeyHTTPDomainMappingList, mappingID)
}
