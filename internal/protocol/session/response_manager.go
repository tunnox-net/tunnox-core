package session

import (
	"context"
	"fmt"
	"tunnox-core/internal/command"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/utils"
)

// ResponseManager 响应管理器
type ResponseManager struct {
	session types.Session
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

// SendResponse 发送响应
func (rm *ResponseManager) SendResponse(connID string, response *command.CommandResponse) error {
	// 获取连接信息
	_, exists := rm.session.GetConnection(connID)
	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	// 这里应该实现具体的响应发送逻辑
	// 目前只是记录日志
	utils.Infof("Sending response to connection %s: success=%v", connID, response.Success)
	return nil
}

// onClose 资源清理回调
func (rm *ResponseManager) onClose() error {
	utils.Infof("Response manager resources cleaned up")
	return nil
}
