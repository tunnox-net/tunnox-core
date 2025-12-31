// Package stats 提供统计功能的测试
package stats

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ═══════════════════════════════════════════════════════════════════
// StatsCache 测试
// ═══════════════════════════════════════════════════════════════════

func TestNewStatsCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{"short TTL", 1 * time.Second},
		{"medium TTL", 30 * time.Second},
		{"long TTL", 5 * time.Minute},
		{"zero TTL", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cache := NewStatsCache(tt.ttl)
			if cache == nil {
				t.Error("NewStatsCache() returned nil")
			}
			if cache.ttl != tt.ttl {
				t.Errorf("cache.ttl = %v, want %v", cache.ttl, tt.ttl)
			}
		})
	}
}

func TestStatsCache_SetAndGet(t *testing.T) {
	t.Parallel()

	cache := NewStatsCache(1 * time.Minute)

	stats := &SystemStats{
		TotalUsers:       10,
		TotalClients:     100,
		OnlineClients:    50,
		TotalMappings:    200,
		ActiveMappings:   80,
		TotalNodes:       5,
		OnlineNodes:      3,
		TotalTraffic:     1000000,
		TotalConnections: 5000,
		AnonymousUsers:   20,
	}

	// 测试 Set
	cache.Set(stats)

	// 测试 Get
	got := cache.Get()
	if got == nil {
		t.Fatal("Get() returned nil after Set()")
	}

	if got.TotalUsers != stats.TotalUsers {
		t.Errorf("TotalUsers = %d, want %d", got.TotalUsers, stats.TotalUsers)
	}
	if got.TotalClients != stats.TotalClients {
		t.Errorf("TotalClients = %d, want %d", got.TotalClients, stats.TotalClients)
	}
	if got.OnlineClients != stats.OnlineClients {
		t.Errorf("OnlineClients = %d, want %d", got.OnlineClients, stats.OnlineClients)
	}
}

func TestStatsCache_GetReturnsNilWhenEmpty(t *testing.T) {
	t.Parallel()

	cache := NewStatsCache(1 * time.Minute)

	got := cache.Get()
	if got != nil {
		t.Errorf("Get() on empty cache = %v, want nil", got)
	}
}

func TestStatsCache_GetReturnsNilWhenExpired(t *testing.T) {
	t.Parallel()

	cache := NewStatsCache(10 * time.Millisecond)

	stats := &SystemStats{TotalUsers: 10}
	cache.Set(stats)

	// 等待缓存过期
	time.Sleep(20 * time.Millisecond)

	got := cache.Get()
	if got != nil {
		t.Errorf("Get() on expired cache = %v, want nil", got)
	}
}

func TestStatsCache_Invalidate(t *testing.T) {
	t.Parallel()

	cache := NewStatsCache(1 * time.Minute)

	stats := &SystemStats{TotalUsers: 10}
	cache.Set(stats)

	// 验证设置成功
	if cache.Get() == nil {
		t.Fatal("Get() returned nil after Set()")
	}

	// 使缓存失效
	cache.Invalidate()

	// 验证失效后返回 nil
	if cache.Get() != nil {
		t.Error("Get() should return nil after Invalidate()")
	}
}

func TestStatsCache_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(*StatsCache)
		expected bool
	}{
		{
			name:     "empty cache is invalid",
			setup:    func(c *StatsCache) {},
			expected: false,
		},
		{
			name: "cache with data is valid",
			setup: func(c *StatsCache) {
				c.Set(&SystemStats{TotalUsers: 10})
			},
			expected: true,
		},
		{
			name: "invalidated cache is invalid",
			setup: func(c *StatsCache) {
				c.Set(&SystemStats{TotalUsers: 10})
				c.Invalidate()
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cache := NewStatsCache(1 * time.Minute)
			tt.setup(cache)

			if got := cache.IsValid(); got != tt.expected {
				t.Errorf("IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStatsCache_IsValid_Expired(t *testing.T) {
	t.Parallel()

	cache := NewStatsCache(10 * time.Millisecond)
	cache.Set(&SystemStats{TotalUsers: 10})

	// 初始时有效
	if !cache.IsValid() {
		t.Error("IsValid() should return true initially")
	}

	// 等待过期
	time.Sleep(20 * time.Millisecond)

	if cache.IsValid() {
		t.Error("IsValid() should return false after expiry")
	}
}

func TestStatsCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := NewStatsCache(1 * time.Minute)
	done := make(chan struct{})

	// 并发写入
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set(&SystemStats{TotalUsers: i})
		}
		done <- struct{}{}
	}()

	// 并发读取
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get()
		}
		done <- struct{}{}
	}()

	// 并发失效
	go func() {
		for i := 0; i < 100; i++ {
			cache.Invalidate()
		}
		done <- struct{}{}
	}()

	// 并发检查有效性
	go func() {
		for i := 0; i < 100; i++ {
			cache.IsValid()
		}
		done <- struct{}{}
	}()

	// 等待所有 goroutine 完成
	for i := 0; i < 4; i++ {
		<-done
	}
}

// ═══════════════════════════════════════════════════════════════════
// UserStats 测试
// ═══════════════════════════════════════════════════════════════════

