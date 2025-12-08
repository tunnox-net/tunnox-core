package protocols

import (
	"context"
	"fmt"

	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/protocol/session"
	coreErrors "tunnox-core/internal/core/errors"
)

// WebSocketProtocol WebSocket 协议实现
type WebSocketProtocol struct{}

// NewWebSocketProtocol 创建 WebSocket 协议
func NewWebSocketProtocol() *WebSocketProtocol {
	return &WebSocketProtocol{}
}

// Name 返回协议名称
func (p *WebSocketProtocol) Name() string {
	return "websocket"
}

// Dependencies 返回依赖服务
func (p *WebSocketProtocol) Dependencies() []string {
	return []string{"session_manager"}
}

// ValidateConfig 验证配置
func (p *WebSocketProtocol) ValidateConfig(config *registry.Config) error {
	if config.Port <= 0 || config.Port > 65535 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "WebSocket port must be in range [1, 65535], got %d", config.Port)
	}
	return nil
}

// Initialize 初始化协议
func (p *WebSocketProtocol) Initialize(ctx context.Context, container registry.Container, config *registry.Config) (adapter.Adapter, error) {
	var sessionMgr *session.SessionManager
	if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to resolve session_manager")
	}

	adapter := adapter.NewWebSocketAdapter(ctx, sessionMgr)
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	adapter.SetAddr(addr)

	return adapter, nil
}

