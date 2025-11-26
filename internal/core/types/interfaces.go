package types

import (
	"context"
	"io"
	"net"
	"reflect"
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
	RawConn   net.Conn // ✅ 原始的底层连接（用于纯流转发）
	CreatedAt time.Time
	UpdatedAt time.Time
	// 具体的类型化字段
	LastHeartbeat time.Time // 最后心跳时间
	ClientInfo    string    // 客户端信息
	Protocol      string    // 协议类型
}

// ConnectionMeta 用于在接入阶段传递额外的连接元信息
type ConnectionMeta struct {
	Protocol   string
	RemoteAddr net.Addr
	LocalAddr  net.Addr
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
	GetActiveChannels() int

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

	// 事件驱动相关方法
	// SetEventBus 设置事件总线
	SetEventBus(eventBus interface{}) error

	// GetEventBus 获取事件总线
	GetEventBus() interface{}

	// ==================== Command集成相关方法 ====================
	// RegisterCommandHandler 注册命令处理器
	RegisterCommandHandler(cmdType packet.CommandType, handler CommandHandler) error

	// UnregisterCommandHandler 注销命令处理器
	UnregisterCommandHandler(cmdType packet.CommandType) error

	// ProcessCommand 处理命令（直接处理，不通过事件总线）
	ProcessCommand(connID string, cmd *packet.CommandPacket) (*CommandResponse, error)

	// GetCommandRegistry 获取命令注册表
	GetCommandRegistry() CommandRegistry

	// GetCommandExecutor 获取命令执行器
	GetCommandExecutor() CommandExecutor

	// SetCommandExecutor 设置命令执行器
	SetCommandExecutor(executor CommandExecutor) error
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

// CommandHandler 命令处理器接口
type CommandHandler interface {
	// Handle 处理命令
	Handle(ctx *CommandContext) (*CommandResponse, error)

	// GetDirection 获取命令流向（同时表示响应类型）
	GetDirection() CommandDirection

	// GetCommandType 获取命令类型
	GetCommandType() packet.CommandType

	// GetCategory 获取命令分类
	GetCategory() CommandCategory

	// GetRequestType 获取请求类型（可以为nil，表示无请求体）
	GetRequestType() reflect.Type

	// GetResponseType 获取响应类型（可以为nil，表示无响应体）
	GetResponseType() reflect.Type
}

// CommandCategory 命令分类
type CommandCategory int

const (
	CategoryConnection CommandCategory = iota // 连接管理类命令
	CategoryMapping                           // 端口映射类命令
	CategoryTransport                         // 数据传输类命令
	CategoryManagement                        // 系统管理类命令
	CategoryRPC                               // RPC调用类命令
)

func (c CommandCategory) String() string {
	switch c {
	case CategoryConnection:
		return "connection"
	case CategoryMapping:
		return "mapping"
	case CategoryTransport:
		return "transport"
	case CategoryManagement:
		return "management"
	case CategoryRPC:
		return "rpc"
	default:
		return "unknown"
	}
}

// CommandDirection 命令流向
type CommandDirection int

const (
	DirectionOneway CommandDirection = iota // 单向命令，不等待响应
	DirectionDuplex                         // 双工命令，需要等待响应
)

func (d CommandDirection) String() string {
	switch d {
	case DirectionOneway:
		return "oneway"
	case DirectionDuplex:
		return "duplex"
	default:
		return "unknown"
	}
}

// CommandContext 命令上下文
type CommandContext struct {
	ConnectionID string             // 连接ID
	CommandType  packet.CommandType // 命令类型
	CommandId    string             // 客户端生成的唯一命令ID
	RequestID    string             // 请求ID（Token）
	SenderID     string             // 发送者ID
	ReceiverID   string             // 接收者ID
	RequestBody  string             // JSON请求字符串
	Context      context.Context    // 上下文
	// 具体的类型化字段
	IsAuthenticated bool      // 是否已认证
	UserID          string    // 用户ID
	StartTime       time.Time // 开始时间
	EndTime         time.Time // 结束时间
}

// CommandResponse 命令响应
type CommandResponse struct {
	Success   bool   `json:"success"`
	Data      string `json:"data,omitempty"` // JSON字符串，避免数据丢失
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	CommandId string `json:"command_id,omitempty"` // 对应的命令ID
	// 具体的类型化字段
	ProcessingTime time.Duration `json:"processing_time,omitempty"` // 处理时间
	HandlerName    string        `json:"handler_name,omitempty"`    // 处理器名称
}

// CommandRegistry 命令注册表接口
type CommandRegistry interface {
	// Register 注册命令处理器
	Register(handler CommandHandler) error

	// Unregister 注销命令处理器
	Unregister(commandType packet.CommandType) error

	// GetHandler 获取命令处理器
	GetHandler(commandType packet.CommandType) (CommandHandler, bool)

	// ListHandlers 列出所有已注册的命令类型
	ListHandlers() []packet.CommandType

	// GetHandlerCount 获取处理器数量
	GetHandlerCount() int
}

// Middleware 中间件接口
type Middleware interface {
	// Process 处理中间件逻辑
	Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)
}

// CommandExecutor 命令执行器接口
type CommandExecutor interface {
	// Execute 执行命令
	Execute(streamPacket *StreamPacket) error

	// AddMiddleware 添加中间件
	AddMiddleware(middleware Middleware)

	// SetSession 设置会话引用
	SetSession(session Session)

	// GetRegistry 获取命令注册表
	GetRegistry() CommandRegistry
}
