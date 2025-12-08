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

// ValidateConfig 验证配置（使用统一的验证接口）
func (p *TCPProtocol) ValidateConfig(config *registry.Config) error {
	// 先调用基础验证
	if err := config.Validate(); err != nil {
		return err
	}
	// TCP 协议需要端口
	if config.Port <= 0 {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "TCP port is required")
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

