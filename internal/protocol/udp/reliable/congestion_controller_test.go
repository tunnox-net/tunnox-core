package reliable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCongestionController_SlowStart(t *testing.T) {
	cc := NewCongestionController()

	// 初始状态
	assert.Equal(t, InitialCwnd, cc.GetCwnd())
	assert.Equal(t, SlowStart, cc.GetState())

	// 收到 ACK，cwnd 指数增长
	for i := 0; i < 10; i++ {
		cc.OnAck(uint32(i), 1400)
	}

	assert.Greater(t, cc.GetCwnd(), InitialCwnd)
}

func TestCongestionController_CongestionAvoid(t *testing.T) {
	cc := NewCongestionController()

	// 进入拥塞避免阶段
	cc.cwnd = cc.ssthresh
	cc.state = CongestionAvoid

	initialCwnd := cc.GetCwnd()

	// 收到多个 ACK
	for i := 0; i < 100; i++ {
		cc.OnAck(uint32(i), 1400)
	}

	// cwnd 应该线性增长
	assert.Greater(t, cc.GetCwnd(), initialCwnd)
	assert.Equal(t, CongestionAvoid, cc.GetState())
}

func TestCongestionController_FastRetransmit(t *testing.T) {
	cc := NewCongestionController()

	initialCwnd := cc.GetCwnd()

	// 先收到一个正常ACK，设置lastAck
	cc.OnAck(100, 1400)
	
	// 然后收到 DupAckThreshold 个重复 ACK
	for i := 0; i < DupAckThreshold; i++ {
		cc.OnAck(100, 1400)
	}

	// 应该进入快速恢复
	assert.Equal(t, FastRecovery, cc.GetState())
	assert.Less(t, cc.GetCwnd(), initialCwnd)
}

func TestCongestionController_Timeout(t *testing.T) {
	cc := NewCongestionController()

	// 增长 cwnd
	for i := 0; i < 20; i++ {
		cc.OnAck(uint32(i), 1400)
	}

	cwndBeforeTimeout := cc.GetCwnd()

	// 超时
	cc.OnTimeout()

	// 应该重置到慢启动
	assert.Equal(t, SlowStart, cc.GetState())
	assert.Equal(t, InitialCwnd, cc.GetCwnd())
	assert.Less(t, cc.GetCwnd(), cwndBeforeTimeout)
}

func TestCongestionController_FastRecovery(t *testing.T) {
	cc := NewCongestionController()

	// 进入快速恢复
	cc.state = FastRecovery
	cc.cwnd = 20
	cc.ssthresh = 10

	// 收到新 ACK
	cc.OnAck(200, 1400)

	// 应该退出快速恢复，进入拥塞避免
	assert.Equal(t, CongestionAvoid, cc.GetState())
	assert.Equal(t, cc.ssthresh, cc.GetCwnd())
}

func TestCongestionController_Stats(t *testing.T) {
	cc := NewCongestionController()

	cwnd, ssthresh, state := cc.GetStats()
	assert.Equal(t, InitialCwnd, cwnd)
	assert.Equal(t, MaxCwnd/2, ssthresh)
	assert.Equal(t, SlowStart, state)
}
