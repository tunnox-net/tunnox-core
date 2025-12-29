package session

import (
	"fmt"
	"net"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// handleExistingBridge 处理已有bridge的隧道连接（目标端连接或源端重连）
func (s *SessionManager) handleExistingBridge(
	connPacket *types.StreamPacket,
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	bridge *TunnelBridge,
) error {
	// ✅ 隧道连接（有 MappingID）不应该被注册为控制连接
	// 由于现在在 Handshake 中已经通过 ConnectionType 识别，这里只需要清理可能的误注册
	if req.MappingID != "" {
		s.controlConnLock.Lock()
		if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
			// 如果这个连接被错误注册为控制连接，移除它
			delete(s.controlConnMap, connPacket.ConnectionID)
			if controlConn.IsAuthenticated() && controlConn.GetClientID() > 0 {
				// 如果 clientIDIndexMap 中也指向这个连接，移除它
				if currentControlConn, exists := s.clientIDIndexMap[controlConn.ClientID]; exists && currentControlConn.ConnID == connPacket.ConnectionID {
					delete(s.clientIDIndexMap, controlConn.ClientID)
				}
			}
		}
		s.controlConnLock.Unlock()
	}

	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	netConn := s.extractNetConn(conn)
	// 如果无法提取 net.Conn，尝试从 Stream 创建数据转发器（通过接口抽象）
	if netConn == nil && conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		writer := conn.Stream.GetWriter()
		if reader == nil || writer == nil {
			// 该协议不支持桥接（如 HTTP 长轮询），数据已通过协议本身传输
		}
	}

	// ✅ 判断是源端还是目标端连接，更新对应的连接
	// 通过 cloudControl 获取映射配置，判断 clientID 是源端还是目标端
	netConn = s.extractNetConn(conn)
	var isSourceClient bool
	if s.cloudControl != nil && req.MappingID != "" {
		mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
		if err == nil {
			// 从连接中获取 clientID（使用 extractClientID 函数，支持多种方式）
			connClientID := extractClientID(conn.Stream, netConn)
			// 如果 extractClientID 返回 0，稍后从控制连接获取（clientConn 在后面定义）
			isSourceClient = (connClientID == mapping.ListenClientID)
		}
	}

	// 创建统一接口连接
	clientID := extractClientID(conn.Stream, netConn)
	tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)

	if isSourceClient {
		// 源端重连，更新 sourceConn
		bridge.SetSourceConnection(tunnelConn)
	} else {
		// 目标端连接
		bridge.SetTargetConnection(tunnelConn)
	}

	// ✅ 切换到流模式（通过接口调用，协议无关）
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if streamModeConn, ok := reader.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			streamModeConn.SetStreamMode(true)
		}
	}

	// ✅ 判断是否应该保留在 connMap（通过接口判断，协议无关）
	shouldKeep := false
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if keepConn, ok := reader.(interface {
			ShouldKeepInConnMap() bool
		}); ok {
			shouldKeep = keepConn.ShouldKeepInConnMap()
		}
	}

	if !shouldKeep && req.MappingID != "" {
		s.connLock.Lock()
		delete(s.connMap, connPacket.ConnectionID)
		s.connLock.Unlock()
	}

	return fmt.Errorf("tunnel connected to existing bridge, switching to stream mode")
}

// handleSourceBridge 处理源端连接：创建新的bridge
func (s *SessionManager) handleSourceBridge(
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	netConn net.Conn,
) error {
	// 源端连接：创建新的bridge
	var sourceConn net.Conn
	var sourceStream stream.PackageStreamer
	if conn != nil {
		sourceConn = netConn // 可能为 nil（某些协议不支持 net.Conn）
		sourceStream = conn.Stream
		// 如果 net.Conn 为 nil，尝试从 Stream 创建数据转发器（通过接口抽象）
		if netConn == nil && sourceStream != nil {
			reader := sourceStream.GetReader()
			writer := sourceStream.GetWriter()
			if reader == nil || writer == nil {
				// 该协议不支持桥接（如 HTTP 长轮询），数据已通过协议本身传输
			}
		}
	}

	// ✅ 切换到流模式（通过接口调用，协议无关）
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if streamModeConn, ok := reader.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			streamModeConn.SetStreamMode(true)
		}
	}

	if err := s.startSourceBridge(req, sourceConn, sourceStream); err != nil {
		corelog.Errorf("Tunnel[%s]: failed to start bridge: %v", req.TunnelID, err)
		return err
	}
	return nil
}

// handleTargetBridge 处理目标端连接：查找已存在的bridge并设置target连接
func (s *SessionManager) handleTargetBridge(
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	netConn net.Conn,
) error {
	s.bridgeLock.RLock()
	bridge, exists := s.tunnelBridges[req.TunnelID]
	s.bridgeLock.RUnlock()

	if !exists {
		// 本地未找到 Bridge，尝试跨节点转发
		if err := s.handleCrossNodeTargetConnection(req, conn, netConn); err != nil {
			corelog.Errorf("Tunnel[%s]: cross-node forwarding failed: %v", req.TunnelID, err)
			return fmt.Errorf("bridge not found for tunnel %s: %w", req.TunnelID, err)
		}
		return nil
	}

	// 创建统一接口连接
	clientID := extractClientID(conn.Stream, netConn)
	tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)

	// 设置目标端连接
	bridge.SetTargetConnection(tunnelConn)

	// ✅ 切换到流模式（通过接口调用，协议无关）
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if streamModeConn, ok := reader.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			streamModeConn.SetStreamMode(true)
		}
	}
	return nil
}

// cleanupTunnelFromControlConn 清理控制连接中的隧道引用
func (s *SessionManager) cleanupTunnelFromControlConn(
	connPacket *types.StreamPacket,
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
) {
	if req.MappingID == "" {
		return
	}

	s.controlConnLock.Lock()
	if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
		delete(s.controlConnMap, connPacket.ConnectionID)
		if controlConn.IsAuthenticated() && controlConn.GetClientID() > 0 {
			if currentControlConn, exists := s.clientIDIndexMap[controlConn.GetClientID()]; exists && currentControlConn.GetConnID() == connPacket.ConnectionID {
				delete(s.clientIDIndexMap, controlConn.GetClientID())
			}
		}
	}
	s.controlConnLock.Unlock()

	// ✅ 判断是否应该保留在 connMap（通过接口判断，协议无关）
	shouldKeep := false
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if keepConn, ok := reader.(interface {
			ShouldKeepInConnMap() bool
		}); ok {
			shouldKeep = keepConn.ShouldKeepInConnMap()
		}
	}

	if !shouldKeep && req.MappingID != "" {
		s.connLock.Lock()
		delete(s.connMap, connPacket.ConnectionID)
		s.connLock.Unlock()
	}
}
