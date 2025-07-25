package command

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// CommandStats 命令统计信息
type CommandStats struct {
	TotalCommands   int64         `json:"total_commands"`
	SuccessCommands int64         `json:"success_commands"`
	FailedCommands  int64         `json:"failed_commands"`
	AverageLatency  time.Duration `json:"average_latency"`
	LastCommandTime time.Time     `json:"last_command_time"`
	ActiveCommands  int64         `json:"active_commands"`
	mu              sync.RWMutex
}

// IncrementTotal 增加总命令数
func (cs *CommandStats) IncrementTotal() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.TotalCommands++
	cs.LastCommandTime = time.Now()
}

// IncrementSuccess 增加成功命令数
func (cs *CommandStats) IncrementSuccess() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.SuccessCommands++
}

// IncrementFailed 增加失败命令数
func (cs *CommandStats) IncrementFailed() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.FailedCommands++
}

// UpdateLatency 更新平均延迟
func (cs *CommandStats) UpdateLatency(latency time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.TotalCommands > 0 {
		// 计算移动平均
		totalLatency := cs.AverageLatency * time.Duration(cs.TotalCommands-1)
		cs.AverageLatency = (totalLatency + latency) / time.Duration(cs.TotalCommands)
	} else {
		cs.AverageLatency = latency
	}
}

// IncrementActive 增加活跃命令数
func (cs *CommandStats) IncrementActive() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.ActiveCommands++
}

// DecrementActive 减少活跃命令数
func (cs *CommandStats) DecrementActive() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ActiveCommands > 0 {
		cs.ActiveCommands--
	}
}

// GetStats 获取统计信息副本
func (cs *CommandStats) GetStats() CommandStats {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return CommandStats{
		TotalCommands:   cs.TotalCommands,
		SuccessCommands: cs.SuccessCommands,
		FailedCommands:  cs.FailedCommands,
		AverageLatency:  cs.AverageLatency,
		LastCommandTime: cs.LastCommandTime,
		ActiveCommands:  cs.ActiveCommands,
	}
}

// CommandService 命令服务接口
type CommandService interface {
	// Execute 执行命令（保持向后兼容）
	Execute(ctx *CommandContext) (*CommandResponse, error)

	// ExecuteAsync 异步执行命令（保持向后兼容）
	ExecuteAsync(ctx *CommandContext) (<-chan *CommandResponse, <-chan error)

	// Use 注册中间件
	Use(middleware Middleware)

	// RegisterHandler 注册命令处理器
	RegisterHandler(handler CommandHandler) error

	// UnregisterHandler 注销命令处理器
	UnregisterHandler(commandType packet.CommandType) error

	// GetStats 获取统计信息
	GetStats() *CommandStats

	// SetResponseSender 设置响应发送器
	SetResponseSender(sender ResponseSender)

	// SetEventBus 设置事件总线
	SetEventBus(eventBus events.EventBus) error

	// Start 启动命令服务（开始监听事件）
	Start() error

	// Close 关闭服务
	Close() error
}

// ResponseSender 响应发送接口
type ResponseSender interface {
	SendResponse(connID string, response *CommandResponse) error
}

// CommandServiceImpl 命令服务实现
type CommandServiceImpl struct {
	registry       *CommandRegistry
	executor       *CommandExecutor
	middleware     []Middleware
	stats          *CommandStats
	responseSender ResponseSender
	eventBus       events.EventBus
	mu             sync.RWMutex

	utils.Dispose
}

// NewCommandService 创建新的命令服务
func NewCommandService(parentCtx context.Context) CommandService {
	registry := NewCommandRegistry(parentCtx)
	executor := NewCommandExecutor(registry, parentCtx)

	service := &CommandServiceImpl{
		registry:   registry,
		executor:   executor,
		middleware: make([]Middleware, 0),
		stats:      &CommandStats{},
	}

	// 设置Dispose上下文和清理回调
	service.SetCtx(parentCtx, service.onClose)

	return service
}

// SetEventBus 设置事件总线
func (cs *CommandServiceImpl) SetEventBus(eventBus events.EventBus) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.eventBus = eventBus
	utils.Infof("Event bus set for command service")
	return nil
}

// Start 启动命令服务
func (cs *CommandServiceImpl) Start() error {
	cs.mu.RLock()
	eventBus := cs.eventBus
	cs.mu.RUnlock()

	if eventBus == nil {
		return fmt.Errorf("event bus not set")
	}

	// 订阅命令接收事件
	if err := eventBus.Subscribe("CommandReceived", cs.handleCommandEvent); err != nil {
		return fmt.Errorf("failed to subscribe to CommandReceived events: %w", err)
	}

	utils.Infof("Command service started, listening for events")
	return nil
}

