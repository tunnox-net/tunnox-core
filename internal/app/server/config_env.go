package server

import (
	"os"
	"strconv"
)

// ApplyEnvOverrides 应用环境变量覆盖配置
// 环境变量优先级高于配置文件
func ApplyEnvOverrides(config *Config) {
	// Storage配置
	if v := os.Getenv("STORAGE_TYPE"); v != "" {
		config.Storage.Type = v
	}
	if v := os.Getenv("STORAGE_REDIS_ADDR"); v != "" {
		config.Storage.Redis.Addr = v
	}
	if v := os.Getenv("STORAGE_REDIS_PASSWORD"); v != "" {
		config.Storage.Redis.Password = v
	}
	if v := os.Getenv("STORAGE_REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			config.Storage.Redis.DB = db
		}
	}

	// Hybrid Storage配置
	if v := os.Getenv("STORAGE_HYBRID_CACHETYPE"); v != "" {
		config.Storage.Hybrid.CacheType = v
	}
	if v := os.Getenv("STORAGE_HYBRID_ENABLEPERSISTENT"); v != "" {
		config.Storage.Hybrid.EnablePersistent = (v == "true" || v == "1")
	}
	if v := os.Getenv("STORAGE_HYBRID_JSON_FILEPATH"); v != "" {
		config.Storage.Hybrid.JSON.FilePath = v
	}
	if v := os.Getenv("STORAGE_HYBRID_JSON_AUTOSAVE"); v != "" {
		config.Storage.Hybrid.JSON.AutoSave = (v == "true" || v == "1")
	}
	if v := os.Getenv("STORAGE_HYBRID_JSON_SAVEINTERVAL"); v != "" {
		if interval, err := strconv.Atoi(v); err == nil {
			config.Storage.Hybrid.JSON.SaveInterval = interval
		}
	}

	// MessageBroker配置
	if v := os.Getenv("MESSAGE_BROKER_TYPE"); v != "" {
		config.MessageBroker.Type = v
	}

	// NODE_ID优先
	if v := os.Getenv("NODE_ID"); v != "" {
		config.MessageBroker.NodeID = v
	}

	// MESSAGE_BROKER_NODE_ID优先级更高（可覆盖NODE_ID）
	if v := os.Getenv("MESSAGE_BROKER_NODE_ID"); v != "" {
		config.MessageBroker.NodeID = v
	}

	// Metrics 配置
	if v := os.Getenv("METRICS_TYPE"); v != "" {
		config.Metrics.Type = v
	}
	if v := os.Getenv("MESSAGE_BROKER_REDIS_ADDR"); v != "" {
		config.MessageBroker.Redis.Addr = v
	}
	if v := os.Getenv("MESSAGE_BROKER_REDIS_PASSWORD"); v != "" {
		config.MessageBroker.Redis.Password = v
	}
	if v := os.Getenv("MESSAGE_BROKER_REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			config.MessageBroker.Redis.DB = db
		}
	}
	if v := os.Getenv("MESSAGE_BROKER_REDIS_CHANNEL"); v != "" {
		config.MessageBroker.Redis.Channel = v
	}

	// Server配置
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		// 解析为host:port (简化处理)
		config.Server.Host = "0.0.0.0"
	}
	if v := os.Getenv("API_ADDR"); v != "" {
		config.ManagementAPI.ListenAddr = v
	}

	// Log配置
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Log.Level = v
	}

	// 协议端口配置
	// 注意：websocket 和 httppoll 不需要独立端口，它们通过 HTTP 服务容器提供
	if v := os.Getenv("SERVER_TCP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			if config.Server.Protocols == nil {
				config.Server.Protocols = make(map[string]ProtocolConfig)
			}
			pc := config.Server.Protocols["tcp"]
			pc.Port = port
			pc.Enabled = true
			config.Server.Protocols["tcp"] = pc
		}
	}
	// KCP 端口
	if v := os.Getenv("SERVER_KCP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			if config.Server.Protocols == nil {
				config.Server.Protocols = make(map[string]ProtocolConfig)
			}
			pc := config.Server.Protocols["kcp"]
			pc.Port = port
			pc.Enabled = true
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
			config.Server.Protocols["quic"] = pc
		}
	}

	// Management API 配置
	if v := os.Getenv("MANAGEMENT_API_LISTEN"); v != "" {
		config.ManagementAPI.ListenAddr = v
	}
	if v := os.Getenv("MANAGEMENT_API_AUTH_TYPE"); v != "" {
		config.ManagementAPI.Auth.Type = v
	}
	if v := os.Getenv("MANAGEMENT_API_AUTH_TOKEN"); v != "" {
		config.ManagementAPI.Auth.Token = v
	}
}
