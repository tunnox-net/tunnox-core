package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// StreamPacket 包装结构，包含连接信息
type StreamPacket struct {
	ConnectionID string
	Packet       *packet.TransferPacket
	Timestamp    time.Time
}

// StreamConnectionInfo 连接信息
type StreamConnectionInfo struct {
	ID       string
	Stream   *stream.StreamProcessor
	Metadata map[string]interface{}
}

// Session 接口定义
type Session interface {
	// 初始化连接
	InitConnection(reader io.Reader, writer io.Writer) (*StreamConnectionInfo, error)

	// 处理带连接信息的数据包
	HandlePacket(packet *StreamPacket) error

	// 关闭连接
	CloseConnection(connectionId string) error
}

// ConnectionSession 实现 Session 接口
type ConnectionSession struct {
	connMap  map[string]*StreamConnectionInfo
	connLock sync.RWMutex
	idGen    *generators.ConnectionIDGenerator

	utils.Dispose
}

// NewConnectionSession 创建新的连接会话
func NewConnectionSession(parentCtx context.Context) *ConnectionSession {
	session := &ConnectionSession{
		connMap: make(map[string]*StreamConnectionInfo),
		idGen:   generators.NewConnectionIDGenerator(),
	}
	session.SetCtx(parentCtx, session.onClose)
	return session
}

// InitConnection 初始化连接
func (s *ConnectionSession) InitConnection(reader io.Reader, writer io.Writer) (*StreamConnectionInfo, error) {
	// 生成连接ID
	connID := s.idGen.GenerateID()

	// 创建数据流
	ps := stream.NewStreamProcessor(reader, writer, s.Ctx())

	// 创建连接信息
	connInfo := &StreamConnectionInfo{
		ID:       connID,
		Stream:   ps,
		Metadata: make(map[string]interface{}),
	}

	// 保存连接信息
	s.connLock.Lock()
	s.connMap[connID] = connInfo
	s.connLock.Unlock()

	utils.Infof("Connection initialized: %s", connID)
	return connInfo, nil
}

// HandlePacket 处理数据包
func (s *ConnectionSession) HandlePacket(connPacket *StreamPacket) error {
	utils.Debugf("Handling packet for connection: %s, type: %s",
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
	utils.Warnf("Unsupported packet type for connection %s: %s",
		connPacket.ConnectionID, connPacket.Packet.PacketType)
	return nil
}

// CloseConnection 关闭连接
func (s *ConnectionSession) CloseConnection(connectionId string) error {
	s.connLock.Lock()
	defer s.connLock.Unlock()

	connInfo, exists := s.connMap[connectionId]
	if !exists {
		return fmt.Errorf("connection not found: %s", connectionId)
	}

	// 关闭数据流
	if connInfo.Stream != nil {
		connInfo.Stream.Close()
	}

	// 从映射中删除
	delete(s.connMap, connectionId)

	utils.Infof("Connection closed: %s", connectionId)
	return nil
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
	utils.Infof("Processing command packet for connection: %s, command: %s",
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
		utils.Warnf("Unknown command type for connection %s: %s",
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
func (s *ConnectionSession) onClose() {
	utils.Infof("Cleaning up connection session resources...")

	// 获取所有连接信息的副本，避免在锁内调用 Close
	var connections []*StreamConnectionInfo
	s.connLock.Lock()
	for _, connInfo := range s.connMap {
		connections = append(connections, connInfo)
	}
	// 清空连接映射
	s.connMap = make(map[string]*StreamConnectionInfo)
	s.connLock.Unlock()

	// 在锁外记录清理的连接
	for _, connInfo := range connections {
		utils.Infof("Cleaned up connection: %s", connInfo.ID)
	}

	utils.Infof("Connection session resources cleanup completed")
}

// GetStreamConnectionInfo 获取连接信息（用于调试和监控）
func (s *ConnectionSession) GetStreamConnectionInfo(connectionId string) (*StreamConnectionInfo, bool) {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	connInfo, exists := s.connMap[connectionId]
	return connInfo, exists
}

// GetActiveConnections 获取活跃连接数量
func (s *ConnectionSession) GetActiveConnections() int {
	s.connLock.RLock()
	defer s.connLock.RUnlock()

	return len(s.connMap)
}
