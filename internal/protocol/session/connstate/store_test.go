// Package connstate 连接状态存储测试
package connstate

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"tunnox-core/internal/core/storage"
)

// ============================================================================
// MockStorage 模拟存储
// ============================================================================

type mockStorage struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		data: make(map[string]interface{}),
	}
}

func (s *mockStorage) Get(key string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.data[key]
	if !ok {
		return nil, storage.ErrKeyNotFound
	}
	return val, nil
}

func (s *mockStorage) Set(key string, value interface{}, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 对于字符串类型，直接存储（模拟 Redis 的 string 类型存储行为）
	if str, ok := value.(string); ok {
		s.data[key] = str
		return nil
	}

	// 将值序列化为JSON再反序列化为map，模拟Redis行为
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	var mapVal map[string]interface{}
	if err := json.Unmarshal(data, &mapVal); err != nil {
		// 如果不是对象，直接存储字符串
		s.data[key] = string(data)
	} else {
		s.data[key] = mapVal
	}

	return nil
}

func (s *mockStorage) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	return nil
}

func (s *mockStorage) Exists(key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.data[key]
	return ok, nil
}

func (s *mockStorage) SetExpiration(key string, ttl time.Duration) error {
	return nil
}

func (s *mockStorage) GetExpiration(key string) (time.Duration, error) {
	return 0, nil
}

func (s *mockStorage) CleanupExpired() error {
	return nil
}

func (s *mockStorage) Close() error {
	return nil
}

// ============================================================================
// Store 创建测试
// ============================================================================

func TestNewStore(t *testing.T) {
	ms := newMockStorage()

	tests := []struct {
		name        string
		nodeID      string
		ttl         time.Duration
		expectedTTL time.Duration
	}{
		{"with_ttl", "node-1", 10 * time.Minute, 10 * time.Minute},
		{"zero_ttl", "node-2", 0, 5 * time.Minute},
		{"short_ttl", "node-3", 1 * time.Minute, 1 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(ms, tt.nodeID, tt.ttl)

			if store == nil {
				t.Fatal("NewStore should not return nil")
			}

			if store.nodeID != tt.nodeID {
				t.Errorf("nodeID should be %s, got %s", tt.nodeID, store.nodeID)
			}

			if store.ttl != tt.expectedTTL {
				t.Errorf("ttl should be %v, got %v", tt.expectedTTL, store.ttl)
			}

			if store.storage == nil {
				t.Error("storage should be set")
			}
		})
	}
}

// ============================================================================
// RegisterConnection 测试
// ============================================================================

func TestStore_RegisterConnection(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	state := &Info{
		ConnectionID: "conn-001",
		ClientID:     100,
		Protocol:     "tcp",
		ConnType:     "control",
		MappingID:    "mapping-001",
	}

	err := store.RegisterConnection(ctx, state)
	if err != nil {
		t.Errorf("RegisterConnection should not return error: %v", err)
	}

	// 验证 NodeID 已设置
	if state.NodeID != "node-test" {
		t.Errorf("NodeID should be set to 'node-test', got %s", state.NodeID)
	}

	// 验证时间戳已设置
	if state.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	if state.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}

	// 验证数据已存储
	key := store.makeConnectionKey(state.ConnectionID)
	_, exists := ms.data[key]
	if !exists {
		t.Error("connection state should be stored")
	}
}

func TestStore_RegisterConnection_EmptyConnectionID(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	state := &Info{
		ConnectionID: "",
		ClientID:     100,
	}

	err := store.RegisterConnection(ctx, state)
	if err == nil {
		t.Error("RegisterConnection with empty connection_id should return error")
	}
}

func TestStore_RegisterConnection_ControlWithClientIndex(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	state := &Info{
		ConnectionID: "conn-002",
		ClientID:     200,
		ConnType:     "control",
	}

	err := store.RegisterConnection(ctx, state)
	if err != nil {
		t.Errorf("RegisterConnection should not return error: %v", err)
	}

	// 验证客户端索引已创建
	clientKey := store.makeClientKey(state.ClientID)
	val, exists := ms.data[clientKey]
	if !exists {
		t.Error("client index should be created for control connection")
	}

	// 验证索引值
	if val != "conn-002" {
		t.Errorf("client index should point to connection ID, got %v", val)
	}
}

