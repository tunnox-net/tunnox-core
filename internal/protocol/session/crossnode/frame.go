// Package crossnode 提供跨节点通信功能
package crossnode

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
)

// 帧类型常量
const (
	FrameTypeData            byte = 0x01 // 数据帧
	FrameTypeTargetReady     byte = 0x02 // Target 就绪通知
	FrameTypeClose           byte = 0x03 // 关闭通知（双向关闭）
	FrameTypeAck             byte = 0x04 // 确认帧
	FrameTypeHTTPProxy       byte = 0x05 // HTTP 代理请求
	FrameTypeHTTPResponse    byte = 0x06 // HTTP 代理响应
	FrameTypeDNSQuery        byte = 0x07 // DNS 查询请求 (deprecated, use FrameTypeCommand)
	FrameTypeDNSResponse     byte = 0x08 // DNS 查询响应 (deprecated, use FrameTypeCommandResponse)
	FrameTypeEOF             byte = 0x09 // 半关闭通知（单方向结束，用于支持 HTTP 请求-响应模式）
	FrameTypeCommand         byte = 0x10 // 通用命令请求（统一跨节点命令转发）
	FrameTypeCommandResponse byte = 0x11 // 通用命令响应
)

// 帧头大小常量
const (
	FrameHeaderSize = 21        // TunnelID(16) + FrameType(1) + Length(4)
	MaxFrameSize    = 64 * 1024 // 最大帧大小 64KB
)

// FrameHeader 帧头结构
type FrameHeader struct {
	TunnelID  [16]byte // UUID
	FrameType byte     // 帧类型
	Length    uint32   // 数据长度
}

// WriteFrame 写入帧（使用 net.Buffers 减少系统调用）
func WriteFrame(conn *net.TCPConn, tunnelID [16]byte, frameType byte, data []byte) error {
	if conn == nil {
		return coreerrors.New(coreerrors.CodeNetworkError, "connection is nil")
	}

	if len(data) > MaxFrameSize {
		return coreerrors.Newf(coreerrors.CodeInvalidPacket, "frame too large: %d > %d", len(data), MaxFrameSize)
	}

	// 构造帧头
	header := make([]byte, FrameHeaderSize)
	copy(header[0:16], tunnelID[:])
	header[16] = frameType
	binary.BigEndian.PutUint32(header[17:21], uint32(len(data)))

	// 使用 net.Buffers 一次写入（减少系统调用）
	bufs := net.Buffers{header, data}
	_, err := bufs.WriteTo(conn)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write frame")
	}

	return nil
}

// WriteFrameToWriter 写入帧到通用 Writer
func WriteFrameToWriter(w io.Writer, tunnelID [16]byte, frameType byte, data []byte) error {
	if w == nil {
		return coreerrors.New(coreerrors.CodeNetworkError, "writer is nil")
	}

	if len(data) > MaxFrameSize {
		return coreerrors.Newf(coreerrors.CodeInvalidPacket, "frame too large: %d > %d", len(data), MaxFrameSize)
	}

	// 构造帧头
	header := make([]byte, FrameHeaderSize)
	copy(header[0:16], tunnelID[:])
	header[16] = frameType
	binary.BigEndian.PutUint32(header[17:21], uint32(len(data)))

	// 写入帧头
	if _, err := w.Write(header); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write frame header")
	}

	// 写入数据
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write frame data")
		}
	}

	return nil
}

// ReadFrame 读取帧
func ReadFrame(conn *net.TCPConn) (tunnelID [16]byte, frameType byte, data []byte, err error) {
	return ReadFrameFromReader(conn)
}

