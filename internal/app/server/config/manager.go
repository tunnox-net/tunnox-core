// Package config 服务端配置管理
package config

import (
	"tunnox-core/internal/app/server"
)

// LoadConfig 加载配置(兼容旧接口)
func LoadConfig(configPath string) (*server.Config, error) {
	return server.LoadConfig(configPath)
}

// ExportConfigTemplate 导出配置模板
func ExportConfigTemplate(path string) error {
	return server.ExportConfigTemplate(path)
}
