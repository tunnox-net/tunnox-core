package client

import (
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/validation"
)

// ClientConfig 客户端配置
type ClientConfig struct {
	// 注册客户端认证
	ClientID  int64  `yaml:"client_id"`
	AuthToken string `yaml:"auth_token"`

	// 匿名客户端认证
	Anonymous bool   `yaml:"anonymous"`
	DeviceID  string `yaml:"device_id"`
	SecretKey string `yaml:"secret_key"` // 匿名客户端的密钥（服务端分配后保存）

	Server struct {
		Address  string `yaml:"address"`  // 服务器地址，例如 "localhost:7000"
		Protocol string `yaml:"protocol"` // tcp/websocket/quic
	} `yaml:"server"`

	// 日志配置
	Log LogConfig `yaml:"log"`

	// 注意：映射配置由服务器通过指令连接动态推送，不在配置文件中
}

// Validate 验证客户端配置（使用统一的验证接口）
func (c *ClientConfig) Validate() error {
	result := &validation.ValidationResult{}

	// 验证认证配置
	if !c.Anonymous {
		if c.ClientID <= 0 {
			result.AddError(coreErrors.New(coreErrors.ErrorTypePermanent, "client_id is required for authenticated mode"))
		}
	} else {
		if c.DeviceID == "" {
			// 匿名模式下，如果没有 device_id，使用默认值（在调用方设置）
			// 这里只验证格式
		} else {
			if err := validation.ValidateNonEmptyString(c.DeviceID, "device_id"); err != nil {
				result.AddError(err)
			}
		}
	}

	// 验证服务器地址（如果提供）
	if c.Server.Address != "" {
		// 验证地址格式（支持 http:// 或 https:// 开头的 URL，也支持 host:port 格式）
		if err := validateServerAddress(c.Server.Address); err != nil {
			result.AddError(err)
		}
	}

	// 验证协议（如果提供）
	if c.Server.Protocol != "" {
		validProtocols := []string{"tcp", "websocket", "udp", "quic", "httppoll", "http-long-polling", "httplp"}
		if err := validation.ValidateStringInList(c.Server.Protocol, "server.protocol", validProtocols); err != nil {
			result.AddError(err)
		}
	}

	// 验证日志配置
	if err := c.Log.Validate(); err != nil {
		result.AddError(err)
	}

	if !result.IsValid() {
		return result
	}
	return nil
}

// validateServerAddress 验证服务器地址格式
func validateServerAddress(addr string) error {
	// 支持 http:// 或 https:// 开头的 URL
	if len(addr) >= 7 && (addr[:7] == "http://" || addr[:8] == "https://") {
		return validation.ValidateURL(addr, "server.address")
	}
	// 支持 host:port 格式
	return validation.ValidateAddress(addr, "server.address")
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"`  // 日志级别：debug, info, warn, error
	Format string `yaml:"format"` // 日志格式：text, json
	Output string `yaml:"output"` // 输出目标：stdout, stderr, file
	File   string `yaml:"file"`   // 日志文件路径（当output=file时）
}

// Validate 验证日志配置
func (l *LogConfig) Validate() error {
	result := &validation.ValidationResult{}

	// 验证日志级别
	if l.Level != "" {
		validLevels := []string{"debug", "info", "warn", "error", "fatal"}
		if err := validation.ValidateStringInList(l.Level, "log.level", validLevels); err != nil {
			result.AddError(err)
		}
	}

	// 验证日志格式
	if l.Format != "" {
		validFormats := []string{"text", "json"}
		if err := validation.ValidateStringInList(l.Format, "log.format", validFormats); err != nil {
			result.AddError(err)
		}
	}

	// 验证输出目标
	if l.Output != "" {
		validOutputs := []string{"stdout", "stderr", "file"}
		if err := validation.ValidateStringInList(l.Output, "log.output", validOutputs); err != nil {
			result.AddError(err)
		}
	}

	// 如果输出到文件，验证文件路径
	if l.Output == "file" && l.File != "" {
		if err := validation.ValidateNonEmptyString(l.File, "log.file"); err != nil {
			result.AddError(err)
		}
	}

	if !result.IsValid() {
		return result
	}
	return nil
}
