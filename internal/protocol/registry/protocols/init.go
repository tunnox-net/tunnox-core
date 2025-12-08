package protocols

import (
	"tunnox-core/internal/protocol/registry"
)

var (
	globalRegistry *registry.Registry
)

// GetGlobalRegistry 获取全局协议注册表
func GetGlobalRegistry() *registry.Registry {
	if globalRegistry == nil {
		globalRegistry = registry.NewRegistry()
		registerAll(globalRegistry)
	}
	return globalRegistry
}

// registerAll 注册所有协议
func registerAll(r *registry.Registry) {
	_ = r.Register(NewTCPProtocol())
	_ = r.Register(NewUDPProtocol())
	_ = r.Register(NewWebSocketProtocol())
	_ = r.Register(NewQUICProtocol())
	_ = r.Register(NewHTTPPollProtocol())
}

