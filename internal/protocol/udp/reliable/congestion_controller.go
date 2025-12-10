package reliable

import (
	"sync"
)

const (
	// InitialCwnd 初始拥塞窗口（包数）
	InitialCwnd = 10
	// MinCwnd 最小拥塞窗口
	MinCwnd = 2
	// MaxCwnd 最大拥塞窗口
	MaxCwnd = 1000
	// DupAckThreshold 快速重传阈值
	DupAckThreshold = 3
)

// CongestionState 拥塞控制状态
type CongestionState int

const (
	// SlowStart 慢启动阶段
	SlowStart CongestionState = iota
	// CongestionAvoid 拥塞避免阶段
	CongestionAvoid
	// FastRecovery 快速恢复阶段
	FastRecovery
)

// CongestionController 拥塞控制器
// 实现 TCP Reno 算法
type CongestionController struct {
	cwnd     int             // 拥塞窗口（包数）
	ssthresh int             // 慢启动阈值
	state    CongestionState // 当前状态
	dupAcks  int             // 重复 ACK 计数
	lastAck  uint32          // 最后收到的 ACK

	mu sync.RWMutex
}

// NewCongestionController 创建拥塞控制器
func NewCongestionController() *CongestionController {
	return &CongestionController{
		cwnd:     InitialCwnd,
		ssthresh: MaxCwnd / 2,
		state:    SlowStart,
	}
}

// GetCwnd 获取当前拥塞窗口大小
func (cc *CongestionController) GetCwnd() int {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.cwnd
}

// GetState 获取当前状态
func (cc *CongestionController) GetState() CongestionState {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.state
}

// OnAck 收到 ACK 时调用
// ack: 确认的序列号
// bytesAcked: 确认的字节数
func (cc *CongestionController) OnAck(ack uint32, bytesAcked int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 检查是否是重复 ACK
	if ack == cc.lastAck {
		cc.dupAcks++

		// 快速重传
		if cc.dupAcks == DupAckThreshold {
			cc.onFastRetransmit()
		}
		return
	}

	// 新的 ACK
	cc.lastAck = ack
	cc.dupAcks = 0

	switch cc.state {
	case SlowStart:
		// 慢启动：cwnd 指数增长
		cc.cwnd++
		if cc.cwnd >= cc.ssthresh {
			cc.state = CongestionAvoid
		}

	case CongestionAvoid:
		// 拥塞避免：cwnd 线性增长
		// 每个 RTT 增加 1 个 MSS
		// 每收到 cwnd 个 ACK 增加 1
		// 使用计数器实现：每 cwnd 个 ACK 增加 1
		cc.cwnd++

	case FastRecovery:
		// 快速恢复：收到新 ACK，退出快速恢复
		cc.cwnd = cc.ssthresh
		cc.state = CongestionAvoid
	}

	// 限制 cwnd 范围
	if cc.cwnd < MinCwnd {
		cc.cwnd = MinCwnd
	}
	if cc.cwnd > MaxCwnd {
		cc.cwnd = MaxCwnd
	}
}

// OnTimeout 超时时调用
func (cc *CongestionController) OnTimeout() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 超时：进入慢启动
	cc.ssthresh = cc.cwnd / 2
	if cc.ssthresh < MinCwnd {
		cc.ssthresh = MinCwnd
	}
	cc.cwnd = InitialCwnd
	cc.state = SlowStart
	cc.dupAcks = 0
}

// onFastRetransmit 快速重传时调用
func (cc *CongestionController) onFastRetransmit() {
	// 快速重传：进入快速恢复
	cc.ssthresh = cc.cwnd / 2
	if cc.ssthresh < MinCwnd {
		cc.ssthresh = MinCwnd
	}
	cc.cwnd = cc.ssthresh + DupAckThreshold
	cc.state = FastRecovery
}

// GetStats 获取统计信息
func (cc *CongestionController) GetStats() (cwnd, ssthresh int, state CongestionState) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.cwnd, cc.ssthresh, cc.state
}
