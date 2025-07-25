package common

import (
	"context"
	"time"
	"tunnox-core/internal/packet"
)

// CommandHandler 命令处理器接口
type CommandHandler interface {
	// Handle 处理命令
	Handle(ctx *CommandContext) (*CommandResponse, error)

	// GetResponseType 获取响应类型
	GetResponseType() CommandResponseType

	// GetCommandType 获取命令类型
	GetCommandType() packet.CommandType

	// GetCategory 获取命令分类
	GetCategory() CommandCategory

	// GetDirection 获取命令流向
	GetDirection() CommandDirection
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

// CommandResponseType 响应类型
type CommandResponseType = CommandDirection

const (
	ResponseOneway CommandResponseType = DirectionOneway
	ResponseDuplex CommandResponseType = DirectionDuplex
)

// CommandContext 命令上下文
type CommandContext struct {
	ConnectionID string             // 连接ID
	CommandType  packet.CommandType // 命令类型
	CommandId    string             // 客户端生成的唯一命令ID
	RequestID    string             // 请求ID（Token）
	SenderID     string             // 发送者ID
	ReceiverID   string             // 接收者ID
	RequestBody  string             // JSON请求字符串
	Session      Session            // 会话对象
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
