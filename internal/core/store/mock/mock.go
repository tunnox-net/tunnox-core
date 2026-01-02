// Package mock 提供测试用的 Mock Store 实现
//
// MockStore 特性:
//   - 预设返回值（SetReturnValue）
//   - 预设错误（SetError）
//   - 调用记录（GetCalls）
//   - 支持所有 Store 接口
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// CallRecord 调用记录
// =============================================================================

// CallRecord 记录一次方法调用
type CallRecord struct {
	Method    string        // 方法名
	Args      []interface{} // 参数
	Timestamp time.Time     // 调用时间
}

// =============================================================================
// MockStore 通用 Mock 存储
// =============================================================================

// MockStore 测试用 Mock 存储
//
// 支持的功能:
//   - 预设返回值
//   - 预设错误
//   - 调用记录
//   - 线程安全
type MockStore[K comparable, V any] struct {
	mu sync.RWMutex

	// data 内存存储
	data map[K]V

	// ttls TTL 存储
	ttls map[K]time.Time

	// errors 预设错误（方法名 → 错误）
	errors map[string]error

	// returns 预设返回值（方法名 → 返回值）
	returns map[string]interface{}

	// calls 调用记录
	calls []CallRecord

	// recordCalls 是否记录调用
	recordCalls bool
}

// NewMockStore 创建 MockStore
func NewMockStore[K comparable, V any]() *MockStore[K, V] {
	return &MockStore[K, V]{
		data:        make(map[K]V),
		ttls:        make(map[K]time.Time),
		errors:      make(map[string]error),
		returns:     make(map[string]interface{}),
		calls:       make([]CallRecord, 0),
		recordCalls: true,
	}
}

// =============================================================================
// Mock 配置方法
// =============================================================================

// SetError 设置方法的预设错误
func (m *MockStore[K, V]) SetError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[method] = err
}

// ClearError 清除方法的预设错误
func (m *MockStore[K, V]) ClearError(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.errors, method)
}

// ClearAllErrors 清除所有预设错误
func (m *MockStore[K, V]) ClearAllErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = make(map[string]error)
}

// SetReturn 设置方法的预设返回值
func (m *MockStore[K, V]) SetReturn(method string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.returns[method] = value
}

// ClearReturn 清除方法的预设返回值
func (m *MockStore[K, V]) ClearReturn(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.returns, method)
}

// SetRecordCalls 设置是否记录调用
func (m *MockStore[K, V]) SetRecordCalls(record bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCalls = record
}

// GetCalls 获取所有调用记录
func (m *MockStore[K, V]) GetCalls() []CallRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]CallRecord, len(m.calls))
	copy(result, m.calls)
	return result
}

// GetCallsForMethod 获取指定方法的调用记录
func (m *MockStore[K, V]) GetCallsForMethod(method string) []CallRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []CallRecord
	for _, call := range m.calls {
		if call.Method == method {
			result = append(result, call)
		}
	}
	return result
}

// ClearCalls 清除所有调用记录
func (m *MockStore[K, V]) ClearCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]CallRecord, 0)
}

// Reset 重置所有状态
func (m *MockStore[K, V]) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[K]V)
	m.ttls = make(map[K]time.Time)
	m.errors = make(map[string]error)
	m.returns = make(map[string]interface{})
	m.calls = make([]CallRecord, 0)
}

// recordCall 记录方法调用
func (m *MockStore[K, V]) recordCall(method string, args ...interface{}) {
	if m.recordCalls {
		m.calls = append(m.calls, CallRecord{
			Method:    method,
			Args:      args,
			Timestamp: time.Now(),
		})
	}
}

// getError 获取方法的预设错误
func (m *MockStore[K, V]) getError(method string) error {
	return m.errors[method]
}

// =============================================================================
// Store[K, V] 接口实现
// =============================================================================

