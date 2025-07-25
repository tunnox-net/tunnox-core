package command

import (
	"context"
	"fmt"
	"sync"
	"time"
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
	// Execute 执行命令
	Execute(ctx *CommandContext) (*CommandResponse, error)

	// ExecuteAsync 异步执行命令
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
	mu             sync.RWMutex

	utils.Dispose
}

// NewCommandService 创建新的命令服务
func NewCommandService(parentCtx context.Context) CommandService {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

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
}

// onClose 资源清理回调
func (cs *CommandServiceImpl) onClose() error {
	utils.Infof("Cleaning up command service resources...")

	// 清理统计信息
	cs.stats = &CommandStats{}

	// 清理中间件
	cs.mu.Lock()
	cs.middleware = make([]Middleware, 0)
	cs.responseSender = nil
	cs.mu.Unlock()

	utils.Infof("Command service resources cleanup completed")
	return nil
}

// Close 关闭服务
func (cs *CommandServiceImpl) Close() error {
	result := cs.Dispose.Close()
	if result.HasErrors() {
		return fmt.Errorf("command service cleanup failed: %s", result.Error())
	}
	return nil
}

// buildPipeline 构建命令处理管道
func (cs *CommandServiceImpl) buildPipeline(ctx *CommandContext) *CommandPipeline {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// 获取命令处理器
	handler, exists := cs.registry.GetHandler(ctx.CommandType)
	if !exists {
		// 使用默认处理器
		handler = NewDefaultHandler()
	}

	// 创建管道
	return NewCommandPipeline(cs.middleware, handler)
}
