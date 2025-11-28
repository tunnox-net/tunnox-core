package storage

import "time"

// PersistentStorage 持久化存储接口
// 用于数据库或远程 gRPC 存储
type PersistentStorage interface {
	// Set 设置键值对（持久化，不设置 TTL）
	Set(key string, value interface{}) error

	// Get 获取值
	Get(key string) (interface{}, error)

	// Delete 删除键
	Delete(key string) error

	// Exists 检查键是否存在
	Exists(key string) (bool, error)

	// BatchSet 批量设置
	BatchSet(items map[string]interface{}) error

	// BatchGet 批量获取
	BatchGet(keys []string) (map[string]interface{}, error)

	// BatchDelete 批量删除
	BatchDelete(keys []string) error

	// QueryByField 按字段查询（扫描匹配前缀的所有键，解析 JSON，过滤字段）
	// keyPrefix: 键前缀（如 "tunnox:port_mapping:"）
	// fieldName: 字段名（如 "listen_client_id"）
	// fieldValue: 字段值（如 int64(19072689)）
	// 返回：匹配的 JSON 字符串列表
	QueryByField(keyPrefix string, fieldName string, fieldValue interface{}) ([]string, error)

	// Close 关闭连接
	Close() error
}

// NullPersistentStorage 空持久化存储（用于纯内存模式）
type NullPersistentStorage struct{}

// NewNullPersistentStorage 创建空持久化存储
func NewNullPersistentStorage() PersistentStorage {
	return &NullPersistentStorage{}
}

func (n *NullPersistentStorage) Set(key string, value interface{}) error {
	return nil // 空操作
}

func (n *NullPersistentStorage) Get(key string) (interface{}, error) {
	return nil, ErrKeyNotFound
}

func (n *NullPersistentStorage) Delete(key string) error {
	return nil // 空操作
}

func (n *NullPersistentStorage) Exists(key string) (bool, error) {
	return false, nil
}

func (n *NullPersistentStorage) BatchSet(items map[string]interface{}) error {
	return nil // 空操作
}

func (n *NullPersistentStorage) BatchGet(keys []string) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

func (n *NullPersistentStorage) BatchDelete(keys []string) error {
	return nil // 空操作
}

func (n *NullPersistentStorage) QueryByField(keyPrefix string, fieldName string, fieldValue interface{}) ([]string, error) {
	return nil, ErrKeyNotFound
}

func (n *NullPersistentStorage) Close() error {
	return nil
}

// CacheStorage 缓存存储接口（对 Storage 的子集）
type CacheStorage interface {
	Set(key string, value interface{}, ttl time.Duration) error
	Get(key string) (interface{}, error)
	Delete(key string) error
	Exists(key string) (bool, error)
	Close() error
}
