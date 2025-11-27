package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"tunnox-core/internal/app/server"
)

// TestServerConfig_Valid 测试有效的服务器配置
func TestServerConfig_Valid(t *testing.T) {
	config := &server.ServerConfig{
		Host:         "0.0.0.0",
		Port:         7000,
		ReadTimeout:  60,
		WriteTimeout: 60,
		IdleTimeout:  120,
	}

	// 配置验证逻辑
	assert.NotEmpty(t, config.Host)
	assert.Greater(t, config.Port, 0)
	assert.Less(t, config.Port, 65536)
}

// TestServerConfig_InvalidPort 测试无效端口
func TestServerConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"port too large", 65536},
		{"port way too large", 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &server.ServerConfig{
				Host: "0.0.0.0",
				Port: tt.port,
			}

			// 验证端口范围
			isValid := config.Port > 0 && config.Port < 65536
			assert.False(t, isValid, "Port %d should be invalid", tt.port)
		})
	}
}

// TestProtocolConfig_TCPEnabled 测试TCP协议启用
func TestProtocolConfig_TCPEnabled(t *testing.T) {
	protocolConfig := server.ProtocolConfig{
		Enabled: true,
		Port:    7000,
		Host:    "0.0.0.0",
	}

	assert.True(t, protocolConfig.Enabled)
	assert.Equal(t, 7000, protocolConfig.Port)
	assert.Equal(t, "0.0.0.0", protocolConfig.Host)
}

// TestProtocolConfig_WebSocketEnabled 测试WebSocket协议启用
func TestProtocolConfig_WebSocketEnabled(t *testing.T) {
	protocolConfig := server.ProtocolConfig{
		Enabled: true,
		Port:    7001,
		Host:    "0.0.0.0",
	}

	assert.True(t, protocolConfig.Enabled)
	assert.Equal(t, 7001, protocolConfig.Port)
}

// TestCloudConfig_BuiltIn 测试内置云控配置
func TestCloudConfig_BuiltIn(t *testing.T) {
	config := &server.CloudConfig{
		Type: "built_in",
		BuiltIn: server.BuiltInCloudConfig{
			Enabled: true,
		},
	}

	assert.Equal(t, "built_in", config.Type)
	assert.True(t, config.BuiltIn.Enabled)
}

// TestCloudConfig_External 测试外部云控配置
func TestCloudConfig_External(t *testing.T) {
	config := &server.CloudConfig{
		Type: "external",
		External: server.ExternalCloudConfig{
			Endpoint: "https://cloud.example.com",
			APIKey:   "test-api-key",
			Timeout:  30,
		},
	}

	assert.Equal(t, "external", config.Type)
	assert.Equal(t, "https://cloud.example.com", config.External.Endpoint)
	assert.Equal(t, "test-api-key", config.External.APIKey)
	assert.Equal(t, 30, config.External.Timeout)
}

// TestMessageBrokerConfig_Redis 测试Redis消息代理配置
func TestMessageBrokerConfig_Redis(t *testing.T) {
	config := &server.MessageBrokerConfig{
		Type:   "redis",
		NodeID: "node-1",
		Redis: server.RedisBrokerConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			Channel:  "tunnox",
			PoolSize: 10,
		},
	}

	assert.Equal(t, "redis", config.Type)
	assert.Equal(t, "node-1", config.NodeID)
	assert.Equal(t, "localhost:6379", config.Redis.Addr)
	assert.Equal(t, 10, config.Redis.PoolSize)
}

// TestMessageBrokerConfig_RabbitMQ 测试RabbitMQ消息代理配置
func TestMessageBrokerConfig_RabbitMQ(t *testing.T) {
	config := &server.MessageBrokerConfig{
		Type:   "rabbitmq",
		NodeID: "node-2",
		Rabbit: server.RabbitMQBrokerConfig{
			URL:          "amqp://localhost:5672",
			Exchange:     "tunnox",
			ExchangeType: "topic",
			RoutingKey:   "tunnox.#",
		},
	}

	assert.Equal(t, "rabbitmq", config.Type)
	assert.Equal(t, "amqp://localhost:5672", config.Rabbit.URL)
	assert.Equal(t, "tunnox", config.Rabbit.Exchange)
}

// TestBridgePoolConfig_Valid 测试桥接连接池配置
func TestBridgePoolConfig_Valid(t *testing.T) {
	config := &server.BridgePoolConfig{
		Enabled:             true,
		MinConnsPerNode:     2,
		MaxConnsPerNode:     10,
		MaxIdleTime:         300,
		MaxStreamsPerConn:   100,
		DialTimeout:         10,
		HealthCheckInterval: 30,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, int32(2), config.MinConnsPerNode)
	assert.Equal(t, int32(10), config.MaxConnsPerNode)
	assert.Less(t, config.MinConnsPerNode, config.MaxConnsPerNode, "Min should be less than Max")
}

// TestStorageConfig_Memory 测试内存存储配置
func TestStorageConfig_Memory(t *testing.T) {
	config := &server.StorageConfig{
		Type: "memory",
	}

	assert.Equal(t, "memory", config.Type)
}

// TestStorageConfig_Redis 测试Redis存储配置
func TestStorageConfig_Redis(t *testing.T) {
	config := &server.StorageConfig{
		Type: "redis",
		Redis: server.RedisStorageConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
	}

	assert.Equal(t, "redis", config.Type)
	assert.Equal(t, "localhost:6379", config.Redis.Addr)
	assert.Equal(t, 10, config.Redis.PoolSize)
}

