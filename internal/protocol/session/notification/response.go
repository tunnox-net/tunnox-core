package notification

import (
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/command"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/events"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// CommandResponseData 命令响应数据结构（强类型）
type CommandResponseData struct {
	Success        bool          `json:"success"`
	CommandID      string        `json:"command_id"`
	RequestID      string        `json:"request_id"`
	Data           string        `json:"data,omitempty"`
	Error          string        `json:"error,omitempty"`
	ProcessingTime time.Duration `json:"processing_time,omitempty"`
}

// ResponseManager 响应管理器
type ResponseManager struct {
	session  types.Session
	eventBus events.EventBus
	dispose.Dispose
}

// NewResponseManager 创建新的响应管理器
func NewResponseManager(session types.Session, parentCtx context.Context) *ResponseManager {
	manager := &ResponseManager{
		session: session,
	}
	manager.SetCtx(parentCtx, manager.onClose)
	return manager
}

// SetEventBus 设置事件总线
func (rm *ResponseManager) SetEventBus(eventBus events.EventBus) error {
	rm.eventBus = eventBus

	// 订阅命令完成事件
	if eventBus != nil {
		if err := eventBus.Subscribe("CommandCompleted", rm.handleCommandCompletedEvent); err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to subscribe to CommandCompleted events")
		}
		corelog.Infof("Response manager subscribed to CommandCompleted events")
	}

	return nil
}

// handleCommandCompletedEvent 处理命令完成事件
func (rm *ResponseManager) handleCommandCompletedEvent(event events.Event) error {
	completedEvent, ok := event.(*events.CommandCompletedEvent)
	if !ok {
		return coreerrors.New(coreerrors.CodeInvalidParam, "invalid event type: expected CommandCompletedEvent")
	}

	corelog.Infof("Handling command completed event for connection: %s, success: %v",
		completedEvent.ConnectionID, completedEvent.Success)

	// 创建命令响应
	response := &command.CommandResponse{
		Success:        completedEvent.Success,
		Data:           completedEvent.Response,
		Error:          completedEvent.Error,
		RequestID:      completedEvent.RequestID,
		CommandId:      completedEvent.CommandId,
		ProcessingTime: completedEvent.ProcessingTime,
	}

	// 发送响应
	return rm.SendResponse(completedEvent.ConnectionID, response)
}

// SendResponse 发送响应
func (rm *ResponseManager) SendResponse(connID string, response *command.CommandResponse) error {
	// 获取连接信息
	conn, exists := rm.session.GetConnection(connID)
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "connection %s not found", connID)
	}

	// 检查连接状态
	if conn.State == types.StateClosed || conn.State == types.StateClosing {
		return coreerrors.Newf(coreerrors.CodeConnectionError, "connection %s is closed or closing", connID)
	}

	corelog.Debugf("Sending response to connection %s: success=%v",
		connID, response.Success)

	// 1. 构造响应数据（使用强类型）
	responseData := &CommandResponseData{
		Success:        response.Success,
		CommandID:      response.CommandId,
		RequestID:      response.RequestID,
		Data:           response.Data,
		Error:          response.Error,
		ProcessingTime: response.ProcessingTime,
	}

	// 2. 序列化响应
	dataBytes, err := json.Marshal(responseData)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal response")
	}

	// 3. 构造 CommandPacket
	cmdPacket := &packet.CommandPacket{
		CommandType: packet.Disconnect, // 临时使用一个 CommandType，后续可以定义专门的响应类型
		CommandId:   response.CommandId,
		Token:       "",
		SenderId:    "server",
		ReceiverId:  connID,
		CommandBody: string(dataBytes),
	}

	// 4. 构造 TransferPacket
	transferPacket := &packet.TransferPacket{
		PacketType:    packet.CommandResp,
		CommandPacket: cmdPacket,
	}

	// 5. 通过连接的 Stream 发送数据包
	if conn.Stream == nil {
		return coreerrors.Newf(coreerrors.CodeConnectionError, "connection %s has no stream", connID)
	}

	if _, err := conn.Stream.WritePacket(transferPacket, true, 0); err != nil {
		corelog.Errorf("Failed to send response to connection %s: %v", connID, err)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write response packet")
	}

	corelog.Infof("Response sent successfully to connection %s, CommandId=%s, Success=%v",
		connID, response.CommandId, response.Success)

	return nil
}

// onClose 资源清理回调
func (rm *ResponseManager) onClose() error {
	corelog.Infof("Cleaning up response manager resources...")

	// 取消事件订阅
	if rm.eventBus != nil {
		if err := rm.eventBus.Unsubscribe("CommandCompleted", rm.handleCommandCompletedEvent); err != nil {
			corelog.Warnf("Failed to unsubscribe from CommandCompleted events: %v", err)
		}
		corelog.Infof("Response manager unsubscribed from CommandCompleted events")
	}

	corelog.Infof("Response manager resources cleanup completed")
	return nil
}
