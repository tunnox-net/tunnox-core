package server

import (
	"tunnox-core/internal/api"
	"tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/core/storage"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	// 节点信息
	NodeID   string `yaml:"node_id"`
	NodeName string `yaml:"node_name"`
	BindAddr string `yaml:"bind_addr"` // 服务监听地址
	
	// 核心组件配置（运行时注入）
	CloudControl   *managers.CloudControl
	Storage        storage.Storage
	MessageBroker  broker.MessageBroker
	BridgeManager  *bridge.BridgeManager
	
	// Management API 配置
	ManagementAPI  *api.APIConfig `yaml:"management_api"`
	
	// 可选配置
	EnableBridge bool `yaml:"enable_bridge"`
}

