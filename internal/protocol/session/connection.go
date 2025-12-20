package session

import (
	"net"
	"time"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/stream"
)

// ControlConnectionInterface 控制连接接口
type ControlConnectionInterface interface {
	// GetConnID 获取连接ID
	GetConnID() string

	// GetStream 获取流（返回接口类型）
	GetStream() stream.PackageStreamer

	// GetRemoteAddr 获取远程地址
	GetRemoteAddr() net.Addr

	// Close 关闭连接
	Close() error

	// GetClientID 获取客户端ID
	GetClientID() int64

	// SetClientID 设置客户端ID
	SetClientID(clientID int64)

	// GetUserID 获取用户ID
	GetUserID() string

	// SetUserID 设置用户ID
	SetUserID(userID string)

	// IsAuthenticated 是否已认证
	IsAuthenticated() bool

	// SetAuthenticated 设置认证状态
	SetAuthenticated(authenticated bool)

	// GetProtocol 获取协议类型
	GetProtocol() string

	// UpdateActivity 更新活跃时间
	UpdateActivity()
}

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

// GetStream 获取Stream（实现 Connection 接口）
func (c *ControlConnection) GetStream() stream.PackageStreamer {
	if c == nil {
		return nil
	}
	return c.Stream
}

// GetClientID 获取客户端ID（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) GetClientID() int64 {
	if c == nil {
		return 0
	}
	return c.ClientID
}

// GetUserID 获取用户ID（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) GetUserID() string {
	if c == nil {
		return ""
	}
	return c.UserID
}

// IsAuthenticated 是否已认证（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) IsAuthenticated() bool {
	if c == nil {
		return false
	}
	return c.Authenticated
}

// GetProtocol 获取协议类型（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) GetProtocol() string {
	if c == nil {
		return ""
	}
	return c.Protocol
}

// SetClientID 设置客户端ID（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) SetClientID(clientID int64) {
	if c == nil {
		return
	}
	c.ClientID = clientID
}

// SetUserID 设置用户ID（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) SetUserID(userID string) {
	if c == nil {
		return
	}
	c.UserID = userID
}

// SetAuthenticated 设置认证状态（实现 ControlConnectionInterface 接口）
func (c *ControlConnection) SetAuthenticated(authenticated bool) {
	if c == nil {
		return
	}
	c.Authenticated = authenticated
}

// Close 关闭连接（实现 Connection 接口）
func (c *ControlConnection) Close() error {
	if c == nil || c.Stream == nil {
		return nil
	}
	c.Stream.Close()
	return nil
}

// GetConnID 获取连接ID
func (c *ControlConnection) GetConnID() string {
	if c == nil {
		return ""
	}
	return c.ConnID
}

// GetRemoteAddr 获取远程地址（实现 Connection 接口）
func (c *ControlConnection) GetRemoteAddr() net.Addr {
	if c == nil {
		return nil
	}
	return c.RemoteAddr
}

// GetRemoteAddrString 获取远程地址字符串（向后兼容）
func (c *ControlConnection) GetRemoteAddrString() string {
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

	// ✨ Phase 2: 隧道迁移支持
	sendBuffer    *TunnelSendBuffer    // 发送缓冲区（支持重传）
	receiveBuffer *TunnelReceiveBuffer // 接收缓冲区（支持重组）
	enableSeqNum  bool                 // 是否启用序列号（默认false，保持兼容）
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

		// ✨ Phase 2: 初始化缓冲区（默认不启用）
		sendBuffer:    NewTunnelSendBuffer(),
		receiveBuffer: NewTunnelReceiveBuffer(),
		enableSeqNum:  false, // 默认禁用，保持向后兼容
	}
}

// EnableSequenceNumbers 启用序列号支持（用于支持迁移的隧道）
func (t *TunnelConnection) EnableSequenceNumbers() {
	t.enableSeqNum = true
}

// IsSequenceNumbersEnabled 检查是否启用序列号
func (t *TunnelConnection) IsSequenceNumbersEnabled() bool {
	return t.enableSeqNum
}

// GetStream 获取Stream（实现 Connection 接口）
func (t *TunnelConnection) GetStream() stream.PackageStreamer {
	if t == nil {
		return nil
	}
	return t.Stream
}

// GetConnID 获取连接ID（实现 Connection 接口）
func (t *TunnelConnection) GetConnID() string {
	if t == nil {
		return ""
	}
	return t.ConnID
}

// GetRemoteAddr 获取远程地址（实现 Connection 接口）
func (t *TunnelConnection) GetRemoteAddr() net.Addr {
	if t == nil {
		return nil
	}
	return t.RemoteAddr
}

// GetTunnelID 获取隧道ID（实现 TunnelConnectionInterface 接口）
func (t *TunnelConnection) GetTunnelID() string {
	if t == nil {
		return ""
	}
	return t.TunnelID
}

// GetMappingID 获取映射ID（实现 TunnelConnectionInterface 接口）
func (t *TunnelConnection) GetMappingID() string {
	if t == nil {
		return ""
	}
	return t.MappingID
}

// IsAuthenticated 是否已认证（实现 TunnelConnectionInterface 接口）
func (t *TunnelConnection) IsAuthenticated() bool {
	if t == nil {
		return false
	}
	return t.Authenticated
}

// GetProtocol 获取协议类型（实现 TunnelConnectionInterface 接口）
func (t *TunnelConnection) GetProtocol() string {
	if t == nil {
		return ""
	}
	return t.Protocol
}

// UpdateActivity 更新活跃时间（实现 TunnelConnectionInterface 接口）
func (t *TunnelConnection) UpdateActivity() {
	if t == nil {
		return
	}
	t.LastActiveAt = time.Now()
}

// Close 关闭连接（实现 Connection 接口）
func (t *TunnelConnection) Close() error {
	if t == nil || t.Stream == nil {
		return nil
	}
	t.Stream.Close()
	return nil
}

// ClientConnection 通用客户端连接别名
// 这是设计的一部分，用于提供更通用的接口名称
// 底层实现为 ControlConnection（指令连接）
type ClientConnection = ControlConnection

// NewClientConnection 创建客户端连接的别名
var NewClientConnection = NewControlConnection