func TestStore_RegisterConnection_TunnelWithoutClientIndex(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	state := &Info{
		ConnectionID: "conn-003",
		ClientID:     300,
		ConnType:     "tunnel", // 隧道连接不创建客户端索引
	}

	err := store.RegisterConnection(ctx, state)
	if err != nil {
		t.Errorf("RegisterConnection should not return error: %v", err)
	}

	// 验证客户端索引未创建
	clientKey := store.makeClientKey(state.ClientID)
	_, exists := ms.data[clientKey]
	if exists {
		t.Error("client index should not be created for tunnel connection")
	}
}

// ============================================================================
// UnregisterConnection 测试
// ============================================================================

func TestStore_UnregisterConnection(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	// 先注册连接
	state := &Info{
		ConnectionID: "conn-unregister",
		ClientID:     400,
		ConnType:     "control",
	}
	store.RegisterConnection(ctx, state)

	// 注销连接
	err := store.UnregisterConnection(ctx, "conn-unregister")
	if err != nil {
		t.Errorf("UnregisterConnection should not return error: %v", err)
	}

	// 验证连接状态已删除
	connKey := store.makeConnectionKey("conn-unregister")
	_, exists := ms.data[connKey]
	if exists {
		t.Error("connection state should be deleted")
	}

	// 验证客户端索引已删除
	clientKey := store.makeClientKey(400)
	_, exists = ms.data[clientKey]
	if exists {
		t.Error("client index should be deleted")
	}
}

func TestStore_UnregisterConnection_EmptyConnectionID(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	err := store.UnregisterConnection(ctx, "")
	if err == nil {
		t.Error("UnregisterConnection with empty connection_id should return error")
	}
}

func TestStore_UnregisterConnection_NonExistent(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	// 注销不存在的连接不应该返回错误
	err := store.UnregisterConnection(ctx, "non-existent")
	if err != nil {
		t.Errorf("UnregisterConnection for non-existent should not return error: %v", err)
	}
}

// ============================================================================
// GetConnectionState 测试
// ============================================================================

func TestStore_GetConnectionState(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	// 先注册连接
	originalState := &Info{
		ConnectionID: "conn-get",
		ClientID:     500,
		Protocol:     "websocket",
		ConnType:     "control",
		MappingID:    "mapping-get",
	}
	store.RegisterConnection(ctx, originalState)

	// 获取连接状态
	state, err := store.GetConnectionState(ctx, "conn-get")
	if err != nil {
		t.Errorf("GetConnectionState should not return error: %v", err)
	}

	if state.ConnectionID != originalState.ConnectionID {
		t.Errorf("ConnectionID should be %s, got %s", originalState.ConnectionID, state.ConnectionID)
	}

	if state.ClientID != originalState.ClientID {
		t.Errorf("ClientID should be %d, got %d", originalState.ClientID, state.ClientID)
	}

	if state.Protocol != originalState.Protocol {
		t.Errorf("Protocol should be %s, got %s", originalState.Protocol, state.Protocol)
	}

	if state.NodeID != "node-test" {
		t.Errorf("NodeID should be 'node-test', got %s", state.NodeID)
	}
}

func TestStore_GetConnectionState_EmptyConnectionID(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	_, err := store.GetConnectionState(ctx, "")
	if err == nil {
		t.Error("GetConnectionState with empty connection_id should return error")
	}
}

func TestStore_GetConnectionState_NotFound(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	_, err := store.GetConnectionState(ctx, "non-existent")
	if err != ErrConnectionNotFound {
		t.Errorf("GetConnectionState for non-existent should return ErrConnectionNotFound, got %v", err)
	}
}

func TestStore_GetConnectionState_Expired(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 1*time.Millisecond) // 非常短的TTL
	ctx := context.Background()

	// 注册连接
	state := &Info{
		ConnectionID: "conn-expired",
		ClientID:     600,
	}
	store.RegisterConnection(ctx, state)

	// 等待过期
	time.Sleep(10 * time.Millisecond)

	// 获取应该返回过期错误
	_, err := store.GetConnectionState(ctx, "conn-expired")
	if err != ErrConnectionExpired {
		t.Errorf("GetConnectionState for expired should return ErrConnectionExpired, got %v", err)
	}
}

// ============================================================================
// FindConnectionNode 测试
// ============================================================================

