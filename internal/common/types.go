package common

import (
	"io"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// Session 会话接口
type Session interface {
	// AcceptConnection 初始化连接
	AcceptConnection(reader io.Reader, writer io.Writer) (*StreamConnectionInfo, error)

	// HandlePacket 处理数据包
	HandlePacket(connPacket *StreamPacket) error

	// CloseConnection 关闭连接
	CloseConnection(connectionId string) error

	// GetStreamManager 获取流管理器
	GetStreamManager() *stream.StreamManager

	// GetStreamConnectionInfo 获取连接信息
	GetStreamConnectionInfo(connectionId string) (*StreamConnectionInfo, bool)

	// GetActiveConnections 获取活跃连接数
	GetActiveConnections() int
}

// StreamPacket 包装结构，包含连接信息
type StreamPacket struct {
	ConnectionID string
	Packet       *packet.TransferPacket
	Timestamp    time.Time
}

// StreamConnectionInfo 连接信息
type StreamConnectionInfo struct {
	ID       string
	Stream   stream.PackageStreamer
	Metadata map[string]interface{}
}
