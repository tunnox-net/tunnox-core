package protocols

import (
	"context"
	"fmt"

	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/protocol/session"
	coreErrors "tunnox-core/internal/core/errors"
)

// TCPProtocol TCP 协议实现
type TCPProtocol struct{}

// NewTCPProtocol 创建 TCP 协议
func NewTCPProtocol() *TCPProtocol {
	return &TCPProtocol{}
}

// Name 返回协议名称
func (p *TCPProtocol) Name() string {
	return "tcp"
}

// Dependencies 返回依赖服务
func (p *TCPProtocol) Dependencies() []string {
	return []string{"session_manager"}
}

// ValidateConfig 验证配置
func (p *TCPProtocol) ValidateConfig(config *registry.Config) error {
	if config.Port <= 0 || config.Port > 65535 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "TCP port must be in range [1, 65535], got %d", config.Port)
	}
	return nil
}

// Initialize 初始化协议
func (p *TCPProtocol) Initialize(ctx context.Context, container registry.Container, config *registry.Config) (adapter.Adapter, error) {
	// 1. 解析 SessionManager
	var sessionMgr *session.SessionManager
	if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to resolve session_manager")
	}

	// 2. 创建适配器（使用 dispose 体系的上下文）
	adapter := adapter.NewTcpAdapter(ctx, sessionMgr)

	// 3. 配置地址
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	adapter.SetAddr(addr)

	return adapter, nil
}

