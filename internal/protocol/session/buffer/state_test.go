// Package buffer 状态管理测试
package buffer

import (
	"encoding/json"
	"testing"
	"time"

	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"
)

// ============================================================================
// MockStorage 模拟存储
// ============================================================================

type mockStorage struct {
	data map[string]interface{}
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		data: make(map[string]interface{}),
	}
}

func (s *mockStorage) Get(key string) (interface{}, error) {
	val, ok := s.data[key]
	if !ok {
		return nil, storage.ErrKeyNotFound
	}
	return val, nil
}

func (s *mockStorage) Set(key string, value interface{}, ttl time.Duration) error {
	s.data[key] = value
	return nil
}

func (s *mockStorage) Delete(key string) error {
	delete(s.data, key)
	return nil
}

func (s *mockStorage) Exists(key string) (bool, error) {
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
// StateManager 测试
// ============================================================================

func TestNewStateManager(t *testing.T) {
	ms := newMockStorage()

	tests := []struct {
		name      string
		secretKey string
	}{
		{"with_key", "my-secret-key"},
		{"empty_key", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateManager(ms, tt.secretKey)

			if sm == nil {
				t.Fatal("NewStateManager should not return nil")
			}

			if sm.storage == nil {
				t.Error("storage should be set")
			}

			if tt.secretKey == "" {
				if sm.secretKey == "" {
					t.Error("default secret key should be set")
				}
			} else {
				if sm.secretKey != tt.secretKey {
					t.Errorf("secret key should be %s, got %s", tt.secretKey, sm.secretKey)
				}
			}
		})
	}
}

func TestStateManager_SaveState(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	state := &TunnelState{
		TunnelID:        "tunnel-001",
		MappingID:       "mapping-001",
		ListenClientID:  100,
		TargetClientID:  200,
		LastSeqNum:      10,
		LastAckNum:      5,
		NextExpectedSeq: 11,
		BufferedPackets: []BufferedState{
			{SeqNum: 6, Data: []byte("data6"), SentAt: time.Now().Unix()},
			{SeqNum: 7, Data: []byte("data7"), SentAt: time.Now().Unix()},
		},
	}

	err := sm.SaveState(state)
	if err != nil {
		t.Errorf("SaveState should not return error: %v", err)
	}

	// 验证时间戳已设置
	if state.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	if state.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// 验证签名已计算
	if state.Signature == "" {
		t.Error("Signature should be computed")
	}

	// 验证数据已存储
	key := StateKeyPrefix + state.TunnelID
	_, exists := ms.data[key]
	if !exists {
		t.Error("state should be stored")
	}
}

func TestStateManager_SaveState_NilState(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	err := sm.SaveState(nil)
	if err == nil {
		t.Error("SaveState with nil state should return error")
	}
}

func TestStateManager_LoadState(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	// 先保存状态
	originalState := &TunnelState{
		TunnelID:        "tunnel-002",
		MappingID:       "mapping-002",
		ListenClientID:  101,
		TargetClientID:  201,
		LastSeqNum:      20,
		LastAckNum:      15,
		NextExpectedSeq: 21,
	}

	err := sm.SaveState(originalState)
	if err != nil {
		t.Fatalf("SaveState should not return error: %v", err)
	}

	// 加载状态
	loadedState, err := sm.LoadState("tunnel-002")
	if err != nil {
		t.Fatalf("LoadState should not return error: %v", err)
	}

	// 验证字段
	if loadedState.TunnelID != originalState.TunnelID {
		t.Errorf("TunnelID should be %s, got %s", originalState.TunnelID, loadedState.TunnelID)
	}

	if loadedState.MappingID != originalState.MappingID {
		t.Errorf("MappingID should be %s, got %s", originalState.MappingID, loadedState.MappingID)
	}

	if loadedState.ListenClientID != originalState.ListenClientID {
		t.Errorf("ListenClientID should be %d, got %d", originalState.ListenClientID, loadedState.ListenClientID)
	}

	if loadedState.TargetClientID != originalState.TargetClientID {
		t.Errorf("TargetClientID should be %d, got %d", originalState.TargetClientID, loadedState.TargetClientID)
	}

	if loadedState.LastSeqNum != originalState.LastSeqNum {
		t.Errorf("LastSeqNum should be %d, got %d", originalState.LastSeqNum, loadedState.LastSeqNum)
	}
}

