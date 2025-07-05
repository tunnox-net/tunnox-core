package tests

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/utils"
)

func TestServerStartup(t *testing.T) {
	// 创建云控制器
	config := cloud.DefaultConfig()
	cloudControl, err := cloud.NewBuiltinCloudControl(config)
	if err != nil {
		t.Fatalf("Failed to create cloud control: %v", err)
	}
	defer cloudControl.Close()

	// 测试健康检查
	ctx := context.Background()

	// 测试创建用户
	user, err := cloudControl.CreateUser(ctx, "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user == nil {
		t.Fatal("User should not be nil")
	}

	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}

	// 测试创建客户端
	client, err := cloudControl.CreateClient(ctx, user.ID, "testclient")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client == nil {
		t.Fatal("Client should not be nil")
	}

	if client.Name != "testclient" {
		t.Errorf("Expected client name 'testclient', got '%s'", client.Name)
	}

	// 测试认证
	authReq := &cloud.AuthRequest{
		ClientID:  client.ID,
		AuthCode:  client.AuthCode,
		NodeID:    "test-node",
		IPAddress: "127.0.0.1",
	}

	authResp, err := cloudControl.Authenticate(ctx, authReq)
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	if !authResp.Success {
		t.Errorf("Authentication should succeed, got: %s", authResp.Message)
	}
}

func TestLoggerInitialization(t *testing.T) {
	// 测试日志初始化
	config := &utils.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	}

	err := utils.InitLogger(config)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// 测试日志记录
	utils.Info("Test log message")
	utils.Debug("Test debug message")
	utils.Warn("Test warning message")
	utils.Error("Test error message")
}

func TestHTTPRateLimiter(t *testing.T) {
	// 测试限流器
	limiter := utils.NewRateLimiter(5, time.Second)
	defer limiter.Close()

	// 测试允许请求
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 测试限制请求
	if limiter.Allow() {
		t.Error("Request should be limited")
	}

	// 等待窗口过期
	time.Sleep(time.Second)

	// 测试重新允许请求
	if !limiter.Allow() {
		t.Error("Request should be allowed after window reset")
	}
}

func TestResponseUtils(t *testing.T) {
	// 测试响应工具函数
	response := utils.NewSuccessResponse(map[string]string{"test": "value"})

	if response.Code != 200 {
		t.Errorf("Expected code 200, got %d", response.Code)
	}

	if response.Message != "Success" {
		t.Errorf("Expected message 'Success', got '%s'", response.Message)
	}

	if response.Data == nil {
		t.Error("Response data should not be nil")
	}

	// 测试错误响应
	errorResp := utils.NewBadRequestResponse("Test error", nil)

	if errorResp.Code != 400 {
		t.Errorf("Expected code 400, got %d", errorResp.Code)
	}

	if errorResp.Message != "Test error" {
		t.Errorf("Expected message 'Test error', got '%s'", errorResp.Message)
	}
}

func TestTimeUtils(t *testing.T) {
	// 测试时间工具函数
	timestamp := utils.GetCurrentTimestamp()
	if timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}

	timestampMillis := utils.GetCurrentTimestampMillis()
	if timestampMillis <= 0 {
		t.Error("Millisecond timestamp should be positive")
	}

	// 测试时间范围
	start, end, err := utils.GetTimeRange("1h")
	if err != nil {
		t.Fatalf("Failed to get time range: %v", err)
	}

	if start.After(end) {
		t.Error("Start time should be before end time")
	}

	// 测试过期检查
	if utils.IsExpired(timestamp, time.Hour) {
		t.Error("Current timestamp should not be expired")
	}

	// 测试过期时间戳
	expiredTimestamp := timestamp - 7200 // 2小时前
	if !utils.IsExpired(expiredTimestamp, time.Hour) {
		t.Error("Expired timestamp should be marked as expired")
	}
}
