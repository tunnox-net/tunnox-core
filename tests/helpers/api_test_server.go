package helpers

import (
	"context"
	"fmt"
	"net"
	"time"

	"tunnox-core/internal/api"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"
)

// TestAPIServer 测试API服务器封装
type TestAPIServer struct {
	*dispose.ResourceBase

	apiServer    *api.ManagementAPIServer
	cloudControl managers.CloudControlAPI
	storage      storage.Storage
	config       *api.APIConfig
	address      string
}

// TestAPIServerConfig 测试服务器配置
type TestAPIServerConfig struct {
	ListenAddr string
	AuthType   string // "none", "api_key", "jwt"
	APISecret  string
	EnableCORS bool
}

// DefaultTestAPIConfig 默认测试配置
func DefaultTestAPIConfig() *TestAPIServerConfig {
	return &TestAPIServerConfig{
		ListenAddr: findAvailablePort(), // 找一个可用端口
		AuthType:   "none",              // 测试时默认不需要认证
		APISecret:  "",
		EnableCORS: false,
	}
}

// findAvailablePort 找一个可用的端口
func findAvailablePort() string {
	// 尝试在高端口范围找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// 如果失败，使用一个固定的高端口
		return "127.0.0.1:18080"
	}
	defer listener.Close()
	return listener.Addr().String()
}

// NewTestAPIServer 创建测试API服务器
func NewTestAPIServer(ctx context.Context, cfg *TestAPIServerConfig) (*TestAPIServer, error) {
	if cfg == nil {
		cfg = DefaultTestAPIConfig()
	}

	// 创建内存存储
	memStorage := storage.NewMemoryStorage(ctx)

	// 创建CloudControl实例
	controlConfig := managers.DefaultConfig()
	controlConfig.UseBuiltIn = true
	cloudControl := managers.NewCloudControl(controlConfig, memStorage)

	// 创建API配置
	apiConfig := &api.APIConfig{
		Enabled:    true,
		ListenAddr: cfg.ListenAddr,
		Auth: api.AuthConfig{
			Type:   cfg.AuthType,
			Secret: cfg.APISecret,
		},
		CORS: api.CORSConfig{
			Enabled:        cfg.EnableCORS,
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		},
		RateLimit: api.RateLimitConfig{
			Enabled: false, // 测试时禁用限流
		},
	}

	// 创建API服务器（测试环境传入 nil 作为可选参数）
	apiServer := api.NewManagementAPIServer(ctx, apiConfig, cloudControl, nil, nil)

	server := &TestAPIServer{
		ResourceBase: dispose.NewResourceBase("TestAPIServer"),
		apiServer:    apiServer,
		cloudControl: cloudControl,
		storage:      memStorage,
		config:       apiConfig,
	}

	// 添加清理处理器
	server.AddCleanHandler(func() error {
		if server.apiServer != nil {
			result := server.apiServer.Dispose.Close()
			if result.HasErrors() {
				return fmt.Errorf("failed to close API server: %s", result.Error())
			}
		}
		if server.cloudControl != nil {
			if err := server.cloudControl.Close(); err != nil {
				return fmt.Errorf("failed to close cloud control: %w", err)
			}
		}
		if server.storage != nil {
			if err := server.storage.Close(); err != nil {
				return fmt.Errorf("failed to close storage: %w", err)
			}
		}
		return nil
	})

	// 初始化资源
	server.Initialize(ctx)

	return server, nil
}

// Start 启动测试服务器
func (s *TestAPIServer) Start() error {
	s.address = s.config.ListenAddr

	// 启动API服务器
	if err := s.apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	// 等待服务器准备就绪
	if err := s.waitForServerReady(); err != nil {
		return fmt.Errorf("server failed to become ready: %w", err)
	}

	return nil
}

// waitForServerReady 等待服务器准备就绪
func (s *TestAPIServer) waitForServerReady() error {
	timeout := time.After(3 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for server to start on %s", s.address)
		case <-ticker.C:
			// 尝试连接服务器
			conn, err := net.DialTimeout("tcp", s.address, 200*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

// GetAddress 获取服务器地址
func (s *TestAPIServer) GetAddress() string {
	return s.address
}

// GetBaseURL 获取服务器基础URL
func (s *TestAPIServer) GetBaseURL() string {
	return fmt.Sprintf("http://%s", s.address)
}

// GetAPIURL 获取API基础URL
func (s *TestAPIServer) GetAPIURL() string {
	// 使用统一的 API 路径：/tunnox/v1（API 路由直接注册在 /tunnox/v1 下）
	return fmt.Sprintf("http://%s/tunnox/v1", s.address)
}

// GetCloudControl 获取CloudControl实例
func (s *TestAPIServer) GetCloudControl() managers.CloudControlAPI {
	return s.cloudControl
}

// GetStorage 获取Storage实例
func (s *TestAPIServer) GetStorage() storage.Storage {
	return s.storage
}

// GetConfig 获取API配置
func (s *TestAPIServer) GetConfig() *api.APIConfig {
	return s.config
}

// Stop 停止测试服务器
func (s *TestAPIServer) Stop() error {
	return s.Close()
}
