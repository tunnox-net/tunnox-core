package services

import (
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/constants"
)

// ============================================================================
// 辅助方法
// ============================================================================

// getDefaultClientConfig 获取默认客户端配置
func (s *clientService) getDefaultClientConfig() configs.ClientConfig {
	return configs.ClientConfig{
		EnableCompression: constants.DefaultEnableCompression,
		BandwidthLimit:    constants.DefaultClientBandwidthLimit,
		MaxConnections:    constants.DefaultClientMaxConnections,
		AllowedPorts:      constants.DefaultAllowedPorts,
		BlockedPorts:      constants.DefaultBlockedPorts,
		AutoReconnect:     constants.DefaultAutoReconnect,
		HeartbeatInterval: constants.DefaultHeartbeatInterval,
	}
}
