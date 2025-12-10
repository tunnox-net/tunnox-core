package reliable

import (
	"sync"
)

const (
	// DefaultFlowWindowSize 默认流量控制窗口大小
	// 注意：协议头使用 uint16，最大 65535 字节
	DefaultFlowWindowSize = 65535
	// MinFlowWindowSize 最小流量控制窗口大小（16KB）
	MinFlowWindowSize = 16 * 1024
	// MaxFlowWindowSize 最大流量控制窗口大小
	// 受限于协议头的 uint16 WindowSize 字段
	MaxFlowWindowSize = 65535
)

// FlowController 流量控制器
// 实现滑动窗口流量控制，防止接收端缓冲区溢出
type FlowController struct {
	// 接收窗口
	recvWindow     uint32 // 当前接收窗口大小
	recvBufUsed    uint32 // 已使用的接收缓冲区大小
	recvBufMaxSize uint32 // 接收缓冲区最大大小

	// 发送窗口
	sendWindow uint32 // 对端的接收窗口大小

	mu sync.RWMutex
}

// NewFlowController 创建流量控制器
func NewFlowController() *FlowController {
	return &FlowController{
		recvWindow:     DefaultFlowWindowSize,
		recvBufMaxSize: DefaultFlowWindowSize,
		sendWindow:     DefaultFlowWindowSize,
	}
}

// GetReceiveWindow 获取当前接收窗口大小
// 返回可用的接收窗口大小，用于在 ACK 包中通知对端
func (fc *FlowController) GetReceiveWindow() uint32 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	available := fc.recvBufMaxSize - fc.recvBufUsed
	if available < MinFlowWindowSize {
		return 0 // 窗口已满
	}
	return available
}

// UpdateSendWindow 更新对端的接收窗口大小
// 从对端的 ACK 包中获取窗口大小
func (fc *FlowController) UpdateSendWindow(window uint32) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.sendWindow = window
}

// GetSendWindow 获取发送窗口大小
func (fc *FlowController) GetSendWindow() uint32 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return fc.sendWindow
}

// CanSend 检查是否可以发送数据
// 根据对端的接收窗口判断
func (fc *FlowController) CanSend(dataSize uint32) bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return dataSize <= fc.sendWindow
}

// OnDataReceived 数据接收时调用
// 增加已使用的接收缓冲区大小
func (fc *FlowController) OnDataReceived(dataSize uint32) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.recvBufUsed += dataSize
	if fc.recvBufUsed > fc.recvBufMaxSize {
		fc.recvBufUsed = fc.recvBufMaxSize
	}
}

// OnDataConsumed 数据被应用层消费时调用
// 减少已使用的接收缓冲区大小
func (fc *FlowController) OnDataConsumed(dataSize uint32) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.recvBufUsed >= dataSize {
		fc.recvBufUsed -= dataSize
	} else {
		fc.recvBufUsed = 0
	}
}

// OnDataSent 数据发送时调用
// 减少发送窗口
func (fc *FlowController) OnDataSent(dataSize uint32) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.sendWindow >= dataSize {
		fc.sendWindow -= dataSize
	} else {
		fc.sendWindow = 0
	}
}

// GetStats 获取统计信息
func (fc *FlowController) GetStats() (recvUsed, recvMax, sendWindow uint32) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return fc.recvBufUsed, fc.recvBufMaxSize, fc.sendWindow
}
