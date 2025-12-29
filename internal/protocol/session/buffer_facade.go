// Package session 提供会话管理功能
// 本文件为 buffer 子包提供向后兼容的类型别名和函数包装
package session

import (
	"time"

	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session/buffer"
)

// ============================================================================
// buffer 子包类型别名（向后兼容）
// ============================================================================

// TunnelSendBuffer 隧道发送缓冲区（类型别名）
type TunnelSendBuffer = buffer.SendBuffer

// TunnelReceiveBuffer 隧道接收缓冲区（类型别名）
type TunnelReceiveBuffer = buffer.ReceiveBuffer

// BufferedPacket 缓冲的数据包（类型别名）
type BufferedPacket = buffer.BufferedPacket

// TunnelState 隧道状态（类型别名）
type TunnelState = buffer.TunnelState

// BufferedState 缓冲包状态（类型别名）
type BufferedState = buffer.BufferedState

// TunnelStateManager 隧道状态管理器（类型别名）
type TunnelStateManager = buffer.StateManager

// ============================================================================
// buffer 常量重新导出
// ============================================================================

const (
	// 发送缓冲区常量
	DefaultMaxBufferSize      = buffer.DefaultMaxBufferSize
	DefaultMaxBufferedPackets = buffer.DefaultMaxBufferedPackets
	DefaultResendTimeout      = buffer.DefaultResendTimeout

	// 接收缓冲区常量
	DefaultMaxOutOfOrder = buffer.DefaultMaxOutOfOrder

	// 状态管理常量
	TunnelStateTTL       = buffer.StateTTL
	TunnelStateKeyPrefix = buffer.StateKeyPrefix
)

// ============================================================================
// buffer 函数包装（向后兼容）
// ============================================================================

// NewTunnelSendBuffer 创建发送缓冲区
func NewTunnelSendBuffer() *TunnelSendBuffer {
	return buffer.NewSendBuffer()
}

// NewTunnelSendBufferWithConfig 使用自定义配置创建发送缓冲区
func NewTunnelSendBufferWithConfig(maxBufferSize, maxPackets int, resendTimeout time.Duration) *TunnelSendBuffer {
	return buffer.NewSendBufferWithConfig(maxBufferSize, maxPackets, resendTimeout)
}

// NewTunnelReceiveBuffer 创建接收缓冲区
func NewTunnelReceiveBuffer() *TunnelReceiveBuffer {
	return buffer.NewReceiveBuffer()
}

// NewTunnelReceiveBufferWithConfig 使用自定义配置创建接收缓冲区
func NewTunnelReceiveBufferWithConfig(maxOutOfOrder int) *TunnelReceiveBuffer {
	return buffer.NewReceiveBufferWithConfig(maxOutOfOrder)
}

// NewTunnelStateManager 创建隧道状态管理器
func NewTunnelStateManager(storage storage.Storage, secretKey string) *TunnelStateManager {
	return buffer.NewStateManager(storage, secretKey)
}

// CaptureSendBufferState 捕获发送缓冲区状态
func CaptureSendBufferState(sendBuffer *TunnelSendBuffer) []BufferedState {
	return buffer.CaptureSendBufferState(sendBuffer)
}

// RestoreToSendBuffer 恢复到发送缓冲区
func RestoreToSendBuffer(sendBuffer *TunnelSendBuffer, bufferedStates []BufferedState) {
	buffer.RestoreToSendBuffer(sendBuffer, bufferedStates)
}

// ============================================================================
// 接口适配（保持与旧代码兼容）
// ============================================================================

// SendBufferSend 发送缓冲区发送方法包装
func SendBufferSend(b *TunnelSendBuffer, data []byte, pkt *packet.TransferPacket) (uint64, error) {
	return b.Send(data, pkt)
}

// ReceiveBufferReceive 接收缓冲区接收方法包装
func ReceiveBufferReceive(b *TunnelReceiveBuffer, pkt *packet.TransferPacket) ([][]byte, error) {
	return b.Receive(pkt)
}
