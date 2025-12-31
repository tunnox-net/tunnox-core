package repos

import (
	"fmt"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/core/storage/types"
)

// ============================================================================
// 类型安全的 Repository 基类
// ============================================================================

// TypedRepository 泛型 Repository 基类
// 提供类型安全的存储访问，无需手动进行 JSON 序列化/反序列化
// 使用示例:
//
//	type UserRepository struct {
//	    *repos.TypedRepository[models.User]
//	}
//
//	func NewUserRepository(s storage.FullStorage) *UserRepository {
//	    return &UserRepository{
//	        TypedRepository: repos.NewTypedRepository[models.User](s),
//	    }
//	}
type TypedRepository[T any] struct {
	storage *types.TypedFullStorageAdapter[T]
	raw     storage.FullStorage
	dispose.Dispose
}

// NewTypedRepository 创建泛型 Repository
func NewTypedRepository[T any](s storage.FullStorage) *TypedRepository[T] {
	return &TypedRepository[T]{
		storage: types.NewTypedFullStorageAdapter[T](s),
		raw:     s,
	}
}

// ============================================================================
// 基础 CRUD 操作
// ============================================================================

// Save 保存实体
func (r *TypedRepository[T]) Save(key string, entity T, ttl time.Duration) error {
	return r.storage.Set(key, entity, ttl)
}

// SaveWithPrefix 保存实体（使用键前缀）
func (r *TypedRepository[T]) SaveWithPrefix(keyPrefix string, id string, entity T, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Set(key, entity, ttl)
}

// Get 获取实体
func (r *TypedRepository[T]) Get(key string) (T, error) {
	return r.storage.Get(key)
}

// GetWithPrefix 获取实体（使用键前缀）
func (r *TypedRepository[T]) GetWithPrefix(keyPrefix string, id string) (T, error) {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Get(key)
}

// Delete 删除实体
func (r *TypedRepository[T]) Delete(key string) error {
	return r.storage.Delete(key)
}

// DeleteWithPrefix 删除实体（使用键前缀）
func (r *TypedRepository[T]) DeleteWithPrefix(keyPrefix string, id string) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Delete(key)
}

// Exists 检查实体是否存在
func (r *TypedRepository[T]) Exists(key string) (bool, error) {
	return r.storage.Exists(key)
}

// ExistsWithPrefix 检查实体是否存在（使用键前缀）
func (r *TypedRepository[T]) ExistsWithPrefix(keyPrefix string, id string) (bool, error) {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Exists(key)
}

// ============================================================================
// Create/Update 操作（带检查）
// ============================================================================

// Create 创建实体（仅创建，不允许覆盖）
func (r *TypedRepository[T]) Create(keyPrefix string, id string, entity T, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)

	// 使用 SetNX 原子操作
	ok, err := r.storage.SetNX(key, entity, ttl)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "create entity failed")
	}
	if !ok {
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "entity with ID %s already exists", id)
	}

	return nil
}

// Update 更新实体（仅更新，不允许创建）
func (r *TypedRepository[T]) Update(keyPrefix string, id string, entity T, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)

	// 检查实体是否存在
	exists, err := r.storage.Exists(key)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "check entity existence failed")
	}
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "entity with ID %s does not exist", id)
	}

	return r.storage.Set(key, entity, ttl)
}

// ============================================================================
// 列表操作
// ============================================================================

// GetList 获取列表
func (r *TypedRepository[T]) GetList(key string) ([]T, error) {
	return r.storage.GetList(key)
}

// SetList 设置列表
func (r *TypedRepository[T]) SetList(key string, entities []T, ttl time.Duration) error {
	return r.storage.SetList(key, entities, ttl)
}

// AppendToList 添加实体到列表
func (r *TypedRepository[T]) AppendToList(key string, entity T) error {
	return r.storage.AppendToList(key, entity)
}

// RemoveFromList 从列表移除实体
func (r *TypedRepository[T]) RemoveFromList(key string, entity T) error {
	return r.storage.RemoveFromList(key, entity)
}

// ============================================================================
// 哈希操作
// ============================================================================

// GetHash 获取哈希字段
func (r *TypedRepository[T]) GetHash(key string, field string) (T, error) {
	return r.storage.GetHash(key, field)
}

