package api

import "tunnox-core/internal/config"

// ConfigPushData 配置推送数据结构
type ConfigPushData struct {
	Mappings []config.MappingConfig `json:"mappings"`
}

// ConfigRemovalData 配置移除数据结构
type ConfigRemovalData struct {
	Mappings       []config.MappingConfig `json:"mappings"`
	RemoveMappings []string               `json:"remove_mappings,omitempty"`
}

// KickClientInfo 踢下线信息
type KickClientInfo struct {
	Reason string `json:"reason"`
	Code   string `json:"code"`
}

