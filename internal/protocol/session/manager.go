package session

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// SessionManager 实现 Session 接口（双连接模型）
type SessionManager struct {
	// 基础连接映射（所有连接）
	connMap  map[string]*types.Connection
	connLock sync.RWMutex

	// 指令连接（Control Connection）
	controlConnMap    map[string]*ControlConnection // connID -> 指令连接
	clientIDIndexMap  map[int64]*ControlConnection  // clientID -> 指令连接（快速查找）
	controlConnLock   sync.RWMutex

	// 映射连接（Tunnel Connection）
	tunnelConnMap   map[string]*TunnelConnection // connID -> 映射连接
	tunnelIDMap     map[string]*TunnelConnection // tunnelID -> 映射连接
	tunnelConnLock  sync.RWMutex

	idManager     *idgen.IDManager
	streamMgr     *stream.StreamManager
	streamFactory stream.StreamFactory

	// 事件驱动架构
	eventBus        events.EventBus
	responseManager *ResponseManager

	// Command集成
	commandRegistry types.CommandRegistry
	commandExecutor types.CommandExecutor
	
	// 隧道和认证处理器
	tunnelHandler TunnelHandler
	authHandler   AuthHandler

	// 临时兼容字段（迁移完成后删除）
	clientConnMap map[string]*ClientConnection

	dispose.Dispose
}

// NewSessionManager 创建新的会话管理器（双连接模型）
func NewSessionManager(idManager *idgen.IDManager, parentCtx context.Context) *SessionManager {
	// 创建默认流工厂
	streamFactory := stream.NewDefaultStreamFactory(parentCtx)

	// 创建流管理器
	streamMgr := stream.NewStreamManager(streamFactory, parentCtx)

	session := &SessionManager{
		// 基础连接
		connMap: make(map[string]*types.Connection),

		// 指令连接
		controlConnMap:   make(map[string]*ControlConnection),
		clientIDIndexMap: make(map[int64]*ControlConnection),

		// 映射连接
		tunnelConnMap: make(map[string]*TunnelConnection),
		tunnelIDMap:   make(map[string]*TunnelConnection),

		// 临时兼容
		clientConnMap: make(map[string]*ClientConnection),

		idManager:     idManager,
		streamMgr:     streamMgr,
		streamFactory: streamFactory,
		// 事件驱动架构将在后续设置
		eventBus:        nil,
		responseManager: nil,
		tunnelHandler:   nil,
		authHandler:     nil,
	}

	// 设置资源清理回调
	session.SetCtx(parentCtx, session.onClose)

	return session
}

// SetEventBus 设置事件总线
func (s *SessionManager) SetEventBus(eventBus interface{}) error {
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
	s.responseManager = NewResponseManager(s, s.Ctx())

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
func (s *SessionManager) GetEventBus() interface{} {
	return s.eventBus
}

// GetResponseManager 获取响应管理器
func (s *SessionManager) GetResponseManager() *ResponseManager {
	return s.responseManager
}

// ==================== Command集成相关方法实现 ====================

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

// ProcessCommand 处理命令（直接处理，不通过事件总线）
func (s *SessionManager) ProcessCommand(connID string, cmd *packet.CommandPacket) (*types.CommandResponse, error) {
	if s.commandExecutor == nil {
		return &types.CommandResponse{
			Success: false,
			Error:   "command executor not initialized",
		}, fmt.Errorf("command executor not initialized")
	}

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
	// 设置会话引用
	executor.SetSession(s)
	return nil
}

// CreateConnection 创建新连接
func (s *SessionManager) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	// 生成连接ID
	connID, err := s.idManager.GenerateConnectionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate connection ID: %v", err)
	}

	// 创建流处理器
	streamProcessor, err := s.streamMgr.CreateStream(connID, reader, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream processor: %v", err)
	}

	// 创建连接信息
	conn := &types.Connection{
		ID:            connID,
		State:         types.StateInitializing,
		Stream:        streamProcessor,
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

// AcceptConnection 初始化连接
func (s *SessionManager) AcceptConnection(reader io.Reader, writer io.Writer) (*types.StreamConnection, error) {
	// 创建连接
	conn, err := s.CreateConnection(reader, writer)
	if err != nil {
		return nil, err
	}

	// 更新连接状态
	if err := s.UpdateConnectionState(conn.ID, types.StateConnected); err != nil {
		return nil, err
	}

	// 发布连接建立事件
	if s.eventBus != nil {
		event := events.NewConnectionEstablishedEvent(conn.ID, conn.ClientInfo, conn.Protocol)
		if err := s.eventBus.Publish(event); err != nil {
			utils.Warnf("Failed to publish connection established event: %v", err)
		}
	}

	// 尝试读取第一个数据包来确定连接类型
	streamConn := &types.StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}

	// 读取数据包
	packet, _, err := conn.Stream.ReadPacket()
	if err != nil {
		utils.Warnf("Failed to read initial packet for connection %s: %v", conn.ID, err)
		// 即使读取失败，也返回连接信息，让上层决定如何处理
		return streamConn, nil
	}

	// 处理数据包
	if err := s.ProcessPacket(conn.ID, packet); err != nil {
		utils.Warnf("Failed to process initial packet for connection %s: %v", conn.ID, err)
	}

	return streamConn, nil
}

