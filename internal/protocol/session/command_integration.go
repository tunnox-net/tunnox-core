package session

import (
	"fmt"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ============================================================================
// Command 集成
// ============================================================================

// SetEventBus 设置事件总线
func (s *SessionManager) SetEventBus(eventBus interface{}) error {
	if eventBus == nil {
		return fmt.Errorf("event bus cannot be nil")
	}

	if eb, ok := eventBus.(events.EventBus); ok {
		s.eventBus = eb
		s.responseManager = NewResponseManager(s, s.Ctx())

		// 订阅断开连接请求事件
		if err := s.eventBus.Subscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
			return fmt.Errorf("failed to subscribe to disconnect request events: %w", err)
		}

		utils.Debug("Event bus configured in SessionManager")
		return nil
	}

	return fmt.Errorf("invalid event bus type")
}

// GetEventBus 获取事件总线
func (s *SessionManager) GetEventBus() interface{} {
	return s.eventBus
}

// GetResponseManager 获取响应管理器
func (s *SessionManager) GetResponseManager() *ResponseManager {
	return s.responseManager
}

// RegisterCommandHandler 注册命令处理器
func (s *SessionManager) RegisterCommandHandler(cmdType packet.CommandType, handler types.CommandHandler) error {
	if s.commandRegistry == nil {
		return fmt.Errorf("command registry not initialized")
	}
	return s.commandRegistry.Register(handler)
}

// UnregisterCommandHandler 注销命令处理器
func (s *SessionManager) UnregisterCommandHandler(cmdType packet.CommandType) error {
	if s.commandRegistry == nil {
		return fmt.Errorf("command registry not initialized")
	}
	return s.commandRegistry.Unregister(cmdType)
}

// ProcessCommand 处理命令
func (s *SessionManager) ProcessCommand(connID string, cmd *packet.CommandPacket) (*types.CommandResponse, error) {
	if s.commandExecutor == nil {
		return nil, fmt.Errorf("command executor not initialized")
	}

	// 构建 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet: &packet.TransferPacket{
			CommandPacket: cmd,
		},
	}

	// 执行命令
	if err := s.commandExecutor.Execute(streamPacket); err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return &types.CommandResponse{Success: true}, nil
}

// GetCommandRegistry 获取命令注册表
func (s *SessionManager) GetCommandRegistry() types.CommandRegistry {
	return s.commandRegistry
}

// GetCommandExecutor 获取命令执行器
func (s *SessionManager) GetCommandExecutor() types.CommandExecutor {
	return s.commandExecutor
}

// SetCommandExecutor 设置命令执行器
func (s *SessionManager) SetCommandExecutor(executor types.CommandExecutor) error {
	if executor == nil {
		return fmt.Errorf("command executor cannot be nil")
	}
	s.commandExecutor = executor
	utils.Debug("Command executor configured in SessionManager")
	return nil
}

// ============================================================================
// Command 数据包处理
// ============================================================================

// handleCommandPacket 处理命令数据包
func (s *SessionManager) handleCommandPacket(connPacket *types.StreamPacket) error {
	// 优先使用 Command 执行器
	if s.commandExecutor != nil {
		if err := s.commandExecutor.Execute(connPacket); err != nil {
			utils.Errorf("Command execution failed: %v", err)
			return err
		}
		return nil
	}

	// 回退到默认处理
	return s.handleDefaultCommand(connPacket)
}

// handleDefaultCommand 处理默认命令（回退）
func (s *SessionManager) handleDefaultCommand(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return fmt.Errorf("command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket
	utils.Debugf("Processing command: type=%v, id=%s, conn=%s",
		cmd.CommandType, cmd.CommandId, connPacket.ConnectionID)

	// 默认简单处理：记录日志
	return nil
}

// handleHeartbeat 处理心跳
func (s *SessionManager) handleHeartbeat(connPacket *types.StreamPacket) error {
	// 更新连接活跃时间
	s.connLock.Lock()
	if conn, exists := s.connMap[connPacket.ConnectionID]; exists {
		conn.LastHeartbeat = connPacket.Timestamp
		conn.UpdatedAt = connPacket.Timestamp
	}
	s.connLock.Unlock()

	// 更新 Control Connection 活跃时间
	s.controlConnLock.Lock()
	if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
		controlConn.UpdateActivity()
	}
	s.controlConnLock.Unlock()

	utils.Debugf("Heartbeat received from connection: %s", connPacket.ConnectionID)

	// 发送心跳响应
	conn, exists := s.GetConnection(connPacket.ConnectionID)
	if exists && conn.Stream != nil {
		respPacket := &packet.TransferPacket{
			PacketType: packet.Heartbeat, // 心跳响应也用 Heartbeat
		}
		if _, err := conn.Stream.WritePacket(respPacket, false, 0); err != nil {
			utils.Errorf("Failed to send heartbeat response: %v", err)
		}
	}

	return nil
}
