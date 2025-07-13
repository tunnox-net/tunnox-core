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
	metadata     map[string]interface{}
	requestID    string
	connectionID string
	session      protocol.Session
	ctx          context.Context
	errorHandler func(error) error
}

// NewCommandUtils 创建新的命令工具实例
func NewCommandUtils(session protocol.Session) *CommandUtils {
	return &CommandUtils{
		session:      session,
		metadata:     make(map[string]interface{}),
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

// WithMetadata 添加元数据
func (cu *CommandUtils) WithMetadata(key string, value interface{}) *CommandUtils {
	cu.metadata[key] = value
	return cu
}

// WithRequestID 设置请求ID
func (cu *CommandUtils) WithRequestID(requestID string) *CommandUtils {
	cu.requestID = requestID
	return cu
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

	return &CommandResponse{
		Success: true,
		Data:    nil,
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
					// 解析响应数据
					var response CommandResponse
					if err := json.Unmarshal([]byte(responsePacket.CommandPacket.CommandBody), &response); err != nil {
						return nil, cu.errorHandler(fmt.Errorf("failed to unmarshal response: %w", err))
					}

					// 如果指定了响应数据结构，尝试解析
					if cu.responseData != nil && response.Data != nil {
						dataBytes, err := json.Marshal(response.Data)
						if err != nil {
							return nil, cu.errorHandler(fmt.Errorf("failed to marshal response data: %w", err))
						}

						if err := json.Unmarshal(dataBytes, cu.responseData); err != nil {
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
