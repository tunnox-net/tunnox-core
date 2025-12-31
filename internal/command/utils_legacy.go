package command

import (
	"context"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// ==================== 旧版 CommandUtils 保留用于向后兼容 ====================
// 已标记为 Deprecated，推荐使用 TypedCommandUtils

// CommandUtils 命令工具类，支持链式调用
// Deprecated: 请使用 TypedCommandUtils 以获得类型安全
// 此版本仅保留用于不需要请求/响应数据的简单命令场景
type CommandUtils struct {
	commandType  packet.CommandType
	timeout      time.Duration
	requestID    string
	commandId    string // 客户端生成的唯一命令ID
	connectionID string
	session      types.Session
	ctx          context.Context
	errorHandler func(error) error
	// 替换 metadata 为具体的类型化字段
	isAuthenticated bool
	userID          string
	startTime       time.Time
	endTime         time.Time
}

// NewCommandUtils 创建新的命令工具实例
func NewCommandUtils(session types.Session) *CommandUtils {
	return &CommandUtils{
		session:      session,
		timeout:      30 * time.Second,
		errorHandler: defaultErrorHandler,
	}
}

// WithCommand 设置命令类型
func (cu *CommandUtils) WithCommand(commandType packet.CommandType) *CommandUtils {
	cu.commandType = commandType
	return cu
}

// PutRequest 已废弃，请使用 TypedCommandUtils
// Deprecated: 请使用 TypedCommandUtils.PutRequest 以获得类型安全
// 此方法已移除，如需使用请求数据，请迁移到 TypedCommandUtils

// ResultAs 已废弃，请使用 TypedCommandUtils
// Deprecated: 请使用 TypedCommandUtils.ResultAs 以获得类型安全
// 此方法已移除，如需使用响应数据，请迁移到 TypedCommandUtils

// Timeout 设置超时时间
func (cu *CommandUtils) Timeout(timeout time.Duration) *CommandUtils {
	cu.timeout = timeout
	return cu
}

// WithAuthentication 设置认证状态
func (cu *CommandUtils) WithAuthentication(isAuthenticated bool) *CommandUtils {
	cu.isAuthenticated = isAuthenticated
	return cu
}

// WithUserID 设置用户ID
func (cu *CommandUtils) WithUserID(userID string) *CommandUtils {
	cu.userID = userID
	return cu
}

// WithStartTime 设置开始时间
func (cu *CommandUtils) WithStartTime(startTime time.Time) *CommandUtils {
	cu.startTime = startTime
	return cu
}

// WithEndTime 设置结束时间
func (cu *CommandUtils) WithEndTime(endTime time.Time) *CommandUtils {
	cu.endTime = endTime
	return cu
}

// WithRequestID 设置请求ID
func (cu *CommandUtils) WithRequestID(requestID string) *CommandUtils {
	cu.requestID = requestID
	return cu
}

// WithCommandId 设置命令ID
func (cu *CommandUtils) WithCommandId(commandId string) *CommandUtils {
	cu.commandId = commandId
	return cu
}

// generateCommandId 生成唯一的命令ID
func (cu *CommandUtils) generateCommandId() string {
	return fmt.Sprintf("cmd_%d_%s", time.Now().UnixNano(), cu.connectionID)
}

// WithConnectionID 设置连接ID
func (cu *CommandUtils) WithConnectionID(connectionID string) *CommandUtils {
	cu.connectionID = connectionID
	return cu
}

// WithContext 设置上下文
func (cu *CommandUtils) WithContext(ctx context.Context) *CommandUtils {
	cu.ctx = ctx
	return cu
}

// ThrowOn 设置错误处理函数
func (cu *CommandUtils) ThrowOn(errorHandler func(error) error) *CommandUtils {
	cu.errorHandler = errorHandler
	return cu
}

// Execute 执行命令（简化版，不支持请求/响应数据）
// 如需使用请求/响应数据，请使用 TypedCommandUtils
func (cu *CommandUtils) Execute() (*CommandResponse, error) {
	// 验证必要参数
	if cu.session == nil {
		return nil, coreerrors.New(coreerrors.CodeInvalidParam, "session is required")
	}
	if cu.commandType == 0 {
		return nil, coreerrors.New(coreerrors.CodeInvalidParam, "command type is required")
	}
	if cu.connectionID == "" {
		return nil, coreerrors.New(coreerrors.CodeInvalidParam, "connection ID is required")
	}

	// 设置默认上下文
	// 注意：推荐调用方使用 WithContext 传入正确的 parent context
	// 此处使用 context.Background() 仅作为后备，用于简单场景或测试
	if cu.ctx == nil {
		cu.ctx = context.Background()
	}

	// 生成CommandId（如果未设置）
	if cu.commandId == "" {
		cu.commandId = cu.generateCommandId()
	}

	// 设置开始时间（如果未设置）
	if cu.startTime.IsZero() {
		cu.startTime = time.Now()
	}

	// 创建命令包（无请求数据）
	commandPacket := &packet.CommandPacket{
		CommandType: cu.commandType,
		CommandId:   cu.commandId,
		Token:       cu.requestID,
		SenderId:    cu.connectionID,
		ReceiverId:  "", // 由服务端处理
		CommandBody: "",
	}

	// 创建传输包
	transferPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: commandPacket,
	}

	// 获取连接信息
	connInfo, exists := cu.session.GetStreamManager().GetStream(cu.connectionID)
	if !exists {
		return nil, cu.errorHandler(coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", cu.connectionID))
	}

	// 记录日志
	corelog.Debugf("Executing command: %v", cu.commandType)

	// 发送命令包
	_, err := connInfo.WritePacket(transferPacket, false, 0)
	if err != nil {
		return nil, cu.errorHandler(coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send command"))
	}

	// 设置结束时间
	cu.endTime = time.Now()

	return &CommandResponse{
		Success:        true,
		Data:           "",
		ProcessingTime: cu.endTime.Sub(cu.startTime),
		HandlerName:    "command_utils",
	}, nil
}

// ExecuteAsync 异步执行命令
func (cu *CommandUtils) ExecuteAsync() (<-chan *CommandResponse, <-chan error) {
	responseChan := make(chan *CommandResponse, 1)
	errorChan := make(chan error, 1)

	go func() {
		response, err := cu.Execute()
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

// waitForResponse 已废弃
// Deprecated: 请使用 TypedCommandUtils 以获得类型安全的响应等待功能
// 此方法已移除，如需等待响应，请迁移到 TypedCommandUtils

// defaultErrorHandler 默认错误处理函数
func defaultErrorHandler(err error) error {
	corelog.Errorf("Command execution error: %v", err)
	return err
}
