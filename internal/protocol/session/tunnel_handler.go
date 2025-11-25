package session

import (
	"encoding/json"
	"fmt"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// TunnelHandler 隧道处理器接口（避免循环依赖）
type TunnelHandler interface {
	HandleTunnelOpen(conn *ClientConnection, req *packet.TunnelOpenRequest) error
	// ✅ HandleTunnelData 和 HandleTunnelClose 已删除
	// 前置包后直接 io.Copy，不再有数据包
}

// AuthHandler 认证处理器接口
type AuthHandler interface {
	HandleHandshake(conn *ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error)
}

// SetTunnelHandler 设置隧道处理器
func (s *SessionManager) SetTunnelHandler(handler TunnelHandler) {
	s.tunnelHandler = handler
	utils.Infof("SessionManager: tunnel handler set")
}

// SetAuthHandler 设置认证处理器
func (s *SessionManager) SetAuthHandler(handler AuthHandler) {
	s.authHandler = handler
	utils.Infof("SessionManager: auth handler set")
}

// handleTunnelPacket 处理隧道数据包
func (s *SessionManager) handleTunnelPacket(connPacket *types.StreamPacket) error {
	packetType := connPacket.Packet.PacketType & 0x3F // 去除压缩/加密标志
	
	switch packetType {
	case packet.Handshake:
		return s.handleHandshake(connPacket)
		
	case packet.TunnelOpen:
		return s.handleTunnelOpen(connPacket)
		
	case packet.TunnelData, packet.TunnelClose:
		// ✅ TunnelData 和 TunnelClose 包不再处理
		// 前置包（TunnelOpen）后，连接已切换到裸连接模式（io.Copy）
		utils.Warnf("SessionManager: received %v packet after tunnel established (should not happen)", packetType)
		return nil
		
	default:
		return fmt.Errorf("unknown tunnel packet type: %v", packetType)
	}
}

// handleHandshake 处理握手包
func (s *SessionManager) handleHandshake(connPacket *types.StreamPacket) error {
	if s.authHandler == nil {
		return fmt.Errorf("auth handler not set")
	}
	
	// 解析握手请求
	var req packet.HandshakeRequest
	if err := json.Unmarshal(connPacket.Packet.Payload, &req); err != nil {
		return fmt.Errorf("failed to unmarshal handshake request: %w", err)
	}
	
	// 获取或创建 ClientConnection
	conn := s.getOrCreateClientConnection(connPacket.ConnectionID, connPacket.Packet)
	
	// 调用认证处理器
	resp, err := s.authHandler.HandleHandshake(conn, &req)
	if err != nil {
		utils.Errorf("Handshake failed for connection %s: %v", connPacket.ConnectionID, err)
		
		// 发送失败响应
		resp = &packet.HandshakeResponse{
			Success: false,
			Error:   err.Error(),
		}
	}
	
	// 发送响应
	respData, _ := json.Marshal(resp)
	respPacket := &packet.TransferPacket{
		PacketType: packet.HandshakeResp,
		Payload:    respData,
	}
	
	_, err = conn.Stream.WritePacket(respPacket, false, 0)
	return err
}

// handleTunnelOpen 处理隧道打开
func (s *SessionManager) handleTunnelOpen(connPacket *types.StreamPacket) error {
	if s.tunnelHandler == nil {
		return fmt.Errorf("tunnel handler not set")
	}
	
	// 解析隧道打开请求
	var req packet.TunnelOpenRequest
	if err := json.Unmarshal(connPacket.Packet.Payload, &req); err != nil {
		return fmt.Errorf("failed to unmarshal tunnel open request: %w", err)
	}
	
	// 获取 ClientConnection
	conn := s.getClientConnection(connPacket.ConnectionID)
	if conn == nil {
		return fmt.Errorf("client connection not found: %s", connPacket.ConnectionID)
	}
	
	// 调用隧道处理器
	return s.tunnelHandler.HandleTunnelOpen(conn, &req)
}

// ✅ handleTunnelData 和 handleTunnelClose 已删除
// 原因：前置包后直接切换到裸连接模式，不再处理 TunnelData/TunnelClose 包

// getOrCreateClientConnection 获取或创建客户端连接
func (s *SessionManager) getOrCreateClientConnection(connID string, pkt *packet.TransferPacket) *ClientConnection {
	s.connLock.Lock()
	defer s.connLock.Unlock()
	
	// 尝试从现有连接中查找
	if baseConn, exists := s.connMap[connID]; exists {
		// 如果已有，尝试从扩展映射中获取
		if clientConn, ok := s.clientConnMap[connID]; ok {
			return clientConn
		}
		
		// 创建新的 ClientConnection 包装
		clientConn := &ClientConnection{
			ConnID:     connID,
			Stream:     baseConn.Stream,
			CreatedAt:  baseConn.CreatedAt,
			baseConn:   baseConn,
		}
		s.clientConnMap[connID] = clientConn
		return clientConn
	}
	
	// 全新连接（通常不应该走到这里）
	return nil
}

// getClientConnection 获取客户端连接
func (s *SessionManager) getClientConnection(connID string) *ClientConnection {
	s.connLock.RLock()
	defer s.connLock.RUnlock()
	
	return s.clientConnMap[connID]
}

