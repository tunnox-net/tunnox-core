package command

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// CommandExecutor 命令执行器
type CommandExecutor struct {
	*dispose.ManagerBase
	registry   *CommandRegistry
	middleware []Middleware
	rpcManager *RPCManager
	session    types.Session // 添加会话引用
	mu         sync.RWMutex
}

// NewCommandExecutor 创建新的命令执行器
func NewCommandExecutor(registry *CommandRegistry, parentCtx context.Context) *CommandExecutor {
	executor := &CommandExecutor{
		ManagerBase: dispose.NewManager("CommandExecutor", parentCtx),
		registry:    registry,
		middleware:  make([]Middleware, 0),
		rpcManager:  NewRPCManager(parentCtx),
		session:     nil,
	}
	return executor
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

	utils.Debugf("CommandExecutor: executing duplex command, ConnectionID=%s, CommandType=%d, CommandID=%s, RequestID=%s",
		ctx.ConnectionID, ctx.CommandType, ctx.CommandId, requestID)

	// 创建响应通道
	responseChan := make(chan *types.CommandResponse, 1)
	ce.rpcManager.RegisterRequest(requestID, responseChan)
	defer ce.rpcManager.UnregisterRequest(requestID)

	// 获取超时时间
	timeout := ce.rpcManager.GetTimeout()
	utils.Debugf("CommandExecutor: timeout set to %v", timeout)

	// 异步执行命令
	go func() {
		execCtx, cancel := context.WithTimeout(ctx.Context, timeout)
		defer cancel()

		ctx.Context = execCtx
		utils.Debugf("CommandExecutor: calling handler.Handle, ConnectionID=%s, CommandID=%s", ctx.ConnectionID, ctx.CommandId)
		response, err := ce.executeWithMiddleware(ctx, handler)
		if err != nil {
			utils.Errorf("CommandExecutor: handler execution failed: %v", err)
			response = &types.CommandResponse{
				Success: false,
				Error:   err.Error(),
			}
		} else {
			utils.Debugf("CommandExecutor: handler execution succeeded, Success=%v, CommandID=%s", response.Success, ctx.CommandId)
		}

		// 发送响应
		utils.Debugf("CommandExecutor: sending response, ConnectionID=%s, CommandID=%s", ctx.ConnectionID, ctx.CommandId)
		if err := ce.sendResponse(ctx.ConnectionID, response); err != nil {
			utils.Errorf("CommandExecutor: failed to send response: %v", err)
		} else {
			utils.Debugf("CommandExecutor: response sent successfully, ConnectionID=%s, CommandID=%s", ctx.ConnectionID, ctx.CommandId)
		}

		// 将响应发送到通道
		select {
		case responseChan <- response:
			utils.Debugf("CommandExecutor: response sent to channel, CommandID=%s", ctx.CommandId)
		default:
			utils.Warnf("CommandExecutor: response channel is full, dropping response")
		}
	}()

	// 等待响应
	utils.Debugf("CommandExecutor: waiting for response, CommandID=%s, timeout=%v", ctx.CommandId, timeout)
	select {
	case response := <-responseChan:
		utils.Debugf("CommandExecutor: received response from channel, Success=%v, CommandID=%s", response.Success, ctx.CommandId)
		if !response.Success {
			return fmt.Errorf("command execution failed: %s", response.Error)
		}
		return nil
	case <-time.After(timeout):
		utils.Errorf("CommandExecutor: command timeout, CommandID=%s, ConnectionID=%s", ctx.CommandId, ctx.ConnectionID)
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
	utils.Debugf("CommandExecutor.sendResponse: sending response, ConnectionID=%s, CommandID=%s, Success=%v",
		connectionID, response.CommandId, response.Success)

	if ce.session == nil {
		return fmt.Errorf("session not set, cannot send response")
	}

	// 通过 Session 接口获取连接
	conn, exists := ce.session.GetConnection(connectionID)
	if !exists || conn == nil {
		utils.Errorf("CommandExecutor.sendResponse: connection not found, ConnectionID=%s", connectionID)
		return fmt.Errorf("connection not found: %s", connectionID)
	}

	if conn.Stream == nil {
		utils.Errorf("CommandExecutor.sendResponse: stream is nil, ConnectionID=%s", connectionID)
		return fmt.Errorf("stream is nil for connection %s", connectionID)
	}

	pkgStream := conn.Stream
	utils.Debugf("CommandExecutor.sendResponse: got stream, ConnectionID=%s, StreamType=%T", connectionID, pkgStream)

	// 构造响应数据
	responseData := map[string]interface{}{
		"success":    response.Success,
		"command_id": response.CommandId,
		"request_id": response.RequestID,
	}

	if response.Data != "" {
		// 如果 Data 是 JSON 字符串，直接使用；否则序列化
		var dataObj interface{}
		if err := json.Unmarshal([]byte(response.Data), &dataObj); err == nil {
			responseData["data"] = dataObj
		} else {
			responseData["data"] = response.Data
		}
	}

	if response.Error != "" {
		responseData["error"] = response.Error
	}

	// 序列化响应
	dataBytes, err := json.Marshal(responseData)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// 构造响应包（CommandResp 是 PacketType，不是 CommandType）
	cmdPacket := &packet.CommandPacket{
		CommandType: 0, // 响应不需要 CommandType
		CommandId:   response.CommandId,
		Token:       response.RequestID,
		CommandBody: string(dataBytes),
	}

	transferPacket := &packet.TransferPacket{
		PacketType:    packet.CommandResp,
		CommandPacket: cmdPacket,
	}

	// 发送响应
	utils.Debugf("CommandExecutor.sendResponse: writing packet, ConnectionID=%s, PacketType=%d, CommandID=%s",
		connectionID, transferPacket.PacketType, response.CommandId)
	written, err := pkgStream.WritePacket(transferPacket, false, 0)
	if err != nil {
		utils.Errorf("CommandExecutor.sendResponse: failed to write packet, ConnectionID=%s, Error=%v", connectionID, err)
		return fmt.Errorf("failed to write response packet: %w", err)
	}

	utils.Infof("CommandExecutor.sendResponse: response sent successfully, ConnectionID=%s, CommandID=%s, Success=%v, Bytes=%d",
		connectionID, response.CommandId, response.Success, written)

	return nil
}

// generateRequestID 生成请求ID
func (ce *CommandExecutor) generateRequestID() string {
	// 使用纳秒时间戳 + 随机数确保唯一性
	randomSuffix, _ := utils.GenerateRandomDigits(4)
	return fmt.Sprintf("req_%d%s", time.Now().UnixNano(), randomSuffix)
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
