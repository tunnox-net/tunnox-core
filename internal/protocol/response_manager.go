package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"tunnox-core/internal/command"
	"tunnox-core/internal/common"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ResponseManager 响应管理器
type ResponseManager struct {
	session common.Session
	mu      sync.RWMutex

	utils.Dispose
}

// NewResponseManager 创建新的响应管理器
func NewResponseManager(session common.Session, parentCtx context.Context) *ResponseManager {
	manager := &ResponseManager{
		session: session,
	}

	// 设置Dispose上下文和清理回调
	manager.SetCtx(parentCtx, manager.onClose)

	return manager
}

// SendResponse 发送响应
func (rm *ResponseManager) SendResponse(connID string, response *command.CommandResponse) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.session == nil {
		return fmt.Errorf("session is nil")
	}

	// 序列化响应
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// 创建响应包
	responsePacket := &packet.CommandPacket{
		CommandType: 0,                  // 响应包使用特殊类型
		CommandId:   response.CommandId, // 包含对应的命令ID
		Token:       response.RequestID,
		SenderId:    "", // 服务端发送
		ReceiverId:  connID,
		CommandBody: string(responseData),
	}

	// 创建传输包
	transferPacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: responsePacket,
	}

	// 获取连接信息
	conn, exists := rm.session.GetConnection(connID)
	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	// 通过流处理器发送响应
	if conn.Stream != nil {
		_, err := conn.Stream.WritePacket(transferPacket, false, 0)
		if err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}

		utils.Debugf("Sent response to connection %s: success=%v", connID, response.Success)
		return nil
	}

	return fmt.Errorf("connection %s has no stream", connID)
}

// SendErrorResponse 发送错误响应
func (rm *ResponseManager) SendErrorResponse(connID, commandId, requestID, errorMsg string) error {
	response := &command.CommandResponse{
		Success:   false,
		Error:     errorMsg,
		CommandId: commandId,
		RequestID: requestID,
	}

	return rm.SendResponse(connID, response)
}

// SendSuccessResponse 发送成功响应
func (rm *ResponseManager) SendSuccessResponse(connID, commandId, requestID string, data interface{}) error {
	var responseData string
	if data != nil {
		if jsonData, err := json.Marshal(data); err == nil {
			responseData = string(jsonData)
		}
	}

	response := &command.CommandResponse{
		Success:   true,
		Data:      responseData,
		CommandId: commandId,
		RequestID: requestID,
	}

	return rm.SendResponse(connID, response)
}

// SetSession 设置会话
func (rm *ResponseManager) SetSession(session common.Session) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.session = session
}

// onClose 资源清理回调
func (rm *ResponseManager) onClose() error {
	utils.Infof("Cleaning up response manager resources...")

	rm.mu.Lock()
	rm.session = nil
	rm.mu.Unlock()

	utils.Infof("Response manager resources cleanup completed")
	return nil
}

// GetSession 获取会话
func (rm *ResponseManager) GetSession() common.Session {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.session
}
