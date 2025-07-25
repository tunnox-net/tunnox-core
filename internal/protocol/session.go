package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
	"tunnox-core/internal/command"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// 类型别名，保持向后兼容
type StreamPacket = types.StreamPacket
type StreamConnection = types.StreamConnection
type Session = types.Session
type Connection = types.Connection
type ConnectionState = types.ConnectionState
type CommandHandler = types.CommandHandler
type CommandContext = types.CommandContext
type CommandResponse = types.CommandResponse
type CommandRegistry = types.CommandRegistry

// ConnectionSession 实现 Session 接口 (已废弃，请使用 SessionManager)
type ConnectionSession struct {
	connMap       map[string]*Connection
	connLock      sync.RWMutex
	idManager     *idgen.IDManager
	streamMgr     *stream.StreamManager
	streamFactory stream.StreamFactory

	// 事件驱动架构
	eventBus        events.EventBus
	responseManager *session.ResponseManager

	// Command集成
	commandRegistry *command.CommandRegistry
	commandExecutor *command.CommandExecutor

	dispose.Dispose
}

// NewConnectionSession 创建新的连接会话 (已废弃，请使用 NewSessionManager)
func NewConnectionSession(idManager *idgen.IDManager, parentCtx context.Context) *ConnectionSession {
	// 创建默认流工厂
	streamFactory := stream.NewDefaultStreamFactory(parentCtx)

	// 创建流管理器
	streamMgr := stream.NewStreamManager(streamFactory, parentCtx)

	// 创建命令注册表
	commandRegistry := command.NewCommandRegistry()

	// 创建命令执行器
	commandExecutor := command.NewCommandExecutor(commandRegistry)

	session := &ConnectionSession{
		connMap:       make(map[string]*Connection),
		idManager:     idManager,
		streamMgr:     streamMgr,
		streamFactory: streamFactory,
		// 事件驱动架构将在后续设置
		eventBus:        nil,
		responseManager: nil,
		// Command集成
		commandRegistry: commandRegistry,
		commandExecutor: commandExecutor,
	}

	// 设置命令执行器的会话引用
	commandExecutor.SetSession(session)

	// 设置资源清理回调
	session.SetCtx(parentCtx, session.onClose)

	return session
}

// SetEventBus 设置事件总线
func (s *ConnectionSession) SetEventBus(eventBus interface{}) error {
	if eventBus == nil {
		return fmt.Errorf("event bus cannot be nil")
	}

	// 类型断言
	bus, ok := eventBus.(events.EventBus)
	if !ok {
		return fmt.Errorf("invalid event bus type: expected events.EventBus")
	}

	s.eventBus = bus

	// 创建响应管理器
	s.responseManager = session.NewResponseManager(s, s.Ctx())

	// 设置响应管理器的事件总线
	if err := s.responseManager.SetEventBus(bus); err != nil {
		return fmt.Errorf("failed to set event bus for response manager: %w", err)
	}

	// 订阅断开连接请求事件
	if err := bus.Subscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
		return fmt.Errorf("failed to subscribe to DisconnectRequest events: %w", err)
	}

	utils.Infof("Event bus set with response manager and disconnect handler")
	return nil
}

// GetEventBus 获取事件总线
func (s *ConnectionSession) GetEventBus() interface{} {
	return s.eventBus
}

// GetResponseManager 获取响应管理器
func (s *ConnectionSession) GetResponseManager() *session.ResponseManager {
	return s.responseManager
}

// ==================== Command集成相关方法实现 ====================

// RegisterCommandHandler 注册命令处理器
func (s *ConnectionSession) RegisterCommandHandler(cmdType packet.CommandType, handler types.CommandHandler) error {
	return s.commandRegistry.Register(handler)
}

// UnregisterCommandHandler 注销命令处理器
func (s *ConnectionSession) UnregisterCommandHandler(cmdType packet.CommandType) error {
	return s.commandRegistry.Unregister(cmdType)
}

