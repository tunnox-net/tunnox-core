package session

import (
	"encoding/json"
	"fmt"
	"net"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"

	"tunnox-core/internal/packet"
)

// handleTunnelOpen 处理隧道打开请求
// 这个方法处理两种情况：
// 1. 源端客户端发起的隧道连接（需要创建bridge并通知目标端）
// 2. 目标端客户端响应的隧道连接（连接到已有的bridge）
//
// 具体的分支处理已移至 packet_handler_tunnel_bridge.go:
// - handleExistingBridge: 处理已有bridge的连接
// - handleSourceBridge: 处理源端连接
// - handleTargetBridge: 处理目标端连接
// - cleanupTunnelFromControlConn: 清理控制连接引用
func (s *SessionManager) handleTunnelOpen(connPacket *types.StreamPacket) error {
	if s.tunnelHandler == nil {
		return fmt.Errorf("tunnel handler not configured")
	}

	// 获取底层连接
	conn := s.getConnectionByConnID(connPacket.ConnectionID)
	if conn == nil {
		return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
	}

	// 解析隧道打开请求（从 Payload）
	req := &packet.TunnelOpenRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			corelog.Errorf("Failed to parse tunnel open request: %v", err)
			s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
				TunnelID: "",
				Success:  false,
				Error:    fmt.Sprintf("invalid tunnel open request format: %v", err),
			})
			return fmt.Errorf("invalid tunnel open request format: %w", err)
		}
	}

	// 设置 mappingID
	s.setMappingIDOnConnection(conn, req.MappingID)

	// 检查是否已有bridge（目标端连接或源端重连）
	s.bridgeLock.Lock()
	bridge, exists := s.tunnelBridges[req.TunnelID]
	s.bridgeLock.Unlock()

	if exists {
		return s.handleExistingBridge(connPacket, conn, req, bridge)
	}

	// 检查跨节点路由
	if s.tunnelRouting != nil {
		_, err := s.tunnelRouting.LookupWaitingTunnel(s.Ctx(), req.TunnelID)
		if err == nil {
			netConn := s.extractNetConn(conn)
			return s.handleCrossNodeTargetConnection(req, conn, netConn)
		} else if err != ErrTunnelNotFound && err != ErrTunnelExpired {
			corelog.Errorf("Tunnel[%s]: failed to lookup routing state: %v", req.TunnelID, err)
		}
	}

	// 获取或创建控制连接
	clientConn := s.findOrCreateControlConnection(connPacket, conn, req)
	if clientConn == nil {
		return fmt.Errorf("control connection not found: %s", connPacket.ConnectionID)
	}

	// 调用隧道处理器
	if err := s.tunnelHandler.HandleTunnelOpen(clientConn, req); err != nil {
		corelog.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    err.Error(),
		})
		return err
	}

	// 清理控制连接映射
	s.removeFromControlConnMap(connPacket.ConnectionID, clientConn)

	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	// 设置映射ID
	s.setMappingIDAfterAuth(conn, req.MappingID, clientConn)

	// 处理源端/目标端连接
	netConn := s.extractNetConn(conn)
	isSourceClient := s.isSourceClient(conn, req, clientConn, netConn)

	if isSourceClient {
		if err := s.handleSourceBridge(conn, req, netConn); err != nil {
			return err
		}
	} else {
		if err := s.handleTargetBridge(conn, req, netConn); err != nil {
			return err
		}
	}

	// 清理
	s.cleanupTunnelFromControlConn(connPacket, conn, req)

	return fmt.Errorf("tunnel source connected, switching to stream mode")
}

// setMappingIDOnConnection 设置连接的 mappingID
func (s *SessionManager) setMappingIDOnConnection(conn *types.Connection, mappingID string) {
	if mappingID == "" || conn == nil || conn.Stream == nil {
		return
	}
	reader := conn.Stream.GetReader()
	if mappingConn, ok := reader.(interface {
		GetClientID() int64
		SetMappingID(mappingID string)
	}); ok {
		clientID := mappingConn.GetClientID()
		if clientID > 0 {
			mappingConn.SetMappingID(mappingID)
		}
	}
}