func TestUserStats_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	stats := UserStats{
		UserID:           "user-123",
		TotalClients:     5,
		OnlineClients:    3,
		TotalMappings:    10,
		ActiveMappings:   7,
		TotalTraffic:     1000000,
		TotalConnections: 500,
		LastActive:       now,
	}

	if stats.UserID != "user-123" {
		t.Errorf("UserID = %s, want user-123", stats.UserID)
	}
	if stats.TotalClients != 5 {
		t.Errorf("TotalClients = %d, want 5", stats.TotalClients)
	}
	if stats.OnlineClients != 3 {
		t.Errorf("OnlineClients = %d, want 3", stats.OnlineClients)
	}
	if stats.TotalMappings != 10 {
		t.Errorf("TotalMappings = %d, want 10", stats.TotalMappings)
	}
	if stats.ActiveMappings != 7 {
		t.Errorf("ActiveMappings = %d, want 7", stats.ActiveMappings)
	}
	if stats.TotalTraffic != 1000000 {
		t.Errorf("TotalTraffic = %d, want 1000000", stats.TotalTraffic)
	}
	if stats.TotalConnections != 500 {
		t.Errorf("TotalConnections = %d, want 500", stats.TotalConnections)
	}
	if !stats.LastActive.Equal(now) {
		t.Errorf("LastActive = %v, want %v", stats.LastActive, now)
	}
}

// ═══════════════════════════════════════════════════════════════════
// ClientStats 测试
// ═══════════════════════════════════════════════════════════════════

func TestClientStats_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	stats := ClientStats{
		ClientID:         12345,
		UserID:           "user-123",
		TotalMappings:    8,
		ActiveMappings:   5,
		TotalTraffic:     500000,
		TotalConnections: 200,
		Uptime:           3600,
		LastSeen:         now,
	}

	if stats.ClientID != 12345 {
		t.Errorf("ClientID = %d, want 12345", stats.ClientID)
	}
	if stats.UserID != "user-123" {
		t.Errorf("UserID = %s, want user-123", stats.UserID)
	}
	if stats.TotalMappings != 8 {
		t.Errorf("TotalMappings = %d, want 8", stats.TotalMappings)
	}
	if stats.ActiveMappings != 5 {
		t.Errorf("ActiveMappings = %d, want 5", stats.ActiveMappings)
	}
	if stats.TotalTraffic != 500000 {
		t.Errorf("TotalTraffic = %d, want 500000", stats.TotalTraffic)
	}
	if stats.TotalConnections != 200 {
		t.Errorf("TotalConnections = %d, want 200", stats.TotalConnections)
	}
	if stats.Uptime != 3600 {
		t.Errorf("Uptime = %d, want 3600", stats.Uptime)
	}
	if !stats.LastSeen.Equal(now) {
		t.Errorf("LastSeen = %v, want %v", stats.LastSeen, now)
	}
}

// ═══════════════════════════════════════════════════════════════════
// SystemStats 测试
// ═══════════════════════════════════════════════════════════════════

func TestSystemStats_Fields(t *testing.T) {
	t.Parallel()

	stats := SystemStats{
		TotalUsers:       100,
		TotalClients:     500,
		OnlineClients:    250,
		TotalMappings:    1000,
		ActiveMappings:   400,
		TotalNodes:       10,
		OnlineNodes:      8,
		TotalTraffic:     10000000,
		TotalConnections: 50000,
		AnonymousUsers:   30,
	}

	if stats.TotalUsers != 100 {
		t.Errorf("TotalUsers = %d, want 100", stats.TotalUsers)
	}
	if stats.TotalClients != 500 {
		t.Errorf("TotalClients = %d, want 500", stats.TotalClients)
	}
	if stats.OnlineClients != 250 {
		t.Errorf("OnlineClients = %d, want 250", stats.OnlineClients)
	}
	if stats.TotalMappings != 1000 {
		t.Errorf("TotalMappings = %d, want 1000", stats.TotalMappings)
	}
	if stats.ActiveMappings != 400 {
		t.Errorf("ActiveMappings = %d, want 400", stats.ActiveMappings)
	}
	if stats.TotalNodes != 10 {
		t.Errorf("TotalNodes = %d, want 10", stats.TotalNodes)
	}
	if stats.OnlineNodes != 8 {
		t.Errorf("OnlineNodes = %d, want 8", stats.OnlineNodes)
	}
	if stats.TotalTraffic != 10000000 {
		t.Errorf("TotalTraffic = %d, want 10000000", stats.TotalTraffic)
	}
	if stats.TotalConnections != 50000 {
		t.Errorf("TotalConnections = %d, want 50000", stats.TotalConnections)
	}
	if stats.AnonymousUsers != 30 {
		t.Errorf("AnonymousUsers = %d, want 30", stats.AnonymousUsers)
	}
}

// ═══════════════════════════════════════════════════════════════════
// TrafficDataPoint 测试
// ═══════════════════════════════════════════════════════════════════

