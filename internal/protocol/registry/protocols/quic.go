package protocols

import (
	"context"
	"fmt"

	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/protocol/session"
	coreErrors "tunnox-core/internal/core/errors"
)

// QUICProtocol QUIC 协议实现
type QUICProtocol struct{}

// NewQUICProtocol 创建 QUIC 协议
func NewQUICProtocol() *QUICProtocol {
	return &QUICProtocol{}
}

// Name 返回协议名称
func (p *QUICProtocol) Name() string {
	return "quic"
}

// Dependencies 返回依赖服务
func (p *QUICProtocol) Dependencies() []string {
	return []string{"session_manager"}
}

// ValidateConfig 验证配置（使用统一的验证接口）
func (p *QUICProtocol) ValidateConfig(config *registry.Config) error {
	// 先调用基础验证
	if err := config.Validate(); err != nil {
		return err
	}
	// QUIC 协议需要端口
	if config.Port <= 0 {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "QUIC port is required")
	}
	return nil
}

// Initialize 初始化协议
func (p *QUICProtocol) Initialize(ctx context.Context, container registry.Container, config *registry.Config) (adapter.Adapter, error) {
	var sessionMgr *session.SessionManager
	if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to resolve session_manager")
	}

	adapter := adapter.NewQuicAdapter(ctx, sessionMgr)
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	adapter.SetAddr(addr)

	return adapter, nil
}

