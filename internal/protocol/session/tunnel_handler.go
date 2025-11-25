package session

import (
	"tunnox-core/internal/packet"
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

// ============================================================================
// 辅助方法（供 packet_handler.go 使用）
// ============================================================================
//
// 注：SetTunnelHandler 和 SetAuthHandler 已移至 manager.go
// handleHandshake 和 handleTunnelOpen 已移至 packet_handler.go
// ============================================================================

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
			ConnID:    connID,
			Stream:    baseConn.Stream,
			CreatedAt: baseConn.CreatedAt,
			baseConn:  baseConn,
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
