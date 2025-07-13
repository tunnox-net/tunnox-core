package command

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"
)

// CommandExecutor 命令执行器
type CommandExecutor struct {
	registry   *CommandRegistry
	middleware []Middleware
	rpcManager *RPCManager
	mu         sync.RWMutex
}

// NewCommandExecutor 创建新的命令执行器
func NewCommandExecutor(registry *CommandRegistry) *CommandExecutor {
	return &CommandExecutor{
		registry:   registry,
		middleware: make([]Middleware, 0),
		rpcManager: NewRPCManager(),
	}
}

// AddMiddleware 添加中间件
func (ce *CommandExecutor) AddMiddleware(middleware Middleware) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.middleware = append(ce.middleware, middleware)
}

// Execute 执行命令
func (ce *CommandExecutor) Execute(streamPacket *protocol.StreamPacket) error {
	// 创建命令上下文
	ctx := ce.createCommandContext(streamPacket)

	// 获取命令处理器
	handler, exists := ce.registry.GetHandler(ctx.CommandType)
	if !exists {
		return fmt.Errorf("no handler registered for command type: %v", ctx.CommandType)
	}

	// 根据响应类型处理
	switch handler.GetResponseType() {
	case Oneway:
		return ce.executeOneway(ctx, handler)
	case Duplex:
		return ce.executeDuplex(ctx, handler)
	default:
		return fmt.Errorf("unknown response type: %v", handler.GetResponseType())
	}
}

// executeOneway 执行单向命令
func (ce *CommandExecutor) executeOneway(ctx *CommandContext, handler CommandHandler) error {
	// 异步执行，不等待响应
	go func() {
		execCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
		defer cancel()

		ctx.Context = execCtx
		_, err := ce.executeWithMiddleware(ctx, handler)
		if err != nil {
			utils.Errorf("Oneway command handler failed: %v", err)
		}
	}()

	return nil
}

// executeDuplex 执行双工命令
func (ce *CommandExecutor) executeDuplex(ctx *CommandContext, handler CommandHandler) error {
	// 生成请求ID
	requestID := ce.generateRequestID()
	ctx.RequestID = requestID

	// 创建响应通道
	responseChan := make(chan *CommandResponse, 1)
	ce.rpcManager.RegisterRequest(requestID, responseChan)
	defer ce.rpcManager.UnregisterRequest(requestID)

	// 异步执行命令
	go func() {
		execCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
		defer cancel()

		ctx.Context = execCtx
		response, err := ce.executeWithMiddleware(ctx, handler)
		if err != nil {
			response = &CommandResponse{
				Success: false,
				Error:   err.Error(),
			}
		}

		response.RequestID = requestID
		responseChan <- response
	}()

	// 等待响应
	select {
	case response := <-responseChan:
		return ce.sendResponse(ctx.ConnectionID, response)
	case <-time.After(ce.rpcManager.GetTimeout()):
		return fmt.Errorf("command timeout")
	}
}

// executeWithMiddleware 执行带中间件的命令
func (ce *CommandExecutor) executeWithMiddleware(ctx *CommandContext, handler CommandHandler) (*CommandResponse, error) {
	ce.mu.RLock()
	middleware := make([]Middleware, len(ce.middleware))
	copy(middleware, ce.middleware)
	ce.mu.RUnlock()

	// 构建中间件链
	var next func(*CommandContext) (*CommandResponse, error)
	next = func(ctx *CommandContext) (*CommandResponse, error) {
		return handler.Handle(ctx)
	}

	// 从后往前包装中间件
	for i := len(middleware) - 1; i >= 0; i-- {
		currentMiddleware := middleware[i]
		currentNext := next
		next = func(ctx *CommandContext) (*CommandResponse, error) {
			return currentMiddleware.Process(ctx, currentNext)
		}
	}

	return next(ctx)
}

// createCommandContext 创建命令上下文
func (ce *CommandExecutor) createCommandContext(streamPacket *protocol.StreamPacket) *CommandContext {
	commandPacket := streamPacket.Packet.CommandPacket

	return &CommandContext{
		ConnectionID: streamPacket.ConnectionID,
		CommandType:  commandPacket.CommandType,
		RequestID:    commandPacket.Token,
		SenderID:     commandPacket.SenderId,
		ReceiverID:   commandPacket.ReceiverId,
		RequestBody:  commandPacket.CommandBody,
		Session:      nil, // 需要从外部设置
		Context:      context.Background(),
		Metadata:     make(map[string]interface{}),
	}
}

// sendResponse 发送响应
func (ce *CommandExecutor) sendResponse(connectionID string, response *CommandResponse) error {
	// 序列化响应
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// 创建响应包
	responsePacket := &packet.CommandPacket{
		CommandType: 0, // 响应包使用特殊类型
		Token:       response.RequestID,
		SenderId:    "", // 服务端发送
		ReceiverId:  connectionID,
		CommandBody: string(responseData),
	}

	// 创建传输包
	transferPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: responsePacket,
	}

	// 获取连接并发送
	// 这里需要从Session获取Stream，暂时返回nil
	// TODO: 实现从Session获取Stream的逻辑
	_ = transferPacket // 避免未使用变量警告
	return nil
}

// generateRequestID 生成请求ID
func (ce *CommandExecutor) generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// SetSession 设置会话对象
func (ce *CommandExecutor) SetSession(session protocol.Session) {
	// 这个方法需要在外部调用时设置Session
	// 暂时为空实现
}