// ProcessCommand 处理命令（直接处理，不通过事件总线）
func (s *ConnectionSession) ProcessCommand(connID string, cmd *packet.CommandPacket) (*types.CommandResponse, error) {
	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet: &packet.TransferPacket{
			CommandPacket: cmd,
		},
		Timestamp: time.Now(),
	}

	// 通过命令执行器处理
	err := s.commandExecutor.Execute(streamPacket)
	if err != nil {
		return &types.CommandResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return &types.CommandResponse{
		Success: true,
	}, nil
}

// GetCommandRegistry 获取命令注册表
func (s *ConnectionSession) GetCommandRegistry() types.CommandRegistry {
	return s.commandRegistry
}

// GetCommandExecutor 获取命令执行器
func (s *ConnectionSession) GetCommandExecutor() types.CommandExecutor {
	return s.commandExecutor
}

// SetCommandExecutor 设置命令执行器
func (s *ConnectionSession) SetCommandExecutor(executor types.CommandExecutor) error {
	if executor == nil {
		return fmt.Errorf("command executor cannot be nil")
	}

	// 类型断言
	if cmdExecutor, ok := executor.(*command.CommandExecutor); ok {
		s.commandExecutor = cmdExecutor
		// 设置会话引用
		cmdExecutor.SetSession(s)
		return nil
	}

	return fmt.Errorf("invalid command executor type")
}

// CreateConnection 创建新连接
func (s *ConnectionSession) CreateConnection(reader io.Reader, writer io.Writer) (*Connection, error) {
	// 生成连接ID
	connID, err := s.idManager.GenerateConnectionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate connection ID: %w", err)
	}

	// 创建流连接
	streamConn := s.streamFactory.NewStreamProcessor(reader, writer)

	// 创建连接对象
	conn := &Connection{
		ID:            connID,
		State:         types.StateInitializing,
		Stream:        streamConn,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		LastHeartbeat: time.Now(),
		ClientInfo:    "",
		Protocol:      "",
	}

	// 添加到连接映射
	s.connLock.Lock()
	s.connMap[connID] = conn
	s.connLock.Unlock()

	utils.Infof("Created connection: %s", connID)
	return conn, nil
}

// AcceptConnection 接受连接（实现 Session 接口）
func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) (*StreamConnection, error) {
	// 创建连接
	conn, err := s.CreateConnection(reader, writer)
	if err != nil {
		return nil, err
	}

	// 更新连接状态为已连接
	if err := s.UpdateConnectionState(conn.ID, types.StateConnected); err != nil {
		utils.Warnf("Failed to update connection state: %v", err)
	}

	// 发布连接建立事件
	if s.eventBus != nil {
		event := events.NewConnectionEstablishedEvent(conn.ID, conn.ClientInfo, conn.Protocol)
		if err := s.eventBus.Publish(event); err != nil {
			utils.Warnf("Failed to publish connection established event: %v", err)
		}
	}

	// 返回流连接
	return &StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}, nil
}

// ProcessPacket 处理数据包
func (s *ConnectionSession) ProcessPacket(connID string, packet *packet.TransferPacket) error {
	// 更新连接状态为活跃
	if err := s.UpdateConnectionState(connID, types.StateActive); err != nil {
		utils.Warnf("Failed to update connection state: %v", err)
	}

	// 创建流数据包
	streamPacket := &StreamPacket{
		ConnectionID: connID,
		Packet:       packet,
		Timestamp:    time.Now(),
	}

	// 处理数据包
	return s.HandlePacket(streamPacket)
}

// HandlePacket 处理数据包
func (s *ConnectionSession) HandlePacket(connPacket *StreamPacket) error {
	utils.Infof("Processing packet for connection: %s, type: %v",
		connPacket.ConnectionID, connPacket.Packet.PacketType)

	// 检查是否为心跳包
	if connPacket.Packet.PacketType.IsHeartbeat() {
		return s.handleHeartbeat(connPacket)
	}

	// 检查是否为命令包
	if connPacket.Packet.PacketType.IsJsonCommand() && connPacket.Packet.CommandPacket != nil {
		return s.handleCommandPacket(connPacket)
	}

	// 其他类型的数据包，直接转发
	utils.Infof("Forwarding data packet for connection: %s", connPacket.ConnectionID)
	return nil
}

