package services

import (
	"testing"
	"time"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltInCloudControl_Dispose(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)

	// 验证初始状态
	assert.False(t, cloudControl.IsClosed())

	// 启动云控
	cloudControl.Start()

	// 等待一段时间让清理例程运行
	time.Sleep(200 * time.Millisecond)

	// 验证仍在运行
	assert.False(t, cloudControl.IsClosed())

	// 关闭云控
	err := cloudControl.Close()
	require.NoError(t, err)

	// 验证已关闭
	assert.True(t, cloudControl.IsClosed())

	// 等待资源清理完成
	time.Sleep(100 * time.Millisecond)
}

func TestBuiltInCloudControl_ContextCancellation(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)

	// 启动云控
	cloudControl.Start()

	// 等待一段时间让清理例程运行
	time.Sleep(200 * time.Millisecond)

	// 验证仍在运行
	assert.False(t, cloudControl.IsClosed())

	// 取消上下文
	_ = cloudControl.Close()

	// 等待资源清理完成
	time.Sleep(200 * time.Millisecond)

	// 验证已关闭
	assert.True(t, cloudControl.IsClosed())
}

func TestBuiltInCloudControl_StartAfterClose(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)

	// 关闭云控
	err := cloudControl.Close()
	require.NoError(t, err)

	// 验证已关闭
	assert.True(t, cloudControl.IsClosed())

	// 尝试再次启动（应该被忽略）
	cloudControl.Start()

	// 验证仍然关闭
	assert.True(t, cloudControl.IsClosed())
}

func TestBuiltInCloudControl_StopAfterClose(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)

	// 关闭云控
	err := cloudControl.Close()
	require.NoError(t, err)

	// 验证已关闭
	assert.True(t, cloudControl.IsClosed())

}

func TestBuiltInCloudControl_CloseMultipleTimes(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)

	// 启动云控
	cloudControl.Start()

	// 第一次关闭
	err := cloudControl.Close()
	require.NoError(t, err)
	assert.True(t, cloudControl.IsClosed())

	// 第二次关闭（应该返回nil）
	err = cloudControl.Close()
	require.NoError(t, err)
	assert.True(t, cloudControl.IsClosed())
}

func TestBuiltInCloudControl_NodeRegistration(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)
	defer cloudControl.Close()

	// 启动云控
	cloudControl.Start()

	// 注册节点
	req := &models.NodeRegisterRequest{
		Address: "127.0.0.1:8080",
		Version: "1.0.0",
		Meta: map[string]string{
			"region": "test",
		},
	}

	resp, err := cloudControl.NodeRegister(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.NodeID)

	// 验证节点注册成功
	assert.NotEmpty(t, resp.NodeID)
	assert.True(t, resp.Success)
}

func TestBuiltInCloudControl_ClientCreation(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)
	defer cloudControl.Close()

	// 启动云控
	cloudControl.Start()

	// 创建用户
	user, err := cloudControl.CreateUser("testuser", "test@example.com")
	require.NoError(t, err)
	require.NotNil(t, user)

	// 创建客户端
	client, err := cloudControl.CreateClient(user.ID, "test-client")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, user.ID, client.UserID)
	assert.Equal(t, "test-client", client.Name)
	assert.Equal(t, models.ClientTypeRegistered, client.Type)
}

func TestBuiltInCloudControl_JWTTokenGeneration(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)
	defer cloudControl.Close()

	// 启动云控
	cloudControl.Start()

	// 创建用户和客户端
	user, err := cloudControl.CreateUser("testuser", "test@example.com")
	require.NoError(t, err)

	client, err := cloudControl.CreateClient(user.ID, "test-client")
	require.NoError(t, err)

	// 生成JWT令牌
	tokenInfo, err := cloudControl.GenerateJWTToken(client.ID)
	require.NoError(t, err)
	require.NotNil(t, tokenInfo)
	assert.NotEmpty(t, tokenInfo.Token)
	assert.NotEmpty(t, tokenInfo.RefreshToken)
	assert.Equal(t, client.ID, tokenInfo.ClientId)

	// 验证令牌
	validatedToken, err := cloudControl.ValidateJWTToken(tokenInfo.Token)
	require.NoError(t, err)
	require.NotNil(t, validatedToken)
	assert.Equal(t, client.ID, validatedToken.ClientId)
}

func TestBuiltInCloudControl_AnonymousCredentials(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)
	defer cloudControl.Close()

	// 启动云控
	cloudControl.Start()

	// 生成匿名凭据
	client, err := cloudControl.GenerateAnonymousCredentials()
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, models.ClientTypeAnonymous, client.Type)
	assert.NotEmpty(t, client.ID)

	// 验证匿名客户端存在
	retrievedClient, err := cloudControl.GetAnonymousClient(client.ID)
	require.NoError(t, err)
	require.NotNil(t, retrievedClient)
	assert.Equal(t, client.ID, retrievedClient.ID)
}

func TestBuiltInCloudControl_SystemStats(t *testing.T) {
	// 创建配置
	config := &managers.ControlConfig{
		JWTSecretKey:      "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
		UseBuiltIn:        true,
	}

	// 创建云控实例
	cloudControl := managers.NewBuiltinCloudControl(config)
	require.NotNil(t, cloudControl)
	defer cloudControl.Close()

	// 启动云控
	cloudControl.Start()

	// 获取系统统计信息
	stats, err := cloudControl.GetSystemStats()
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.TotalUsers, 0)
	assert.GreaterOrEqual(t, stats.TotalClients, 0)
	assert.GreaterOrEqual(t, stats.TotalMappings, 0)
	assert.GreaterOrEqual(t, stats.TotalNodes, 0)
}