// Get 获取值
func (m *MockStore[K, V]) Get(ctx context.Context, key K) (V, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Get", key)

	if err := m.getError("Get"); err != nil {
		var zero V
		return zero, err
	}

	// 检查 TTL
	if expireAt, ok := m.ttls[key]; ok {
		if time.Now().After(expireAt) {
			delete(m.data, key)
			delete(m.ttls, key)
			var zero V
			return zero, store.ErrNotFound
		}
	}

	value, ok := m.data[key]
	if !ok {
		var zero V
		return zero, store.ErrNotFound
	}

	return value, nil
}

// Set 设置值
func (m *MockStore[K, V]) Set(ctx context.Context, key K, value V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Set", key, value)

	if err := m.getError("Set"); err != nil {
		return err
	}

	m.data[key] = value
	delete(m.ttls, key) // 清除 TTL

	return nil
}

// Delete 删除值
func (m *MockStore[K, V]) Delete(ctx context.Context, key K) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Delete", key)

	if err := m.getError("Delete"); err != nil {
		return err
	}

	delete(m.data, key)
	delete(m.ttls, key)

	return nil
}

// Exists 检查键是否存在
func (m *MockStore[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Exists", key)

	if err := m.getError("Exists"); err != nil {
		return false, err
	}

	// 检查 TTL
	if expireAt, ok := m.ttls[key]; ok {
		if time.Now().After(expireAt) {
			delete(m.data, key)
			delete(m.ttls, key)
			return false, nil
		}
	}

	_, ok := m.data[key]
	return ok, nil
}

// =============================================================================
// TTLStore[K, V] 接口实现
// =============================================================================

// SetWithTTL 设置值并指定 TTL
func (m *MockStore[K, V]) SetWithTTL(ctx context.Context, key K, value V, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("SetWithTTL", key, value, ttl)

	if err := m.getError("SetWithTTL"); err != nil {
		return err
	}

	m.data[key] = value
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	} else {
		delete(m.ttls, key)
	}

	return nil
}

// GetTTL 获取剩余 TTL
func (m *MockStore[K, V]) GetTTL(ctx context.Context, key K) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("GetTTL", key)

	if err := m.getError("GetTTL"); err != nil {
		return 0, err
	}

	// 检查键是否存在
	if _, ok := m.data[key]; !ok {
		return 0, store.ErrNotFound
	}

	// 获取 TTL
	expireAt, ok := m.ttls[key]
	if !ok {
		return -1, nil // 无过期时间
	}

	remaining := time.Until(expireAt)
	if remaining < 0 {
		// 已过期
		delete(m.data, key)
		delete(m.ttls, key)
		return 0, store.ErrNotFound
	}

	return remaining, nil
}

// Refresh 刷新 TTL
func (m *MockStore[K, V]) Refresh(ctx context.Context, key K, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Refresh", key, ttl)

	if err := m.getError("Refresh"); err != nil {
		return err
	}

	// 检查键是否存在
	if _, ok := m.data[key]; !ok {
		return store.ErrNotFound
	}

	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	} else {
		delete(m.ttls, key)
	}

	return nil
}

// =============================================================================
// BatchStore[K, V] 接口实现
// =============================================================================

// BatchGet 批量获取
func (m *MockStore[K, V]) BatchGet(ctx context.Context, keys []K) (map[K]V, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("BatchGet", keys)

	if err := m.getError("BatchGet"); err != nil {
		return nil, err
	}

	result := make(map[K]V)
	now := time.Now()

	for _, key := range keys {
		// 检查 TTL
		if expireAt, ok := m.ttls[key]; ok {
			if now.After(expireAt) {
				delete(m.data, key)
				delete(m.ttls, key)
				continue
			}
		}

		if value, ok := m.data[key]; ok {
			result[key] = value
		}
	}

	return result, nil
}