func TestStateManager_LoadState_NotFound(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	_, err := sm.LoadState("non-existent")
	if err == nil {
		t.Error("LoadState for non-existent tunnel should return error")
	}
}

func TestStateManager_LoadState_InvalidSignature(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	// 手动存储一个带有错误签名的状态
	state := &TunnelState{
		TunnelID:        "tunnel-003",
		MappingID:       "mapping-003",
		ListenClientID:  102,
		TargetClientID:  202,
		LastSeqNum:      30,
		LastAckNum:      25,
		NextExpectedSeq: 31,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Signature:       "invalid-signature",
	}

	data, _ := json.Marshal(state)
	key := StateKeyPrefix + state.TunnelID
	ms.data[key] = string(data)

	// 加载应该因签名验证失败而失败
	_, err := sm.LoadState("tunnel-003")
	if err == nil {
		t.Error("LoadState with invalid signature should return error")
	}
}

func TestStateManager_DeleteState(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	// 先保存状态
	state := &TunnelState{
		TunnelID:  "tunnel-004",
		MappingID: "mapping-004",
	}
	sm.SaveState(state)

	// 删除状态
	err := sm.DeleteState("tunnel-004")
	if err != nil {
		t.Errorf("DeleteState should not return error: %v", err)
	}

	// 验证已删除
	_, err = sm.LoadState("tunnel-004")
	if err == nil {
		t.Error("LoadState after delete should return error")
	}
}

// ============================================================================
// 签名测试
// ============================================================================

