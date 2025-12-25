package utils

import (
	"io"
)

// CopyStrategy 拷贝策略接口
// 定义不同协议的数据拷贝策略
type CopyStrategy interface {
	// Copy 执行双向数据拷贝
	// connA: 本地连接（如 MySQL 客户端连接）
	// connB: 隧道连接（如 WebSocket 连接）
	// options: 拷贝选项
	// 返回拷贝结果
	Copy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult
}

// DefaultCopyStrategy 默认拷贝策略（适用于 TCP、WebSocket、QUIC 等）
// 使用标准的 io.Copy 进行数据拷贝
type DefaultCopyStrategy struct{}

// NewDefaultCopyStrategy 创建默认拷贝策略
func NewDefaultCopyStrategy() CopyStrategy {
	return &DefaultCopyStrategy{}
}

// Copy 执行默认双向数据拷贝
func (s *DefaultCopyStrategy) Copy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	return BidirectionalCopy(connA, connB, options)
}

// UDPCopyStrategy UDP 拷贝策略（保持包边界）
// 使用长度前缀协议来保持 UDP 包边界
type UDPCopyStrategy struct{}

// NewUDPCopyStrategy 创建 UDP 拷贝策略
func NewUDPCopyStrategy() CopyStrategy {
	return &UDPCopyStrategy{}
}

// Copy 执行 UDP 双向数据拷贝（保持包边界）
func (s *UDPCopyStrategy) Copy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	return UDPBidirectionalCopy(connA, connB, options)
}

// CopyStrategyFactory 拷贝策略工厂
type CopyStrategyFactory struct{}

// NewCopyStrategyFactory 创建拷贝策略工厂
func NewCopyStrategyFactory() *CopyStrategyFactory {
	return &CopyStrategyFactory{}
}

// CreateStrategy 根据协议创建对应的拷贝策略
// protocol: 协议名称（如 "tcp", "websocket", "udp" 等）
func (f *CopyStrategyFactory) CreateStrategy(protocol string) CopyStrategy {
	if protocol == "udp" {
		return NewUDPCopyStrategy()
	}
	return NewDefaultCopyStrategy()
}