// BatchSet 批量设置
func (m *MockStore[K, V]) BatchSet(ctx context.Context, items map[K]V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("BatchSet", items)

	if err := m.getError("BatchSet"); err != nil {
		return err
	}

	for key, value := range items {
		m.data[key] = value
		delete(m.ttls, key)
	}

	return nil
}

// BatchDelete 批量删除
func (m *MockStore[K, V]) BatchDelete(ctx context.Context, keys []K) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("BatchDelete", keys)

	if err := m.getError("BatchDelete"); err != nil {
		return err
	}

	for _, key := range keys {
		delete(m.data, key)
		delete(m.ttls, key)
	}

	return nil
}

// =============================================================================
// AtomicStore[K, V] 接口实现
// =============================================================================

// SetNX 仅当键不存在时设置
func (m *MockStore[K, V]) SetNX(ctx context.Context, key K, value V) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("SetNX", key, value)

	if err := m.getError("SetNX"); err != nil {
		return false, err
	}

	// 检查 TTL 过期
	if expireAt, ok := m.ttls[key]; ok {
		if time.Now().After(expireAt) {
			delete(m.data, key)
			delete(m.ttls, key)
		}
	}

	// 检查键是否存在
	if _, ok := m.data[key]; ok {
		return false, nil
	}

	m.data[key] = value
	return true, nil
}

// SetNXWithTTL 仅当键不存在时设置并指定 TTL
func (m *MockStore[K, V]) SetNXWithTTL(ctx context.Context, key K, value V, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("SetNXWithTTL", key, value, ttl)

	if err := m.getError("SetNXWithTTL"); err != nil {
		return false, err
	}

	// 检查 TTL 过期
	if expireAt, ok := m.ttls[key]; ok {
		if time.Now().After(expireAt) {
			delete(m.data, key)
			delete(m.ttls, key)
		}
	}

	// 检查键是否存在
	if _, ok := m.data[key]; ok {
		return false, nil
	}

	m.data[key] = value
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}

	return true, nil
}

// =============================================================================
// 接口验证
// =============================================================================

var (
	_ store.Store[string, string]       = (*MockStore[string, string])(nil)
	_ store.TTLStore[string, string]    = (*MockStore[string, string])(nil)
	_ store.BatchStore[string, string]  = (*MockStore[string, string])(nil)
	_ store.AtomicStore[string, string] = (*MockStore[string, string])(nil)
)

// =============================================================================
// MockSetStore 集合 Mock 存储
// =============================================================================

// MockSetStore 测试用 Mock 集合存储
type MockSetStore[K comparable, V comparable] struct {
	mu sync.RWMutex

	// sets 集合数据（key → set of values）
	sets map[K]map[V]struct{}

	// errors 预设错误
	errors map[string]error

	// calls 调用记录
	calls []CallRecord

	// recordCalls 是否记录调用
	recordCalls bool
}

// NewMockSetStore 创建 MockSetStore
func NewMockSetStore[K comparable, V comparable]() *MockSetStore[K, V] {
	return &MockSetStore[K, V]{
		sets:        make(map[K]map[V]struct{}),
		errors:      make(map[string]error),
		calls:       make([]CallRecord, 0),
		recordCalls: true,
	}
}

// =============================================================================
// Mock 配置方法
// =============================================================================

// SetError 设置方法的预设错误
func (m *MockSetStore[K, V]) SetError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[method] = err
}

// ClearError 清除方法的预设错误
func (m *MockSetStore[K, V]) ClearError(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.errors, method)
}

// ClearAllErrors 清除所有预设错误
func (m *MockSetStore[K, V]) ClearAllErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = make(map[string]error)
}

// GetCalls 获取所有调用记录
func (m *MockSetStore[K, V]) GetCalls() []CallRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]CallRecord, len(m.calls))
	copy(result, m.calls)
	return result
}

// GetCallsForMethod 获取指定方法的调用记录
func (m *MockSetStore[K, V]) GetCallsForMethod(method string) []CallRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []CallRecord
	for _, call := range m.calls {
		if call.Method == method {
			result = append(result, call)
		}
	}
	return result
}

