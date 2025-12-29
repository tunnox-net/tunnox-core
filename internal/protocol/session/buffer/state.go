// Package buffer 提供隧道数据缓冲功能
package buffer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 隧道状态持久化
// ============================================================================

const (
	// StateTTL 隧道状态在Redis中的TTL（5分钟）
	StateTTL = 5 * time.Minute

	// StateKeyPrefix 隧道状态存储的key前缀
	StateKeyPrefix = "tunnel:state:"
)

// TunnelState 隧道状态
//
// 用于在服务器切换时恢复隧道，包含序列号、缓冲数据等关键信息。
type TunnelState struct {
	TunnelID        string          `json:"tunnel_id"`
	MappingID       string          `json:"mapping_id"`
	ListenClientID  int64           `json:"listen_client_id"`
	TargetClientID  int64           `json:"target_client_id"`
	LastSeqNum      uint64          `json:"last_seq_num"`      // 发送端最后序列号
	LastAckNum      uint64          `json:"last_ack_num"`      // 接收端最后确认号
	NextExpectedSeq uint64          `json:"next_expected_seq"` // 接收端期望序列号
	BufferedPackets []BufferedState `json:"buffered_packets"`  // 缓冲的包
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Signature       string          `json:"signature"` // HMAC签名（防篡改）
}

// BufferedState 缓冲包状态（用于序列化）
type BufferedState struct {
	SeqNum uint64 `json:"seq_num"`
	Data   []byte `json:"data"`
	SentAt int64  `json:"sent_at"` // Unix timestamp
}

// StateManager 隧道状态管理器
type StateManager struct {
	storage   storage.Storage
	secretKey string // 签名密钥
}

// NewStateManager 创建隧道状态管理器
func NewStateManager(storage storage.Storage, secretKey string) *StateManager {
	if secretKey == "" {
		secretKey = "tunnox-tunnel-state-secret-change-me"
	}

	return &StateManager{
		storage:   storage,
		secretKey: secretKey,
	}
}

// ============================================================================
// 状态存储
// ============================================================================

// SaveState 保存隧道状态
func (m *StateManager) SaveState(state *TunnelState) error {
	if state == nil {
		return errors.New("state is nil")
	}

	// 更新时间戳
	state.UpdatedAt = time.Now()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}

	// 计算签名
	signature, err := m.computeSignature(state)
	if err != nil {
		return fmt.Errorf("failed to compute signature: %w", err)
	}
	state.Signature = signature

	// 序列化
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// 存储到Redis
	key := StateKeyPrefix + state.TunnelID
	if err := m.storage.Set(key, string(data), StateTTL); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// LoadState 加载隧道状态
func (m *StateManager) LoadState(tunnelID string) (*TunnelState, error) {
	key := StateKeyPrefix + tunnelID

	// 从Redis读取
	data, err := m.storage.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	// 类型断言
	dataStr, ok := data.(string)
	if !ok {
		return nil, errors.New("invalid state data type")
	}

	// 反序列化
	var state TunnelState
	if err := json.Unmarshal([]byte(dataStr), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// 验证签名
	expectedSignature, err := m.computeSignature(&state)
	if err != nil {
		return nil, fmt.Errorf("failed to compute signature: %w", err)
	}
	if state.Signature != expectedSignature {
		return nil, errors.New("state signature mismatch (possible tampering)")
	}

	return &state, nil
}

// DeleteState 删除隧道状态
func (m *StateManager) DeleteState(tunnelID string) error {
	key := StateKeyPrefix + tunnelID
	return m.storage.Delete(key)
}

// ============================================================================
// 签名验证
// ============================================================================

// computeSignature 计算状态签名
func (m *StateManager) computeSignature(state *TunnelState) (string, error) {
	// 构造签名数据（不包含Signature字段）
	data := fmt.Sprintf("%s|%s|%d|%d|%d|%d|%d|%d|%d",
		state.TunnelID,
		state.MappingID,
		state.ListenClientID,
		state.TargetClientID,
		state.LastSeqNum,
		state.LastAckNum,
		state.NextExpectedSeq,
		state.CreatedAt.Unix(),
		state.UpdatedAt.Unix(),
	)

	// 使用HMAC-SHA256签名
	h := hmac.New(sha256.New, []byte(m.secretKey))
	h.Write([]byte(data))
	signature := hex.EncodeToString(h.Sum(nil))

	return signature, nil
}

// ============================================================================
// 缓冲区状态转换
// ============================================================================

// CaptureSendBufferState 捕获发送缓冲区状态
func CaptureSendBufferState(sendBuffer *SendBuffer) []BufferedState {
	sendBuffer.RLock()
	defer sendBuffer.RUnlock()

	buffered := make([]BufferedState, 0, len(sendBuffer.Buffer))
	for _, pkt := range sendBuffer.Buffer {
		buffered = append(buffered, BufferedState{
			SeqNum: pkt.SeqNum,
			Data:   pkt.Data,
			SentAt: pkt.SentAt.Unix(),
		})
	}

	return buffered
}

// RestoreToSendBuffer 恢复到发送缓冲区
func RestoreToSendBuffer(sendBuffer *SendBuffer, bufferedStates []BufferedState) {
	sendBuffer.Lock()
	defer sendBuffer.Unlock()

	for _, state := range bufferedStates {
		sendBuffer.Buffer[state.SeqNum] = &BufferedPacket{
			SeqNum: state.SeqNum,
			Data:   state.Data,
			SentAt: time.Unix(state.SentAt, 0),
			Packet: &packet.TransferPacket{
				SeqNum:  state.SeqNum,
				Payload: state.Data,
			},
		}
		sendBuffer.CurrentBufferSize += len(state.Data)
	}
}
