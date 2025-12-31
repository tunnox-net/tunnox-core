package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// TypedRequest 类型安全的请求数据接口约束
// 所有请求类型都应该可以被 JSON 序列化
type TypedRequest interface {
	any
}

// TypedResponse 类型安全的响应数据接口约束
// 所有响应类型都应该可以被 JSON 反序列化
type TypedResponse interface {
	any
}

// TypedCommandUtils 类型安全的命令工具类，使用泛型确保请求和响应的类型安全
// TReq 是请求数据类型，TResp 是响应数据类型
type TypedCommandUtils[TReq TypedRequest, TResp TypedResponse] struct {
	commandType  packet.CommandType
	requestData  *TReq
	responseData *TResp
	timeout      time.Duration
	requestID    string
	commandId    string
	connectionID string
	session      types.Session
	ctx          context.Context
	errorHandler func(error) error
	// 类型化元数据字段
	isAuthenticated bool
	userID          string
	startTime       time.Time
	endTime         time.Time
}

// NewTypedCommandUtils 创建新的类型安全命令工具实例
func NewTypedCommandUtils[TReq TypedRequest, TResp TypedResponse](session types.Session) *TypedCommandUtils[TReq, TResp] {
	return &TypedCommandUtils[TReq, TResp]{
		session:      session,
		timeout:      30 * time.Second,
		errorHandler: defaultErrorHandler,
	}
}

// WithCommand 设置命令类型
func (cu *TypedCommandUtils[TReq, TResp]) WithCommand(commandType packet.CommandType) *TypedCommandUtils[TReq, TResp] {
	cu.commandType = commandType
	return cu
}

// PutRequest 设置请求数据（类型安全）
func (cu *TypedCommandUtils[TReq, TResp]) PutRequest(data *TReq) *TypedCommandUtils[TReq, TResp] {
	cu.requestData = data
	return cu
}

// ResultAs 设置响应数据结构（类型安全）
func (cu *TypedCommandUtils[TReq, TResp]) ResultAs(responseData *TResp) *TypedCommandUtils[TReq, TResp] {
	cu.responseData = responseData
	return cu
}

// Timeout 设置超时时间
func (cu *TypedCommandUtils[TReq, TResp]) Timeout(timeout time.Duration) *TypedCommandUtils[TReq, TResp] {
	cu.timeout = timeout
	return cu
}

// WithAuthentication 设置认证状态
func (cu *TypedCommandUtils[TReq, TResp]) WithAuthentication(isAuthenticated bool) *TypedCommandUtils[TReq, TResp] {
	cu.isAuthenticated = isAuthenticated
	return cu
}

// WithUserID 设置用户ID
func (cu *TypedCommandUtils[TReq, TResp]) WithUserID(userID string) *TypedCommandUtils[TReq, TResp] {
	cu.userID = userID
	return cu
}

// WithStartTime 设置开始时间
func (cu *TypedCommandUtils[TReq, TResp]) WithStartTime(startTime time.Time) *TypedCommandUtils[TReq, TResp] {
	cu.startTime = startTime
	return cu
}

// WithEndTime 设置结束时间
func (cu *TypedCommandUtils[TReq, TResp]) WithEndTime(endTime time.Time) *TypedCommandUtils[TReq, TResp] {
	cu.endTime = endTime
	return cu
}

// WithRequestID 设置请求ID
func (cu *TypedCommandUtils[TReq, TResp]) WithRequestID(requestID string) *TypedCommandUtils[TReq, TResp] {
	cu.requestID = requestID
	return cu
}

// WithCommandId 设置命令ID
func (cu *TypedCommandUtils[TReq, TResp]) WithCommandId(commandId string) *TypedCommandUtils[TReq, TResp] {
	cu.commandId = commandId
	return cu
}

// generateCommandId 生成唯一的命令ID
func (cu *TypedCommandUtils[TReq, TResp]) generateCommandId() string {
	return fmt.Sprintf("cmd_%d_%s", time.Now().UnixNano(), cu.connectionID)
}

// WithConnectionID 设置连接ID
func (cu *TypedCommandUtils[TReq, TResp]) WithConnectionID(connectionID string) *TypedCommandUtils[TReq, TResp] {
	cu.connectionID = connectionID
	return cu
}