// ProcessPacket 处理数据包
func (s *SessionManager) ProcessPacket(connID string, packet *packet.TransferPacket) error {
	// 获取连接信息
	_, exists := s.GetConnection(connID)
	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	// 更新连接状态
	if err := s.UpdateConnectionState(connID, types.StateActive); err != nil {
		return err
	}

	// 创建数据包上下文
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet:       packet,
		Timestamp:    time.Now(),
	}

	// 处理数据包
	return s.HandlePacket(streamPacket)
}

// HandlePacket 处理数据包
func (s *SessionManager) HandlePacket(connPacket *types.StreamPacket) error {
	utils.Infof("Processing packet for connection: %s, type: %v",
		connPacket.ConnectionID, connPacket.Packet.PacketType)

	// 检查是否为心跳包
	if connPacket.Packet.PacketType.IsHeartbeat() {
		return s.handleHeartbeat(connPacket)
	}
	
	// 检查是否为握手包
	if connPacket.Packet.PacketType.IsHandshake() {
		return s.handleTunnelPacket(connPacket)
	}
	
	// 检查是否为隧道数据包
	if connPacket.Packet.PacketType.IsTunnelPacket() {
		return s.handleTunnelPacket(connPacket)
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
func (s *SessionManager) handleCommandPacket(connPacket *types.StreamPacket) error {
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
func (s *SessionManager) handleDefaultCommand(connPacket *types.StreamPacket) error {
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
func (s *SessionManager) handleHeartbeat(connPacket *types.StreamPacket) error {
	utils.Debugf("Received heartbeat for connection: %s", connPacket.ConnectionID)

	// 更新连接的最后活动时间
	conn, exists := s.GetConnection(connPacket.ConnectionID)
	if !exists {
		return fmt.Errorf("connection %s not found", connPacket.ConnectionID)
	}
	
	conn.UpdatedAt = time.Now()
	conn.LastHeartbeat = time.Now()

	// 发送心跳响应
	heartbeatResp := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
		Payload:    []byte{},
	}
	
	if _, err := conn.Stream.WritePacket(heartbeatResp, false, 0); err != nil {
		utils.Warnf("Failed to send heartbeat response for connection %s: %v", connPacket.ConnectionID, err)
		// 继续处理，不中断
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
func (s *SessionManager) handleDisconnectRequestEvent(event events.Event) error {
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
func (s *SessionManager) GetConnection(connID string) (*types.Connection, bool) {
	s.connLock.RLock()
	defer s.connLock.RUnlock()
	conn, exists := s.connMap[connID]
	return conn, exists
}

// ListConnections 列出所有连接
func (s *SessionManager) ListConnections() []*types.Connection {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	connections := make([]*types.Connection, 0, len(s.connMap))
	for _, conn := range s.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// UpdateConnectionState 更新连接状态
func (s *SessionManager) UpdateConnectionState(connID string, state types.ConnectionState) error {
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
func (s *SessionManager) CloseConnection(connectionId string) error {
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
func (s *SessionManager) GetStreamManager() *stream.StreamManager {
	return s.streamMgr
}

// GetStreamConnectionInfo 获取流连接信息
func (s *SessionManager) GetStreamConnectionInfo(connectionId string) (*types.StreamConnection, bool) {
	conn, exists := s.GetConnection(connectionId)
	if !exists {
		return nil, false
	}

	return &types.StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}, true
}

// GetActiveConnections 获取活跃连接数
func (s *SessionManager) GetActiveConnections() int {
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
func (s *SessionManager) onClose() error {
	utils.Infof("Cleaning up session manager resources...")

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
	s.connMap = make(map[string]*types.Connection)
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

	utils.Infof("Session manager resources cleanup completed")
	return nil
}

// ============================================================================
// 双连接模型：指令连接管理
// ============================================================================

// RegisterControlConnection 注册指令连接
func (s *SessionManager) RegisterControlConnection(conn *ControlConnection) {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	s.controlConnMap[conn.ConnID] = conn
	
	// 如果已认证，建立 ClientID 索引
	if conn.Authenticated && conn.ClientID > 0 {
		s.clientIDIndexMap[conn.ClientID] = conn
		utils.Infof("SessionManager: control connection registered and indexed, client_id=%d, conn_id=%s", conn.ClientID, conn.ConnID)
	} else {
		utils.Infof("SessionManager: control connection registered (not authenticated), conn_id=%s", conn.ConnID)
	}
}

// UpdateControlConnectionAuth 更新指令连接认证状态（认证成功后调用）
func (s *SessionManager) UpdateControlConnectionAuth(connID string, clientID int64, userID string) error {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	conn, exists := s.controlConnMap[connID]
	if !exists {
		return fmt.Errorf("control connection not found: %s", connID)
	}

	conn.ClientID = clientID
	conn.UserID = userID
	conn.Authenticated = true
	conn.UpdateActivity()

	// 建立 ClientID 索引
	s.clientIDIndexMap[clientID] = conn
	
	utils.Infof("SessionManager: control connection authenticated, client_id=%d, conn_id=%s", clientID, connID)
	return nil
}

// GetControlConnectionByClientID 根据 ClientID 获取指令连接（O(1) 查找）
func (s *SessionManager) GetControlConnectionByClientID(clientID int64) *ControlConnection {
	s.controlConnLock.RLock()
	defer s.controlConnLock.RUnlock()

	return s.clientIDIndexMap[clientID]
}

// KickOldControlConnection 踢掉旧的指令连接（同一 ClientID 只能有1条）
func (s *SessionManager) KickOldControlConnection(clientID int64, newConnID string) {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	oldConn, exists := s.clientIDIndexMap[clientID]
	if !exists || oldConn.ConnID == newConnID {
		return // 没有旧连接，或者就是当前连接
	}

	utils.Warnf("SessionManager: kicking old control connection for client_id=%d, old_conn_id=%s, new_conn_id=%s",
		clientID, oldConn.ConnID, newConnID)

	// 关闭旧连接
	if oldConn.Stream != nil {
		oldConn.Stream.Close()
	}

	// 从映射中删除
	delete(s.controlConnMap, oldConn.ConnID)
	
	utils.Infof("SessionManager: old control connection kicked for client_id=%d", clientID)
}

// RemoveControlConnection 移除指令连接
func (s *SessionManager) RemoveControlConnection(connID string) {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()

	conn, exists := s.controlConnMap[connID]
	if !exists {
		return
	}

	// 从 ClientID 索引中删除
	if conn.Authenticated && conn.ClientID > 0 {
		delete(s.clientIDIndexMap, conn.ClientID)
	}

	delete(s.controlConnMap, connID)
	utils.Infof("SessionManager: control connection removed, conn_id=%s", connID)
}

// ============================================================================
// 双连接模型：映射连接管理
// ============================================================================

// RegisterTunnelConnection 注册映射连接
func (s *SessionManager) RegisterTunnelConnection(conn *TunnelConnection) {
	s.tunnelConnLock.Lock()
	defer s.tunnelConnLock.Unlock()

	s.tunnelConnMap[conn.ConnID] = conn
	
	if conn.TunnelID != "" {
		s.tunnelIDMap[conn.TunnelID] = conn
	}

	utils.Infof("SessionManager: tunnel connection registered, conn_id=%s, tunnel_id=%s", conn.ConnID, conn.TunnelID)
}

// UpdateTunnelConnectionAuth 更新映射连接认证状态（TunnelOpen 成功后调用）
func (s *SessionManager) UpdateTunnelConnectionAuth(connID string, tunnelID string, mappingID string) error {
	s.tunnelConnLock.Lock()
	defer s.tunnelConnLock.Unlock()

	conn, exists := s.tunnelConnMap[connID]
	if !exists {
		return fmt.Errorf("tunnel connection not found: %s", connID)
	}

	conn.TunnelID = tunnelID
	conn.MappingID = mappingID
	conn.Authenticated = true
	conn.UpdateActivity()

	// 建立 TunnelID 索引
	s.tunnelIDMap[tunnelID] = conn
	
	utils.Infof("SessionManager: tunnel connection authenticated, tunnel_id=%s, conn_id=%s", tunnelID, connID)
	return nil
}

// GetTunnelConnectionByTunnelID 根据 TunnelID 获取映射连接
func (s *SessionManager) GetTunnelConnectionByTunnelID(tunnelID string) *TunnelConnection {
	s.tunnelConnLock.RLock()
	defer s.tunnelConnLock.RUnlock()

	return s.tunnelIDMap[tunnelID]
}

// GetTunnelConnectionByConnID 根据 ConnID 获取映射连接
func (s *SessionManager) GetTunnelConnectionByConnID(connID string) *TunnelConnection {
	s.tunnelConnLock.RLock()
	defer s.tunnelConnLock.RUnlock()

	return s.tunnelConnMap[connID]
}

// RemoveTunnelConnection 移除映射连接
func (s *SessionManager) RemoveTunnelConnection(connID string) {
	s.tunnelConnLock.Lock()
	defer s.tunnelConnLock.Unlock()

	conn, exists := s.tunnelConnMap[connID]
	if !exists {
		return
	}

	// 从 TunnelID 索引中删除
	if conn.TunnelID != "" {
		delete(s.tunnelIDMap, conn.TunnelID)
	}

	delete(s.tunnelConnMap, connID)
	utils.Infof("SessionManager: tunnel connection removed, conn_id=%s, tunnel_id=%s", connID, conn.TunnelID)
}

// ============================================================================
// 临时兼容方法（迁移完成后删除）
// ============================================================================

// GetConnectionByClientID 临时兼容方法，优先返回指令连接
func (s *SessionManager) GetConnectionByClientID(clientID int64) *ClientConnection {
	return s.GetControlConnectionByClientID(clientID)
}
