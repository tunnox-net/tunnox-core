// Package session 提供会话管理功能
// 本文件为 crossnode 子包提供向后兼容的类型别名和函数包装
package session

import (
	"context"
	"io"
	"net"

	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/protocol/session/crossnode"
)

// ============================================================================
// crossnode 子包类型别名（向后兼容）
// ============================================================================

// CrossNodeConn 跨节点连接（类型别名）
type CrossNodeConn = crossnode.Conn

// CrossNodePool 跨节点连接池（类型别名）
type CrossNodePool = crossnode.Pool

// CrossNodePoolConfig 跨节点连接池配置（类型别名）
type CrossNodePoolConfig = crossnode.PoolConfig

// NodeConnectionPool 单节点连接池（类型别名）
type NodeConnectionPool = crossnode.NodeConnectionPool

// FrameStream 基于帧协议的数据流（类型别名）
type FrameStream = crossnode.FrameStream

// FrameHeader 帧头结构（类型别名）
type FrameHeader = crossnode.FrameHeader

// TunnelStateTracker 隧道状态跟踪器接口（类型别名）
type TunnelStateTracker = crossnode.TunnelStateTracker

// TargetReadyMessage Target就绪消息（类型别名）
type TargetReadyMessage = crossnode.TargetReadyMessage

// HTTPProxyMessage HTTP代理消息（类型别名）
type HTTPProxyMessage = crossnode.HTTPProxyMessage

// HTTPProxyResponseMessage HTTP代理响应消息（类型别名）
type HTTPProxyResponseMessage = crossnode.HTTPProxyResponseMessage

// CommandMessage 通用跨节点命令消息（类型别名）
type CommandMessage = crossnode.CommandMessage

// CommandResponseMessage 通用跨节点命令响应消息（类型别名）
type CommandResponseMessage = crossnode.CommandResponseMessage

// ============================================================================
// crossnode 常量重新导出
// ============================================================================

const (
	// 帧类型常量
	FrameTypeData            = crossnode.FrameTypeData
	FrameTypeTargetReady     = crossnode.FrameTypeTargetReady
	FrameTypeClose           = crossnode.FrameTypeClose
	FrameTypeAck             = crossnode.FrameTypeAck
	FrameTypeHTTPProxy       = crossnode.FrameTypeHTTPProxy
	FrameTypeHTTPResponse    = crossnode.FrameTypeHTTPResponse
	FrameTypeDNSQuery        = crossnode.FrameTypeDNSQuery
	FrameTypeDNSResponse     = crossnode.FrameTypeDNSResponse
	FrameTypeCommand         = crossnode.FrameTypeCommand
	FrameTypeCommandResponse = crossnode.FrameTypeCommandResponse

	// 帧大小常量
	FrameHeaderSize = crossnode.FrameHeaderSize
	MaxFrameSize    = crossnode.MaxFrameSize
)

// ============================================================================
// crossnode 函数包装（向后兼容）
// ============================================================================

// NewCrossNodeConn 创建跨节点连接
func NewCrossNodeConn(
	parentCtx context.Context,
	nodeID string,
	tcpConn *net.TCPConn,
	pool *NodeConnectionPool,
) *CrossNodeConn {
	return crossnode.NewConn(parentCtx, nodeID, tcpConn, pool)
}

// NewCrossNodePool 创建跨节点连接池
func NewCrossNodePool(
	parentCtx context.Context,
	storage storage.Storage,
	nodeID string,
	config CrossNodePoolConfig,
) *CrossNodePool {
	return crossnode.NewPool(parentCtx, storage, nodeID, config)
}

// DefaultCrossNodePoolConfig 返回默认连接池配置
func DefaultCrossNodePoolConfig() CrossNodePoolConfig {
	return crossnode.DefaultPoolConfig()
}

// NewNodeConnectionPool 创建单节点连接池
func NewNodeConnectionPool(
	parentCtx context.Context,
	nodeID string,
	nodeAddr string,
	config CrossNodePoolConfig,
	totalCreated *int64,
) *NodeConnectionPool {
	return crossnode.NewNodeConnectionPool(parentCtx, nodeID, nodeAddr, config, totalCreated)
}

// NewFrameStream 创建帧数据流
func NewFrameStream(conn *CrossNodeConn, tunnelID [16]byte) *FrameStream {
	return crossnode.NewFrameStream(conn, tunnelID)
}

// NewFrameStreamWithTracker 创建带状态跟踪的帧数据流
func NewFrameStreamWithTracker(conn *CrossNodeConn, tunnelID [16]byte, tracker TunnelStateTracker) *FrameStream {
	return crossnode.NewFrameStreamWithTracker(conn, tunnelID, tracker)
}

// WriteFrame 写入帧
func WriteFrame(conn *net.TCPConn, tunnelID [16]byte, frameType byte, data []byte) error {
	return crossnode.WriteFrame(conn, tunnelID, frameType, data)
}

// WriteFrameToWriter 写入帧到通用 Writer
func WriteFrameToWriter(w io.Writer, tunnelID [16]byte, frameType byte, data []byte) error {
	return crossnode.WriteFrameToWriter(w, tunnelID, frameType, data)
}

// ReadFrame 读取帧
func ReadFrame(conn *net.TCPConn) (tunnelID [16]byte, frameType byte, data []byte, err error) {
	return crossnode.ReadFrame(conn)
}

// ReadFrameFromReader 从通用 Reader 读取帧
func ReadFrameFromReader(r io.Reader) (tunnelID [16]byte, frameType byte, data []byte, err error) {
	return crossnode.ReadFrameFromReader(r)
}

// TunnelIDFromString 从字符串解析 TunnelID
func TunnelIDFromString(s string) ([16]byte, error) {
	return crossnode.TunnelIDFromString(s)
}

// TunnelIDToString 将 TunnelID 转换为字符串
func TunnelIDToString(id [16]byte) string {
	return crossnode.TunnelIDToString(id)
}

// EncodeTargetReadyMessage 编码 Target 就绪消息
func EncodeTargetReadyMessage(tunnelID, targetNodeID string) []byte {
	return crossnode.EncodeTargetReadyMessage(tunnelID, targetNodeID)
}

// DecodeTargetReadyMessage 解码 Target 就绪消息
func DecodeTargetReadyMessage(data []byte) (tunnelID, targetNodeID string, err error) {
	return crossnode.DecodeTargetReadyMessage(data)
}
