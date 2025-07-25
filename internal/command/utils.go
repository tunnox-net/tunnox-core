package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"
)

// CommandUtils 命令工具类，支持链式调用
type CommandUtils struct {
	commandType  packet.CommandType
	requestData  interface{}
	responseData interface{}
	timeout      time.Duration
	requestID    string
	commandId    string // 客户端生成的唯一命令ID
	connectionID string
	session      protocol.Session
	ctx          context.Context
	errorHandler func(error) error
	// 替换 metadata 为具体的类型化字段
	isAuthenticated bool
	userID          string
	startTime       time.Time
	endTime         time.Time
}

// NewCommandUtils 创建新的命令工具实例
func NewCommandUtils(session protocol.Session) *CommandUtils {
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

// PutRequest 设置请求数据
func (cu *CommandUtils) PutRequest(data interface{}) *CommandUtils {
	cu.requestData = data
	return cu
}

// ResultAs 设置响应数据结构
func (cu *CommandUtils) ResultAs(responseData interface{}) *CommandUtils {
	cu.responseData = responseData
	return cu
}

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

// Execute 执行命令
func (cu *CommandUtils) Execute() (*CommandResponse, error) {
	// 验证必要参数
	if cu.session == nil {
		return nil, fmt.Errorf("session is required")
	}
	if cu.commandType == 0 {
		return nil, fmt.Errorf("command type is required")
	}
	if cu.connectionID == "" {
		return nil, fmt.Errorf("connection ID is required")
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
			return nil, cu.errorHandler(fmt.Errorf("failed to marshal request data: %w", err))
		}
		commandBody = string(data)
	}

	// 创建命令包
	commandPacket := &packet.CommandPacket{
		CommandType: cu.commandType,
		CommandId:   cu.commandId,
		Token:       cu.requestID,
		SenderId:    cu.connectionID,
		ReceiverId:  "", // 由服务端处理
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
		return nil, cu.errorHandler(fmt.Errorf("connection not found: %s", cu.connectionID))
	}

	// 记录日志
	utils.Debugf("Executing command: %v, request: %+v", cu.commandType, cu.requestData)

	// 发送命令包
	_, err := connInfo.WritePacket(transferPacket, false, 0)
	if err != nil {
		return nil, cu.errorHandler(fmt.Errorf("failed to send command: %w", err))
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

// waitForResponse 等待响应
func (cu *CommandUtils) waitForResponse() (*CommandResponse, error) {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(cu.ctx, cu.timeout)
	defer cancel()

	// 获取连接信息
	connInfo, exists := cu.session.GetStreamManager().GetStream(cu.connectionID)
	if !exists {
		return nil, cu.errorHandler(fmt.Errorf("connection not found: %s", cu.connectionID))
	}

	// 等待响应
	for {
		select {
		case <-ctx.Done():
			return nil, cu.errorHandler(fmt.Errorf("command timeout after %v", cu.timeout))

		default:
			// 读取响应包
			responsePacket, _, err := connInfo.ReadPacket()
			if err != nil {
				return nil, cu.errorHandler(fmt.Errorf("failed to read response: %w", err))
			}

			// 检查是否是命令响应
			if responsePacket.PacketType.IsJsonCommand() && responsePacket.CommandPacket != nil {
				// 检查请求ID是否匹配
				if responsePacket.CommandPacket.Token == cu.requestID {
					// 检查CommandId是否匹配（如果响应中包含CommandId）
					if responsePacket.CommandPacket.CommandId != "" && responsePacket.CommandPacket.CommandId != cu.commandId {
						utils.Warnf("CommandId mismatch: expected %s, got %s", cu.commandId, responsePacket.CommandPacket.CommandId)
						continue // 继续等待正确的响应
					}

					// 解析响应数据
					var response CommandResponse
					if err := json.Unmarshal([]byte(responsePacket.CommandPacket.CommandBody), &response); err != nil {
						return nil, cu.errorHandler(fmt.Errorf("failed to unmarshal response: %w", err))
					}

					// 如果指定了响应数据结构，尝试解析
					if cu.responseData != nil && response.Data != "" {
						if err := json.Unmarshal([]byte(response.Data), cu.responseData); err != nil {
							return nil, cu.errorHandler(fmt.Errorf("failed to unmarshal response data: %w", err))
						}
					}

					// 记录日志
					utils.Debugf("Command executed successfully: %v", cu.commandType)

					return &response, nil
				}
			}

			// 短暂休眠，避免CPU占用过高
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// defaultErrorHandler 默认错误处理函数
func defaultErrorHandler(err error) error {
	utils.Errorf("Command execution error: %v", err)
	return err
}

// 便捷方法：创建TCP映射命令
func (cu *CommandUtils) TcpMap() *CommandUtils {
	return cu.WithCommand(packet.TcpMap)
}

// 便捷方法：创建HTTP映射命令
func (cu *CommandUtils) HttpMap() *CommandUtils {
	return cu.WithCommand(packet.HttpMap)
}

// 便捷方法：创建SOCKS映射命令
func (cu *CommandUtils) SocksMap() *CommandUtils {
	return cu.WithCommand(packet.SocksMap)
}

// 便捷方法：创建数据输入命令
func (cu *CommandUtils) DataIn() *CommandUtils {
	return cu.WithCommand(packet.DataIn)
}

// 便捷方法：创建转发命令
func (cu *CommandUtils) Forward() *CommandUtils {
	return cu.WithCommand(packet.Forward)
}

// 便捷方法：创建数据输出命令
func (cu *CommandUtils) DataOut() *CommandUtils {
	return cu.WithCommand(packet.DataOut)
}

// 便捷方法：创建断开连接命令
func (cu *CommandUtils) Disconnect() *CommandUtils {
	return cu.WithCommand(packet.Disconnect)
}

func (cu *CommandUtils) RpcInvoke() *CommandUtils {
	return cu.WithCommand(packet.RpcInvoke)
}