// ReadFrameFromReader 从通用 Reader 读取帧
func ReadFrameFromReader(r io.Reader) (tunnelID [16]byte, frameType byte, data []byte, err error) {
	if r == nil {
		err = coreerrors.New(coreerrors.CodeNetworkError, "reader is nil")
		return
	}

	// 读取帧头
	header := make([]byte, FrameHeaderSize)
	if _, err = io.ReadFull(r, header); err != nil {
		if err == io.EOF {
			return
		}
		err = coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read frame header")
		return
	}

	// 解析帧头
	copy(tunnelID[:], header[0:16])
	frameType = header[16]
	length := binary.BigEndian.Uint32(header[17:21])

	// 检查长度
	if length > MaxFrameSize {
		err = coreerrors.Newf(coreerrors.CodeInvalidPacket, "frame too large: %d > %d", length, MaxFrameSize)
		return
	}

	// 读取数据
	if length > 0 {
		data = make([]byte, length)
		if _, err = io.ReadFull(r, data); err != nil {
			err = coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read frame data")
			return
		}
	}

	return
}

// TunnelIDFromString 从字符串解析 TunnelID
func TunnelIDFromString(s string) ([16]byte, error) {
	var id [16]byte
	if len(s) > 16 {
		s = s[:16]
	}
	copy(id[:], s)
	return id, nil
}

// TunnelIDToString 将 TunnelID 转换为字符串
func TunnelIDToString(id [16]byte) string {
	// 找到第一个 0 字节
	for i, b := range id {
		if b == 0 {
			return string(id[:i])
		}
	}
	return string(id[:])
}

// TargetReadyMessage Target 就绪消息
type TargetReadyMessage struct {
	TunnelID     string `json:"tunnel_id"`
	TargetNodeID string `json:"target_node_id"`
}

// EncodeTargetReadyMessage 编码 Target 就绪消息
func EncodeTargetReadyMessage(tunnelID, targetNodeID string) []byte {
	// 简单格式：tunnelID|targetNodeID
	return []byte(fmt.Sprintf("%s|%s", tunnelID, targetNodeID))
}

// DecodeTargetReadyMessage 解码 Target 就绪消息
func DecodeTargetReadyMessage(data []byte) (tunnelID, targetNodeID string, err error) {
	s := string(data)
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			tunnelID = s[:i]
			targetNodeID = s[i+1:]
			return
		}
	}
	err = coreerrors.New(coreerrors.CodeInvalidPacket, "invalid target ready message format")
	return
}

// HTTPProxyMessage 跨节点 HTTP 代理消息
type HTTPProxyMessage struct {
	RequestID string `json:"request_id"` // 请求ID
	ClientID  int64  `json:"client_id"`  // 目标客户端ID
	Request   []byte `json:"request"`    // 序列化的 HTTPProxyRequest
}

// HTTPProxyResponseMessage 跨节点 HTTP 代理响应消息
type HTTPProxyResponseMessage struct {
	RequestID string `json:"request_id"` // 请求ID
	Response  []byte `json:"response"`   // 序列化的 HTTPProxyResponse
	Error     string `json:"error"`      // 错误信息
}

// CommandMessage 通用跨节点命令消息
// 用于统一处理所有需要跨节点转发的命令
type CommandMessage struct {
	CommandID      string `json:"command_id"`       // 命令ID（用于匹配响应）
	CommandType    byte   `json:"command_type"`     // 命令类型（对应 packet.CommandType）
	TargetClientID int64  `json:"target_client_id"` // 目标客户端ID
	SourceNodeID   string `json:"source_node_id"`   // 源节点ID（用于响应路由）
	SourceConnID   string `json:"source_conn_id"`   // 源连接ID（用于响应路由）
	Payload        []byte `json:"payload"`          // 命令载荷（序列化的命令体）
}

// CommandResponseMessage 通用跨节点命令响应消息
type CommandResponseMessage struct {
	CommandID    string `json:"command_id"`     // 命令ID（用于匹配请求）
	CommandType  byte   `json:"command_type"`   // 命令类型
	Success      bool   `json:"success"`        // 是否成功
	Payload      []byte `json:"payload"`        // 响应载荷
	Error        string `json:"error"`          // 错误信息
	SourceConnID string `json:"source_conn_id"` // 源连接ID（用于响应路由）
}