func TestStateManager_ComputeSignature(t *testing.T) {
	ms := newMockStorage()
	sm := NewStateManager(ms, "test-secret")

	state := &TunnelState{
		TunnelID:        "tunnel-sig",
		MappingID:       "mapping-sig",
		ListenClientID:  100,
		TargetClientID:  200,
		LastSeqNum:      10,
		LastAckNum:      5,
		NextExpectedSeq: 11,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	sig1, err := sm.computeSignature(state)
	if err != nil {
		t.Errorf("computeSignature should not return error: %v", err)
	}

	if sig1 == "" {
		t.Error("signature should not be empty")
	}

	// 相同状态应该产生相同签名
	sig2, err := sm.computeSignature(state)
	if err != nil {
		t.Errorf("computeSignature should not return error: %v", err)
	}

	if sig1 != sig2 {
		t.Error("same state should produce same signature")
	}

	// 修改状态应该产生不同签名
	state.LastSeqNum = 11
	sig3, err := sm.computeSignature(state)
	if err != nil {
		t.Errorf("computeSignature should not return error: %v", err)
	}

	if sig1 == sig3 {
		t.Error("different state should produce different signature")
	}
}

func TestStateManager_DifferentSecretKeys(t *testing.T) {
	ms := newMockStorage()
	sm1 := NewStateManager(ms, "secret-1")
	sm2 := NewStateManager(ms, "secret-2")

	state := &TunnelState{
		TunnelID:        "tunnel-key",
		MappingID:       "mapping-key",
		ListenClientID:  100,
		TargetClientID:  200,
		LastSeqNum:      10,
		LastAckNum:      5,
		NextExpectedSeq: 11,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	sig1, _ := sm1.computeSignature(state)
	sig2, _ := sm2.computeSignature(state)

	if sig1 == sig2 {
		t.Error("different secret keys should produce different signatures")
	}
}

// ============================================================================
// 缓冲区状态转换测试
// ============================================================================

func TestCaptureSendBufferState(t *testing.T) {
	sb := NewSendBuffer()

	// 发送一些包
	for i := 1; i <= 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	// 捕获状态
	buffered := CaptureSendBufferState(sb)

	if len(buffered) != 3 {
		t.Errorf("should capture 3 packets, got %d", len(buffered))
	}

	// 验证序列号
	seqNums := make(map[uint64]bool)
	for _, bs := range buffered {
		seqNums[bs.SeqNum] = true
	}

	for i := uint64(1); i <= 3; i++ {
		if !seqNums[i] {
			t.Errorf("should capture seqNum %d", i)
		}
	}
}

func TestRestoreToSendBuffer(t *testing.T) {
	sb := NewSendBuffer()

	// 创建缓冲状态
	bufferedStates := []BufferedState{
		{SeqNum: 1, Data: []byte("data1"), SentAt: time.Now().Unix()},
		{SeqNum: 2, Data: []byte("data2"), SentAt: time.Now().Unix()},
		{SeqNum: 3, Data: []byte("data3"), SentAt: time.Now().Unix()},
	}

	// 恢复到缓冲区
	RestoreToSendBuffer(sb, bufferedStates)

	if sb.GetBufferedCount() != 3 {
		t.Errorf("should restore 3 packets, got %d", sb.GetBufferedCount())
	}

	// 验证数据
	sb.RLock()
	for _, bs := range bufferedStates {
		pkt, exists := sb.Buffer[bs.SeqNum]
		if !exists {
			t.Errorf("packet %d should be restored", bs.SeqNum)
			continue
		}
		if string(pkt.Data) != string(bs.Data) {
			t.Errorf("packet %d data should be %s, got %s", bs.SeqNum, string(bs.Data), string(pkt.Data))
		}
	}
	sb.RUnlock()

	// 验证缓冲区大小
	expectedSize := len("data1") + len("data2") + len("data3")
	if sb.GetBufferSize() != expectedSize {
		t.Errorf("buffer size should be %d, got %d", expectedSize, sb.GetBufferSize())
	}
}

func TestCaptureSendBufferState_Empty(t *testing.T) {
	sb := NewSendBuffer()

	buffered := CaptureSendBufferState(sb)

	if len(buffered) != 0 {
		t.Errorf("should capture 0 packets from empty buffer, got %d", len(buffered))
	}
}

func TestRestoreToSendBuffer_Empty(t *testing.T) {
	sb := NewSendBuffer()

	// 恢复空状态
	RestoreToSendBuffer(sb, []BufferedState{})

	if sb.GetBufferedCount() != 0 {
		t.Errorf("buffer should remain empty, got %d", sb.GetBufferedCount())
	}
}

// ============================================================================
// 常量测试
// ============================================================================

func TestConstants(t *testing.T) {
	if StateTTL != 5*time.Minute {
		t.Errorf("StateTTL should be 5 minutes, got %v", StateTTL)
	}

	if StateKeyPrefix != "tunnel:state:" {
		t.Errorf("StateKeyPrefix should be 'tunnel:state:', got %s", StateKeyPrefix)
	}
}

// ============================================================================
// 类型测试
// ============================================================================

func TestTunnelState_JSONSerialization(t *testing.T) {
	state := &TunnelState{
		TunnelID:        "tunnel-json",
		MappingID:       "mapping-json",
		ListenClientID:  100,
		TargetClientID:  200,
		LastSeqNum:      10,
		LastAckNum:      5,
		NextExpectedSeq: 11,
		BufferedPackets: []BufferedState{
			{SeqNum: 6, Data: []byte("data6"), SentAt: time.Now().Unix()},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Signature: "test-signature",
	}

	// 序列化
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded TunnelState
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	// 验证字段
	if loaded.TunnelID != state.TunnelID {
		t.Errorf("TunnelID should be %s, got %s", state.TunnelID, loaded.TunnelID)
	}

	if loaded.ListenClientID != state.ListenClientID {
		t.Errorf("ListenClientID should be %d, got %d", state.ListenClientID, loaded.ListenClientID)
	}

	if len(loaded.BufferedPackets) != len(state.BufferedPackets) {
		t.Errorf("BufferedPackets length should be %d, got %d",
			len(state.BufferedPackets), len(loaded.BufferedPackets))
	}
}

func TestBufferedState_JSONSerialization(t *testing.T) {
	bs := BufferedState{
		SeqNum: 42,
		Data:   []byte("test data"),
		SentAt: time.Now().Unix(),
	}

	// 序列化
	data, err := json.Marshal(bs)
	if err != nil {
		t.Fatalf("JSON marshal should not return error: %v", err)
	}

	// 反序列化
	var loaded BufferedState
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("JSON unmarshal should not return error: %v", err)
	}

	if loaded.SeqNum != bs.SeqNum {
		t.Errorf("SeqNum should be %d, got %d", bs.SeqNum, loaded.SeqNum)
	}

	if string(loaded.Data) != string(bs.Data) {
		t.Errorf("Data should be %s, got %s", string(bs.Data), string(loaded.Data))
	}

	if loaded.SentAt != bs.SentAt {
		t.Errorf("SentAt should be %d, got %d", bs.SentAt, loaded.SentAt)
	}
}
