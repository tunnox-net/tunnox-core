// Package config 提供通用配置管理框架
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// 泛型版本接口（推荐使用）
// ============================================================================

// TypedLoader 泛型配置加载器接口
type TypedLoader[T any] interface {
	Load(path string) (T, error)
}

// TypedValidator 泛型配置验证器接口
type TypedValidator[T any] interface {
	Validate(config T) error
}

// TypedExporter 泛型配置导出器接口
type TypedExporter[T any] interface {
	Export(config T, path string, options ExportOptions) error
}

// TypedManager 泛型配置管理器接口
type TypedManager[T any] interface {
	Load(path string) (T, error)
	Validate(config T) error
	Export(config T, path string, options ExportOptions) error
}

// ============================================================================
// 泛型版本实现
// ============================================================================

// TypedConfigLoader 泛型配置加载器实现
type TypedConfigLoader[T any] struct {
	// DefaultsProvider 提供默认配置（必须返回指针类型以便 yaml.Unmarshal 修改）
	DefaultsProvider func() *T
	// EnvOverrider 环境变量覆盖
	EnvOverrider func(*T) error
}

// Load 加载配置文件
func (l *TypedConfigLoader[T]) Load(path string) (T, error) {
	var zero T

	// 如果文件不存在，返回默认配置
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if l.DefaultsProvider != nil {
			configPtr := l.DefaultsProvider()
			if l.EnvOverrider != nil {
				if err := l.EnvOverrider(configPtr); err != nil {
					return zero, fmt.Errorf("failed to apply env overrides: %w", err)
				}
			}
			return *configPtr, nil
		}
		return zero, fmt.Errorf("config file not found and no defaults provider: %s", path)
	}

	// 读取文件
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("failed to read config file: %w", err)
	}

	// 先获取默认配置
	if l.DefaultsProvider == nil {
		return zero, fmt.Errorf("defaults provider is required")
	}
	configPtr := l.DefaultsProvider()

	// 解析 YAML 到配置对象
	if err := yaml.Unmarshal(data, configPtr); err != nil {
		return zero, fmt.Errorf("failed to parse config: %w", err)
	}

	// 应用环境变量覆盖
	if l.EnvOverrider != nil {
		if err := l.EnvOverrider(configPtr); err != nil {
			return zero, fmt.Errorf("failed to apply env overrides: %w", err)
		}
	}

	return *configPtr, nil
}

// TypedConfigExporter 泛型配置导出器实现
type TypedConfigExporter[T any] struct {
	// DefaultsProvider 提供默认配置用于对比
	DefaultsProvider func() *T
	// TemplateProvider 提供配置模板
	TemplateProvider func() string
}

// Export 导出配置到文件
func (e *TypedConfigExporter[T]) Export(config T, path string, options ExportOptions) error {
	var data []byte
	var err error

	if options.Template && e.TemplateProvider != nil {
		// 导出模板
		data = []byte(e.TemplateProvider())
	} else {
		// 导出配置（完整或最小）
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
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
		_ = os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// TypedConfigManager 泛型配置管理器实现
type TypedConfigManager[T any] struct {
	loader    TypedLoader[T]
	validator TypedValidator[T]
	exporter  TypedExporter[T]
}

// NewTypedConfigManager 创建泛型配置管理器
func NewTypedConfigManager[T any](
	loader TypedLoader[T],
	validator TypedValidator[T],
	exporter TypedExporter[T],
) *TypedConfigManager[T] {
	return &TypedConfigManager[T]{
		loader:    loader,
		validator: validator,
		exporter:  exporter,
	}
}

// Load 加载并验证配置
func (m *TypedConfigManager[T]) Load(path string) (T, error) {
	var zero T
	config, err := m.loader.Load(path)
	if err != nil {
		return zero, err
	}
	if m.validator != nil {
		if err := m.validator.Validate(config); err != nil {
			return zero, fmt.Errorf("config validation failed: %w", err)
		}
	}
	return config, nil
}

// Validate 验证配置
func (m *TypedConfigManager[T]) Validate(config T) error {
	if m.validator == nil {
		return nil
	}
	return m.validator.Validate(config)
}

// Export 导出配置到文件
func (m *TypedConfigManager[T]) Export(config T, path string, options ExportOptions) error {
	if m.exporter == nil {
		return fmt.Errorf("exporter not configured")
	}
	return m.exporter.Export(config, path, options)
}

// ============================================================================
// 便捷函数
// ============================================================================

// ValidatorFunc 验证函数类型适配器
type ValidatorFunc[T any] func(T) error

// Validate 实现 TypedValidator 接口
func (f ValidatorFunc[T]) Validate(config T) error {
	return f(config)
}

// ============================================================================
// 旧版接口（保持向后兼容，已废弃）
// Deprecated: 请使用泛型版本 TypedManager, TypedLoader 等
// ============================================================================

// LegacyManager 配置管理器接口
// Deprecated: 请使用新的 config.Manager 结构体
type LegacyManager interface {
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
// Deprecated: 请使用 TypedConfigLoader[T]
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
// Deprecated: 请使用 TypedConfigExporter[T]
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
