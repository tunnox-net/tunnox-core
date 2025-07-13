package command

import (
	"context"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
	//"tunnox-core/internal/protocol"
)

// ResponseType 响应类型
type ResponseType int

const (
	Oneway ResponseType = iota // 单向调用，不等待响应
	Duplex                     // 双工方式，需要等待响应
)

// CommandContext 命令上下文，包含所有必要信息
type CommandContext struct {
	ConnectionID string                 // 连接ID
	CommandType  packet.CommandType     // 命令类型
	CommandId    string                 // 客户端生成的唯一命令ID
	RequestID    string                 // 请求ID（Token）
	SenderID     string                 // 发送者ID
	ReceiverID   string                 // 接收者ID
	RequestBody  string                 // JSON请求字符串
	Session      protocol.Session       // 会话对象
	Context      context.Context        // 上下文
	Metadata     map[string]interface{} // 元数据
}

// CommandResponse 命令响应
type CommandResponse struct {
	Success   bool                   `json:"success"`
	Data      interface{}            `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	CommandId string                 `json:"command_id,omitempty"` // 对应的命令ID
}

// CommandHandler 命令处理器接口
type CommandHandler interface {
	// Handle 处理命令
	Handle(ctx *CommandContext) (*CommandResponse, error)

	// GetResponseType 获取响应类型
	GetResponseType() ResponseType

	// GetCommandType 获取命令类型
	GetCommandType() packet.CommandType
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
