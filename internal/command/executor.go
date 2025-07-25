package command

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/utils"
)

// CommandExecutor 命令执行器
type CommandExecutor struct {
	registry   *CommandRegistry
	middleware []Middleware
	rpcManager *RPCManager
	session    types.Session // 添加会话引用
	mu         sync.RWMutex
}

// NewCommandExecutor 创建新的命令执行器
func NewCommandExecutor(registry *CommandRegistry) *CommandExecutor {
	return &CommandExecutor{
		registry:   registry,
		middleware: make([]Middleware, 0),
		rpcManager: NewRPCManager(),
		session:    nil,
	}
}

// AddMiddleware 添加中间件
func (ce *CommandExecutor) AddMiddleware(middleware types.Middleware) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.middleware = append(ce.middleware, middleware)
}

// Execute 执行命令
func (ce *CommandExecutor) Execute(streamPacket *types.StreamPacket) error {
	// 创建命令上下文
	ctx := ce.createCommandContext(streamPacket)

	// 获取命令处理器
	handler, exists := ce.registry.GetHandler(ctx.CommandType)
	if !exists {
		return fmt.Errorf("no handler registered for command type: %v", ctx.CommandType)
	}

	// 根据响应类型处理
	switch handler.GetDirection() {
	case types.DirectionOneway:
		return ce.executeOneway(ctx, handler)
	case types.DirectionDuplex:
		return ce.executeDuplex(ctx, handler)
	default:
		return fmt.Errorf("unknown response type: %v", handler.GetDirection())
	}
}

// executeOneway 执行单向命令
func (ce *CommandExecutor) executeOneway(ctx *types.CommandContext, handler types.CommandHandler) error {
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
func (ce *CommandExecutor) executeDuplex(ctx *types.CommandContext, handler types.CommandHandler) error {
	// 生成请求ID
	requestID := ce.generateRequestID()
	ctx.RequestID = requestID

	// 创建响应通道
	responseChan := make(chan *types.CommandResponse, 1)
	ce.rpcManager.RegisterRequest(requestID, responseChan)
	defer ce.rpcManager.UnregisterRequest(requestID)

	// 获取超时时间
	timeout := ce.rpcManager.GetTimeout()

	// 异步执行命令
	go func() {
		execCtx, cancel := context.WithTimeout(ctx.Context, timeout)
		defer cancel()

		ctx.Context = execCtx
		response, err := ce.executeWithMiddleware(ctx, handler)
		if err != nil {
			response = &types.CommandResponse{
				Success: false,
				Error:   err.Error(),
			}
		}

		// 发送响应
		if err := ce.sendResponse(ctx.ConnectionID, response); err != nil {
			utils.Errorf("Failed to send response: %v", err)
		}

		// 将响应发送到通道
		select {
		case responseChan <- response:
		default:
			utils.Warnf("Response channel is full, dropping response")
		}
	}()

	// 等待响应
	select {
	case response := <-responseChan:
		if !response.Success {
			return fmt.Errorf("command execution failed: %s", response.Error)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("command timeout")
	}
}

// executeWithMiddleware 使用中间件执行命令
func (ce *CommandExecutor) executeWithMiddleware(ctx *types.CommandContext, handler types.CommandHandler) (*types.CommandResponse, error) {
	ce.mu.RLock()
	middleware := make([]types.Middleware, len(ce.middleware))
	copy(middleware, ce.middleware)
	ce.mu.RUnlock()

	// 构建中间件链
	var next func(*types.CommandContext) (*types.CommandResponse, error)
	next = func(ctx *types.CommandContext) (*types.CommandResponse, error) {
		return handler.Handle(ctx)
	}

	// 从后往前应用中间件
	for i := len(middleware) - 1; i >= 0; i-- {
		current := middleware[i]
		nextFunc := next
		next = func(ctx *types.CommandContext) (*types.CommandResponse, error) {
			return current.Process(ctx, nextFunc)
		}
	}

	return next(ctx)
}

// createCommandContext 创建命令上下文
func (ce *CommandExecutor) createCommandContext(streamPacket *types.StreamPacket) *types.CommandContext {
	return &types.CommandContext{
		ConnectionID:    streamPacket.ConnectionID,
		CommandType:     streamPacket.Packet.CommandPacket.CommandType,
		CommandId:       streamPacket.Packet.CommandPacket.CommandId,
		RequestID:       streamPacket.Packet.CommandPacket.Token,
		SenderID:        streamPacket.Packet.CommandPacket.SenderId,
		ReceiverID:      streamPacket.Packet.CommandPacket.ReceiverId,
		RequestBody:     streamPacket.Packet.CommandPacket.CommandBody,
		Context:         context.Background(),
		IsAuthenticated: false,
		UserID:          "",
		StartTime:       time.Now(),
		EndTime:         time.Time{},
	}
}

// sendResponse 发送响应
func (ce *CommandExecutor) sendResponse(connectionID string, response *types.CommandResponse) error {
	// 如果有会话引用，通过会话发送响应
	if ce.session != nil {
		// 这里可以通过会话的流管理器发送响应
		utils.Infof("Sending response to connection %s via session: success=%v", connectionID, response.Success)
	} else {
		// 否则只是记录日志
		utils.Infof("Sending response to connection %s: success=%v", connectionID, response.Success)
	}
	return nil
}

// generateRequestID 生成请求ID
func (ce *CommandExecutor) generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// SetSession 设置会话
func (ce *CommandExecutor) SetSession(session types.Session) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.session = session
	utils.Infof("Session set in command executor")
}

// GetRegistry 获取命令注册表
func (ce *CommandExecutor) GetRegistry() types.CommandRegistry {
	return ce.registry
}
