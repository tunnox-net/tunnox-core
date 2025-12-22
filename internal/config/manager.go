// Package config 提供通用配置管理框架
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manager 配置管理器接口
type Manager interface {
	// Load 加载配置
	Load(path string) (interface{}, error)
	// Validate 验证配置
	Validate(config interface{}) error
	// Export 导出配置
	Export(config interface{}, path string, options ExportOptions) error
}

// ExportOptions 导出选项
type ExportOptions struct {
	// Minimal 只导出非默认值
	Minimal bool
	// WithComments 包含注释
	WithComments bool
	// Template 导出模板而不是实际配置
	Template bool
}

// Loader 配置加载器
type Loader struct {
	// DefaultsProvider 提供默认配置
	DefaultsProvider func() interface{}
	// EnvOverrider 环境变量覆盖
	EnvOverrider func(interface{}) error
}

// Load 加载配置文件
func (l *Loader) Load(path string) (interface{}, error) {
	// 如果文件不存在,返回默认配置
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if l.DefaultsProvider != nil {
			config := l.DefaultsProvider()
			if l.EnvOverrider != nil {
				if err := l.EnvOverrider(config); err != nil {
					return nil, fmt.Errorf("failed to apply env overrides: %w", err)
				}
			}
			return config, nil
		}
		return nil, fmt.Errorf("config file not found and no defaults provider: %s", path)
	}

	// 读取文件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 先获取默认配置
	config := l.DefaultsProvider()

	// 解析 YAML 到配置对象
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 应用环境变量覆盖
	if l.EnvOverrider != nil {
		if err := l.EnvOverrider(config); err != nil {
			return nil, fmt.Errorf("failed to apply env overrides: %w", err)
		}
	}

	return config, nil
}

// Exporter 配置导出器
type Exporter struct {
	// DefaultsProvider 提供默认配置用于对比
	DefaultsProvider func() interface{}
	// TemplateProvider 提供配置模板
	TemplateProvider func() string
}

// Export 导出配置到文件
func (e *Exporter) Export(config interface{}, path string, options ExportOptions) error {
	var data []byte
	var err error

	if options.Template && e.TemplateProvider != nil {
		// 导出模板
		data = []byte(e.TemplateProvider())
	} else if options.Minimal && e.DefaultsProvider != nil {
		// 导出最小配置(只包含非默认值)
		// 这里简化实现,实际可以用反射对比差异
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
	} else {
		// 导出完整配置
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
	}

	// 确保目录存在
	dir := path[:len(path)-len(path[len(path)-1:])]
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// 原子写入
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
