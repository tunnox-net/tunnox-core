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

// ConnectionSession 实现 Session 接口
type ConnectionSession struct {
	connMap       map[string]*Connection
	connLock      sync.RWMutex
	idManager     *generators.IDManager
	streamMgr     *stream.StreamManager
	streamFactory stream.StreamFactory

	utils.Dispose
}

// NewConnectionSession 创建新的连接会话
func NewConnectionSession(idManager *generators.IDManager, parentCtx context.Context) *ConnectionSession {
	// 创建默认流工厂
	streamFactory := stream.NewDefaultStreamFactory(parentCtx)

	// 创建流管理器
	streamMgr := stream.NewStreamManager(streamFactory, parentCtx)

	session := &ConnectionSession{
		connMap:       make(map[string]*Connection),
		idManager:     idManager,
		streamMgr:     streamMgr,
		streamFactory: streamFactory,
	}
	session.SetCtx(parentCtx, session.onClose)
	return session
}

// CreateConnection 创建新连接（新接口）
func (s *ConnectionSession) CreateConnection(reader io.Reader, writer io.Writer) (*Connection, error) {
	// 生成连接ID
	connID, err := s.idManager.GenerateConnectionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate connection ID: %w", err)
	}

	// 使用流管理器创建数据流
	ps, err := s.streamMgr.CreateStream(connID, reader, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	now := time.Now()
	connection := &Connection{
		ID:        connID,
		State:     common.StateInitializing,
		Stream:    ps,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]interface{}),
	}

	// 保存连接信息
	s.connLock.Lock()
	s.connMap[connID] = connection
	s.connLock.Unlock()

	utils.Infof("Connection created: %s", connID)
	return connection, nil
}

// AcceptConnection 初始化连接（保持向后兼容）
func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) (*StreamConnection, error) {
	// 使用新的CreateConnection方法
	conn, err := s.CreateConnection(reader, writer)
	if err != nil {
		return nil, err
	}

	// 读取初始数据包
	initPacket, _, err := conn.Stream.ReadPacket()
	if err != nil {
		return nil, fmt.Errorf("failed to read packet: %w", err)
	}

	if !initPacket.PacketType.IsJsonCommand() {
		return nil, fmt.Errorf("invalid packet type: %s", initPacket.PacketType)
	}

	// 更新连接状态为已连接
	s.UpdateConnectionState(conn.ID, common.StateConnected)

	utils.Infof("Connection initialized: %s", conn.ID)

	// 处理心跳包
	streamPacket := &StreamPacket{
		ConnectionID: conn.ID,
		Packet:       initPacket,
		Timestamp:    time.Now(),
	}

	err = s.handleHeartbeat(streamPacket)
	if err != nil {
		return nil, err
	}

	// 返回兼容的StreamConnection
	return &StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}, err
}

// ProcessPacket 处理数据包（新接口）
func (s *ConnectionSession) ProcessPacket(connID string, packet *packet.TransferPacket) error {
	utils.Debugf("Processing packet for connection: %s, type: %v", connID, packet.PacketType)

	// 处理心跳包
	if packet.PacketType.IsHeartbeat() {
		streamPacket := &StreamPacket{
			ConnectionID: connID,
			Packet:       packet,
			Timestamp:    time.Now(),
		}
		return s.handleHeartbeat(streamPacket)
	}

	// 处理JSON命令包
	if packet.PacketType.IsJsonCommand() && packet.CommandPacket != nil {
		streamPacket := &StreamPacket{
			ConnectionID: connID,
			Packet:       packet,
			Timestamp:    time.Now(),
		}
		return s.handleCommandPacket(streamPacket)
	}

	// 处理其他类型的包
	utils.Warnf("Unsupported packet type for connection %s: %v", connID, packet.PacketType)
	return nil
}

// HandlePacket 处理数据包（保持向后兼容）
func (s *ConnectionSession) HandlePacket(connPacket *StreamPacket) error {
	utils.Debugf("Handling packet for connection: %s, type: %v",
		connPacket.ConnectionID, connPacket.Packet.PacketType)

	// 处理心跳包
	if connPacket.Packet.PacketType.IsHeartbeat() {
		return s.handleHeartbeat(connPacket)
	}

	// 处理JSON命令包
	if connPacket.Packet.PacketType.IsJsonCommand() && connPacket.Packet.CommandPacket != nil {
		return s.handleCommandPacket(connPacket)
	}

	// 处理其他类型的包
	utils.Warnf("Unsupported packet type for connection %s: %v",
		connPacket.ConnectionID, connPacket.Packet.PacketType)
	return nil
}

