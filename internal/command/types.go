package command

import (
	"context"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
)

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

// ResponseType 响应类型（保持向后兼容）
type ResponseType = CommandDirection

const (
	Oneway ResponseType = DirectionOneway
	Duplex ResponseType = DirectionDuplex
)

// CommandType 重新定义的命令类型
type CommandType struct {
	ID          packet.CommandType // 原始命令ID
	Category    CommandCategory    // 命令分类
	Direction   CommandDirection   // 命令流向
	Name        string             // 命令名称
	Description string             // 命令描述
}

// 预定义命令类型
var (
	// 连接管理类命令
	TcpMapCmd = CommandType{
		ID:          packet.TcpMap,
		Category:    CategoryMapping,
		Direction:   DirectionOneway,
		Name:        "tcp_map",
		Description: "TCP端口映射",
	}

	HttpMapCmd = CommandType{
		ID:          packet.HttpMap,
		Category:    CategoryMapping,
		Direction:   DirectionOneway,
		Name:        "http_map",
		Description: "HTTP端口映射",
	}

	SocksMapCmd = CommandType{
		ID:          packet.SocksMap,
		Category:    CategoryMapping,
		Direction:   DirectionOneway,
		Name:        "socks_map",
		Description: "SOCKS代理映射",
	}

	// 数据传输类命令
	DataInCmd = CommandType{
		ID:          packet.DataIn,
		Category:    CategoryTransport,
		Direction:   DirectionOneway,
		Name:        "data_in",
		Description: "数据输入通知",
	}

	DataOutCmd = CommandType{
		ID:          packet.DataOut,
		Category:    CategoryTransport,
		Direction:   DirectionOneway,
		Name:        "data_out",
		Description: "数据输出通知",
	}

	ForwardCmd = CommandType{
		ID:          packet.Forward,
		Category:    CategoryTransport,
		Direction:   DirectionOneway,
		Name:        "forward",
		Description: "服务端间转发",
	}

	// 连接管理类命令
	DisconnectCmd = CommandType{
		ID:          packet.Disconnect,
		Category:    CategoryConnection,
		Direction:   DirectionOneway,
		Name:        "disconnect",
		Description: "连接断开",
	}

	// RPC类命令
	RpcInvokeCmd = CommandType{
		ID:          packet.RpcInvoke,
		Category:    CategoryRPC,
		Direction:   DirectionDuplex,
		Name:        "rpc_invoke",
		Description: "RPC调用",
	}
)

// CommandContext 命令上下文，包含所有必要信息
type CommandContext struct {
	ConnectionID string             // 连接ID
	CommandType  packet.CommandType // 命令类型
	CommandId    string             // 客户端生成的唯一命令ID
	RequestID    string             // 请求ID（Token）
	SenderID     string             // 发送者ID
	ReceiverID   string             // 接收者ID
	RequestBody  string             // JSON请求字符串
	Session      protocol.Session   // 会话对象
	Context      context.Context    // 上下文
	// 移除 Metadata map[string]interface{}，添加具体的字段
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
	// 移除 Metadata map[string]interface{}，添加具体的字段
	ProcessingTime time.Duration `json:"processing_time,omitempty"` // 处理时间
	HandlerName    string        `json:"handler_name,omitempty"`    // 处理器名称
}

// CommandHandler 命令处理器接口
type CommandHandler interface {
	// Handle 处理命令
	Handle(ctx *CommandContext) (*CommandResponse, error)

	// GetResponseType 获取响应类型
	GetResponseType() ResponseType

	// GetCommandType 获取命令类型
	GetCommandType() packet.CommandType

	// GetCategory 获取命令分类
	GetCategory() CommandCategory

	// GetDirection 获取命令流向
	GetDirection() CommandDirection
}

// Middleware 中间件接口
type Middleware interface {
	// Process 处理中间件逻辑
	Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)
}

// MiddlewareFunc 中间件函数类型
type MiddlewareFunc func(*CommandContext, func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error)

// Process 实现Middleware接口
func (f MiddlewareFunc) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	return f(ctx, next)
}
