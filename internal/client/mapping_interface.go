package client

import "context"

// MappingHandlerInterface 映射处理器接口
type MappingHandlerInterface interface {
	Start() error
	Stop()
	GetConfig() MappingConfig
	GetContext() context.Context
}

