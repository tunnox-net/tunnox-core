package session

import (
	"encoding/json"

	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/events"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httptypes"
)

// ============================================================================
// Command 集成
// ============================================================================

// SetEventBus 设置事件总线
func (s *SessionManager) SetEventBus(eventBus events.EventBus) error {
	if eventBus == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "event bus cannot be nil")
	}

	s.eventBus = eventBus
	s.responseManager = NewResponseManager(s, s.Ctx())

	// 订阅断开连接请求事件
	if err := s.eventBus.Subscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to subscribe to disconnect request events")
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
		return coreerrors.New(coreerrors.CodeNotConfigured, "command registry not initialized")
	}
	return s.commandRegistry.Register(handler)
}

// UnregisterCommandHandler 注销命令处理器
func (s *SessionManager) UnregisterCommandHandler(cmdType packet.CommandType) error {
	if s.commandRegistry == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "command registry not initialized")
	}
	return s.commandRegistry.Unregister(cmdType)
}

// ProcessCommand 处理命令
func (s *SessionManager) ProcessCommand(connID string, cmd *packet.CommandPacket) (*types.CommandResponse, error) {
	if s.commandExecutor == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "command executor not initialized")
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "command execution failed")
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
		return coreerrors.New(coreerrors.CodeInvalidParam, "command executor cannot be nil")
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

		// 特殊处理 HTTP 代理响应
		if connPacket.Packet.PacketType.IsCommandResp() &&
			connPacket.Packet.CommandPacket.CommandType == packet.HTTPProxyResponse {
			return s.handleHTTPProxyResponsePacket(connPacket)
		}

		// 特殊处理 SOCKS5 隧道请求
		if connPacket.Packet.CommandPacket.CommandType == packet.SOCKS5TunnelRequestCmd {
			return s.HandleSOCKS5TunnelRequest(connPacket)
		}
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

// handleHTTPProxyResponsePacket 处理 HTTP 代理响应包
func (s *SessionManager) handleHTTPProxyResponsePacket(connPacket *types.StreamPacket) error {
	cmd := connPacket.Packet.CommandPacket
	if cmd == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	// 解析响应
	var resp httptypes.HTTPProxyResponse
	if err := json.Unmarshal([]byte(cmd.CommandBody), &resp); err != nil {
		corelog.Errorf("SessionManager: failed to parse HTTP proxy response: %v", err)
		return err
	}

	// 如果 RequestID 为空，使用 CommandId
	if resp.RequestID == "" {
		resp.RequestID = cmd.CommandId
	}

	corelog.Debugf("SessionManager: received HTTP proxy response for request %s, status=%d",
		resp.RequestID, resp.StatusCode)

	// 转发到 HTTP 代理管理器
	s.HandleHTTPProxyResponse(&resp)

	return nil
}

// handleDefaultCommand 处理默认命令（回退）
func (s *SessionManager) handleDefaultCommand(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
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
	// 获取控制连接 - 使用 clientRegistry
	controlConn := s.clientRegistry.GetByConnID(connPacket.ConnectionID)
	if controlConn == nil {
		return coreerrors.Newf(coreerrors.CodeNotFound, "control connection not found: %s", connPacket.ConnectionID)
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

	// 更新 Control Connection 活跃时间 - 使用 clientRegistry
	if controlConn := s.clientRegistry.GetByConnID(connPacket.ConnectionID); controlConn != nil {
		controlConn.UpdateActivity()
	}

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