// WithContext 设置上下文
func (cu *TypedCommandUtils[TReq, TResp]) WithContext(ctx context.Context) *TypedCommandUtils[TReq, TResp] {
	cu.ctx = ctx
	return cu
}

// ThrowOn 设置错误处理函数
func (cu *TypedCommandUtils[TReq, TResp]) ThrowOn(errorHandler func(error) error) *TypedCommandUtils[TReq, TResp] {
	cu.errorHandler = errorHandler
	return cu
}

// Execute 执行命令
func (cu *TypedCommandUtils[TReq, TResp]) Execute() (*CommandResponse, error) {
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

	// 序列化请求数据
	var commandBody string
	if cu.requestData != nil {
		data, err := json.Marshal(cu.requestData)
		if err != nil {
			return nil, cu.errorHandler(coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal request data"))
		}
		commandBody = string(data)
	}

	// 创建命令包
	commandPacket := &packet.CommandPacket{
		CommandType: cu.commandType,
		CommandId:   cu.commandId,
		Token:       cu.requestID,
		SenderId:    cu.connectionID,
		ReceiverId:  "",
		CommandBody: commandBody,
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
	corelog.Debugf("Executing command: %v, request: %+v", cu.commandType, cu.requestData)

	// 发送命令包
	_, err := connInfo.WritePacket(transferPacket, false, 0)
	if err != nil {
		return nil, cu.errorHandler(coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send command"))
	}

	// 等待响应（如果需要）
	if cu.responseData != nil {
		return cu.waitForResponse()
	}

	// 设置结束时间
	cu.endTime = time.Now()

	return &CommandResponse{
		Success:        true,
		Data:           "",
		ProcessingTime: cu.endTime.Sub(cu.startTime),
		HandlerName:    "typed_command_utils",
	}, nil
}

// ExecuteAsync 异步执行命令
func (cu *TypedCommandUtils[TReq, TResp]) ExecuteAsync() (<-chan *CommandResponse, <-chan error) {
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

// waitForResponse 等待响应
func (cu *TypedCommandUtils[TReq, TResp]) waitForResponse() (*CommandResponse, error) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(cu.ctx, cu.timeout)
	defer cancel()

	// 获取连接信息
	connInfo, exists := cu.session.GetStreamManager().GetStream(cu.connectionID)
	if !exists {
		return nil, cu.errorHandler(coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", cu.connectionID))
	}

	// 等待响应
	for {
		select {
		case <-ctx.Done():
			return nil, cu.errorHandler(coreerrors.Newf(coreerrors.CodeTimeout, "command timeout after %v", cu.timeout))

		default:
			// 读取响应包
			responsePacket, _, err := connInfo.ReadPacket()
			if err != nil {
				return nil, cu.errorHandler(coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read response"))
			}

			// 检查是否是命令响应
			if responsePacket.PacketType.IsJsonCommand() && responsePacket.CommandPacket != nil {
				// 检查请求ID是否匹配
				if responsePacket.CommandPacket.Token == cu.requestID {
					// 检查CommandId是否匹配
					if responsePacket.CommandPacket.CommandId != "" && responsePacket.CommandPacket.CommandId != cu.commandId {
						corelog.Warnf("CommandId mismatch: expected %s, got %s", cu.commandId, responsePacket.CommandPacket.CommandId)
						continue
					}

					// 解析响应数据
					var response CommandResponse
					if err := json.Unmarshal([]byte(responsePacket.CommandPacket.CommandBody), &response); err != nil {
						return nil, cu.errorHandler(coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal response"))
					}

					// 如果指定了响应数据结构，尝试解析到类型安全的结构
					if cu.responseData != nil && response.Data != "" {
						if err := json.Unmarshal([]byte(response.Data), cu.responseData); err != nil {
							return nil, cu.errorHandler(coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal response data"))
						}
					}

					// 记录日志
					corelog.Debugf("Command executed successfully: %v", cu.commandType)

					return &response, nil
				}
			}

			// 短暂休眠，避免CPU占用过高
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// GetResponse 获取解析后的响应数据（类型安全）
func (cu *TypedCommandUtils[TReq, TResp]) GetResponse() *TResp {
	return cu.responseData
}
