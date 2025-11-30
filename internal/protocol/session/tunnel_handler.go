package session

import (
	"tunnox-core/internal/packet"
)

// TunnelHandler 隧道处理器接口（避免循环依赖）
type TunnelHandler interface {
	HandleTunnelOpen(conn ControlConnectionInterface, req *packet.TunnelOpenRequest) error
	// ✅ HandleTunnelData 和 HandleTunnelClose 已删除
	// 前置包后直接 io.Copy，不再有数据包
}

// AuthHandler 认证处理器接口
type AuthHandler interface {
	HandleHandshake(conn ControlConnectionInterface, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error)
	GetClientConfig(conn ControlConnectionInterface) (string, error)
}

// ============================================================================
// 接口定义（本文件仅包含接口定义）
// ============================================================================
//
// 注：
// - SetTunnelHandler 和 SetAuthHandler 已移至 manager.go
// - handleHandshake 和 handleTunnelOpen 已移至 packet_handler.go
// - getOrCreateClientConnection 和 getClientConnection 已移至 connection_lifecycle.go
// ============================================================================
