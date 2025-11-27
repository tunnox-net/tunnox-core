package helpers

import (
	"context"
	"testing"
	"time"
)

func TestNewTestAPIServer(t *testing.T) {
	ctx := context.Background()

	t.Run("创建默认配置的测试服务器", func(t *testing.T) {
		server, err := NewTestAPIServer(ctx, nil)
		if err != nil {
			t.Fatalf("创建测试服务器失败: %v", err)
		}
		defer server.Stop()

		if server == nil {
			t.Fatal("服务器实例为nil")
		}

		if server.cloudControl == nil {
			t.Fatal("CloudControl实例为nil")
		}

		if server.storage == nil {
			t.Fatal("Storage实例为nil")
		}

		if server.apiServer == nil {
			t.Fatal("APIServer实例为nil")
		}

		if server.config == nil {
			t.Fatal("配置为nil")
		}

		// 验证默认配置
		if server.config.Auth.Type != "none" {
			t.Errorf("期望认证类型为 'none', 实际为 '%s'", server.config.Auth.Type)
		}

		if server.config.Enabled != true {
			t.Error("期望服务器启用")
		}
	})

	t.Run("创建自定义配置的测试服务器", func(t *testing.T) {
		cfg := &TestAPIServerConfig{
			ListenAddr: "127.0.0.1:9999",
			AuthType:   "api_key",
			APISecret:  "test-secret",
			EnableCORS: true,
		}

		server, err := NewTestAPIServer(ctx, cfg)
		if err != nil {
			t.Fatalf("创建测试服务器失败: %v", err)
		}
		defer server.Stop()

		if server.config.Auth.Type != "api_key" {
			t.Errorf("期望认证类型为 'api_key', 实际为 '%s'", server.config.Auth.Type)
		}

		if server.config.Auth.Secret != "test-secret" {
			t.Errorf("期望密钥为 'test-secret', 实际为 '%s'", server.config.Auth.Secret)
		}

		if !server.config.CORS.Enabled {
			t.Error("期望CORS启用")
		}
	})
}

func TestTestAPIServer_StartStop(t *testing.T) {
	ctx := context.Background()

	t.Run("启动和停止服务器", func(t *testing.T) {
		// 使用默认配置（会自动分配可用端口）
		server, err := NewTestAPIServer(ctx, nil)
		if err != nil {
			t.Fatalf("创建测试服务器失败: %v", err)
		}

		// 启动服务器
		if err := server.Start(); err != nil {
			t.Fatalf("启动服务器失败: %v", err)
		}

		// 验证服务器地址已设置
		if server.GetAddress() == "" {
			t.Error("服务器地址未设置")
		}

		// 验证URL方法
		baseURL := server.GetBaseURL()
		if baseURL == "" {
			t.Error("基础URL为空")
		}

		apiURL := server.GetAPIURL()
		if apiURL == "" {
			t.Error("API URL为空")
		}

		// 停止服务器
		if err := server.Stop(); err != nil {
			t.Errorf("停止服务器失败: %v", err)
		}
	})

	t.Run("服务器超时测试", func(t *testing.T) {
		// 这个测试确保服务器能在合理时间内启动
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		server, err := NewTestAPIServer(ctx, nil)
		if err != nil {
			t.Fatalf("创建测试服务器失败: %v", err)
		}
		defer server.Stop()

		startChan := make(chan error, 1)
		go func() {
			startChan <- server.Start()
		}()

		select {
		case err := <-startChan:
			if err != nil {
				t.Fatalf("启动服务器失败: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("服务器启动超时")
		}
	})
}

func TestTestAPIServer_GetMethods(t *testing.T) {
	ctx := context.Background()

	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	t.Run("获取CloudControl实例", func(t *testing.T) {
		cloudControl := server.GetCloudControl()
		if cloudControl == nil {
			t.Error("CloudControl实例为nil")
		}
	})

	t.Run("获取Storage实例", func(t *testing.T) {
		storage := server.GetStorage()
		if storage == nil {
			t.Error("Storage实例为nil")
		}
	})

	t.Run("获取Config实例", func(t *testing.T) {
		config := server.GetConfig()
		if config == nil {
			t.Error("Config实例为nil")
		}
	})
}

func TestTestAPIServer_DisposablePattern(t *testing.T) {
	ctx := context.Background()

	t.Run("验证dispose模式", func(t *testing.T) {
		server, err := NewTestAPIServer(ctx, nil)
		if err != nil {
			t.Fatalf("创建测试服务器失败: %v", err)
		}

		// 验证ResourceBase初始化
		if server.ResourceBase == nil {
			t.Fatal("ResourceBase未初始化")
		}

		// 第一次关闭
		if err := server.Close(); err != nil {
			t.Errorf("第一次关闭失败: %v", err)
		}

		// 验证IsClosed状态
		if !server.IsClosed() {
			t.Error("服务器应该已关闭")
		}

		// 第二次关闭应该是安全的
		if err := server.Close(); err != nil {
			t.Errorf("第二次关闭失败: %v", err)
		}
	})

	t.Run("上下文取消时自动清理", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		server, err := NewTestAPIServer(ctx, nil)
		if err != nil {
			t.Fatalf("创建测试服务器失败: %v", err)
		}

		// 取消上下文
		cancel()

		// 等待一小段时间让清理完成
		time.Sleep(100 * time.Millisecond)

		// 验证服务器已关闭
		if !server.IsClosed() {
			t.Error("服务器应该在上下文取消时自动关闭")
		}
	})
}

func TestDefaultTestAPIConfig(t *testing.T) {
	cfg := DefaultTestAPIConfig()

	if cfg == nil {
		t.Fatal("默认配置为nil")
	}

	// ListenAddr应该是一个有效的地址（不检查具体值，因为是动态分配的）
	if cfg.ListenAddr == "" {
		t.Error("期望监听地址不为空")
	}

	if cfg.AuthType != "none" {
		t.Errorf("期望认证类型为 'none', 实际为 '%s'", cfg.AuthType)
	}

	if cfg.EnableCORS {
		t.Error("期望CORS默认禁用")
	}

	if cfg.APISecret != "" {
		t.Error("期望API密钥默认为空")
	}
}

func TestTestAPIServer_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()

	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	if err := server.Start(); err != nil {
		t.Fatalf("启动服务器失败: %v", err)
	}

	// 并发访问服务器的getter方法
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			_ = server.GetAddress()
			_ = server.GetBaseURL()
			_ = server.GetAPIURL()
			_ = server.GetCloudControl()
			_ = server.GetStorage()
			_ = server.GetConfig()
		}()
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// 成功
		case <-time.After(5 * time.Second):
			t.Fatal("并发访问超时")
		}
	}
}