// SetHash 设置哈希字段
func (r *TypedRepository[T]) SetHash(key string, field string, entity T) error {
	return r.storage.SetHash(key, field, entity)
}

// GetAllHash 获取所有哈希字段
func (r *TypedRepository[T]) GetAllHash(key string) (map[string]T, error) {
	return r.storage.GetAllHash(key)
}

// DeleteHash 删除哈希字段
func (r *TypedRepository[T]) DeleteHash(key string, field string) error {
	return r.storage.DeleteHash(key, field)
}

// ============================================================================
// CAS 原子操作
// ============================================================================

// SetNX 仅当键不存在时设置
func (r *TypedRepository[T]) SetNX(key string, entity T, ttl time.Duration) (bool, error) {
	return r.storage.SetNX(key, entity, ttl)
}

// CompareAndSwap 比较并交换
func (r *TypedRepository[T]) CompareAndSwap(key string, oldEntity, newEntity T, ttl time.Duration) (bool, error) {
	return r.storage.CompareAndSwap(key, oldEntity, newEntity, ttl)
}

// ============================================================================
// 过期时间操作
// ============================================================================

// SetExpiration 设置过期时间
func (r *TypedRepository[T]) SetExpiration(key string, ttl time.Duration) error {
	return r.storage.SetExpiration(key, ttl)
}

// GetExpiration 获取过期时间
func (r *TypedRepository[T]) GetExpiration(key string) (time.Duration, error) {
	return r.storage.GetExpiration(key)
}

// ============================================================================
// 底层存储访问
// ============================================================================

// TypedStorage 返回泛型存储适配器
func (r *TypedRepository[T]) TypedStorage() *types.TypedFullStorageAdapter[T] {
	return r.storage
}

// RawStorage 返回原始存储
func (r *TypedRepository[T]) RawStorage() storage.FullStorage {
	return r.raw
}

// ============================================================================
// 辅助接口：实体 ID 获取器
// ============================================================================

// EntityWithID 带 ID 的实体接口
// 实现此接口的实体可以使用更简洁的 CRUD 方法
type EntityWithID interface {
	// GetID 返回实体的唯一标识符
	GetID() string
}

// TypedEntityRepository 带实体 ID 获取的泛型 Repository
// 适用于实现了 EntityWithID 接口的实体
type TypedEntityRepository[T EntityWithID] struct {
	*TypedRepository[T]
	keyPrefix string
	ttl       time.Duration
}

// NewTypedEntityRepository 创建带实体 ID 获取的泛型 Repository
func NewTypedEntityRepository[T EntityWithID](s storage.FullStorage, keyPrefix string, ttl time.Duration) *TypedEntityRepository[T] {
	return &TypedEntityRepository[T]{
		TypedRepository: NewTypedRepository[T](s),
		keyPrefix:       keyPrefix,
		ttl:             ttl,
	}
}

// SaveEntity 保存实体（自动提取 ID）
func (r *TypedEntityRepository[T]) SaveEntity(entity T) error {
	return r.SaveWithPrefix(r.keyPrefix, entity.GetID(), entity, r.ttl)
}

// GetEntity 获取实体
func (r *TypedEntityRepository[T]) GetEntity(id string) (T, error) {
	return r.GetWithPrefix(r.keyPrefix, id)
}

// DeleteEntity 删除实体
func (r *TypedEntityRepository[T]) DeleteEntity(id string) error {
	return r.DeleteWithPrefix(r.keyPrefix, id)
}

// EntityExists 检查实体是否存在
func (r *TypedEntityRepository[T]) EntityExists(id string) (bool, error) {
	return r.ExistsWithPrefix(r.keyPrefix, id)
}

// CreateEntity 创建实体（不覆盖）
func (r *TypedEntityRepository[T]) CreateEntity(entity T) error {
	return r.Create(r.keyPrefix, entity.GetID(), entity, r.ttl)
}

// UpdateEntity 更新实体（必须存在）
func (r *TypedEntityRepository[T]) UpdateEntity(entity T) error {
	return r.Update(r.keyPrefix, entity.GetID(), entity, r.ttl)
}