// handleCommandEvent 处理命令事件
func (cs *CommandServiceImpl) handleCommandEvent(event events.Event) error {
	cmdEvent, ok := event.(*events.CommandReceivedEvent)
	if !ok {
		return fmt.Errorf("invalid event type: expected CommandReceivedEvent")
	}

	utils.Infof("Handling command event for connection: %s, command: %v",
		cmdEvent.ConnectionID, cmdEvent.CommandType)

	// 创建命令上下文
	ctx := &CommandContext{
		ConnectionID:    cmdEvent.ConnectionID,
		CommandType:     cmdEvent.CommandType,
		CommandId:       cmdEvent.CommandId,
		RequestID:       cmdEvent.RequestID,
		SenderID:        cmdEvent.SenderID,
		ReceiverID:      cmdEvent.ReceiverID,
		RequestBody:     cmdEvent.CommandBody,
		Context:         context.Background(),
		IsAuthenticated: false,
		UserID:          "",
		StartTime:       time.Now(),
		EndTime:         time.Time{},
	}

	// 执行命令
	response, err := cs.Execute(ctx)

	// 获取事件总线引用
	cs.mu.RLock()
	eventBus := cs.eventBus
	cs.mu.RUnlock()

	// 特殊处理断开连接命令
	if cmdEvent.CommandType == packet.Disconnect && eventBus != nil {
		// 发布断开连接请求事件
		disconnectEvent := events.NewDisconnectRequestEvent(
			cmdEvent.ConnectionID,
			cmdEvent.RequestID,
			cmdEvent.CommandId,
		)
		if err := eventBus.Publish(disconnectEvent); err != nil {
			utils.Errorf("Failed to publish disconnect request event: %v", err)
		}
	}

	// 发布命令完成事件
	processingTime := time.Since(ctx.StartTime)
	var responseStr, errorStr string
	if response != nil {
		responseStr = response.Data
	}
	if err != nil {
		errorStr = err.Error()
	}

	completedEvent := events.NewCommandCompletedEvent(
		cmdEvent.ConnectionID,
		cmdEvent.CommandId,
		cmdEvent.RequestID,
		err == nil,
		responseStr,
		errorStr,
		processingTime,
	)

	if eventBus != nil {
		if err := eventBus.Publish(completedEvent); err != nil {
			utils.Errorf("Failed to publish command completed event: %v", err)
		}
	}

	return nil
}

// Execute 执行命令
func (cs *CommandServiceImpl) Execute(ctx *CommandContext) (*CommandResponse, error) {
	cs.mu.RLock()
	if cs.IsClosed() {
		cs.mu.RUnlock()
		return nil, fmt.Errorf("command service is closed")
	}
	cs.mu.RUnlock()

	// 更新统计信息
	cs.stats.IncrementTotal()
	cs.stats.IncrementActive()
	defer cs.stats.DecrementActive()

	startTime := time.Now()
	defer func() {
		latency := time.Since(startTime)
		cs.stats.UpdateLatency(latency)
	}()

	// 创建带中间件的执行上下文
	pipeline := cs.buildPipeline(ctx)

	// 执行命令
	response, err := pipeline.Execute(ctx)

	// 更新统计信息
	if err != nil {
		cs.stats.IncrementFailed()
		utils.Errorf("Command execution failed: %v", err)
	} else {
		cs.stats.IncrementSuccess()
		utils.Debugf("Command executed successfully: %v", ctx.CommandType)
	}

	// 发送响应（如果需要）
	if response != nil && cs.responseSender != nil {
		if err := cs.responseSender.SendResponse(ctx.ConnectionID, response); err != nil {
			utils.Errorf("Failed to send response: %v", err)
		}
	}

	return response, err
}

// ExecuteAsync 异步执行命令
func (cs *CommandServiceImpl) ExecuteAsync(ctx *CommandContext) (<-chan *CommandResponse, <-chan error) {
	responseChan := make(chan *CommandResponse, 1)
	errorChan := make(chan error, 1)

	go func() {
		response, err := cs.Execute(ctx)
		if err != nil {
			errorChan <- err
		} else {
			responseChan <- response
		}
		close(responseChan)
		close(errorChan)
	}()

	return responseChan, errorChan
}

// Use 注册中间件
func (cs *CommandServiceImpl) Use(middleware Middleware) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.middleware = append(cs.middleware, middleware)
	cs.executor.AddMiddleware(middleware)

	utils.Infof("Registered middleware: %T", middleware)
}

// RegisterHandler 注册命令处理器
func (cs *CommandServiceImpl) RegisterHandler(handler CommandHandler) error {
	return cs.registry.Register(handler)
}

// UnregisterHandler 注销命令处理器
func (cs *CommandServiceImpl) UnregisterHandler(commandType packet.CommandType) error {
	return cs.registry.Unregister(commandType)
}

// GetStats 获取统计信息
func (cs *CommandServiceImpl) GetStats() *CommandStats {
	return cs.stats
}

// SetResponseSender 设置响应发送器
func (cs *CommandServiceImpl) SetResponseSender(sender ResponseSender) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.responseSender = sender
	utils.Infof("Response sender set")
}

// onClose 资源清理回调
func (cs *CommandServiceImpl) onClose() error {
	utils.Infof("Cleaning up command service resources...")

	// 取消事件订阅
	cs.mu.RLock()
	eventBus := cs.eventBus
	cs.mu.RUnlock()

	if eventBus != nil {
		// 取消订阅命令接收事件
		if err := eventBus.Unsubscribe("CommandReceived", cs.handleCommandEvent); err != nil {
			utils.Warnf("Failed to unsubscribe from CommandReceived events: %v", err)
		}
		utils.Infof("Unsubscribed from command events")
	}

	// 清理响应发送器
	cs.mu.Lock()
	cs.responseSender = nil
	cs.eventBus = nil
	cs.mu.Unlock()

	utils.Infof("Command service resources cleanup completed")
	return nil
}

// Close 关闭服务
func (cs *CommandServiceImpl) Close() error {
	return cs.Dispose.CloseWithError()
}

// buildPipeline 构建命令处理管道
func (cs *CommandServiceImpl) buildPipeline(ctx *CommandContext) *CommandPipeline {
	cs.mu.RLock()
	middleware := make([]Middleware, len(cs.middleware))
	copy(middleware, cs.middleware)
	cs.mu.RUnlock()

	// 获取命令处理器
	handler, exists := cs.registry.GetHandler(ctx.CommandType)
	if !exists {
		// 使用默认处理器
		handler, _ = cs.registry.GetHandler(0) // 默认处理器
	}

	return NewCommandPipeline(middleware, handler)
}