// ClearCalls 清除所有调用记录
func (m *MockSetStore[K, V]) ClearCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]CallRecord, 0)
}

// Reset 重置所有状态
func (m *MockSetStore[K, V]) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sets = make(map[K]map[V]struct{})
	m.errors = make(map[string]error)
	m.calls = make([]CallRecord, 0)
}

// recordCall 记录方法调用
func (m *MockSetStore[K, V]) recordCall(method string, args ...interface{}) {
	if m.recordCalls {
		m.calls = append(m.calls, CallRecord{
			Method:    method,
			Args:      args,
			Timestamp: time.Now(),
		})
	}
}

// getError 获取方法的预设错误
func (m *MockSetStore[K, V]) getError(method string) error {
	return m.errors[method]
}

// =============================================================================
// SetStore[K, V] 接口实现
// =============================================================================

// Add 向集合添加元素
func (m *MockSetStore[K, V]) Add(ctx context.Context, key K, value V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Add", key, value)

	if err := m.getError("Add"); err != nil {
		return err
	}

	if m.sets[key] == nil {
		m.sets[key] = make(map[V]struct{})
	}
	m.sets[key][value] = struct{}{}

	return nil
}

// Remove 从集合移除元素
func (m *MockSetStore[K, V]) Remove(ctx context.Context, key K, value V) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Remove", key, value)

	if err := m.getError("Remove"); err != nil {
		return err
	}

	if set, ok := m.sets[key]; ok {
		delete(set, value)
		if len(set) == 0 {
			delete(m.sets, key)
		}
	}

	return nil
}

// Contains 检查元素是否在集合中
func (m *MockSetStore[K, V]) Contains(ctx context.Context, key K, value V) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("Contains", key, value)

	if err := m.getError("Contains"); err != nil {
		return false, err
	}

	set, ok := m.sets[key]
	if !ok {
		return false, nil
	}

	_, exists := set[value]
	return exists, nil
}

// Members 获取集合所有成员
func (m *MockSetStore[K, V]) Members(ctx context.Context, key K) ([]V, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("Members", key)

	if err := m.getError("Members"); err != nil {
		return nil, err
	}

	set, ok := m.sets[key]
	if !ok {
		return []V{}, nil
	}

	result := make([]V, 0, len(set))
	for v := range set {
		result = append(result, v)
	}

	return result, nil
}

// Size 获取集合大小
func (m *MockSetStore[K, V]) Size(ctx context.Context, key K) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("Size", key)

	if err := m.getError("Size"); err != nil {
		return 0, err
	}

	set, ok := m.sets[key]
	if !ok {
		return 0, nil
	}

	return int64(len(set)), nil
}

// =============================================================================
// 辅助方法
// =============================================================================

// GetData 获取内部数据（仅用于测试验证）
func (m *MockSetStore[K, V]) GetData() map[K]map[V]struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[K]map[V]struct{})
	for k, v := range m.sets {
		result[k] = make(map[V]struct{})
		for vv := range v {
			result[k][vv] = struct{}{}
		}
	}
	return result
}

// SetData 设置内部数据（仅用于测试准备）
func (m *MockSetStore[K, V]) SetData(data map[K][]V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sets = make(map[K]map[V]struct{})
	for k, values := range data {
		m.sets[k] = make(map[V]struct{})
		for _, v := range values {
			m.sets[k][v] = struct{}{}
		}
	}
}

// =============================================================================
// 接口验证
// =============================================================================

var _ store.SetStore[string, string] = (*MockSetStore[string, string])(nil)

// =============================================================================
// MockPipeline SetStore 管道 Mock
// =============================================================================

// MockSetPipeline Mock 集合管道
type MockSetPipeline[K comparable, V comparable] struct {
	store      *MockSetStore[K, V]
	operations []pipelineOp[K, V]
}