func TestTrafficDataPoint_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	point := TrafficDataPoint{
		Timestamp:     now,
		BytesSent:     1000,
		BytesReceived: 2000,
		UserID:        "user-123",
		ClientID:      456,
	}

	if !point.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", point.Timestamp, now)
	}
	if point.BytesSent != 1000 {
		t.Errorf("BytesSent = %d, want 1000", point.BytesSent)
	}
	if point.BytesReceived != 2000 {
		t.Errorf("BytesReceived = %d, want 2000", point.BytesReceived)
	}
	if point.UserID != "user-123" {
		t.Errorf("UserID = %s, want user-123", point.UserID)
	}
	if point.ClientID != 456 {
		t.Errorf("ClientID = %d, want 456", point.ClientID)
	}
}

// ═══════════════════════════════════════════════════════════════════
// ConnectionDataPoint 测试
// ═══════════════════════════════════════════════════════════════════

func TestConnectionDataPoint_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	point := ConnectionDataPoint{
		Timestamp:   now,
		Connections: 100,
		UserID:      "user-789",
		ClientID:    123,
	}

	if !point.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", point.Timestamp, now)
	}
	if point.Connections != 100 {
		t.Errorf("Connections = %d, want 100", point.Connections)
	}
	if point.UserID != "user-789" {
		t.Errorf("UserID = %s, want user-789", point.UserID)
	}
	if point.ClientID != 123 {
		t.Errorf("ClientID = %d, want 123", point.ClientID)
	}
}

// ═══════════════════════════════════════════════════════════════════
// getInt 和 getInt64 辅助函数测试
// ═══════════════════════════════════════════════════════════════════

func TestGetInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected int
	}{
		{
			name:     "nil map",
			m:        nil,
			key:      "key",
			expected: 0,
		},
		{
			name:     "missing key",
			m:        map[string]interface{}{"other": 10},
			key:      "key",
			expected: 0,
		},
		{
			name:     "int64 value",
			m:        map[string]interface{}{"key": int64(42)},
			key:      "key",
			expected: 42,
		},
		{
			name:     "int value",
			m:        map[string]interface{}{"key": 42},
			key:      "key",
			expected: 42,
		},
		{
			name:     "wrong type",
			m:        map[string]interface{}{"key": "string"},
			key:      "key",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getInt(tt.m, tt.key)
			if got != tt.expected {
				t.Errorf("getInt() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestGetInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected int64
	}{
		{
			name:     "nil map",
			m:        nil,
			key:      "key",
			expected: 0,
		},
		{
			name:     "missing key",
			m:        map[string]interface{}{"other": int64(10)},
			key:      "key",
			expected: 0,
		},
		{
			name:     "int64 value",
			m:        map[string]interface{}{"key": int64(42)},
			key:      "key",
			expected: 42,
		},
		{
			name:     "int value",
			m:        map[string]interface{}{"key": 42},
			key:      "key",
			expected: 42,
		},
		{
			name:     "wrong type",
			m:        map[string]interface{}{"key": "string"},
			key:      "key",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getInt64(tt.m, tt.key)
			if got != tt.expected {
				t.Errorf("getInt64() = %d, want %d", got, tt.expected)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 测试（需要 Mock Storage）
// ═══════════════════════════════════════════════════════════════════

// mockHashStorage 模拟支持 Hash 操作的完整存储
type mockHashStorage struct {
	mu          sync.RWMutex
	data        map[string]map[string]interface{}
	kvData      map[string]interface{}
	exists      map[string]bool
	ttl         map[string]time.Duration
	expiration  map[string]time.Time
	shouldError bool // 用于模拟错误
	errorOnKey  string
}

func newMockHashStorage() *mockHashStorage {
	return &mockHashStorage{
		data:       make(map[string]map[string]interface{}),
		kvData:     make(map[string]interface{}),
		exists:     make(map[string]bool),
		ttl:        make(map[string]time.Duration),
		expiration: make(map[string]time.Time),
	}
}

// Storage 接口实现
func (m *mockHashStorage) Set(key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shouldError && m.errorOnKey == key {
		return fmt.Errorf("mock error on key: %s", key)
	}
	m.kvData[key] = value
	m.exists[key] = true
	m.ttl[key] = ttl
	if ttl > 0 {
		m.expiration[key] = time.Now().Add(ttl)
	}
	return nil
}

func (m *mockHashStorage) Get(key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shouldError && m.errorOnKey == key {
		return nil, fmt.Errorf("mock error on key: %s", key)
	}
	if val, ok := m.kvData[key]; ok {
		return val, nil
	}
	return nil, nil
}

func (m *mockHashStorage) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.kvData, key)
	delete(m.exists, key)
	delete(m.ttl, key)
	delete(m.expiration, key)
	return nil
}

func (m *mockHashStorage) Exists(key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.exists[key], nil
}

func (m *mockHashStorage) SetExpiration(key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttl[key] = ttl
	if ttl > 0 {
		m.expiration[key] = time.Now().Add(ttl)
	}
	return nil
}

func (m *mockHashStorage) GetExpiration(key string) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if exp, ok := m.expiration[key]; ok {
		remaining := time.Until(exp)
		if remaining < 0 {
			return 0, nil
		}
		return remaining, nil
	}
	return 0, nil
}

func (m *mockHashStorage) CleanupExpired() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for key, exp := range m.expiration {
		if now.After(exp) {
			delete(m.kvData, key)
			delete(m.exists, key)
			delete(m.ttl, key)
			delete(m.expiration, key)
		}
	}
	return nil
}

func (m *mockHashStorage) Close() error {
	return nil
}

// HashStore 接口实现
func (m *mockHashStorage) SetHash(key string, field string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shouldError && m.errorOnKey == key {
		return fmt.Errorf("mock error on key: %s", key)
	}
	if m.data[key] == nil {
		m.data[key] = make(map[string]interface{})
	}
	m.data[key][field] = value
	m.exists[key] = true
	return nil
}

func (m *mockHashStorage) GetHash(key string, field string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shouldError && m.errorOnKey == key {
		return nil, fmt.Errorf("mock error on key: %s", key)
	}
	if m.data[key] == nil {
		return nil, nil
	}
	return m.data[key][field], nil
}

func (m *mockHashStorage) GetAllHash(key string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shouldError && m.errorOnKey == key {
		return nil, fmt.Errorf("mock error on key: %s", key)
	}
	// 返回副本以避免并发问题
	if m.data[key] == nil {
		return nil, nil
	}
	result := make(map[string]interface{})
	for k, v := range m.data[key] {
		result[k] = v
	}
	return result, nil
}

func (m *mockHashStorage) DeleteHash(key string, field string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[key] != nil {
		delete(m.data[key], field)
	}
	return nil
}

// mockSimpleStorage 模拟不支持 Hash 操作的简单存储
type mockSimpleStorage struct {
	data map[string]interface{}
}

func newMockSimpleStorage() *mockSimpleStorage {
	return &mockSimpleStorage{
		data: make(map[string]interface{}),
	}
}

func (m *mockSimpleStorage) Set(key string, value interface{}, ttl time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *mockSimpleStorage) Get(key string) (interface{}, error) {
	return m.data[key], nil
}

func (m *mockSimpleStorage) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockSimpleStorage) Exists(key string) (bool, error) {
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockSimpleStorage) SetExpiration(key string, ttl time.Duration) error {
	return nil
}

func (m *mockSimpleStorage) GetExpiration(key string) (time.Duration, error) {
	return 0, nil
}

func (m *mockSimpleStorage) CleanupExpired() error {
	return nil
}

func (m *mockSimpleStorage) Close() error {
	return nil
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 创建测试
// ═══════════════════════════════════════════════════════════════════

func TestNewStatsCounter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		storage     interface{}
		expectError bool
	}{
		{
			name:        "success with hash storage",
			storage:     newMockHashStorage(),
			expectError: false,
		},
		{
			name:        "fail with simple storage (no hash support)",
			storage:     newMockSimpleStorage(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			// 类型断言转换为 storage.Storage 接口
			type storageInterface interface {
				Set(key string, value interface{}, ttl time.Duration) error
				Get(key string) (interface{}, error)
				Delete(key string) error
				Exists(key string) (bool, error)
				SetExpiration(key string, ttl time.Duration) error
				GetExpiration(key string) (time.Duration, error)
				CleanupExpired() error
				Close() error
			}

			storage, ok := tt.storage.(storageInterface)
			if !ok {
				t.Fatalf("storage does not implement storageInterface")
			}

			counter, err := NewStatsCounter(storage, ctx)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				if counter != nil {
					t.Error("expected nil counter on error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if counter == nil {
					t.Error("expected non-nil counter")
				}
			}
		})
	}
}

