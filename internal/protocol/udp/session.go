package udp

import (
	"sync"
	"time"
)

// SessionKey 标识一个 UDP 逻辑会话。
type SessionKey struct {
	SessionID uint32
	StreamID  uint32 // 目前可固定为 0，为未来多 stream 留扩展点
}

// SendPacketState 记录某个 PacketSeq 的发送状态。
type SendPacketState struct {
	Seq       uint32
	Payload   []byte   // 完整逻辑包的 payload（由 Sender 管理生命周期）
	LastSend  time.Time
	Retries   int
	FragCount int
}

// SessionState 单个 UDP 会话状态（窗口、重传统计）
type SessionState struct {
	Key SessionKey

	// 发送侧窗口状态
	sendMutex   sync.Mutex
	sendBase    uint32 // 最早未确认的 PacketSeq
	nextSeq     uint32 // 下一个将要发送的 PacketSeq
	sendWindow  uint16 // 当前窗口大小（包数）
	maxWindow   uint16 // 最大窗口大小
	rto         time.Duration // 当前重传超时
	inFlight    map[uint32]*SendPacketState

	// 接收侧状态
	recvMutex   sync.Mutex
	recvBase    uint32 // 最后一个按序递交给上层的 PacketSeq
	fragments   map[FragmentGroupKey]*FragmentGroup

	// 限制 & 清理
	lastActive  time.Time
	lastActiveMu sync.Mutex
}

// NewSessionState 创建新的会话状态
func NewSessionState(key SessionKey) *SessionState {
	return &SessionState{
		Key:         key,
		sendBase:    0,
		nextSeq:     1,
		sendWindow:  DefaultSendWindowSize,
		maxWindow:   DefaultSendWindowSize,
		rto:         DefaultRetransmitTimeout,
		inFlight:    make(map[uint32]*SendPacketState),
		recvBase:    0,
		fragments:   make(map[FragmentGroupKey]*FragmentGroup),
		lastActive:  time.Now(),
	}
}

// UpdateLastActive 更新最后活跃时间
func (s *SessionState) UpdateLastActive() {
	s.lastActiveMu.Lock()
	defer s.lastActiveMu.Unlock()
	s.lastActive = time.Now()
}

// GetLastActive 获取最后活跃时间
func (s *SessionState) GetLastActive() time.Time {
	s.lastActiveMu.Lock()
	defer s.lastActiveMu.Unlock()
	return s.lastActive
}

// GetInFlightCount 获取正在传输中的包数量
func (s *SessionState) GetInFlightCount() int {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()
	return len(s.inFlight)
}

// GetFragmentGroupCount 获取分片组数量
func (s *SessionState) GetFragmentGroupCount() int {
	s.recvMutex.Lock()
	defer s.recvMutex.Unlock()
	return len(s.fragments)
}

