package common

import (
	"io"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// ConnectionState 连接状态
type ConnectionState int

const (
	StateInitializing ConnectionState = iota
	StateConnected
	StateAuthenticated
	StateActive
	StateClosing
	StateClosed
)

func (s ConnectionState) String() string {
	switch s {
	case StateInitializing:
		return "initializing"
	case StateConnected:
		return "connected"
	case StateAuthenticated:
		return "authenticated"
	case StateActive:
		return "active"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Connection 连接信息
type Connection struct {
	ID        string
	State     ConnectionState
	Stream    stream.PackageStreamer
	CreatedAt time.Time
	UpdatedAt time.Time
	// 移除 Metadata map[string]interface{}，添加具体的字段
	LastHeartbeat time.Time // 最后心跳时间
	ClientInfo    string    // 客户端信息
	Protocol      string    // 协议类型
}

// Session 会话接口
type Session interface {
	// 向后兼容的方法
	AcceptConnection(reader io.Reader, writer io.Writer) (*StreamConnection, error)
	HandlePacket(connPacket *StreamPacket) error
	CloseConnection(connectionId string) error
	GetStreamManager() *stream.StreamManager
	GetStreamConnectionInfo(connectionId string) (*StreamConnection, bool)
	GetActiveConnections() int

	// 新增的清晰接口方法
	// CreateConnection 创建新连接
	CreateConnection(reader io.Reader, writer io.Writer) (*Connection, error)

	// ProcessPacket 处理数据包（更清晰的命名）
	ProcessPacket(connID string, packet *packet.TransferPacket) error

	// GetConnection 获取连接信息
	GetConnection(connID string) (*Connection, bool)

	// ListConnections 列出所有连接
	ListConnections() []*Connection

	// UpdateConnectionState 更新连接状态
	UpdateConnectionState(connID string, state ConnectionState) error
}

// StreamPacket 包装结构，包含连接信息
type StreamPacket struct {
	ConnectionID string
	Packet       *packet.TransferPacket
	Timestamp    time.Time
}

// StreamConnection 连接信息（保持向后兼容）
type StreamConnection struct {
	ID     string
	Stream stream.PackageStreamer
}
