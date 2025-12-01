package utils

import (
	"io"
)

// CopyStrategy 拷贝策略接口
// 定义不同协议的数据拷贝策略
type CopyStrategy interface {
	// Copy 执行双向数据拷贝
	// connA: 本地连接（如 MySQL 客户端连接）
	// connB: 隧道连接（如 HTTP-poll 连接）
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

// HTTPPollCopyStrategy HTTP 长轮询拷贝策略
// 针对 HTTP-poll 的特殊处理（如果需要）
type HTTPPollCopyStrategy struct{}

// NewHTTPPollCopyStrategy 创建 HTTP-poll 拷贝策略
func NewHTTPPollCopyStrategy() CopyStrategy {
	return &HTTPPollCopyStrategy{}
}

// Copy 执行 HTTP-poll 双向数据拷贝
// 当前实现与默认策略相同，但保留扩展点
func (s *HTTPPollCopyStrategy) Copy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	// HTTP-poll 使用标准的 BidirectionalCopy
	// 如果需要特殊处理（如缓冲、批处理等），可以在此实现
	return BidirectionalCopy(connA, connB, options)
}

// CopyStrategyFactory 拷贝策略工厂
type CopyStrategyFactory struct{}

// NewCopyStrategyFactory 创建拷贝策略工厂
func NewCopyStrategyFactory() *CopyStrategyFactory {
	return &CopyStrategyFactory{}
}

// CreateStrategy 根据协议创建对应的拷贝策略
// protocol: 协议名称（如 "tcp", "httppoll", "websocket" 等）
func (f *CopyStrategyFactory) CreateStrategy(protocol string) CopyStrategy {
	switch protocol {
	case "httppoll", "http-long-polling", "httplp":
		return NewHTTPPollCopyStrategy()
	default:
		return NewDefaultCopyStrategy()
	}
}
