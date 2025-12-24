package session

import (
	"fmt"
	"sync"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// PacketHandler 数据包处理器接口
type PacketHandler interface {
	HandlePacket(connPacket *types.StreamPacket) error
}

// PacketHandlerFunc 数据包处理函数类型
type PacketHandlerFunc func(connPacket *types.StreamPacket) error

// HandlePacket 实现 PacketHandler 接口
func (f PacketHandlerFunc) HandlePacket(connPacket *types.StreamPacket) error {
	return f(connPacket)
}

// PacketRouter 数据包路由器
// 负责根据数据包类型分发到对应的处理器
type PacketRouter struct {
	// 处理器映射
	handlers map[packet.Type]PacketHandler
	mu       sync.RWMutex

	// 默认处理器（处理未注册的包类型）
	defaultHandler PacketHandler

	// 日志
	logger corelog.Logger
}

// PacketRouterConfig 数据包路由器配置
type PacketRouterConfig struct {
	Logger         corelog.Logger
	DefaultHandler PacketHandler
}

// NewPacketRouter 创建数据包路由器
func NewPacketRouter(config *PacketRouterConfig) *PacketRouter {
	if config == nil {
		config = &PacketRouterConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &PacketRouter{
		handlers:       make(map[packet.Type]PacketHandler),
		defaultHandler: config.DefaultHandler,
		logger:         logger,
	}
}

// RegisterHandler 注册数据包处理器
func (r *PacketRouter) RegisterHandler(packetType packet.Type, handler PacketHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers[packetType] = handler
}

// RegisterHandlerFunc 注册数据包处理函数
func (r *PacketRouter) RegisterHandlerFunc(packetType packet.Type, handler PacketHandlerFunc) {
	r.RegisterHandler(packetType, handler)
}

// UnregisterHandler 注销数据包处理器
func (r *PacketRouter) UnregisterHandler(packetType packet.Type) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.handlers, packetType)
}

// SetDefaultHandler 设置默认处理器
func (r *PacketRouter) SetDefaultHandler(handler PacketHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultHandler = handler
}

// Route 路由数据包到对应的处理器
func (r *PacketRouter) Route(connPacket *types.StreamPacket) error {
	if connPacket == nil || connPacket.Packet == nil {
		return fmt.Errorf("invalid packet: nil")
	}

	packetType := connPacket.Packet.PacketType

	// 获取基础包类型（忽略压缩/加密标志）
	baseType := packetType & 0x3F

	r.mu.RLock()
	handler, exists := r.handlers[baseType]
	defaultHandler := r.defaultHandler
	r.mu.RUnlock()

	if exists {
		return handler.HandlePacket(connPacket)
	}

	// 尝试使用默认处理器
	if defaultHandler != nil {
		return defaultHandler.HandlePacket(connPacket)
	}

	r.logger.Warnf("PacketRouter: no handler for packet type %v", packetType)
	return fmt.Errorf("no handler for packet type: %v", packetType)
}

// RouteByCategory 根据包类型分类路由
// 这是一个便捷方法，用于处理常见的包类型分类
func (r *PacketRouter) RouteByCategory(connPacket *types.StreamPacket,
	commandHandler, handshakeHandler, tunnelHandler, heartbeatHandler PacketHandler) error {

	if connPacket == nil || connPacket.Packet == nil {
		return fmt.Errorf("invalid packet: nil")
	}

	packetType := connPacket.Packet.PacketType

	// 根据数据包类型分发
	switch {
	case packetType.IsJsonCommand() || packetType.IsCommandResp():
		if commandHandler != nil {
			return commandHandler.HandlePacket(connPacket)
		}

	case packetType&0x3F == packet.Handshake:
		if handshakeHandler != nil {
			return handshakeHandler.HandlePacket(connPacket)
		}

	case packetType&0x3F == packet.TunnelOpen:
		if tunnelHandler != nil {
			return tunnelHandler.HandlePacket(connPacket)
		}

	case packetType.IsHeartbeat():
		if heartbeatHandler != nil {
			return heartbeatHandler.HandlePacket(connPacket)
		}
	}

	// 回退到默认路由
	return r.Route(connPacket)
}
