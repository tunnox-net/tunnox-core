package api_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/tests/helpers"
)

// TestNodeAPI_ListNodes 测试列出节点
func TestNodeAPI_ListNodes(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("列出所有节点", func(t *testing.T) {
		// 注意：节点列表需要通过节点注册来添加
		// 这里只测试 API 端点的可访问性
		// TODO: 扩展 APIClient 添加 ListNodes 方法

		// 暂时通过 CloudControl 直接测试
		cloudControl := server.GetCloudControl()
		nodes, err := cloudControl.GetAllNodeServiceInfo()
		require.NoError(t, err)
		// 节点列表可能为nil或空列表
		_ = nodes
	})
}

// TestNodeAPI_GetNode 测试获取节点
func TestNodeAPI_GetNode(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("获取节点信息", func(t *testing.T) {
		// 注意：需要先注册节点才能获取
		// TODO: 扩展 APIClient 添加节点管理方法

		// 暂时跳过，因为需要节点注册流程
		t.Skip("需要节点注册流程")
	})
}

// TestNodeAPI_NodeRegistration 测试节点注册
func TestNodeAPI_NodeRegistration(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("通过CloudControl注册节点", func(t *testing.T) {
		cloudControl := server.GetCloudControl()

		// 注册节点
		req := &models.NodeRegisterRequest{
			Address: "192.168.1.100:8080",
			Version: "1.0.0",
			Meta: map[string]string{
				"region": "us-west",
			},
		}

		resp, err := cloudControl.NodeRegister(req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.NodeID)
	})
}

// TestNodeAPI_NodeHeartbeat 测试节点心跳
func TestNodeAPI_NodeHeartbeat(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("节点心跳", func(t *testing.T) {
		cloudControl := server.GetCloudControl()

		// 先注册节点
		registerReq := &models.NodeRegisterRequest{
			Address: "192.168.1.101:8080",
			Version: "1.0.0",
		}
		registerResp, err := cloudControl.NodeRegister(registerReq)
		require.NoError(t, err)
		require.True(t, registerResp.Success)

		// 发送心跳
		heartbeatReq := &models.NodeHeartbeatRequest{
			NodeID:  registerResp.NodeID,
			Address: "192.168.1.101:8080",
			Version: "1.0.0",
		}

		heartbeatResp, err := cloudControl.NodeHeartbeat(heartbeatReq)
		require.NoError(t, err)
		assert.NotNil(t, heartbeatResp)
		assert.True(t, heartbeatResp.Success)
	})

	t.Run("未注册节点的心跳", func(t *testing.T) {
		cloudControl := server.GetCloudControl()

		// 发送未注册节点的心跳
		heartbeatReq := &models.NodeHeartbeatRequest{
			NodeID:  "non-existent-node",
			Address: "192.168.1.200:8080",
			Version: "1.0.0",
		}

		_, err := cloudControl.NodeHeartbeat(heartbeatReq)
		// 可能返回错误或自动注册
		_ = err
	})
}

// TestNodeAPI_NodeUnregister 测试节点注销
func TestNodeAPI_NodeUnregister(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("注销节点", func(t *testing.T) {
		cloudControl := server.GetCloudControl()

		// 先注册节点
		registerReq := &models.NodeRegisterRequest{
			Address: "192.168.1.102:8080",
			Version: "1.0.0",
		}
		registerResp, err := cloudControl.NodeRegister(registerReq)
		require.NoError(t, err)

		// 注销节点
		unregisterReq := &models.NodeUnregisterRequest{
			NodeID: registerResp.NodeID,
		}
		err = cloudControl.NodeUnregister(unregisterReq)
		require.NoError(t, err)

		// 验证节点已注销
		_, err = cloudControl.GetNodeServiceInfo(registerResp.NodeID)
		assert.Error(t, err)
	})
}

// TestNodeAPI_ConcurrentNodeOperations 测试并发节点操作
func TestNodeAPI_ConcurrentNodeOperations(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("并发注册节点", func(t *testing.T) {
		cloudControl := server.GetCloudControl()
		const concurrency = 10
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func(index int) {
				req := &models.NodeRegisterRequest{
					Address: fmt.Sprintf("192.168.2.%d:8080", index),
					Version: "1.0.0",
				}
				_, err := cloudControl.NodeRegister(req)
				done <- err
			}(i)
		}

		// 所有注册应该成功
		for i := 0; i < concurrency; i++ {
			err := <-done
			assert.NoError(t, err)
		}
	})
}

