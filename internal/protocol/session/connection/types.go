package connection

import (
	"net"
	"sync"
	"time"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/protocol/session/buffer"
	"tunnox-core/internal/stream"
)

// ============================================================================
// 临时类型别名（等待其他包迁移完成后移除）
// ============================================================================

// TunnelSendBuffer 隧道发送缓冲区（临时别名）
type TunnelSendBuffer = buffer.SendBuffer

// TunnelReceiveBuffer 隧道接收缓冲区（临时别名）
type TunnelReceiveBuffer = buffer.ReceiveBuffer

// NewTunnelSendBuffer 创建发送缓冲区
var NewTunnelSendBuffer = buffer.NewSendBuffer

// NewTunnelReceiveBuffer 创建接收缓冲区
var NewTunnelReceiveBuffer = buffer.NewReceiveBuffer

// ============================================================================
// 通用连接接口（协议无关）
// ============================================================================

// TunnelConnectionInterface 隧道连接接口（所有协议通用）
// 抽象了不同协议的连接管理差异
type TunnelConnectionInterface interface {
	// 基础信息
	GetConnectionID() string // 连接标识（协议特定实现）
	GetClientID() int64      // 客户端ID（所有协议通用）
	GetMappingID() string    // 映射ID（所有协议通用）
	GetTunnelID() string     // 隧道ID（所有协议通用）
	GetProtocol() string     // 协议类型（tcp/websocket/quic）

	// 流接口
	GetStream() stream.PackageStreamer // 获取流（所有协议通用）
	GetNetConn() net.Conn              // 获取底层连接（TCP/WebSocket/QUIC 返回 net.Conn）

	// 连接状态管理（统一接口）
	ConnectionState() ConnectionStateManager     // 获取连接状态管理器
	ConnectionTimeout() ConnectionTimeoutManager // 获取超时管理器
	ConnectionError() ConnectionErrorHandler     // 获取错误处理器
	ConnectionReuse() ConnectionReuseStrategy    // 获取复用策略

	// 生命周期
	Close() error   // 关闭连接（所有协议通用）
	IsClosed() bool // 检查是否已关闭
}

// ControlConnectionInterface 控制连接接口
type ControlConnectionInterface interface {
	GetConnID() string
	GetStream() stream.PackageStreamer
	GetRemoteAddr() net.Addr
	Close() error
	GetClientID() int64
	SetClientID(clientID int64)
	GetUserID() string
	SetUserID(userID string)
	IsAuthenticated() bool
	SetAuthenticated(authenticated bool)
	GetProtocol() string
	UpdateActivity()

	// 挑战-响应认证支持
	SetPendingChallenge(challenge string)
	GetPendingChallenge() string
	ClearPendingChallenge()
}

// ============================================================================
// 连接状态管理接口
// ============================================================================

// ConnectionStateManager 连接状态管理器接口
type ConnectionStateManager interface {
	IsConnected() bool
	IsClosed() bool
	GetState() ConnectionStateType
	SetState(state ConnectionStateType)
	UpdateActivity()
	GetLastActiveTime() time.Time
	GetCreatedTime() time.Time
	IsStale(timeout time.Duration) bool
}

// ConnectionStateType 连接状态类型
type ConnectionStateType int

const (
	StateConnecting ConnectionStateType = iota // 连接中
	StateConnected                             // 已连接
	StateStreaming                             // 流模式（隧道数据传输）
	StateClosing                               // 关闭中
	StateClosed                                // 已关闭
)

// ============================================================================
// 超时管理接口
// ============================================================================

// ConnectionTimeoutManager 连接超时管理器接口
type ConnectionTimeoutManager interface {
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetDeadline(t time.Time) error
	GetReadTimeout() time.Duration
	GetWriteTimeout() time.Duration
	GetIdleTimeout() time.Duration
	IsReadTimeout(err error) bool
	IsWriteTimeout(err error) bool
	IsIdleTimeout() bool
	ResetReadDeadline() error
	ResetWriteDeadline() error
	ResetDeadline() error
}

// ============================================================================
// 错误处理接口
// ============================================================================

// ConnectionErrorHandler 连接错误处理器接口
type ConnectionErrorHandler interface {
	HandleError(err error) error
	IsRetryable(err error) bool
	ShouldClose(err error) bool
	IsTemporary(err error) bool
	ClassifyError(err error) ErrorType
	GetLastError() error
	ClearError()
}

// ErrorType 错误类型
type ErrorType int

const (
	ErrorNone     ErrorType = iota // 无错误
	ErrorNetwork                   // 网络错误（可重试）
	ErrorTimeout                   // 超时错误（可重试）
	ErrorProtocol                  // 协议错误（不可重试）
	ErrorAuth                      // 认证错误（不可重试）
	ErrorClosed                    // 连接已关闭（不可重试）
	ErrorUnknown                   // 未知错误
)

// ============================================================================
// 连接复用策略接口
// ============================================================================

// ConnectionReuseStrategy 连接复用策略接口
type ConnectionReuseStrategy interface {
	CanReuse(conn TunnelConnectionInterface, tunnelID string) bool
	ShouldCreateNew(tunnelID string) bool
	MarkAsReusable(conn TunnelConnectionInterface)
	MarkAsUsed(conn TunnelConnectionInterface, tunnelID string)
	Release(conn TunnelConnectionInterface)
	GetReuseCount(conn TunnelConnectionInterface) int
	GetMaxReuseCount() int
}

