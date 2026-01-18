package repos

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
)

// Common errors
var (
	ErrNotFound = errors.New("entity not found")
)

// GenericRepository 泛型Repository接口
type GenericRepository[T any] interface {
	// 基础CRUD操作
	Save(entity T, keyPrefix string, ttl time.Duration) error
	Create(entity T, keyPrefix string, ttl time.Duration) error
	Update(entity T, keyPrefix string, ttl time.Duration) error
	Get(id string, keyPrefix string) (T, error)
	Delete(id string, keyPrefix string) error

	// 列表操作
	List(listKey string) ([]T, error)
	AddToList(entity T, listKey string) error
	RemoveFromList(entity T, listKey string) error
}

// GenericRepositoryImpl 泛型Repository实现
// 使用 singleflight 防止缓存击穿：当多个 goroutine 同时请求同一个 key 时，
// 只有一个会真正执行查询，其他等待结果共享
type GenericRepositoryImpl[T any] struct {
	*Repository
	getIDFunc func(T) (string, error)
	sf        singleflight.Group // 防缓存击穿
}

// NewGenericRepository 创建泛型Repository
func NewGenericRepository[T any](repo *Repository, getIDFunc func(T) (string, error)) *GenericRepositoryImpl[T] {
	return &GenericRepositoryImpl[T]{
		Repository: repo,
		getIDFunc:  getIDFunc,
	}
}

// Save 保存实体（创建或更新）
func (r *GenericRepositoryImpl[T]) Save(entity T, keyPrefix string, ttl time.Duration) error {
	data, err := json.Marshal(entity)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "marshal entity failed")
	}

	// 使用反射获取ID字段
	id, err := r.getEntityID(entity)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "get entity ID failed")
	}

	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Set(key, string(data), ttl)
}

func (r *GenericRepositoryImpl[T]) Create(entity T, keyPrefix string, ttl time.Duration) error {
	id, err := r.getEntityID(entity)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "get entity ID failed")
	}

	_, err = r.Get(id, keyPrefix)
	if err == nil {
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "entity with ID %s already exists", id)
	}

	if !errors.Is(err, storage.ErrKeyNotFound) && !coreerrors.IsCode(err, coreerrors.CodeNotFound) {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}

	return r.Save(entity, keyPrefix, ttl)
}

func (r *GenericRepositoryImpl[T]) Update(entity T, keyPrefix string, ttl time.Duration) error {
	id, err := r.getEntityID(entity)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "get entity ID failed")
	}

	_, err = r.Get(id, keyPrefix)
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) || coreerrors.IsCode(err, coreerrors.CodeNotFound) {
			return coreerrors.Newf(coreerrors.CodeNotFound, "entity with ID %s does not exist", id)
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}

	return r.Save(entity, keyPrefix, ttl)
}

// Get 获取实体
// 使用 singleflight 防止缓存击穿：当多个 goroutine 同时请求同一个 key 时，
// 只有一个会真正执行查询，其他等待结果共享
func (r *GenericRepositoryImpl[T]) Get(id string, keyPrefix string) (T, error) {
	var zero T
	key := fmt.Sprintf("%s:%s", keyPrefix, id)

	// 使用 singleflight 防止缓存击穿
	// 当缓存失效时，多个 goroutine 同时请求同一个 key，只有一个会执行实际查询
	result, err, _ := r.sf.Do(key, func() (interface{}, error) {
		data, err := r.storage.Get(key)
		if err != nil {
			return zero, err
		}

		entityData, ok := data.(string)
		if !ok {
			return zero, coreerrors.New(coreerrors.CodeStorageError, "invalid entity data type")
		}

		var entity T
		if err := json.Unmarshal([]byte(entityData), &entity); err != nil {
			return zero, coreerrors.Wrap(err, coreerrors.CodeStorageError, "unmarshal entity failed")
		}

		return entity, nil
	})

	if err != nil {
		return zero, err
	}

	entity, ok := result.(T)
	if !ok {
		return zero, coreerrors.Newf(coreerrors.CodeStorageError, "unexpected result type: %T", result)
	}
	return entity, nil
}

// Delete 删除实体
func (r *GenericRepositoryImpl[T]) Delete(id string, keyPrefix string) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Delete(key)
}

// List 列出实体
func (r *GenericRepositoryImpl[T]) List(listKey string) ([]T, error) {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return nil, coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}
	data, err := listStore.GetList(listKey)
	if err != nil {
		return []T{}, nil
	}

	var entities []T
	for i, item := range data {
		if entityData, ok := item.(string); ok {
			var entity T
			if err := json.Unmarshal([]byte(entityData), &entity); err != nil {
				// 记录解析错误，但继续处理其他项
				dispose.Warnf("List: failed to unmarshal item[%d] in %s: %v", i, listKey, err)
				continue
			}
			entities = append(entities, entity)
		}
	}

	return entities, nil
}

// AddToList 添加实体到列表
func (r *GenericRepositoryImpl[T]) AddToList(entity T, listKey string) error {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}
	data, err := json.Marshal(entity)
	if err != nil {
		return err
	}

	return listStore.AppendToList(listKey, string(data))
}

// RemoveFromList 从列表移除实体
func (r *GenericRepositoryImpl[T]) RemoveFromList(entity T, listKey string) error {
	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeStorageError, "storage does not support list operations")
	}
	data, err := json.Marshal(entity)
	if err != nil {
		return err
	}

	return listStore.RemoveFromList(listKey, string(data))
}

// getEntityID 获取实体ID
func (r *GenericRepositoryImpl[T]) getEntityID(entity T) (string, error) {
	if r.getIDFunc == nil {
		return "", coreerrors.New(coreerrors.CodeInternal, "getIDFunc not set")
	}
	return r.getIDFunc(entity)
}

// Repository 数据访问层基类
type Repository struct {
	storage storage.Storage
	dispose.Dispose
}

// NewRepository 创建新的数据访问层
// 注意：Repository 不管理自己的 context，它依赖于 storage 的 context
// 如果 storage 实现了 Disposable 接口，Repository 会自动继承其 context
func NewRepository(storage storage.Storage) *Repository {
	repo := &Repository{
		storage: storage,
	}
	// Repository 不需要独立的 context，它只是 storage 的包装器
	// 如果 storage 实现了 Disposable，可以通过 storage 获取 context
	// 这里不设置 context，避免创建独立的 dispose 子树
	return repo
}

// GetStorage 获取底层存储实例
func (r *Repository) GetStorage() storage.Storage {
	return r.storage
}
