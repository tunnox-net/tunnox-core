package session

import (
	"context"
	"fmt"
	"tunnox-core/internal/command"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/utils"
)

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
			return fmt.Errorf("failed to subscribe to CommandCompleted events: %w", err)
		}
		utils.Infof("Response manager subscribed to CommandCompleted events")
	}

	return nil
}

// handleCommandCompletedEvent 处理命令完成事件
func (rm *ResponseManager) handleCommandCompletedEvent(event events.Event) error {
	completedEvent, ok := event.(*events.CommandCompletedEvent)
	if !ok {
		return fmt.Errorf("invalid event type: expected CommandCompletedEvent")
	}

	utils.Infof("Handling command completed event for connection: %s, success: %v",
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
		return fmt.Errorf("connection %s not found", connID)
	}

	// 检查连接状态
	if conn.State == types.StateClosed || conn.State == types.StateClosing {
		return fmt.Errorf("connection %s is closed or closing", connID)
	}

	// 这里应该实现具体的响应发送逻辑
	// 目前只是记录日志
	utils.Infof("Sending response to connection %s: success=%v, data=%s",
		connID, response.Success, response.Data)

	// TODO: 实现实际的响应发送逻辑
	// 1. 将响应序列化为数据包
	// 2. 通过连接的Stream发送数据包

	return nil
}

// onClose 资源清理回调
func (rm *ResponseManager) onClose() error {
	utils.Infof("Cleaning up response manager resources...")

	// 取消事件订阅
	if rm.eventBus != nil {
		if err := rm.eventBus.Unsubscribe("CommandCompleted", rm.handleCommandCompletedEvent); err != nil {
			utils.Warnf("Failed to unsubscribe from CommandCompleted events: %v", err)
		}
		utils.Infof("Response manager unsubscribed from CommandCompleted events")
	}

	utils.Infof("Response manager resources cleanup completed")
	return nil
}
