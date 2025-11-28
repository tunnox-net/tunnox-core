package server

import (
	"os"
	"strconv"
	"tunnox-core/internal/utils"
)

// ApplyEnvOverrides 应用环境变量覆盖配置
// 环境变量优先级高于配置文件
func ApplyEnvOverrides(config *Config) {
	// Storage配置
	if v := os.Getenv("STORAGE_TYPE"); v != "" {
		config.Storage.Type = v
		utils.Infof("Config override from env: STORAGE_TYPE=%s", v)
	}
	if v := os.Getenv("STORAGE_REDIS_ADDR"); v != "" {
		config.Storage.Redis.Addr = v
		utils.Infof("Config override from env: STORAGE_REDIS_ADDR=%s", v)
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
		utils.Infof("Config override from env: STORAGE_HYBRID_CACHETYPE=%s", v)
	}
	if v := os.Getenv("STORAGE_HYBRID_ENABLEPERSISTENT"); v != "" {
		config.Storage.Hybrid.EnablePersistent = (v == "true" || v == "1")
		utils.Infof("Config override from env: STORAGE_HYBRID_ENABLEPERSISTENT=%v", config.Storage.Hybrid.EnablePersistent)
	}
	if v := os.Getenv("STORAGE_HYBRID_JSON_FILEPATH"); v != "" {
		config.Storage.Hybrid.JSON.FilePath = v
		utils.Infof("Config override from env: STORAGE_HYBRID_JSON_FILEPATH=%s", v)
	}
	if v := os.Getenv("STORAGE_HYBRID_JSON_AUTOSAVE"); v != "" {
		config.Storage.Hybrid.JSON.AutoSave = (v == "true" || v == "1")
		utils.Infof("Config override from env: STORAGE_HYBRID_JSON_AUTOSAVE=%v", config.Storage.Hybrid.JSON.AutoSave)
	}
	if v := os.Getenv("STORAGE_HYBRID_JSON_SAVEINTERVAL"); v != "" {
		if interval, err := strconv.Atoi(v); err == nil {
			config.Storage.Hybrid.JSON.SaveInterval = interval
			utils.Infof("Config override from env: STORAGE_HYBRID_JSON_SAVEINTERVAL=%d", interval)
		}
	}

	// MessageBroker配置 (重要！)
	if v := os.Getenv("MESSAGE_BROKER_TYPE"); v != "" {
		config.MessageBroker.Type = v
		utils.Infof("Config override from env: MESSAGE_BROKER_TYPE=%s", v)
	}

	// ✅ NODE_ID优先（简化版）
	if v := os.Getenv("NODE_ID"); v != "" {
		config.MessageBroker.NodeID = v
		utils.Infof("Config override from env: NODE_ID=%s", v)
	}

	// MESSAGE_BROKER_NODE_ID优先级更高（可覆盖NODE_ID）
	if v := os.Getenv("MESSAGE_BROKER_NODE_ID"); v != "" {
		config.MessageBroker.NodeID = v
		utils.Infof("Config override from env: MESSAGE_BROKER_NODE_ID=%s", v)
	}
	if v := os.Getenv("MESSAGE_BROKER_REDIS_ADDR"); v != "" {
		config.MessageBroker.Redis.Addr = v
		utils.Infof("Config override from env: MESSAGE_BROKER_REDIS_ADDR=%s", v)
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
		utils.Infof("Config override from env: LISTEN_ADDR=%s", v)
	}
	if v := os.Getenv("API_ADDR"); v != "" {
		config.ManagementAPI.ListenAddr = v
		utils.Infof("Config override from env: API_ADDR=%s", v)
	}

	// Log配置
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Log.Level = v
		utils.Infof("Config override from env: LOG_LEVEL=%s", v)
	}
}