// handleCommandPacket 处理命令包
func (s *ConnectionSession) handleCommandPacket(connPacket *StreamPacket) error {
	utils.Infof("Processing command packet for connection: %s, command: %v",
		connPacket.ConnectionID, connPacket.Packet.CommandPacket.CommandType)

	// 优先使用Command集成处理
	if s.commandExecutor != nil {
		// 直接通过命令执行器处理
		err := s.commandExecutor.Execute(connPacket)
		if err != nil {
			utils.Errorf("Command execution failed for connection %s: %v", connPacket.ConnectionID, err)
			return err
		}
		utils.Infof("Command executed successfully for connection %s", connPacket.ConnectionID)
		return nil
	}

	// 如果命令执行器不可用，回退到事件总线
	if s.eventBus != nil {
		// 发布命令接收事件
		event := events.NewCommandReceivedEvent(
			connPacket.ConnectionID,
			connPacket.Packet.CommandPacket.CommandType,
			connPacket.Packet.CommandPacket.CommandId,
			connPacket.Packet.CommandPacket.Token,
			connPacket.Packet.CommandPacket.SenderId,
			connPacket.Packet.CommandPacket.ReceiverId,
			connPacket.Packet.CommandPacket.CommandBody,
		)

		if err := s.eventBus.Publish(event); err != nil {
			utils.Errorf("Failed to publish command received event for connection %s: %v", connPacket.ConnectionID, err)
			return err
		}

		utils.Infof("Command received event published for connection %s", connPacket.ConnectionID)
		return nil
	}

	// 最后回退到默认处理
	utils.Warnf("No command executor or event bus available, using default command handler")
	return s.handleDefaultCommand(connPacket)
}

// handleDefaultCommand 处理默认命令（临时实现）
func (s *ConnectionSession) handleDefaultCommand(connPacket *StreamPacket) error {
	utils.Infof("Handling default command for connection: %s, type: %v",
		connPacket.ConnectionID, connPacket.Packet.CommandPacket.CommandType)

	// 特殊处理断开连接命令
	if connPacket.Packet.CommandPacket.CommandType == packet.Disconnect {
		utils.Infof("Processing disconnect command for connection: %s", connPacket.ConnectionID)
		if err := s.CloseConnection(connPacket.ConnectionID); err != nil {
			utils.Warnf("Failed to close connection %s: %v", connPacket.ConnectionID, err)
		}
		return nil
	}

	// 对于其他未知命令类型，记录警告并返回
	utils.Warnf("Unknown command type: %v for connection %s",
		connPacket.Packet.CommandPacket.CommandType, connPacket.ConnectionID)

	return nil
}

// handleHeartbeat 处理心跳包
func (s *ConnectionSession) handleHeartbeat(connPacket *StreamPacket) error {
	utils.Debugf("Received heartbeat for connection: %s", connPacket.ConnectionID)

	// 更新连接的最后活动时间
	if conn, exists := s.GetConnection(connPacket.ConnectionID); exists {
		conn.UpdatedAt = time.Now()
		conn.LastHeartbeat = time.Now()
	}

	// 发布心跳事件
	if s.eventBus != nil {
		event := events.NewHeartbeatEvent(connPacket.ConnectionID)
		if err := s.eventBus.Publish(event); err != nil {
			utils.Warnf("Failed to publish heartbeat event: %v", err)
		}
	}

	return nil
}

// handleDisconnectRequestEvent 处理断开连接请求事件
func (s *ConnectionSession) handleDisconnectRequestEvent(event events.Event) error {
	disconnectEvent, ok := event.(*events.DisconnectRequestEvent)
	if !ok {
		return fmt.Errorf("invalid event type: expected DisconnectRequestEvent")
	}

	utils.Infof("Handling disconnect request event for connection: %s", disconnectEvent.ConnectionID)

	// 执行实际的连接关闭操作
	if err := s.CloseConnection(disconnectEvent.ConnectionID); err != nil {
		utils.Errorf("Failed to close connection %s: %v", disconnectEvent.ConnectionID, err)
		return err
	}

	utils.Infof("Successfully closed connection %s in response to disconnect request", disconnectEvent.ConnectionID)
	return nil
}