type pipelineOp[K comparable, V comparable] struct {
	op    string // "add" or "remove"
	key   K
	value V
}

// NewMockSetPipeline 创建 Mock 集合管道
func (m *MockSetStore[K, V]) Pipeline() store.SetPipeline[K, V] {
	return &MockSetPipeline[K, V]{
		store:      m,
		operations: make([]pipelineOp[K, V], 0),
	}
}

// SAdd 添加集合添加操作到管道
func (p *MockSetPipeline[K, V]) SAdd(ctx context.Context, key K, member V) {
	p.operations = append(p.operations, pipelineOp[K, V]{
		op:    "add",
		key:   key,
		value: member,
	})
}

// SRem 添加集合移除操作到管道
func (p *MockSetPipeline[K, V]) SRem(ctx context.Context, key K, member V) {
	p.operations = append(p.operations, pipelineOp[K, V]{
		op:    "remove",
		key:   key,
		value: member,
	})
}

// Exec 执行管道中的所有操作
func (p *MockSetPipeline[K, V]) Exec(ctx context.Context) error {
	for _, op := range p.operations {
		switch op.op {
		case "add":
			if err := p.store.Add(ctx, op.key, op.value); err != nil {
				return err
			}
		case "remove":
			if err := p.store.Remove(ctx, op.key, op.value); err != nil {
				return err
			}
		}
	}
	p.operations = nil
	return nil
}

// =============================================================================
// 接口验证
// =============================================================================

var _ store.PipelineSetStore[string, string] = (*MockSetStore[string, string])(nil)

// =============================================================================
// 工厂函数
// =============================================================================

// NewMockStoreString 创建 string→string 的 MockStore
func NewMockStoreString() *MockStore[string, string] {
	return NewMockStore[string, string]()
}

// NewMockSetStoreString 创建 string→string 的 MockSetStore
func NewMockSetStoreString() *MockSetStore[string, string] {
	return NewMockSetStore[string, string]()
}

// =============================================================================
// GetData 辅助方法
// =============================================================================

// GetData 获取内部数据（仅用于测试验证）
func (m *MockStore[K, V]) GetData() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[K]V)
	for k, v := range m.data {
		result[k] = v
	}
	return result
}

// SetData 设置内部数据（仅用于测试准备）
func (m *MockStore[K, V]) SetData(data map[K]V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[K]V)
	for k, v := range data {
		m.data[k] = v
	}
}

// GetTTLs 获取内部 TTL 数据（仅用于测试验证）
func (m *MockStore[K, V]) GetTTLs() map[K]time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[K]time.Time)
	for k, v := range m.ttls {
		result[k] = v
	}
	return result
}

// CallCount 获取方法调用次数
func (m *MockStore[K, V]) CallCount(method string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, call := range m.calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

// CallCount 获取方法调用次数（MockSetStore）
func (m *MockSetStore[K, V]) CallCount(method string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, call := range m.calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

// AssertCalled 断言方法被调用（用于测试）
func (m *MockStore[K, V]) AssertCalled(method string) error {
	if m.CallCount(method) == 0 {
		return fmt.Errorf("expected method %s to be called, but it was not", method)
	}
	return nil
}

// AssertNotCalled 断言方法未被调用（用于测试）
func (m *MockStore[K, V]) AssertNotCalled(method string) error {
	if m.CallCount(method) > 0 {
		return fmt.Errorf("expected method %s not to be called, but it was called %d times", method, m.CallCount(method))
	}
	return nil
}

// AssertCallCount 断言方法被调用指定次数（用于测试）
func (m *MockStore[K, V]) AssertCallCount(method string, expected int) error {
	actual := m.CallCount(method)
	if actual != expected {
		return fmt.Errorf("expected method %s to be called %d times, but it was called %d times", method, expected, actual)
	}
	return nil
}
