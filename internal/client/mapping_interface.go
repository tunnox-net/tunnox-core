package client

import (
	"context"

	"tunnox-core/internal/client/tunnel"
	"tunnox-core/internal/config"
)

// MappingHandler 映射处理器接口
type MappingHandler interface {
	Start() error
	Stop()
	GetConfig() config.MappingConfig
	GetContext() context.Context
	GetTunnelManager() tunnel.TunnelManager
}