// GetConnection 获取连接信息
func (s *ConnectionSession) GetConnection(connID string) (*Connection, bool) {
	s.connLock.RLock()
	defer s.connLock.RUnlock()
	conn, exists := s.connMap[connID]
	return conn, exists
}

// ListConnections 列出所有连接
func (s *ConnectionSession) ListConnections() []*Connection {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	connections := make([]*Connection, 0, len(s.connMap))
	for _, conn := range s.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// UpdateConnectionState 更新连接状态
func (s *ConnectionSession) UpdateConnectionState(connID string, state ConnectionState) error {
	s.connLock.Lock()
	defer s.connLock.Unlock()

	conn, exists := s.connMap[connID]
	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	oldState := conn.State
	conn.State = state
	conn.UpdatedAt = time.Now()

	utils.Infof("Connection %s state changed: %s -> %s", connID, oldState, state)
	return nil
}

// CloseConnection 关闭连接
func (s *ConnectionSession) CloseConnection(connectionId string) error {
	s.connLock.Lock()
	defer s.connLock.Unlock()

	conn, exists := s.connMap[connectionId]
	if !exists {
		return fmt.Errorf("connection %s not found", connectionId)
	}

	// 更新状态为关闭中
	conn.State = types.StateClosing
	conn.UpdatedAt = time.Now()

	// 关闭流处理器
	if conn.Stream != nil {
		conn.Stream.Close()
	}

	// 从映射中移除
	delete(s.connMap, connectionId)

	// 发布连接关闭事件
	if s.eventBus != nil {
		event := events.NewConnectionClosedEvent(connectionId, "manual_close")
		if err := s.eventBus.Publish(event); err != nil {
			utils.Warnf("Failed to publish connection closed event: %v", err)
		}
	}

	utils.Infof("Closed connection: %s", connectionId)
	return nil
}

// GetStreamManager 获取流管理器
func (s *ConnectionSession) GetStreamManager() *stream.StreamManager {
	return s.streamMgr
}

// GetStreamConnectionInfo 获取流连接信息
func (s *ConnectionSession) GetStreamConnectionInfo(connectionId string) (*StreamConnection, bool) {
	conn, exists := s.GetConnection(connectionId)
	if !exists {
		return nil, false
	}

	return &StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}, true
}

// GetActiveConnections 获取活跃连接数
func (s *ConnectionSession) GetActiveConnections() int {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	count := 0
	for _, conn := range s.connMap {
		if conn.State == types.StateActive || conn.State == types.StateConnected {
			count++
		}
	}
	return count
}

// onClose 资源清理回调
func (s *ConnectionSession) onClose() error {
	utils.Infof("Cleaning up connection session resources...")

	// 取消事件订阅
	if s.eventBus != nil {
		if err := s.eventBus.Unsubscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
			utils.Warnf("Failed to unsubscribe from DisconnectRequest events: %v", err)
		}
		utils.Infof("Unsubscribed from disconnect request events")
	}

	// 关闭所有连接
	s.connLock.Lock()
	for connID, conn := range s.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		utils.Infof("Closed connection: %s", connID)
	}
	s.connMap = make(map[string]*Connection)
	s.connLock.Unlock()

	// 关闭流管理器
	if s.streamMgr != nil {
		utils.Infof("Stream manager resources cleaned up")
	}

	// 关闭事件总线
	if s.eventBus != nil {
		if err := s.eventBus.Close(); err != nil {
			utils.Errorf("Failed to close event bus: %v", err)
		}
		utils.Infof("Event bus resources cleaned up")
	}

	// 清理Command相关资源
	if s.commandExecutor != nil {
		// 命令执行器会通过Dispose自动清理
		utils.Infof("Command executor resources cleaned up")
	}

	if s.commandRegistry != nil {
		// 命令注册表不需要特殊清理
		utils.Infof("Command registry resources cleaned up")
	}

	utils.Infof("Connection session resources cleanup completed")
	return nil
}
