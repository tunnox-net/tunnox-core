package session

import (
corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/events"
)

// ============================================================================
// 事件处理
// ============================================================================

// handleDisconnectRequestEvent 处理断开连接请求事件
func (s *SessionManager) handleDisconnectRequestEvent(event events.Event) error {
	// 尝试类型断言为具体的断开连接事件类型
	// 这里简化处理
	corelog.Infof("Handling disconnect request event")

	// 由于无法从 event 获取数据，这里返回nil
	// 实际的断开连接逻辑应该在其他地方处理
	return nil
}

