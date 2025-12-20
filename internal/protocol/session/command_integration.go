package session

import (
corelog "tunnox-core/internal/core/log"
	"fmt"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// ============================================================================
// Command 集成
// ============================================================================

// SetEventBus 设置事件总线
func (s *SessionManager) SetEventBus(eventBus events.EventBus) error {
	if eventBus == nil {
		return fmt.Errorf("event bus cannot be nil")
	}

	s.eventBus = eventBus
		s.responseManager = NewResponseManager(s, s.Ctx())

		// 订阅断开连接请求事件
		if err := s.eventBus.Subscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
			return fmt.Errorf("failed to subscribe to disconnect request events: %w", err)
		}

		corelog.Debug("Event bus configured in SessionManager")
		return nil
}

// GetEventBus 获取事件总线
func (s *SessionManager) GetEventBus() events.EventBus {
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
	corelog.Debug("Command executor configured in SessionManager")
	return nil
}

// ============================================================================
// Command 数据包处理
// ============================================================================

// handleCommandPacket 处理命令数据包
func (s *SessionManager) handleCommandPacket(connPacket *types.StreamPacket) error {
	corelog.Debugf("SessionManager.handleCommandPacket: received command packet, ConnectionID=%s, PacketType=%d",
		connPacket.ConnectionID, connPacket.Packet.PacketType)

	if connPacket.Packet.CommandPacket != nil {
		corelog.Debugf("SessionManager.handleCommandPacket: CommandType=%d, CommandID=%s",
			connPacket.Packet.CommandPacket.CommandType, connPacket.Packet.CommandPacket.CommandId)
	}

	// 优先使用 Command 执行器
	if s.commandExecutor != nil {
		corelog.Debugf("SessionManager.handleCommandPacket: executing command via CommandExecutor")
		if err := s.commandExecutor.Execute(connPacket); err != nil {
			corelog.Errorf("SessionManager.handleCommandPacket: command execution failed: %v", err)
			return err
		}
		corelog.Debugf("SessionManager.handleCommandPacket: command executed successfully")
		return nil
	}

	corelog.Warnf("SessionManager.handleCommandPacket: CommandExecutor is nil, falling back to default handler")
	// 回退到默认处理
	return s.handleDefaultCommand(connPacket)
}

// handleDefaultCommand 处理默认命令（回退）
func (s *SessionManager) handleDefaultCommand(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return fmt.Errorf("command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket
	corelog.Debugf("Processing command: type=%v, id=%s, conn=%s",
		cmd.CommandType, cmd.CommandId, connPacket.ConnectionID)

	// 处理 ConfigGet 命令
	if cmd.CommandType == packet.ConfigGet {
		return s.handleConfigGetCommand(connPacket)
	}

	// 默认简单处理：记录日志
	return nil
}

// handleConfigGetCommand 处理配置获取命令
func (s *SessionManager) handleConfigGetCommand(connPacket *types.StreamPacket) error {
	// 获取控制连接
	s.controlConnLock.RLock()
	controlConn, exists := s.controlConnMap[connPacket.ConnectionID]
	s.controlConnLock.RUnlock()

	if !exists {
		return fmt.Errorf("control connection not found: %s", connPacket.ConnectionID)
	}

	// 获取客户端ID
	clientID := controlConn.ClientID
	if clientID == 0 {
		corelog.Warnf("Client has no ID, cannot get config: conn=%s", connPacket.ConnectionID)
		return s.sendEmptyConfig(controlConn)
	}

	// 从认证处理器获取映射配置（通过CloudControl）
	var configBody string
	if s.authHandler != nil {
		// 使用认证处理器获取配置
		config, err := s.authHandler.GetClientConfig(controlConn)
		if err != nil {
			corelog.Errorf("Failed to get client config for client %d: %v", clientID, err)
			return s.sendEmptyConfig(controlConn)
		}
		configBody = config
	} else {
		// 回退到空配置
		configBody = `{"mappings":[]}`
	}

	corelog.Infof("SessionManager: sending config to client %d (%d bytes)", clientID, len(configBody))

	// 发送配置响应
	responseCmd := &packet.CommandPacket{
		CommandType: packet.ConfigSet, // 使用 ConfigSet 作为配置推送
		CommandBody: configBody,
	}

	responsePacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: responseCmd,
	}

	_, err := controlConn.Stream.WritePacket(responsePacket, true, 0)
	return err
}

// sendEmptyConfig 发送空配置
func (s *SessionManager) sendEmptyConfig(conn *ControlConnection) error {
	responseCmd := &packet.CommandPacket{
		CommandType: packet.ConfigSet,
		CommandBody: `{"mappings":[]}`,
	}

	responsePacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: responseCmd,
	}

	_, err := conn.Stream.WritePacket(responsePacket, false, 0)
	return err
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

	corelog.Debugf("Heartbeat received from connection: %s", connPacket.ConnectionID)

	// 发送心跳响应
	conn, exists := s.GetConnection(connPacket.ConnectionID)
	if exists && conn.Stream != nil {
		respPacket := &packet.TransferPacket{
			PacketType: packet.Heartbeat, // 心跳响应也用 Heartbeat
		}
		if _, err := conn.Stream.WritePacket(respPacket, true, 0); err != nil {
			corelog.Errorf("Failed to send heartbeat response: %v", err)
		}
	}

	return nil
}