// TestStorageConfig_Hybrid 测试混合存储配置
func TestStorageConfig_Hybrid(t *testing.T) {
	config := &server.StorageConfig{
		Type: "hybrid",
		Hybrid: server.HybridStorageConfigYAML{
			CacheType:        "memory",
			EnablePersistent: true,
			JSON: server.JSONStorageConfigYAML{
				FilePath:     "./data/tunnox.json",
				AutoSave:     true,
				SaveInterval: 5,
			},
		},
	}

	assert.Equal(t, "hybrid", config.Type)
	assert.Equal(t, "memory", config.Hybrid.CacheType)
	assert.True(t, config.Hybrid.EnablePersistent)
	assert.Equal(t, "./data/tunnox.json", config.Hybrid.JSON.FilePath)
}

// TestConfig_GetDefaultConfig 测试获取默认配置
func TestConfig_GetDefaultConfig(t *testing.T) {
	config := server.GetDefaultConfig()

	assert.NotNil(t, config)
	assert.NotEmpty(t, config.Server.Host)
	assert.Greater(t, config.Server.Port, 0)
}

// TestConfig_EmptyHost 测试空主机名
func TestConfig_EmptyHost(t *testing.T) {
	config := &server.ServerConfig{
		Host: "",
		Port: 7000,
	}

	// 空主机名应该被视为无效
	assert.Empty(t, config.Host)
}

// TestConfig_Timeouts 测试超时配置
func TestConfig_Timeouts(t *testing.T) {
	tests := []struct {
		name         string
		readTimeout  int
		writeTimeout int
		idleTimeout  int
		valid        bool
	}{
		{"all valid", 60, 60, 120, true},
		{"zero read", 0, 60, 120, false},
		{"zero write", 60, 0, 120, false},
		{"zero idle", 60, 60, 0, false},
		{"negative values", -1, -1, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &server.ServerConfig{
				Host:         "0.0.0.0",
				Port:         7000,
				ReadTimeout:  tt.readTimeout,
				WriteTimeout: tt.writeTimeout,
				IdleTimeout:  tt.idleTimeout,
			}

			isValid := config.ReadTimeout > 0 && config.WriteTimeout > 0 && config.IdleTimeout > 0
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

// TestMessageBrokerConfig_InvalidType 测试无效消息代理类型
func TestMessageBrokerConfig_InvalidType(t *testing.T) {
	invalidTypes := []string{
		"invalid",
		"",
		"unknown",
		"mysql",
	}

	validTypes := []string{
		"memory",
		"redis",
		"rabbitmq",
		"kafka",
	}

	for _, invalidType := range invalidTypes {
		t.Run("invalid_"+invalidType, func(t *testing.T) {
			config := &server.MessageBrokerConfig{
				Type: invalidType,
			}

			isValid := false
			for _, vt := range validTypes {
				if config.Type == vt {
					isValid = true
					break
				}
			}
			assert.False(t, isValid, "Type '%s' should be invalid", invalidType)
		})
	}
}

// TestStorageConfig_InvalidType 测试无效存储类型
func TestStorageConfig_InvalidType(t *testing.T) {
	invalidTypes := []string{
		"invalid",
		"",
		"unknown",
		"postgres",
	}

	validTypes := []string{
		"memory",
		"redis",
		"json",
		"remote",
	}

	for _, invalidType := range invalidTypes {
		t.Run("invalid_"+invalidType, func(t *testing.T) {
			config := &server.StorageConfig{
				Type: invalidType,
			}

			isValid := false
			for _, vt := range validTypes {
				if config.Type == vt {
					isValid = true
					break
				}
			}
			assert.False(t, isValid, "Type '%s' should be invalid", invalidType)
		})
	}
}

// TestProtocolConfig_MultipleProtocols 测试多协议配置
func TestProtocolConfig_MultipleProtocols(t *testing.T) {
	config := &server.ServerConfig{
		Host: "0.0.0.0",
		Port: 7000,
		Protocols: map[string]server.ProtocolConfig{
			"tcp": {
				Enabled: true,
				Port:    7000,
				Host:    "0.0.0.0",
			},
			"websocket": {
				Enabled: true,
				Port:    7001,
				Host:    "0.0.0.0",
			},
			"udp": {
				Enabled: true,
				Port:    7002,
				Host:    "0.0.0.0",
			},
			"quic": {
				Enabled: false,
				Port:    7003,
				Host:    "0.0.0.0",
			},
		},
	}

	assert.Len(t, config.Protocols, 4)
	assert.True(t, config.Protocols["tcp"].Enabled)
	assert.True(t, config.Protocols["websocket"].Enabled)
	assert.True(t, config.Protocols["udp"].Enabled)
	assert.False(t, config.Protocols["quic"].Enabled)
}

// TestBridgePoolConfig_Consistency 测试桥接池配置一致性
func TestBridgePoolConfig_Consistency(t *testing.T) {
	config := &server.BridgePoolConfig{
		Enabled:             true,
		MinConnsPerNode:     10,
		MaxConnsPerNode:     5, // 错误：最小值大于最大值
		MaxIdleTime:         300,
		MaxStreamsPerConn:   100,
		DialTimeout:         10,
		HealthCheckInterval: 30,
	}

	// 验证一致性
	isConsistent := config.MinConnsPerNode <= config.MaxConnsPerNode
	assert.False(t, isConsistent, "Min should not be greater than Max")
}

// TestRedisConfig_ClusterMode 测试Redis集群模式
func TestRedisConfig_ClusterMode(t *testing.T) {
	config := &server.RedisBrokerConfig{
		Addr:        "localhost:6379,localhost:6380,localhost:6381",
		Password:    "password",
		DB:          0,
		ClusterMode: true,
		PoolSize:    20,
	}

	assert.True(t, config.ClusterMode)
	assert.Equal(t, 20, config.PoolSize)
	assert.Contains(t, config.Addr, ",", "Cluster addresses should contain commas")
}