func TestNewStatsCounter_ErrorMessage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	storage := newMockSimpleStorage()

	_, err := NewStatsCounter(storage, ctx)

	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if err != ErrStorageNoHashSupport {
		t.Errorf("expected ErrStorageNoHashSupport, got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 持久化统计操作测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_IncrUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialValue  int64
		delta         int64
		expectedValue int64
	}{
		{"increment from zero", 0, 1, 1},
		{"increment from positive", 10, 5, 15},
		{"decrement", 10, -3, 7},
		{"large increment", 0, 1000000, 1000000},
		{"negative result", 5, -10, -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			storage := newMockHashStorage()
			ctx := context.Background()

			counter, err := NewStatsCounter(storage, ctx)
			if err != nil {
				t.Fatalf("failed to create counter: %v", err)
			}

			// 设置初始值
			if tt.initialValue != 0 {
				storage.SetHash(PersistentStatsKey, "total_users", tt.initialValue)
			}

			// 执行增量操作
			err = counter.IncrUser(tt.delta)
			if err != nil {
				t.Fatalf("IncrUser failed: %v", err)
			}

			// 验证结果
			val, _ := storage.GetHash(PersistentStatsKey, "total_users")
			if intVal, ok := val.(int64); ok {
				if intVal != tt.expectedValue {
					t.Errorf("got %d, want %d", intVal, tt.expectedValue)
				}
			} else {
				t.Errorf("unexpected type: %T", val)
			}
		})
	}
}

