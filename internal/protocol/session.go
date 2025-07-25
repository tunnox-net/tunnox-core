package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/common"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// StreamPacket 包装结构，包含连接信息
type StreamPacket = common.StreamPacket

// StreamConnection 连接信息
type StreamConnection = common.StreamConnection

// Session 接口定义
type Session = common.Session

// Connection 连接信息
type Connection = common.Connection

// ConnectionState 连接状态
type ConnectionState = common.ConnectionState

// 从 common 包导入命令相关类型
type CommandHandler = common.CommandHandler
type CommandContext = common.CommandContext
type CommandResponse = common.CommandResponse
type CommandRegistry = common.CommandRegistry

// ConnectionSession 实现 Session 接口
type ConnectionSession struct {
	connMap         map[string]*Connection
	connLock        sync.RWMutex
	idManager       *generators.IDManager
	streamMgr       *stream.StreamManager
	streamFactory   stream.StreamFactory
	commandRegistry CommandRegistry

	utils.Dispose
}

// NewConnectionSession 创建新的连接会话
func NewConnectionSession(idManager *generators.IDManager, parentCtx context.Context) *ConnectionSession {
	// 创建默认流工厂
	streamFactory := stream.NewDefaultStreamFactory(parentCtx)

	// 创建流管理器
	streamMgr := stream.NewStreamManager(streamFactory, parentCtx)

	session := &ConnectionSession{
		connMap:         make(map[string]*Connection),
		idManager:       idManager,
		streamMgr:       streamMgr,
		streamFactory:   streamFactory,
		commandRegistry: nil, // 暂时设为 nil，通过 SetCommandRegistry 方法设置
	}

	// 设置资源清理回调
	session.SetCtx(parentCtx, session.onClose)

	return session
}

// SetCommandRegistry 设置命令注册表
func (s *ConnectionSession) SetCommandRegistry(registry CommandRegistry) {
	s.commandRegistry = registry
	// 注册默认命令处理器
	s.registerDefaultHandlers()
}

// registerDefaultHandlers 注册默认命令处理器
func (s *ConnectionSession) registerDefaultHandlers() {
	if s.commandRegistry == nil {
		utils.Warnf("Command registry is nil, skipping handler registration")
		return
	}

	// 注意：处理器注册应该在外部通过 SetCommandRegistry 方法传入已配置的注册表
	utils.Infof("Command registry set, handlers should be pre-registered")
}

// CreateConnection 创建新连接
func (s *ConnectionSession) CreateConnection(reader io.Reader, writer io.Writer) (*Connection, error) {
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
	conn := &Connection{
		ID:            connID,
		State:         common.StateInitializing,
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
func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) (*StreamConnection, error) {
	// 创建连接
	conn, err := s.CreateConnection(reader, writer)
	if err != nil {
		return nil, err
	}

	// 更新连接状态
	if err := s.UpdateConnectionState(conn.ID, common.StateConnected); err != nil {
		return nil, err
	}

	// 尝试读取第一个数据包来确定连接类型
	streamConn := &StreamConnection{
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
func (s *ConnectionSession) ProcessPacket(connID string, packet *packet.TransferPacket) error {
	// 获取连接信息
	_, exists := s.GetConnection(connID)
	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	// 更新连接状态
	if err := s.UpdateConnectionState(connID, common.StateActive); err != nil {
		return err
	}

	// 创建数据包上下文
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

	// 检查命令注册表是否已设置
	if s.commandRegistry == nil {
		utils.Warnf("Command registry is nil, using default command handler")
		return s.handleDefaultCommand(connPacket)
	}

	// 获取命令处理器
	handler, exists := s.commandRegistry.GetHandler(connPacket.Packet.CommandPacket.CommandType)
	if !exists {
		utils.Warnf("No handler found for command type %v", connPacket.Packet.CommandPacket.CommandType)
		// 暂时使用简单的处理逻辑
		return s.handleDefaultCommand(connPacket)
	}

	// 创建命令上下文
	cmdCtx := &CommandContext{
		ConnectionID:    connPacket.ConnectionID,
		CommandType:     connPacket.Packet.CommandPacket.CommandType,
		CommandId:       connPacket.Packet.CommandPacket.CommandId,
		RequestID:       connPacket.Packet.CommandPacket.Token,
		SenderID:        connPacket.Packet.CommandPacket.SenderId,
		ReceiverID:      connPacket.Packet.CommandPacket.ReceiverId,
		RequestBody:     connPacket.Packet.CommandPacket.CommandBody,
		Session:         s,
		Context:         context.Background(),
		IsAuthenticated: false,
		UserID:          "",
		StartTime:       time.Now(),
		EndTime:         time.Time{},
	}

	// 执行命令处理
	response, err := handler.Handle(cmdCtx)
	if err != nil {
		utils.Errorf("Failed to handle command for connection %s: %v", connPacket.ConnectionID, err)
		return err
	}

	// 处理响应
	if response != nil {
		utils.Infof("Command response for connection %s: success=%v", connPacket.ConnectionID, response.Success)
		// TODO: 发送响应给客户端
	}

	return nil
}

// handleDefaultCommand 处理默认命令（临时实现）
func (s *ConnectionSession) handleDefaultCommand(connPacket *StreamPacket) error {
	utils.Infof("Handling default command for connection: %s, type: %v",
		connPacket.ConnectionID, connPacket.Packet.CommandPacket.CommandType)

	// 对于未知命令类型，记录警告并返回
	utils.Warnf("Unknown command type: %v for connection %s",
		connPacket.Packet.CommandPacket.CommandType, connPacket.ConnectionID)

	// 特殊处理断开连接命令
	if connPacket.Packet.CommandPacket.CommandType == packet.Disconnect {
		if err := s.CloseConnection(connPacket.ConnectionID); err != nil {
			utils.Warnf("Failed to close connection %s: %v", connPacket.ConnectionID, err)
		}
	}

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

	// - 记录心跳时间
	// - 可选：发送心跳响应

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
	conn.State = common.StateClosing
	conn.UpdatedAt = time.Now()

	// 关闭流处理器
	if conn.Stream != nil {
		conn.Stream.Close()
	}

	// 从映射中移除
	delete(s.connMap, connectionId)

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
		if conn.State == common.StateActive || conn.State == common.StateConnected {
			count++
		}
	}
	return count
}

// onClose 资源清理回调
func (s *ConnectionSession) onClose() error {
	utils.Infof("Cleaning up connection session resources...")

	// 获取所有连接信息的副本，避免在锁内调用 Close
	var connections []*Connection
	s.connLock.Lock()
	for _, connInfo := range s.connMap {
		connections = append(connections, connInfo)
	}
	// 清空连接映射
	s.connMap = make(map[string]*Connection)
	s.connLock.Unlock()

	// 在锁外记录清理的连接
	for _, connInfo := range connections {
		utils.Infof("Cleaned up connection: %s", connInfo.ID)
	}

	utils.Infof("Connection session resources cleanup completed")
	return nil
}