// findOrCreateControlConnection 查找或创建控制连接
func (s *SessionManager) findOrCreateControlConnection(
	connPacket *types.StreamPacket,
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
) ControlConnectionInterface {
	clientConn := s.getControlConnectionByConnID(connPacket.ConnectionID)
	if clientConn != nil {
		return clientConn
	}

	if conn == nil || conn.Stream == nil {
		corelog.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
		return nil
	}

	// 尝试从 Stream 直接获取 clientID
	var clientID int64
	if streamWithClientID, ok := conn.Stream.(interface {
		GetClientID() int64
	}); ok {
		clientID = streamWithClientID.GetClientID()
	} else {
		type streamProcessorGetter interface {
			GetStreamProcessor() interface {
				GetClientID() int64
				GetConnectionID() string
				GetMappingID() string
			}
		}
		if adapter, ok := conn.Stream.(streamProcessorGetter); ok {
			streamProc := adapter.GetStreamProcessor()
			if streamProc != nil {
				clientID = streamProc.GetClientID()
			}
		}
	}

	// 如果获取到 clientID，尝试通过 clientID 查找控制连接
	if clientID > 0 {
		clientConn = s.GetControlConnectionByClientID(clientID)
		if clientConn != nil {
			return clientConn
		}
	}

	// 尝试创建临时控制连接
	reader := conn.Stream.GetReader()
	if tempConn, ok := reader.(interface {
		CanCreateTemporaryControlConn() bool
		GetClientID() int64
	}); ok && tempConn.CanCreateTemporaryControlConn() {
		var remoteAddr net.Addr
		if conn.RawConn != nil {
			remoteAddr = conn.RawConn.RemoteAddr()
		}
		protocol := conn.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		newConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, protocol)
		if clientID > 0 {
			newConn.SetClientID(clientID)
			newConn.SetAuthenticated(true)
		} else {
			tempClientID := tempConn.GetClientID()
			if tempClientID > 0 {
				newConn.SetClientID(tempClientID)
				newConn.SetAuthenticated(true)
			}
		}
		return newConn
	}

	corelog.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  false,
		Error:    "connection not found or not authenticated",
	})
	return nil
}

// removeFromControlConnMap 从控制连接映射中移除
func (s *SessionManager) removeFromControlConnMap(connID string, clientConn ControlConnectionInterface) {
	s.controlConnLock.Lock()
	defer s.controlConnLock.Unlock()
	if _, exists := s.controlConnMap[connID]; exists {
		delete(s.controlConnMap, connID)
		if clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
			if currentControlConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]; exists && currentControlConn.GetConnID() == connID {
				delete(s.clientIDIndexMap, clientConn.GetClientID())
			}
		}
	}
}

// setMappingIDAfterAuth 认证后设置映射ID
func (s *SessionManager) setMappingIDAfterAuth(conn *types.Connection, mappingID string, clientConn ControlConnectionInterface) {
	if mappingID == "" || !clientConn.IsAuthenticated() || clientConn.GetClientID() <= 0 {
		return
	}
	if conn == nil || conn.Stream == nil {
		return
	}
	reader := conn.Stream.GetReader()
	if mappingConn, ok := reader.(interface {
		SetMappingID(mappingID string)
	}); ok {
		mappingConn.SetMappingID(mappingID)
	}
}

// isSourceClient 判断是否为源端客户端
func (s *SessionManager) isSourceClient(
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	clientConn ControlConnectionInterface,
	netConn net.Conn,
) bool {
	if s.cloudControl == nil || req.MappingID == "" {
		return false
	}
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		return false
	}
	connClientID := extractClientID(conn.Stream, netConn)
	if connClientID == 0 && clientConn != nil && clientConn.IsAuthenticated() {
		connClientID = clientConn.GetClientID()
	}
	return connClientID == mapping.ListenClientID
}

// sendTunnelOpenResponse, sendTunnelOpenResponseDirect, notifyTargetClientToOpenTunnel
// 已移至 packet_handler_tunnel_ops.go
//
// handleExistingBridge, handleSourceBridge, handleTargetBridge, cleanupTunnelFromControlConn
// 已移至 packet_handler_tunnel_bridge.go
