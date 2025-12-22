package server

import (
	"os"
	"strconv"
)

// ApplyEnvOverrides 应用环境变量覆盖配置
// 环境变量优先级高于配置文件
func ApplyEnvOverrides(config *Config) {
	// Redis 配置
	if v := os.Getenv("REDIS_ENABLED"); v != "" {
		config.Redis.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		config.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		config.Redis.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			config.Redis.DB = db
		}
	}

	// Persistence 配置
	if v := os.Getenv("PERSISTENCE_ENABLED"); v != "" {
		config.Persistence.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("PERSISTENCE_FILE"); v != "" {
		config.Persistence.File = v
	}
	if v := os.Getenv("PERSISTENCE_AUTO_SAVE"); v != "" {
		config.Persistence.AutoSave = (v == "true" || v == "1")
	}
	if v := os.Getenv("PERSISTENCE_SAVE_INTERVAL"); v != "" {
		if interval, err := strconv.Atoi(v); err == nil {
			config.Persistence.SaveInterval = interval
		}
	}

	// Storage 配置
	if v := os.Getenv("STORAGE_ENABLED"); v != "" {
		config.Storage.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("STORAGE_URL"); v != "" {
		config.Storage.URL = v
	}
	if v := os.Getenv("STORAGE_TOKEN"); v != "" {
		config.Storage.Token = v
	}
	if v := os.Getenv("STORAGE_TIMEOUT"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil {
			config.Storage.Timeout = timeout
		}
	}

	// Platform 配置
	if v := os.Getenv("PLATFORM_ENABLED"); v != "" {
		config.Platform.Enabled = (v == "true" || v == "1")
	}
	if v := os.Getenv("PLATFORM_URL"); v != "" {
		config.Platform.URL = v
	}
	if v := os.Getenv("PLATFORM_TOKEN"); v != "" {
		config.Platform.Token = v
	}
	if v := os.Getenv("PLATFORM_TIMEOUT"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil {
			config.Platform.Timeout = timeout
		}
	}

	// Log 配置
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Log.Level = v
	}
	if v := os.Getenv("LOG_FILE"); v != "" {
		config.Log.File = v
	}

	// 协议端口配置
	if v := os.Getenv("SERVER_TCP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			if config.Server.Protocols == nil {
				config.Server.Protocols = make(map[string]ProtocolConfig)
			}
			pc := config.Server.Protocols["tcp"]
			pc.Port = port
			pc.Enabled = true
			pc.Host = "0.0.0.0"
			config.Server.Protocols["tcp"] = pc
		}
	}
	if v := os.Getenv("SERVER_KCP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			if config.Server.Protocols == nil {
				config.Server.Protocols = make(map[string]ProtocolConfig)
			}
			pc := config.Server.Protocols["kcp"]
			pc.Port = port
			pc.Enabled = true
			pc.Host = "0.0.0.0"
			config.Server.Protocols["kcp"] = pc
		}
	}
	if v := os.Getenv("SERVER_QUIC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			if config.Server.Protocols == nil {
				config.Server.Protocols = make(map[string]ProtocolConfig)
			}
			pc := config.Server.Protocols["quic"]
			pc.Port = port
			pc.Enabled = true
			pc.Host = "0.0.0.0"
			config.Server.Protocols["quic"] = pc
		}
	}

	// Management API 配置
	if v := os.Getenv("MANAGEMENT_LISTEN"); v != "" {
		config.Management.Listen = v
	}
	if v := os.Getenv("MANAGEMENT_AUTH_TYPE"); v != "" {
		config.Management.Auth.Type = v
	}
	if v := os.Getenv("MANAGEMENT_AUTH_TOKEN"); v != "" {
		config.Management.Auth.Token = v
	}
	if v := os.Getenv("MANAGEMENT_PPROF_ENABLED"); v != "" {
		config.Management.PProf.Enabled = (v == "true" || v == "1")
	}
}