func TestStore_FindConnectionNode(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-find", 5*time.Minute)
	ctx := context.Background()

	// 注册连接
	state := &Info{
		ConnectionID: "conn-find-node",
		ClientID:     700,
	}
	store.RegisterConnection(ctx, state)

	// 查找节点
	nodeID, err := store.FindConnectionNode(ctx, "conn-find-node")
	if err != nil {
		t.Errorf("FindConnectionNode should not return error: %v", err)
	}

	if nodeID != "node-find" {
		t.Errorf("nodeID should be 'node-find', got %s", nodeID)
	}
}

func TestStore_FindConnectionNode_NotFound(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	_, err := store.FindConnectionNode(ctx, "non-existent")
	if err == nil {
		t.Error("FindConnectionNode for non-existent should return error")
	}
}

// ============================================================================
// FindClientNode 测试
// ============================================================================

func TestStore_FindClientNode(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-client", 5*time.Minute)
	ctx := context.Background()

	// 注册控制连接
	state := &Info{
		ConnectionID: "conn-client-node",
		ClientID:     800,
		ConnType:     "control",
	}
	store.RegisterConnection(ctx, state)

	// 查找客户端节点
	nodeID, connID, err := store.FindClientNode(ctx, 800)
	if err != nil {
		t.Errorf("FindClientNode should not return error: %v", err)
	}

	if nodeID != "node-client" {
		t.Errorf("nodeID should be 'node-client', got %s", nodeID)
	}

	if connID != "conn-client-node" {
		t.Errorf("connectionID should be 'conn-client-node', got %s", connID)
	}
}

func TestStore_FindClientNode_InvalidClientID(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	_, _, err := store.FindClientNode(ctx, 0)
	if err == nil {
		t.Error("FindClientNode with zero client_id should return error")
	}

	_, _, err = store.FindClientNode(ctx, -1)
	if err == nil {
		t.Error("FindClientNode with negative client_id should return error")
	}
}

func TestStore_FindClientNode_NotFound(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	_, _, err := store.FindClientNode(ctx, 9999)
	if err != ErrConnectionNotFound {
		t.Errorf("FindClientNode for non-existent should return ErrConnectionNotFound, got %v", err)
	}
}

// ============================================================================
// RefreshConnection 测试
// ============================================================================

func TestStore_RefreshConnection(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-refresh", 5*time.Minute)
	ctx := context.Background()

	// 注册连接
	state := &Info{
		ConnectionID: "conn-refresh",
		ClientID:     900,
	}
	store.RegisterConnection(ctx, state)

	originalExpires := state.ExpiresAt

	// 等待一下
	time.Sleep(10 * time.Millisecond)

	// 刷新连接
	err := store.RefreshConnection(ctx, "conn-refresh")
	if err != nil {
		t.Errorf("RefreshConnection should not return error: %v", err)
	}

	// 获取刷新后的状态
	refreshedState, err := store.GetConnectionState(ctx, "conn-refresh")
	if err != nil {
		t.Errorf("GetConnectionState should not return error: %v", err)
	}

	// 过期时间应该延长
	if !refreshedState.ExpiresAt.After(originalExpires) {
		t.Error("ExpiresAt should be extended after refresh")
	}
}

func TestStore_RefreshConnection_NotFound(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	err := store.RefreshConnection(ctx, "non-existent")
	if err == nil {
		t.Error("RefreshConnection for non-existent should return error")
	}
}

// ============================================================================
// IsConnectionLocal 测试
// ============================================================================

func TestStore_IsConnectionLocal(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-local", 5*time.Minute)
	ctx := context.Background()

	// 注册本地连接
	state := &Info{
		ConnectionID: "conn-local",
		ClientID:     1000,
	}
	store.RegisterConnection(ctx, state)

	// 检查是否本地
	isLocal, err := store.IsConnectionLocal(ctx, "conn-local")
	if err != nil {
		t.Errorf("IsConnectionLocal should not return error: %v", err)
	}

	if !isLocal {
		t.Error("connection should be local")
	}
}

