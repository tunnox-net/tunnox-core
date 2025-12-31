package command

import (
	"context"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// CommandPipeline 命令处理管道
type CommandPipeline struct {
	middleware []Middleware
	handler    CommandHandler
}

// NewCommandPipeline 创建新的命令管道
func NewCommandPipeline(middleware []Middleware, handler CommandHandler) *CommandPipeline {
	return &CommandPipeline{
		middleware: middleware,
		handler:    handler,
	}
}

// Execute 执行命令管道
func (cp *CommandPipeline) Execute(ctx *CommandContext) (*CommandResponse, error) {
	// 构建中间件链
	var next func(*CommandContext) (*CommandResponse, error)
	next = func(ctx *CommandContext) (*CommandResponse, error) {
		return cp.handler.Handle(ctx)
	}

	// 从后往前包装中间件
	for i := len(cp.middleware) - 1; i >= 0; i-- {
		currentMiddleware := cp.middleware[i]
		currentNext := next
		next = func(ctx *CommandContext) (*CommandResponse, error) {
			return currentMiddleware.Process(ctx, currentNext)
		}
	}

	// 执行管道
	return next(ctx)
}

// ExecuteWithTimeout 带超时的命令执行
func (cp *CommandPipeline) ExecuteWithTimeout(ctx *CommandContext, timeout time.Duration) (*CommandResponse, error) {
	// 创建带超时的上下文
	execCtx, cancel := context.WithTimeout(ctx.Context, timeout)
	defer cancel()

	ctx.Context = execCtx

	// 创建结果通道
	resultChan := make(chan *CommandResponse, 1)
	errorChan := make(chan error, 1)

	// 异步执行
	go func() {
		response, err := cp.Execute(ctx)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- response
		}
	}()

	// 等待结果或超时
	select {
	case response := <-resultChan:
		return response, nil
	case err := <-errorChan:
		return nil, err
	case <-execCtx.Done():
		return nil, coreerrors.New(coreerrors.CodeTimeout, "command execution timeout")
	}
}

// AddMiddleware 添加中间件到管道
func (cp *CommandPipeline) AddMiddleware(middleware Middleware) {
	cp.middleware = append(cp.middleware, middleware)
	corelog.Debugf("Added middleware to pipeline: %T", middleware)
}

// GetMiddlewareCount 获取中间件数量
func (cp *CommandPipeline) GetMiddlewareCount() int {
	return len(cp.middleware)
}

// GetHandler 获取处理器
func (cp *CommandPipeline) GetHandler() CommandHandler {
	return cp.handler
}
