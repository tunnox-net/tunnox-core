package command

import (
	"context"
	"encoding/json"
	"fmt"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// CommunicationMode 通信模式
type CommunicationMode int

const (
	Simplex    CommunicationMode = iota // 单工模式
	DuplexMode                          // 双工模式
)

// BaseCommandHandler 基础命令处理器，提供通用的辅助方法和类型安全
type BaseCommandHandler[TRequest any, TResponse any] struct {
	commandType       packet.CommandType
	responseType      ResponseType
	communicationMode CommunicationMode
	streamProcessor   stream.PackageStreamer
	session           protocol.Session
}

// NewBaseCommandHandler 创建基础命令处理器
func NewBaseCommandHandler[TRequest any, TResponse any](
	commandType packet.CommandType,
	responseType ResponseType,
	communicationMode CommunicationMode,
) *BaseCommandHandler[TRequest, TResponse] {
	return &BaseCommandHandler[TRequest, TResponse]{
		commandType:       commandType,
		responseType:      responseType,
		communicationMode: communicationMode,
	}
}

// SetStreamProcessor 设置流处理器
func (b *BaseCommandHandler[TRequest, TResponse]) SetStreamProcessor(processor stream.PackageStreamer) {
	b.streamProcessor = processor
}

// SetSession 设置会话
func (b *BaseCommandHandler[TRequest, TResponse]) SetSession(session protocol.Session) {
	b.session = session
}

// GetCommandType 获取命令类型
func (b *BaseCommandHandler[TRequest, TResponse]) GetCommandType() packet.CommandType {
	return b.commandType
}

// GetResponseType 获取响应类型
func (b *BaseCommandHandler[TRequest, TResponse]) GetResponseType() ResponseType {
	return b.responseType
}

// GetCommunicationMode 获取通信模式
func (b *BaseCommandHandler[TRequest, TResponse]) GetCommunicationMode() CommunicationMode {
	return b.communicationMode
}

// ParseRequest 解析请求体为泛型类型
func (b *BaseCommandHandler[TRequest, TResponse]) ParseRequest(ctx *CommandContext) (*TRequest, error) {
	if ctx.RequestBody == "" {
		return nil, fmt.Errorf("request body is empty")
	}

	var request TRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &request); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	return &request, nil
}

// CreateResponse 创建响应
func (b *BaseCommandHandler[TRequest, TResponse]) CreateResponse(
	success bool,
	data *TResponse,
	err error,
	requestID string,
) *CommandResponse {
	response := &CommandResponse{
		Success:   success,
		RequestID: requestID,
	}

	if err != nil {
		response.Error = err.Error()
	}

	if data != nil {
		// 将 data 序列化为 JSON 字符串
		if jsonData, jsonErr := json.Marshal(data); jsonErr == nil {
			response.Data = string(jsonData)
		} else {
			response.Error = fmt.Sprintf("failed to marshal response data: %v", jsonErr)
		}
	}

	return response
}

// CreateSuccessResponse 创建成功响应
func (b *BaseCommandHandler[TRequest, TResponse]) CreateSuccessResponse(
	data *TResponse,
	requestID string,
) *CommandResponse {
	return b.CreateResponse(true, data, nil, requestID)
}

// CreateErrorResponse 创建错误响应
func (b *BaseCommandHandler[TRequest, TResponse]) CreateErrorResponse(
	err error,
	requestID string,
) *CommandResponse {
	return b.CreateResponse(false, nil, err, requestID)
}

// ValidateRequest 验证请求（子类可以重写）
func (b *BaseCommandHandler[TRequest, TResponse]) ValidateRequest(request *TRequest) error {
	// 默认实现：无验证
	return nil
}

// PreProcess 预处理（子类可以重写）
func (b *BaseCommandHandler[TRequest, TResponse]) PreProcess(ctx *CommandContext, request *TRequest) error {
	// 默认实现：无预处理
	return nil
}

// PostProcess 后处理（子类可以重写）
func (b *BaseCommandHandler[TRequest, TResponse]) PostProcess(ctx *CommandContext, response *TResponse) error {
	// 默认实现：无后处理
	return nil
}

// ProcessRequest 处理请求的核心逻辑（子类必须实现）
func (b *BaseCommandHandler[TRequest, TResponse]) ProcessRequest(ctx *CommandContext, request *TRequest) (*TResponse, error) {
	// 子类必须实现这个方法
	panic("ProcessRequest must be implemented by subclass")
}

// GetStreamProcessor 获取流处理器
func (b *BaseCommandHandler[TRequest, TResponse]) GetStreamProcessor() stream.PackageStreamer {
	return b.streamProcessor
}

// GetSession 获取会话
func (b *BaseCommandHandler[TRequest, TResponse]) GetSession() protocol.Session {
	return b.session
}

// LogRequest 记录请求日志
func (b *BaseCommandHandler[TRequest, TResponse]) LogRequest(ctx *CommandContext, request *TRequest) {
	utils.Debugf("Processing %v request from %s to %s",
		b.commandType, ctx.SenderID, ctx.ReceiverID)
}

// LogResponse 记录响应日志
func (b *BaseCommandHandler[TRequest, TResponse]) LogResponse(ctx *CommandContext, response *TResponse, err error) {
	if err != nil {
		utils.Errorf("Failed to process %v request: %v", b.commandType, err)
	} else {
		utils.Debugf("Successfully processed %v request", b.commandType)
	}
}

// IsSimplex 是否为单工模式
func (b *BaseCommandHandler[TRequest, TResponse]) IsSimplex() bool {
	return b.communicationMode == Simplex
}

// IsDuplex 是否为双工模式
func (b *BaseCommandHandler[TRequest, TResponse]) IsDuplex() bool {
	return b.communicationMode == DuplexMode
}

// GetContext 获取上下文
func (b *BaseCommandHandler[TRequest, TResponse]) GetContext() context.Context {
	if b.session != nil {
		return b.session.(interface{ Ctx() context.Context }).Ctx()
	}
	return context.Background()
}

// ValidateContext 验证上下文
func (b *BaseCommandHandler[TRequest, TResponse]) ValidateContext(ctx *CommandContext) error {
	if ctx == nil {
		return fmt.Errorf("command context is nil")
	}
	if ctx.ConnectionID == "" {
		return fmt.Errorf("connection ID is required")
	}
	if ctx.RequestID == "" {
		return fmt.Errorf("request ID is required")
	}
	return nil
}