func TestStore_IsConnectionLocal_Remote(t *testing.T) {
	ms := newMockStorage()

	// 创建两个不同节点的存储
	store1 := NewStore(ms, "node-1", 5*time.Minute)
	store2 := NewStore(ms, "node-2", 5*time.Minute)
	ctx := context.Background()

	// 在 node-1 注册连接
	state := &Info{
		ConnectionID: "conn-remote",
		ClientID:     1100,
	}
	store1.RegisterConnection(ctx, state)

	// 从 node-2 检查
	isLocal, err := store2.IsConnectionLocal(ctx, "conn-remote")
	if err != nil {
		t.Errorf("IsConnectionLocal should not return error: %v", err)
	}

	if isLocal {
		t.Error("connection should not be local for different node")
	}
}

func TestStore_IsConnectionLocal_NotFound(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)
	ctx := context.Background()

	_, err := store.IsConnectionLocal(ctx, "non-existent")
	if err == nil {
		t.Error("IsConnectionLocal for non-existent should return error")
	}
}

// ============================================================================
// Key 生成测试
// ============================================================================

func TestStore_makeConnectionKey(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)

	key := store.makeConnectionKey("conn-123")
	expected := "tunnox:conn_state:conn-123"

	if key != expected {
		t.Errorf("connection key should be %s, got %s", expected, key)
	}
}

func TestStore_makeClientKey(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-test", 5*time.Minute)

	key := store.makeClientKey(12345)
	expected := "tunnox:client_conn:12345"

	if key != expected {
		t.Errorf("client key should be %s, got %s", expected, key)
	}
}

// ============================================================================
// Error 类型测试
// ============================================================================

func TestErrors(t *testing.T) {
	if ErrConnectionNotFound == nil {
		t.Error("ErrConnectionNotFound should not be nil")
	}

	if ErrConnectionExpired == nil {
		t.Error("ErrConnectionExpired should not be nil")
	}

	// 验证错误消息
	if ErrConnectionNotFound.Error() == "" {
		t.Error("ErrConnectionNotFound should have error message")
	}

	if ErrConnectionExpired.Error() == "" {
		t.Error("ErrConnectionExpired should have error message")
	}
}

// ============================================================================
// Info 类型测试
// ============================================================================

func TestInfo_JSONSerialization(t *testing.T) {
	info := &Info{
		ConnectionID: "conn-json",
		ClientID:     1200,
		NodeID:       "node-json",
		Protocol:     "quic",
		ConnType:     "control",
		MappingID:    "mapping-json",
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}

	// 序列化
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded Info
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// 验证字段
	if loaded.ConnectionID != info.ConnectionID {
		t.Errorf("ConnectionID should be %s, got %s", info.ConnectionID, loaded.ConnectionID)
	}

	if loaded.ClientID != info.ClientID {
		t.Errorf("ClientID should be %d, got %d", info.ClientID, loaded.ClientID)
	}

	if loaded.Protocol != info.Protocol {
		t.Errorf("Protocol should be %s, got %s", info.Protocol, loaded.Protocol)
	}

	if loaded.ConnType != info.ConnType {
		t.Errorf("ConnType should be %s, got %s", info.ConnType, loaded.ConnType)
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestStore_ConcurrentRegisterUnregister(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-concurrent", 5*time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines * 2)

	// 并发注册
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			state := &Info{
				ConnectionID: "conn-concurrent-" + string(rune('A'+id%26)),
				ClientID:     int64(id),
				ConnType:     "control",
			}
			store.RegisterConnection(ctx, state)
		}(i)
	}

	// 并发注销
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			store.UnregisterConnection(ctx, "conn-concurrent-"+string(rune('A'+id%26)))
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}

func TestStore_ConcurrentReads(t *testing.T) {
	ms := newMockStorage()
	store := NewStore(ms, "node-concurrent-read", 5*time.Minute)
	ctx := context.Background()

	// 先注册一些连接
	for i := 0; i < 10; i++ {
		state := &Info{
			ConnectionID: "conn-read-" + string(rune('A'+i)),
			ClientID:     int64(i + 1),
			ConnType:     "control",
		}
		store.RegisterConnection(ctx, state)
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines * 3)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			store.GetConnectionState(ctx, "conn-read-"+string(rune('A'+id%10)))
		}(i)

		go func(id int) {
			defer wg.Done()
			store.FindConnectionNode(ctx, "conn-read-"+string(rune('A'+id%10)))
		}(i)

		go func(id int) {
			defer wg.Done()
			store.IsConnectionLocal(ctx, "conn-read-"+string(rune('A'+id%10)))
		}(i)
	}

	wg.Wait()
	// 测试不应该 panic
}