// ============================================================================
// ControlConnection 指令连接
// ============================================================================

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
	mu            sync.RWMutex // 保护 LastActiveAt 和 PendingChallenge 的并发访问
	LastActiveAt  time.Time

	// 挑战-响应认证
	PendingChallenge string // 待验证的挑战（认证第一阶段发送给客户端）
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

func (c *ControlConnection) UpdateActivity() {
	c.mu.Lock()
	c.LastActiveAt = time.Now()
	c.mu.Unlock()
}

func (c *ControlConnection) IsStale(timeout time.Duration) bool {
	if c == nil {
		return true
	}
	c.mu.RLock()
	lastActive := c.LastActiveAt
	c.mu.RUnlock()
	return time.Since(lastActive) > timeout
}

func (c *ControlConnection) GetStream() stream.PackageStreamer {
	if c == nil {
		return nil
	}
	return c.Stream
}

func (c *ControlConnection) GetClientID() int64 {
	if c == nil {
		return 0
	}
	return c.ClientID
}

func (c *ControlConnection) GetUserID() string {
	if c == nil {
		return ""
	}
	return c.UserID
}

func (c *ControlConnection) IsAuthenticated() bool {
	if c == nil {
		return false
	}
	return c.Authenticated
}

func (c *ControlConnection) GetProtocol() string {
	if c == nil {
		return ""
	}
	return c.Protocol
}

func (c *ControlConnection) SetClientID(clientID int64) {
	if c == nil {
		return
	}
	c.ClientID = clientID
}

func (c *ControlConnection) SetUserID(userID string) {
	if c == nil {
		return
	}
	c.UserID = userID
}

func (c *ControlConnection) SetAuthenticated(authenticated bool) {
	if c == nil {
		return
	}
	c.Authenticated = authenticated
}

func (c *ControlConnection) Close() error {
	if c == nil || c.Stream == nil {
		return nil
	}
	c.Stream.Close()
	return nil
}

func (c *ControlConnection) GetConnID() string {
	if c == nil {
		return ""
	}
	return c.ConnID
}

func (c *ControlConnection) GetRemoteAddr() net.Addr {
	if c == nil {
		return nil
	}
	return c.RemoteAddr
}

func (c *ControlConnection) GetRemoteAddrString() string {
	if c == nil || c.RemoteAddr == nil {
		return ""
	}
	return c.RemoteAddr.String()
}

// SetPendingChallenge 设置待验证的挑战
func (c *ControlConnection) SetPendingChallenge(challenge string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.PendingChallenge = challenge
	c.mu.Unlock()
}

// GetPendingChallenge 获取待验证的挑战
func (c *ControlConnection) GetPendingChallenge() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	challenge := c.PendingChallenge
	c.mu.RUnlock()
	return challenge
}

// ClearPendingChallenge 清除待验证的挑战
func (c *ControlConnection) ClearPendingChallenge() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.PendingChallenge = ""
	c.mu.Unlock()
}

// ============================================================================
// TunnelConnection 映射连接
// ============================================================================

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

	// Phase 2: 隧道迁移支持
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
		sendBuffer:    NewTunnelSendBuffer(),
		receiveBuffer: NewTunnelReceiveBuffer(),
		enableSeqNum:  false, // 默认禁用，保持向后兼容
	}
}

func (t *TunnelConnection) EnableSequenceNumbers() {
	t.enableSeqNum = true
}

func (t *TunnelConnection) IsSequenceNumbersEnabled() bool {
	return t.enableSeqNum
}

func (t *TunnelConnection) GetStream() stream.PackageStreamer {
	if t == nil {
		return nil
	}
	return t.Stream
}

func (t *TunnelConnection) GetConnID() string {
	if t == nil {
		return ""
	}
	return t.ConnID
}

func (t *TunnelConnection) GetRemoteAddr() net.Addr {
	if t == nil {
		return nil
	}
	return t.RemoteAddr
}

func (t *TunnelConnection) GetTunnelID() string {
	if t == nil {
		return ""
	}
	return t.TunnelID
}

func (t *TunnelConnection) GetMappingID() string {
	if t == nil {
		return ""
	}
	return t.MappingID
}

func (t *TunnelConnection) IsAuthenticated() bool {
	if t == nil {
		return false
	}
	return t.Authenticated
}

func (t *TunnelConnection) GetProtocol() string {
	if t == nil {
		return ""
	}
	return t.Protocol
}

func (t *TunnelConnection) UpdateActivity() {
	if t == nil {
		return
	}
	t.LastActiveAt = time.Now()
}

func (t *TunnelConnection) Close() error {
	if t == nil || t.Stream == nil {
		return nil
	}
	t.Stream.Close()
	return nil
}

// GetSendBuffer 获取发送缓冲区（用于状态迁移）
func (t *TunnelConnection) GetSendBuffer() *TunnelSendBuffer {
	if t == nil {
		return nil
	}
	return t.sendBuffer
}

// GetReceiveBuffer 获取接收缓冲区（用于状态迁移）
func (t *TunnelConnection) GetReceiveBuffer() *TunnelReceiveBuffer {
	if t == nil {
		return nil
	}
	return t.receiveBuffer
}

// ClientConnection 通用客户端连接别名
type ClientConnection = ControlConnection

// NewClientConnection 创建客户端连接的别名
var NewClientConnection = NewControlConnection

// ============================================================================
// 连接工厂函数已移至 connection_factory.go
// ============================================================================
