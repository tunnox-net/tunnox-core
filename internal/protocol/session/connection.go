package session

import (
	"net"
	"time"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/stream"
)

// ControlConnection 指令连接（长连接，每个客户端1条）
// 用途：命令传输、配置推送、心跳保活
// 认证：Handshake + JWT/API Key
type ControlConnection struct {
	ConnID        string
	ClientID      int64  // 认证后绑定的客户端ID
	UserID        string // 认证后绑定的用户ID
	Stream        stream.PackageStreamer
	Authenticated bool     // 认证状态
	RemoteAddr    net.Addr // 远程地址
	Protocol      string   // 协议类型（tcp/websocket/quic）
	CreatedAt     time.Time
	LastActiveAt  time.Time
}

// NewControlConnection 创建指令连接
func NewControlConnection(connID string, stream stream.PackageStreamer, remoteAddr net.Addr, protocol string) *ControlConnection {
	return &ControlConnection{
		ConnID:        connID,
		Stream:        stream,
		RemoteAddr:    remoteAddr,
		Protocol:      protocol,
		Authenticated: false,
		CreatedAt:     time.Now(),
		LastActiveAt:  time.Now(),
	}
}

// UpdateActivity 更新活跃时间
func (c *ControlConnection) UpdateActivity() {
	c.LastActiveAt = time.Now()
}

// IsStale 判断连接是否因超时而失效
func (c *ControlConnection) IsStale(timeout time.Duration) bool {
	if c == nil {
		return true
	}
	return time.Since(c.LastActiveAt) > timeout
}

// GetStream 获取Stream（用于API推送配置等操作）
func (c *ControlConnection) GetStream() interface{} {
	if c == nil {
		return nil
	}
	return c.Stream
}

// GetConnID 获取连接ID
func (c *ControlConnection) GetConnID() string {
	if c == nil {
		return ""
	}
	return c.ConnID
}

// GetRemoteAddr 获取远程地址
func (c *ControlConnection) GetRemoteAddr() string {
	if c == nil || c.RemoteAddr == nil {
		return ""
	}
	return c.RemoteAddr.String()
}

// TunnelConnection 映射连接（短连接，按需建立）
// 用途：纯数据透传
// 认证：TunnelOpen + mapping.SecretKey
type TunnelConnection struct {
	ConnID        string
	TunnelID      string // 隧道ID（唯一标识）
	MappingID     string // 映射ID
	Stream        stream.PackageStreamer
	Authenticated bool     // 基于 secret_key 认证
	RemoteAddr    net.Addr // 远程地址
	Protocol      string   // 协议类型
	CreatedAt     time.Time
	LastActiveAt  time.Time

	// 底层连接（用于兼容）
	baseConn *types.Connection
}

// NewTunnelConnection 创建映射连接
func NewTunnelConnection(connID string, stream stream.PackageStreamer, remoteAddr net.Addr, protocol string) *TunnelConnection {
	return &TunnelConnection{
		ConnID:        connID,
		Stream:        stream,
		RemoteAddr:    remoteAddr,
		Protocol:      protocol,
		Authenticated: false,
		CreatedAt:     time.Now(),
		LastActiveAt:  time.Now(),
	}
}

// UpdateActivity 更新活跃时间
func (t *TunnelConnection) UpdateActivity() {
	t.LastActiveAt = time.Now()
}

// ClientConnection 通用客户端连接别名
// 这是设计的一部分，用于提供更通用的接口名称
// 底层实现为 ControlConnection（指令连接）
type ClientConnection = ControlConnection

// NewClientConnection 创建客户端连接的别名
var NewClientConnection = NewControlConnection
