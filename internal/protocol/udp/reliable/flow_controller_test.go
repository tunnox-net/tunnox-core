package reliable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlowController_WindowManagement(t *testing.T) {
	fc := NewFlowController()

	// 初始窗口
	assert.Equal(t, uint32(DefaultFlowWindowSize), fc.GetReceiveWindow())
	assert.Equal(t, uint32(DefaultFlowWindowSize), fc.GetSendWindow())

	// 接收数据
	fc.OnDataReceived(1024)
	assert.Equal(t, uint32(DefaultFlowWindowSize-1024), fc.GetReceiveWindow())

	// 消费数据
	fc.OnDataConsumed(1024)
	assert.Equal(t, uint32(DefaultFlowWindowSize), fc.GetReceiveWindow())
}

func TestFlowController_SendWindow(t *testing.T) {
	fc := NewFlowController()

	// 更新发送窗口
	fc.UpdateSendWindow(10000)
	assert.Equal(t, uint32(10000), fc.GetSendWindow())

	// 检查是否可以发送
	assert.True(t, fc.CanSend(5000))
	assert.True(t, fc.CanSend(10000))
	assert.False(t, fc.CanSend(10001))

	// 发送数据
	fc.OnDataSent(5000)
	assert.Equal(t, uint32(5000), fc.GetSendWindow())
}

func TestFlowController_WindowFull(t *testing.T) {
	fc := NewFlowController()

	// 填满接收缓冲区
	fc.OnDataReceived(DefaultFlowWindowSize)
	assert.Equal(t, uint32(0), fc.GetReceiveWindow())

	// 消费一部分数据
	fc.OnDataConsumed(MinFlowWindowSize)
	assert.Equal(t, uint32(MinFlowWindowSize), fc.GetReceiveWindow())
}

func TestFlowController_Stats(t *testing.T) {
	fc := NewFlowController()

	fc.OnDataReceived(1024)
	fc.UpdateSendWindow(10000)

	recvUsed, recvMax, sendWindow := fc.GetStats()
	assert.Equal(t, uint32(1024), recvUsed)
	assert.Equal(t, uint32(DefaultFlowWindowSize), recvMax)
	assert.Equal(t, uint32(10000), sendWindow)
}
