package types

// ============================================================================
// 空持久化存储
// ============================================================================

// NullPersistentStorage 空持久化存储（用于纯内存模式）
// 实现 PersistentStorage 接口，所有操作都是无效操作
type NullPersistentStorage struct{}

// NewNullPersistentStorage 创建空持久化存储
func NewNullPersistentStorage() PersistentStorage {
	return &NullPersistentStorage{}
}

// Set 设置键值对（空操作）
func (n *NullPersistentStorage) Set(key string, value any) error {
	return nil // 空操作
}

// Get 获取值（始终返回 ErrKeyNotFound）
func (n *NullPersistentStorage) Get(key string) (any, error) {
	return nil, ErrKeyNotFound
}

// Delete 删除键（空操作）
func (n *NullPersistentStorage) Delete(key string) error {
	return nil // 空操作
}

// Exists 检查键是否存在（始终返回 false）
func (n *NullPersistentStorage) Exists(key string) (bool, error) {
	return false, nil
}

// BatchSet 批量设置（空操作）
func (n *NullPersistentStorage) BatchSet(items map[string]any) error {
	return nil // 空操作
}

// BatchGet 批量获取（返回空 map）
func (n *NullPersistentStorage) BatchGet(keys []string) (map[string]any, error) {
	return make(map[string]any), nil
}

// BatchDelete 批量删除（空操作）
func (n *NullPersistentStorage) BatchDelete(keys []string) error {
	return nil // 空操作
}

// QueryByField 按字段查询（始终返回 ErrKeyNotFound）
func (n *NullPersistentStorage) QueryByField(keyPrefix string, fieldName string, fieldValue any) ([]string, error) {
	return nil, ErrKeyNotFound
}

// QueryByPrefix 按前缀查询（返回空 map）
func (n *NullPersistentStorage) QueryByPrefix(prefix string, limit int) (map[string]string, error) {
	return make(map[string]string), nil
}

// Close 关闭连接（空操作）
func (n *NullPersistentStorage) Close() error {
	return nil
}