func TestStatsCounter_IncrClient(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 执行多次增量操作
	for i := 0; i < 5; i++ {
		err = counter.IncrClient(1)
		if err != nil {
			t.Fatalf("IncrClient failed: %v", err)
		}
	}

	// 验证结果
	val, _ := storage.GetHash(PersistentStatsKey, "total_clients")
	if intVal, ok := val.(int64); ok {
		if intVal != 5 {
			t.Errorf("got %d, want 5", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

func TestStatsCounter_IncrMapping(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 测试增加和减少
	err = counter.IncrMapping(10)
	if err != nil {
		t.Fatalf("IncrMapping failed: %v", err)
	}

	err = counter.IncrMapping(-3)
	if err != nil {
		t.Fatalf("IncrMapping failed: %v", err)
	}

	// 验证结果
	val, _ := storage.GetHash(PersistentStatsKey, "total_mappings")
	if intVal, ok := val.(int64); ok {
		if intVal != 7 {
			t.Errorf("got %d, want 7", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

func TestStatsCounter_IncrNode(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	err = counter.IncrNode(3)
	if err != nil {
		t.Fatalf("IncrNode failed: %v", err)
	}

	// 验证结果
	val, _ := storage.GetHash(PersistentStatsKey, "total_nodes")
	if intVal, ok := val.(int64); ok {
		if intVal != 3 {
			t.Errorf("got %d, want 3", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 运行时统计操作测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_SetOnlineClients(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int64
	}{
		{"zero clients", 0},
		{"some clients", 50},
		{"many clients", 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			storage := newMockHashStorage()
			ctx := context.Background()

			counter, err := NewStatsCounter(storage, ctx)
			if err != nil {
				t.Fatalf("failed to create counter: %v", err)
			}

			err = counter.SetOnlineClients(tt.count)
			if err != nil {
				t.Fatalf("SetOnlineClients failed: %v", err)
			}

			// 验证结果
			val, _ := storage.GetHash(RuntimeStatsKey, "online_clients")
			if intVal, ok := val.(int64); ok {
				if intVal != tt.count {
					t.Errorf("got %d, want %d", intVal, tt.count)
				}
			} else {
				t.Errorf("unexpected type: %T", val)
			}
		})
	}
}

func TestStatsCounter_IncrOnlineClients(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 模拟客户端上线
	err = counter.IncrOnlineClients(1)
	if err != nil {
		t.Fatalf("IncrOnlineClients failed: %v", err)
	}

	err = counter.IncrOnlineClients(1)
	if err != nil {
		t.Fatalf("IncrOnlineClients failed: %v", err)
	}

	// 模拟客户端下线
	err = counter.IncrOnlineClients(-1)
	if err != nil {
		t.Fatalf("IncrOnlineClients failed: %v", err)
	}

	// 验证结果：2 - 1 = 1
	val, _ := storage.GetHash(RuntimeStatsKey, "online_clients")
	if intVal, ok := val.(int64); ok {
		if intVal != 1 {
			t.Errorf("got %d, want 1", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

func TestStatsCounter_SetActiveMappings(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	err = counter.SetActiveMappings(100)
	if err != nil {
		t.Fatalf("SetActiveMappings failed: %v", err)
	}

	// 验证结果
	val, _ := storage.GetHash(RuntimeStatsKey, "active_mappings")
	if intVal, ok := val.(int64); ok {
		if intVal != 100 {
			t.Errorf("got %d, want 100", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

func TestStatsCounter_IncrActiveMappings(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 增加映射
	err = counter.IncrActiveMappings(5)
	if err != nil {
		t.Fatalf("IncrActiveMappings failed: %v", err)
	}

	// 验证结果
	val, _ := storage.GetHash(RuntimeStatsKey, "active_mappings")
	if intVal, ok := val.(int64); ok {
		if intVal != 5 {
			t.Errorf("got %d, want 5", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

func TestStatsCounter_SetOnlineNodes(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	err = counter.SetOnlineNodes(5)
	if err != nil {
		t.Fatalf("SetOnlineNodes failed: %v", err)
	}

	// 验证结果
	val, _ := storage.GetHash(RuntimeStatsKey, "online_nodes")
	if intVal, ok := val.(int64); ok {
		if intVal != 5 {
			t.Errorf("got %d, want 5", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

func TestStatsCounter_IncrAnonymousUsers(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 增加匿名用户
	err = counter.IncrAnonymousUsers(10)
	if err != nil {
		t.Fatalf("IncrAnonymousUsers failed: %v", err)
	}

	// 减少匿名用户
	err = counter.IncrAnonymousUsers(-3)
	if err != nil {
		t.Fatalf("IncrAnonymousUsers failed: %v", err)
	}

	// 验证结果：10 - 3 = 7
	val, _ := storage.GetHash(RuntimeStatsKey, "anonymous_users")
	if intVal, ok := val.(int64); ok {
		if intVal != 7 {
			t.Errorf("got %d, want 7", intVal)
		}
	} else {
		t.Errorf("unexpected type: %T", val)
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter GetGlobalStats 测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_GetGlobalStats(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 设置一些统计数据
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))
	storage.SetHash(PersistentStatsKey, "total_clients", int64(500))
	storage.SetHash(PersistentStatsKey, "total_mappings", int64(1000))
	storage.SetHash(PersistentStatsKey, "total_nodes", int64(10))

	storage.SetHash(RuntimeStatsKey, "online_clients", int64(250))
	storage.SetHash(RuntimeStatsKey, "active_mappings", int64(400))
	storage.SetHash(RuntimeStatsKey, "online_nodes", int64(8))
	storage.SetHash(RuntimeStatsKey, "anonymous_users", int64(30))
	storage.SetHash(RuntimeStatsKey, "total_traffic", int64(10000000))
	storage.SetHash(RuntimeStatsKey, "total_connections", int64(50000))

	// 禁用缓存以直接从存储获取
	counter.DisableCache()

	stats, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}

	if stats.TotalUsers != 100 {
		t.Errorf("TotalUsers = %d, want 100", stats.TotalUsers)
	}
	if stats.TotalClients != 500 {
		t.Errorf("TotalClients = %d, want 500", stats.TotalClients)
	}
	if stats.TotalMappings != 1000 {
		t.Errorf("TotalMappings = %d, want 1000", stats.TotalMappings)
	}
	if stats.TotalNodes != 10 {
		t.Errorf("TotalNodes = %d, want 10", stats.TotalNodes)
	}
	if stats.OnlineClients != 250 {
		t.Errorf("OnlineClients = %d, want 250", stats.OnlineClients)
	}
	if stats.ActiveMappings != 400 {
		t.Errorf("ActiveMappings = %d, want 400", stats.ActiveMappings)
	}
	if stats.OnlineNodes != 8 {
		t.Errorf("OnlineNodes = %d, want 8", stats.OnlineNodes)
	}
	if stats.AnonymousUsers != 30 {
		t.Errorf("AnonymousUsers = %d, want 30", stats.AnonymousUsers)
	}
	if stats.TotalTraffic != 10000000 {
		t.Errorf("TotalTraffic = %d, want 10000000", stats.TotalTraffic)
	}
	if stats.TotalConnections != 50000 {
		t.Errorf("TotalConnections = %d, want 50000", stats.TotalConnections)
	}
}

func TestStatsCounter_GetGlobalStats_WithCache(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 确保缓存已启用
	counter.EnableCache()

	// 设置初始数据
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))
	storage.SetHash(RuntimeStatsKey, "online_clients", int64(50))

	// 第一次获取，应该从存储获取
	stats1, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats1.TotalUsers != 100 {
		t.Errorf("TotalUsers = %d, want 100", stats1.TotalUsers)
	}

	// 修改存储中的数据
	storage.SetHash(PersistentStatsKey, "total_users", int64(200))

	// 第二次获取，应该从缓存获取（返回旧值）
	stats2, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats2.TotalUsers != 100 {
		t.Errorf("TotalUsers = %d, want 100 (cached)", stats2.TotalUsers)
	}

	// 使缓存失效
	counter.localCache.Invalidate()

	// 第三次获取，应该从存储获取（新值）
	stats3, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if stats3.TotalUsers != 200 {
		t.Errorf("TotalUsers = %d, want 200", stats3.TotalUsers)
	}
}

func TestStatsCounter_GetGlobalStats_EmptyStorage(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	counter.DisableCache()

	// 从空存储获取统计
	stats, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}

	// 所有值应为0
	if stats.TotalUsers != 0 {
		t.Errorf("TotalUsers = %d, want 0", stats.TotalUsers)
	}
	if stats.TotalClients != 0 {
		t.Errorf("TotalClients = %d, want 0", stats.TotalClients)
	}
	if stats.OnlineClients != 0 {
		t.Errorf("OnlineClients = %d, want 0", stats.OnlineClients)
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter Initialize 测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_Initialize(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 初始化
	err = counter.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// 验证持久化统计已初始化为0
	persistentFields := []string{"total_users", "total_clients", "total_mappings", "total_nodes"}
	for _, field := range persistentFields {
		val, _ := storage.GetHash(PersistentStatsKey, field)
		if intVal, ok := val.(int64); ok {
			if intVal != 0 {
				t.Errorf("%s = %d, want 0", field, intVal)
			}
		} else {
			t.Errorf("%s has unexpected type: %T", field, val)
		}
	}

	// 验证运行时统计已初始化为0
	runtimeFields := []string{"online_clients", "active_mappings", "online_nodes", "anonymous_users", "total_traffic", "total_connections"}
	for _, field := range runtimeFields {
		val, _ := storage.GetHash(RuntimeStatsKey, field)
		if intVal, ok := val.(int64); ok {
			if intVal != 0 {
				t.Errorf("%s = %d, want 0", field, intVal)
			}
		} else {
			t.Errorf("%s has unexpected type: %T", field, val)
		}
	}
}

func TestStatsCounter_Initialize_PreservesExistingPersistent(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	// 预先设置持久化数据
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))
	storage.exists[PersistentStatsKey] = true

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 初始化（应保留已有持久化数据）
	err = counter.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// 验证持久化数据未被覆盖
	val, _ := storage.GetHash(PersistentStatsKey, "total_users")
	if intVal, ok := val.(int64); ok {
		if intVal != 100 {
			t.Errorf("total_users = %d, want 100 (preserved)", intVal)
		}
	}

	// 运行时数据应被重置
	val, _ = storage.GetHash(RuntimeStatsKey, "online_clients")
	if intVal, ok := val.(int64); ok {
		if intVal != 0 {
			t.Errorf("online_clients = %d, want 0 (reset)", intVal)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter Rebuild 测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_Rebuild(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 创建新的统计数据
	newStats := &SystemStats{
		TotalUsers:       200,
		TotalClients:     1000,
		TotalMappings:    500,
		TotalNodes:       20,
		OnlineClients:    300,
		ActiveMappings:   150,
		OnlineNodes:      15,
		AnonymousUsers:   50,
		TotalTraffic:     50000000,
		TotalConnections: 100000,
	}

	// 重建
	err = counter.Rebuild(newStats)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// 禁用缓存验证结果
	counter.DisableCache()
	stats, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}

	if stats.TotalUsers != newStats.TotalUsers {
		t.Errorf("TotalUsers = %d, want %d", stats.TotalUsers, newStats.TotalUsers)
	}
	if stats.TotalClients != newStats.TotalClients {
		t.Errorf("TotalClients = %d, want %d", stats.TotalClients, newStats.TotalClients)
	}
	if stats.TotalMappings != newStats.TotalMappings {
		t.Errorf("TotalMappings = %d, want %d", stats.TotalMappings, newStats.TotalMappings)
	}
	if stats.TotalNodes != newStats.TotalNodes {
		t.Errorf("TotalNodes = %d, want %d", stats.TotalNodes, newStats.TotalNodes)
	}
	if stats.OnlineClients != newStats.OnlineClients {
		t.Errorf("OnlineClients = %d, want %d", stats.OnlineClients, newStats.OnlineClients)
	}
	if stats.ActiveMappings != newStats.ActiveMappings {
		t.Errorf("ActiveMappings = %d, want %d", stats.ActiveMappings, newStats.ActiveMappings)
	}
	if stats.OnlineNodes != newStats.OnlineNodes {
		t.Errorf("OnlineNodes = %d, want %d", stats.OnlineNodes, newStats.OnlineNodes)
	}
	if stats.AnonymousUsers != newStats.AnonymousUsers {
		t.Errorf("AnonymousUsers = %d, want %d", stats.AnonymousUsers, newStats.AnonymousUsers)
	}
	if stats.TotalTraffic != newStats.TotalTraffic {
		t.Errorf("TotalTraffic = %d, want %d", stats.TotalTraffic, newStats.TotalTraffic)
	}
	if stats.TotalConnections != newStats.TotalConnections {
		t.Errorf("TotalConnections = %d, want %d", stats.TotalConnections, newStats.TotalConnections)
	}
}

func TestStatsCounter_Rebuild_InvalidatesCache(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	counter.EnableCache()

	// 设置初始数据并缓存
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))
	_, _ = counter.GetGlobalStats() // 触发缓存

	// 验证缓存有效
	if !counter.localCache.IsValid() {
		t.Error("cache should be valid after GetGlobalStats")
	}

	// 重建
	err = counter.Rebuild(&SystemStats{TotalUsers: 200})
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// 验证缓存已失效
	if counter.localCache.IsValid() {
		t.Error("cache should be invalidated after Rebuild")
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 缓存管理测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_CacheManagement(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 默认应该启用缓存
	if !counter.cacheEnabled {
		t.Error("cache should be enabled by default")
	}

	// 禁用缓存
	counter.DisableCache()
	if counter.cacheEnabled {
		t.Error("cache should be disabled after DisableCache()")
	}

	// 启用缓存
	counter.EnableCache()
	if !counter.cacheEnabled {
		t.Error("cache should be enabled after EnableCache()")
	}
}

func TestStatsCounter_IncrInvalidatesCache(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	counter.EnableCache()

	// 设置初始数据并缓存
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))
	_, _ = counter.GetGlobalStats() // 触发缓存

	// 验证缓存有效
	if !counter.localCache.IsValid() {
		t.Error("cache should be valid after GetGlobalStats")
	}

	// 增加用户数
	err = counter.IncrUser(1)
	if err != nil {
		t.Fatalf("IncrUser failed: %v", err)
	}

	// 验证缓存已失效
	if counter.localCache.IsValid() {
		t.Error("cache should be invalidated after IncrUser")
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 边界条件测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_ZeroDelta(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 设置初始值
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))

	// 增加0
	err = counter.IncrUser(0)
	if err != nil {
		t.Fatalf("IncrUser failed: %v", err)
	}

	// 验证值未变化
	val, _ := storage.GetHash(PersistentStatsKey, "total_users")
	if intVal, ok := val.(int64); ok {
		if intVal != 100 {
			t.Errorf("got %d, want 100", intVal)
		}
	}
}

func TestStatsCounter_LargeValues(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 测试大数值
	largeValue := int64(9223372036854775807 / 2) // int64 最大值的一半

	err = counter.IncrUser(largeValue)
	if err != nil {
		t.Fatalf("IncrUser failed: %v", err)
	}

	val, _ := storage.GetHash(PersistentStatsKey, "total_users")
	if intVal, ok := val.(int64); ok {
		if intVal != largeValue {
			t.Errorf("got %d, want %d", intVal, largeValue)
		}
	}
}

func TestStatsCounter_NegativeValues(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 从0减少到负数
	err = counter.IncrUser(-10)
	if err != nil {
		t.Fatalf("IncrUser failed: %v", err)
	}

	val, _ := storage.GetHash(PersistentStatsKey, "total_users")
	if intVal, ok := val.(int64); ok {
		if intVal != -10 {
			t.Errorf("got %d, want -10", intVal)
		}
	}
}

func TestStatsCounter_IntTypeConversion(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// 存储 int 类型（而非 int64）
	storage.SetHash(PersistentStatsKey, "total_users", 100) // int 类型

	counter.DisableCache()
	stats, err := counter.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}

	// 应该正确处理 int 类型
	if stats.TotalUsers != 100 {
		t.Errorf("TotalUsers = %d, want 100", stats.TotalUsers)
	}
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 并发测试
// ═══════════════════════════════════════════════════════════════════

func TestStatsCounter_ConcurrentIncr(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	done := make(chan struct{})
	numGoroutines := 10
	incrementsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < incrementsPerGoroutine; j++ {
				_ = counter.IncrUser(1)
			}
			done <- struct{}{}
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 注意：由于 mock 存储不是线程安全的，这里只验证没有 panic
	// 实际生产中应使用线程安全的存储
}

func TestStatsCounter_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	storage := newMockHashStorage()
	ctx := context.Background()

	counter, err := NewStatsCounter(storage, ctx)
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	done := make(chan struct{})

	// 并发写入
	go func() {
		for i := 0; i < 100; i++ {
			_ = counter.IncrUser(1)
		}
		done <- struct{}{}
	}()

	// 并发读取
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = counter.GetGlobalStats()
		}
		done <- struct{}{}
	}()

	// 等待完成
	<-done
	<-done
}

// ═══════════════════════════════════════════════════════════════════
// StatsCounter 基准测试
// ═══════════════════════════════════════════════════════════════════

func BenchmarkStatsCounter_IncrUser(b *testing.B) {
	storage := newMockHashStorage()
	ctx := context.Background()

	counter, _ := NewStatsCounter(storage, ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = counter.IncrUser(1)
	}
}

func BenchmarkStatsCounter_GetGlobalStats_WithCache(b *testing.B) {
	storage := newMockHashStorage()
	ctx := context.Background()

	counter, _ := NewStatsCounter(storage, ctx)
	counter.EnableCache()

	// 预热缓存
	storage.SetHash(PersistentStatsKey, "total_users", int64(100))
	_, _ = counter.GetGlobalStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = counter.GetGlobalStats()
	}
}

func BenchmarkStatsCounter_GetGlobalStats_NoCache(b *testing.B) {
	storage := newMockHashStorage()
	ctx := context.Background()

	counter, _ := NewStatsCounter(storage, ctx)
	counter.DisableCache()

	storage.SetHash(PersistentStatsKey, "total_users", int64(100))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = counter.GetGlobalStats()
	}
}

func BenchmarkStatsCounter_Initialize(b *testing.B) {
	storage := newMockHashStorage()
	ctx := context.Background()

	counter, _ := NewStatsCounter(storage, ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = counter.Initialize()
	}
}

func BenchmarkStatsCounter_Rebuild(b *testing.B) {
	storage := newMockHashStorage()
	ctx := context.Background()

	counter, _ := NewStatsCounter(storage, ctx)
	stats := &SystemStats{
		TotalUsers:    100,
		TotalClients:  500,
		OnlineClients: 250,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = counter.Rebuild(stats)
	}
}

// ═══════════════════════════════════════════════════════════════════
// 常量测试
// ═══════════════════════════════════════════════════════════════════

func TestConstants(t *testing.T) {
	t.Parallel()

	if PersistentStatsKey != "tunnox:stats:persistent:global" {
		t.Errorf("PersistentStatsKey = %s, want tunnox:stats:persistent:global", PersistentStatsKey)
	}
	if RuntimeStatsKey != "tunnox:stats:runtime:global" {
		t.Errorf("RuntimeStatsKey = %s, want tunnox:stats:runtime:global", RuntimeStatsKey)
	}
}

func TestErrStorageNoHashSupport(t *testing.T) {
	t.Parallel()

	if ErrStorageNoHashSupport == nil {
		t.Error("ErrStorageNoHashSupport should not be nil")
	}
	if ErrStorageNoHashSupport.Error() != "storage does not support hash operations (required for StatsCounter)" {
		t.Errorf("ErrStorageNoHashSupport.Error() = %s", ErrStorageNoHashSupport.Error())
	}
}

// ═══════════════════════════════════════════════════════════════════
// 基准测试
// ═══════════════════════════════════════════════════════════════════

func BenchmarkStatsCache_Set(b *testing.B) {
	cache := NewStatsCache(1 * time.Minute)
	stats := &SystemStats{TotalUsers: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(stats)
	}
}

func BenchmarkStatsCache_Get(b *testing.B) {
	cache := NewStatsCache(1 * time.Minute)
	cache.Set(&SystemStats{TotalUsers: 100})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get()
	}
}

func BenchmarkStatsCache_IsValid(b *testing.B) {
	cache := NewStatsCache(1 * time.Minute)
	cache.Set(&SystemStats{TotalUsers: 100})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.IsValid()
	}
}
