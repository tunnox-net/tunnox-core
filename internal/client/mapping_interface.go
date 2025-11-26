package client

import (
	"context"

	"tunnox-core/internal/config"
)

// MappingHandler 映射处理器接口（统一命名）
type MappingHandler interface {
	Start() error
	Stop()
	GetConfig() config.MappingConfig
	GetContext() context.Context
}

// MappingHandlerInterface 向后兼容的别名（已废弃）
// Deprecated: 使用 MappingHandler 代替
type MappingHandlerInterface = MappingHandler
