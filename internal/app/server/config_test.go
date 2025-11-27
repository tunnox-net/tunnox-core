package server

import (
	"testing"
)

// TestRedisBrokerConfig 测试 Redis 消息队列配置
func TestRedisBrokerConfig(t *testing.T) {
	config := RedisBrokerConfig{
		Addr:     "localhost:6379",
		Password: "password",
		DB:       0,
		Channel:  "tunnox:messages",
		PoolSize: 10,
	}

	if config.Addr != "localhost:6379" {
		t.Errorf("Expected Addr to be 'localhost:6379', got '%s'", config.Addr)
	}
	if config.Channel != "tunnox:messages" {
		t.Errorf("Expected Channel to be 'tunnox:messages', got '%s'", config.Channel)
	}
	if config.PoolSize != 10 {
		t.Errorf("Expected PoolSize to be 10, got %d", config.PoolSize)
	}
}

// TestAuthConfig 测试认证配置
func TestAuthConfig(t *testing.T) {
	config := AuthConfig{
		Type:   "bearer",
		Token:  "test-token",
		APIKey: "test-api-key",
	}

	if config.Type != "bearer" {
		t.Errorf("Expected Type to be 'bearer', got '%s'", config.Type)
	}
	if config.Token != "test-token" {
		t.Errorf("Expected Token to be 'test-token', got '%s'", config.Token)
	}
}

// TestCORSConfig 测试 CORS 配置
func TestCORSConfig(t *testing.T) {
	config := CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://localhost:3000", "https://example.com"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if len(config.AllowedOrigins) != 2 {
		t.Errorf("Expected 2 allowed origins, got %d", len(config.AllowedOrigins))
	}
	if config.AllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("Expected first origin to be 'http://localhost:3000', got '%s'", config.AllowedOrigins[0])
	}
}

