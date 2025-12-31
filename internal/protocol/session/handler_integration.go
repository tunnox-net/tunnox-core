package session

import (
	"tunnox-core/internal/protocol/session/handler"
)

// ============================================================================
// Handler集成说明（子阶段4.2-4.5）
// ============================================================================

// 本文件用于标记handler包的集成状态
//
// 阶段说明：
// - 子阶段4.2: HandshakeHandler已创建（handler/handshake.go）✅
// - 子阶段4.3: TunnelOpenHandler待创建 ⏳
// - 子阶段4.4: TunnelBridgeHandler待创建 ⏳
// - 子阶段4.5: 其他Handlers待创建 ⏳
// - 子阶段4.6: SessionManager委托逻辑切换 ⏳
//
// 集成策略：
// 1. 子阶段4.2-4.5仅创建Handler实现，不修改SessionManager委托逻辑
// 2. 保留现有packet_handler_*.go文件，避免破坏性变更
// 3. 在子阶段4.6统一切换委托逻辑，移除旧代码
//
// 理由：
// - 降低风险：分阶段验证，避免一次性大规模重构
// - 保持稳定：现有功能不受影响
// - 便于回退：如果新架构有问题，可以快速回退

// ============================================================================
// 类型别名（用于未来的委托）
// ============================================================================

// HandshakeHandlerType Handler类型（待集成）
type HandshakeHandlerType = handler.HandshakeHandler

// ⚠️ 注意：
// SessionManager.handleHandshake 目前仍使用 packet_handler_handshake.go 中的实现
// 待子阶段4.6统一切换到 handler/handshake.go 的 HandshakeHandler
