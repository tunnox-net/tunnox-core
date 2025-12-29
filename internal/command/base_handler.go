package command

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
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
	direction         CommandDirection // 替换 responseType
	communicationMode CommunicationMode
	streamProcessor   stream.PackageStreamer
	session           types.Session
}

// NewBaseCommandHandler 创建基础命令处理器
func NewBaseCommandHandler[TRequest any, TResponse any](
	commandType packet.CommandType,
	direction CommandDirection, // 替换 responseType 参数
	communicationMode CommunicationMode,
) *BaseCommandHandler[TRequest, TResponse] {
	return &BaseCommandHandler[TRequest, TResponse]{
		commandType:       commandType,
		direction:         direction, // 替换 responseType
		communicationMode: communicationMode,
	}
}

// SetStreamProcessor 设置流处理器
func (b *BaseCommandHandler[TRequest, TResponse]) SetStreamProcessor(processor stream.PackageStreamer) {
	b.streamProcessor = processor
}

// SetSession 设置会话
func (b *BaseCommandHandler[TRequest, TResponse]) SetSession(session types.Session) {
	b.session = session
}

// GetCommandType 获取命令类型
func (b *BaseCommandHandler[TRequest, TResponse]) GetCommandType() packet.CommandType {
	return b.commandType
}

// GetDirection 获取命令流向（同时表示响应类型）
func (b *BaseCommandHandler[TRequest, TResponse]) GetDirection() CommandDirection {
	return b.direction
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
	return nil
}

// PreProcess 预处理（子类可以重写）
func (b *BaseCommandHandler[TRequest, TResponse]) PreProcess(ctx *CommandContext, request *TRequest) error {
	return nil
}

// PostProcess 后处理（子类可以重写）
func (b *BaseCommandHandler[TRequest, TResponse]) PostProcess(ctx *CommandContext, response *TResponse) error {
	return nil
}

// ProcessRequest 处理请求（子类必须实现）
func (b *BaseCommandHandler[TRequest, TResponse]) ProcessRequest(ctx *CommandContext, request *TRequest) (*TResponse, error) {
	return nil, fmt.Errorf("ProcessRequest not implemented")
}

// GetStreamProcessor 获取流处理器
func (b *BaseCommandHandler[TRequest, TResponse]) GetStreamProcessor() stream.PackageStreamer {
	return b.streamProcessor
}

// GetSession 获取会话
func (b *BaseCommandHandler[TRequest, TResponse]) GetSession() types.Session {
	return b.session
}

// LogRequest 记录请求日志
func (b *BaseCommandHandler[TRequest, TResponse]) LogRequest(ctx *CommandContext, request *TRequest) {
	corelog.Debugf("Processing request for command type: %v, connection: %s", b.commandType, ctx.ConnectionID)
}

// LogResponse 记录响应日志
func (b *BaseCommandHandler[TRequest, TResponse]) LogResponse(ctx *CommandContext, response *TResponse, err error) {
	if err != nil {
		corelog.Errorf("Command handler failed for type: %v, connection: %s, error: %v", b.commandType, ctx.ConnectionID, err)
	} else {
		corelog.Debugf("Command handler succeeded for type: %v, connection: %s", b.commandType, ctx.ConnectionID)
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
// 优先从 session 获取 context，如果 session 未设置则返回 context.Background()
// 注意：调用方应确保在使用 handler 前通过 SetSession 设置 session
func (b *BaseCommandHandler[TRequest, TResponse]) GetContext() context.Context {
	if b.session != nil {
		return b.session.(interface{ Ctx() context.Context }).Ctx()
	}
	// session 未设置时使用 context.Background() 作为后备
	// 这种情况通常只在测试或简单场景中出现
	return context.Background()
}

// ValidateContext 验证上下文
func (b *BaseCommandHandler[TRequest, TResponse]) ValidateContext(ctx *CommandContext) error {
	if ctx == nil {
		return fmt.Errorf("command context is nil")
	}

	if ctx.ConnectionID == "" {
		return fmt.Errorf("connection ID is empty")
	}

	if ctx.CommandType == 0 {
		return fmt.Errorf("command type is invalid")
	}

	return nil
}

// GetCategory 获取命令分类
func (b *BaseCommandHandler[TRequest, TResponse]) GetCategory() CommandCategory {
	// 根据命令类型推断分类，子类可以重写此方法
	switch b.commandType {
	case packet.Connect, packet.Disconnect, packet.Reconnect, packet.HeartbeatCmd:
		return CategoryConnection
	case packet.TcpMapCreate, packet.TcpMapDelete, packet.TcpMapUpdate, packet.TcpMapList, packet.TcpMapStatus,
		packet.HttpMapCreate, packet.HttpMapDelete, packet.HttpMapUpdate, packet.HttpMapList, packet.HttpMapStatus,
		packet.SocksMapCreate, packet.SocksMapDelete, packet.SocksMapUpdate, packet.SocksMapList, packet.SocksMapStatus:
		return CategoryMapping
	case packet.DataTransferStart, packet.DataTransferStop, packet.DataTransferStatus, packet.ProxyForward:
		return CategoryTransport
	case packet.ConfigGet, packet.ConfigSet, packet.StatsGet, packet.LogGet, packet.HealthCheck:
		return CategoryManagement
	case packet.RpcInvoke, packet.RpcRegister, packet.RpcUnregister, packet.RpcList:
		return CategoryRPC
	default:
		return CategoryManagement
	}
}

// GetRequestType 获取请求类型
func (b *BaseCommandHandler[TRequest, TResponse]) GetRequestType() reflect.Type {
	var zero TRequest
	// 检查是否为零值类型（如interface{}）
	if reflect.TypeOf(zero) == reflect.TypeOf((*interface{})(nil)).Elem() {
		return nil // 返回nil表示无请求体
	}
	return reflect.TypeOf(zero)
}

// GetResponseType 获取响应类型
func (b *BaseCommandHandler[TRequest, TResponse]) GetResponseType() reflect.Type {
	var zero TResponse
	// 检查是否为零值类型（如interface{}）
	if reflect.TypeOf(zero) == reflect.TypeOf((*interface{})(nil)).Elem() {
		return nil // 返回nil表示无响应体
	}
	return reflect.TypeOf(zero)
}