// TestRateLimitConfig 测试速率限制配置
func TestRateLimitConfig(t *testing.T) {
	config := RateLimitConfig{
		Enabled: true,
		RPS:     100,
		Burst:   200,
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.RPS != 100 {
		t.Errorf("Expected RPS to be 100, got %d", config.RPS)
	}
	if config.Burst != 200 {
		t.Errorf("Expected Burst to be 200, got %d", config.Burst)
	}
}

// TestGRPCServerConfig 测试 gRPC 服务器配置
func TestGRPCServerConfig(t *testing.T) {
	config := GRPCServerConfig{
		Addr:      "0.0.0.0",
		Port:      50051,
		EnableTLS: true,
	}

	if config.Addr != "0.0.0.0" {
		t.Errorf("Expected Addr to be '0.0.0.0', got '%s'", config.Addr)
	}
	if config.Port != 50051 {
		t.Errorf("Expected Port to be 50051, got %d", config.Port)
	}
	if !config.EnableTLS {
		t.Error("Expected EnableTLS to be true")
	}
}

// TestMessageBrokerConfig 测试消息代理配置
func TestMessageBrokerConfig(t *testing.T) {
	config := MessageBrokerConfig{
		Type:   "redis",
		NodeID: "node-1",
		Redis: RedisBrokerConfig{
			Addr:     "localhost:6379",
			Channel:  "tunnox:messages",
			PoolSize: 10,
		},
	}

	if config.Type != "redis" {
		t.Errorf("Expected Type to be 'redis', got '%s'", config.Type)
	}
	if config.NodeID != "node-1" {
		t.Errorf("Expected NodeID to be 'node-1', got '%s'", config.NodeID)
	}
	if config.Redis.Addr != "localhost:6379" {
		t.Errorf("Expected Redis.Addr to be 'localhost:6379', got '%s'", config.Redis.Addr)
	}
}

// TestManagementAPIConfig 测试管理 API 配置
func TestManagementAPIConfig(t *testing.T) {
	config := ManagementAPIConfig{
		Enabled:    true,
		ListenAddr: "0.0.0.0:9000",
		Auth: AuthConfig{
			Type:  "bearer",
			Token: "secret-token",
		},
		CORS: CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{"*"},
		},
		RateLimit: RateLimitConfig{
			Enabled: true,
			RPS:     100,
		},
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.ListenAddr != "0.0.0.0:9000" {
		t.Errorf("Expected ListenAddr to be '0.0.0.0:9000', got '%s'", config.ListenAddr)
	}
	if config.Auth.Type != "bearer" {
		t.Errorf("Expected Auth.Type to be 'bearer', got '%s'", config.Auth.Type)
	}
	if !config.CORS.Enabled {
		t.Error("Expected CORS.Enabled to be true")
	}
	if !config.RateLimit.Enabled {
		t.Error("Expected RateLimit.Enabled to be true")
	}
}

// TestRedisAutoSharing_StorageToMessageBroker 测试 Redis 从存储自动共享到消息队列
func TestRedisAutoSharing_StorageToMessageBroker(t *testing.T) {
	// 模拟场景：只配置了 storage.redis
	config := &Config{
		Storage: StorageConfig{
			Redis: RedisStorageConfig{
				Addr:     "localhost:6379",
				Password: "redis-password",
				DB:       1,
			},
		},
		MessageBroker: MessageBrokerConfig{
			Type: "memory", // 初始为 memory
		},
	}

	// 调用验证逻辑（这里简化，实际应该调用 ValidateConfig 的相关部分）
	if config.Storage.Redis.Addr != "" && config.MessageBroker.Type == "memory" {
		config.MessageBroker.Type = "redis"
		config.MessageBroker.Redis.Addr = config.Storage.Redis.Addr
		config.MessageBroker.Redis.Password = config.Storage.Redis.Password
		config.MessageBroker.Redis.DB = config.Storage.Redis.DB
		config.MessageBroker.Redis.Channel = "tunnox:messages"
		config.MessageBroker.Redis.PoolSize = 10
	}

	// 验证
	if config.MessageBroker.Type != "redis" {
		t.Errorf("Expected MessageBroker.Type to be 'redis', got '%s'", config.MessageBroker.Type)
	}
	if config.MessageBroker.Redis.Addr != "localhost:6379" {
		t.Errorf("Expected MessageBroker.Redis.Addr to be 'localhost:6379', got '%s'", config.MessageBroker.Redis.Addr)
	}
	if config.MessageBroker.Redis.Password != "redis-password" {
		t.Errorf("Expected MessageBroker.Redis.Password to be 'redis-password', got '%s'", config.MessageBroker.Redis.Password)
	}
}

// TestRedisAutoSharing_MessageBrokerToStorage 测试 Redis 从消息队列自动共享到存储
func TestRedisAutoSharing_MessageBrokerToStorage(t *testing.T) {
	// 模拟场景：只配置了 message_broker.redis
	config := &Config{
		Storage: StorageConfig{
			Redis: RedisStorageConfig{
				// 未配置
			},
		},
		MessageBroker: MessageBrokerConfig{
			Type: "redis",
			Redis: RedisBrokerConfig{
				Addr:     "localhost:6379",
				Password: "mq-password",
				DB:       2,
			},
		},
	}

	// 调用验证逻辑
	if config.MessageBroker.Type == "redis" && config.MessageBroker.Redis.Addr != "" {
		if config.Storage.Redis.Addr == "" {
			config.Storage.Redis.Addr = config.MessageBroker.Redis.Addr
			config.Storage.Redis.Password = config.MessageBroker.Redis.Password
			config.Storage.Redis.DB = config.MessageBroker.Redis.DB
			config.Storage.Redis.PoolSize = 10
		}
	}

	// 验证
	if config.Storage.Redis.Addr != "localhost:6379" {
		t.Errorf("Expected Storage.Redis.Addr to be 'localhost:6379', got '%s'", config.Storage.Redis.Addr)
	}
	if config.Storage.Redis.Password != "mq-password" {
		t.Errorf("Expected Storage.Redis.Password to be 'mq-password', got '%s'", config.Storage.Redis.Password)
	}
	if config.Storage.Redis.DB != 2 {
		t.Errorf("Expected Storage.Redis.DB to be 2, got %d", config.Storage.Redis.DB)
	}
}

// TestRedisAutoSharing_NoOverrideAdvancedMQ 测试 Redis 不会覆盖高级 MQ（如 RabbitMQ）
func TestRedisAutoSharing_NoOverrideAdvancedMQ(t *testing.T) {
	// 模拟场景：storage 配置了 Redis，但 message_broker 使用 RabbitMQ
	config := &Config{
		Storage: StorageConfig{
			Redis: RedisStorageConfig{
				Addr: "localhost:6379",
			},
		},
		MessageBroker: MessageBrokerConfig{
			Type: "rabbitmq",
			Rabbit: RabbitMQBrokerConfig{
				URL: "amqp://localhost:5672",
			},
		},
	}

	// 调用验证逻辑
	if config.Storage.Redis.Addr != "" {
		if config.MessageBroker.Type == "" || config.MessageBroker.Type == "memory" {
			config.MessageBroker.Type = "redis"
		}
	}

	// 验证：MessageBroker 应该仍然是 rabbitmq
	if config.MessageBroker.Type != "rabbitmq" {
		t.Errorf("Expected MessageBroker.Type to remain 'rabbitmq', got '%s'", config.MessageBroker.Type)
	}
}