// GetConnection 获取连接信息（新接口）
func (s *ConnectionSession) GetConnection(connID string) (*Connection, bool) {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	conn, exists := s.connMap[connID]
	return conn, exists
}

// ListConnections 列出所有连接（新接口）
func (s *ConnectionSession) ListConnections() []*Connection {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	connections := make([]*Connection, 0, len(s.connMap))
	for _, conn := range s.connMap {
		connections = append(connections, conn)
	}
	return connections
}

// UpdateConnectionState 更新连接状态（新接口）
func (s *ConnectionSession) UpdateConnectionState(connID string, state ConnectionState) error {
	s.connLock.Lock()
	defer s.connLock.Unlock()

	conn, exists := s.connMap[connID]
	if !exists {
		return fmt.Errorf("connection not found: %s", connID)
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
		return fmt.Errorf("connection not found: %s", connectionId)
	}

	// 更新状态为关闭中
	conn.State = common.StateClosing
	conn.UpdatedAt = time.Now()

	// 从流管理器中移除流
	if err := s.streamMgr.RemoveStream(connectionId); err != nil {
		utils.Warnf("Failed to remove stream from manager: %v", err)
	}

	// 从映射中删除
	delete(s.connMap, connectionId)

	utils.Infof("Connection closed: %s", connectionId)
	return nil
}

// GetStreamManager 获取流管理器
func (s *ConnectionSession) GetStreamManager() *stream.StreamManager {
	return s.streamMgr
}

// GetStreamConnectionInfo 获取连接信息（保持向后兼容）
func (s *ConnectionSession) GetStreamConnectionInfo(connectionId string) (*StreamConnection, bool) {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	conn, exists := s.connMap[connectionId]
	if !exists {
		return nil, false
	}

	return &StreamConnection{
		ID:     conn.ID,
		Stream: conn.Stream,
	}, true
}

// GetActiveConnections 获取活跃连接数量
func (s *ConnectionSession) GetActiveConnections() int {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	count := 0
	for _, conn := range s.connMap {
		if conn.State != common.StateClosed {
			count++
		}
	}
	return count
}

// handleHeartbeat 处理心跳包
func (s *ConnectionSession) handleHeartbeat(connPacket *StreamPacket) error {
	utils.Debugf("Processing heartbeat for connection: %s", connPacket.ConnectionID)

	// TODO: 实现心跳处理逻辑
	// - 更新连接状态
	// - 记录心跳时间
	// - 可选：发送心跳响应

	return nil
}

// handleCommandPacket 处理命令包
func (s *ConnectionSession) handleCommandPacket(connPacket *StreamPacket) error {
	utils.Infof("Processing command packet for connection: %s, command: %v",
		connPacket.ConnectionID, connPacket.Packet.CommandPacket.CommandType)

	// 根据命令类型分发处理
	switch connPacket.Packet.CommandPacket.CommandType {
	case packet.TcpMap:
		return s.handleTcpMapCommand(connPacket)
	case packet.HttpMap:
		return s.handleHttpMapCommand(connPacket)
	case packet.SocksMap:
		return s.handleSocksMapCommand(connPacket)
	case packet.DataIn:
		return s.handleDataInCommand(connPacket)
	case packet.Forward:
		return s.handleForwardCommand(connPacket)
	case packet.DataOut:
		return s.handleDataOutCommand(connPacket)
	case packet.Disconnect:
		return s.handleDisconnectCommand(connPacket)
	default:
		utils.Warnf("Unknown command type for connection %s: %v",
			connPacket.ConnectionID, connPacket.Packet.CommandPacket.CommandType)
		return nil
	}
}

// TODO: 实现各种命令处理函数
func (s *ConnectionSession) handleTcpMapCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle TCP mapping command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现TCP端口映射逻辑
	return nil
}

func (s *ConnectionSession) handleHttpMapCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle HTTP mapping command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现HTTP端口映射逻辑
	return nil
}

func (s *ConnectionSession) handleSocksMapCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle SOCKS mapping command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现SOCKS代理映射逻辑
	return nil
}

func (s *ConnectionSession) handleDataInCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle DataIn command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现数据输入处理逻辑
	return nil
}

func (s *ConnectionSession) handleForwardCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle Forward command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现服务端间转发逻辑
	return nil
}

func (s *ConnectionSession) handleDataOutCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle DataOut command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现数据输出处理逻辑
	return nil
}

func (s *ConnectionSession) handleDisconnectCommand(connPacket *StreamPacket) error {
	utils.Infof("TODO: Handle Disconnect command for connection: %s", connPacket.ConnectionID)
	// TODO: 实现连接断开处理逻辑
	return nil
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
