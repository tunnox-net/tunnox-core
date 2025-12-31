package session

import "tunnox-core/internal/protocol/session/handler"

// ============================================================================
// 临时类型别名（等待阶段四 core 重构后移除）
// ============================================================================

// PacketHandler 数据包处理器接口（临时别名）
type PacketHandler = handler.PacketHandler

// PacketHandlerFunc 数据包处理函数类型（临时别名）
type PacketHandlerFunc = handler.PacketHandlerFunc

// PacketRouter 数据包路由器（临时别名）
type PacketRouter = handler.PacketRouter

// PacketRouterConfig 数据包路由器配置（临时别名）
type PacketRouterConfig = handler.PacketRouterConfig

// NewPacketRouter 创建数据包路由器（临时别名）
var NewPacketRouter = handler.NewPacketRouter
