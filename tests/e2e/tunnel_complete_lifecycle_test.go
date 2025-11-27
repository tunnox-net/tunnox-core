package e2e

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTunnel_CompleteLifecycle 测试完整的隧道生命周期
// 包括：匿名映射 -> 测试 -> 移除 -> 创建用户 -> 关联客户端 -> 新映射 -> 测试
func TestTunnel_CompleteLifecycle(t *testing.T) {
	SkipIfShort(t, "完整生命周期测试")

	t.Log("🚀 Starting Complete Tunnel Lifecycle Test...")
	t.Log("This test covers the full lifecycle:")
	t.Log("  1. Anonymous mapping creation")
	t.Log("  2. Test anonymous mapping")
	t.Log("  3. Remove mapping")
	t.Log("  4. Create user")
	t.Log("  5. Claim anonymous clients")
	t.Log("  6. Create new mapping")
	t.Log("  7. Test new mapping")

	// 启动环境
	compose := SetupE2EEnvironment(t, "docker-compose.full-tunnel.yml")

	// 等待基础设施
	t.Log("📋 Step 0: Waiting for infrastructure...")
	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 90*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 90*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 90*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)
	compose.WaitForHealthy("target-nginx", 30*time.Second)
	compose.WaitForHealthy("target-postgres", 60*time.Second)
	t.Log("✅ Infrastructure ready")

	apiClient := compose.GetAPIClient("http://localhost:19000")

	// 等待并获取匿名客户端
	var clientAID, clientBID int64
	t.Run("等待匿名客户端连接", func(t *testing.T) {
		var onlineClients []ClientResponse
		for i := 0; i < 15; i++ {
			allClients, err := apiClient.ListClients()
			if err != nil {
				t.Logf("  Attempt %d/15: Failed to list clients: %v", i+1, err)
				time.Sleep(2 * time.Second)
				continue
			}

			// 过滤在线匿名客户端
			onlineClientsMap := make(map[int64]ClientResponse)
			for _, client := range allClients {
				if client.Status == "online" && client.Type == "anonymous" {
					onlineClientsMap[client.ID] = client
				}
			}

			onlineClients = make([]ClientResponse, 0, len(onlineClientsMap))
			for _, client := range onlineClientsMap {
				onlineClients = append(onlineClients, client)
			}

			if len(onlineClients) >= 2 {
				t.Logf("✅ Found %d online anonymous clients after %d attempts", len(onlineClients), i+1)
				break
			}

			t.Logf("  Attempt %d/15: Only %d online clients", i+1, len(onlineClients))
			time.Sleep(2 * time.Second)
		}

		require.GreaterOrEqual(t, len(onlineClients), 2, "Need at least 2 online anonymous clients")
		clientAID = onlineClients[0].ID
		clientBID = onlineClients[1].ID
		t.Logf("✅ Client A: %d, Client B: %d", clientAID, clientBID)
	})

	// 创建临时用户用于映射
	var tempUserID string
	var mappingID1 string

	t.Run("1. 创建匿名映射", func(t *testing.T) {
		t.Log("📋 Step 1: Creating anonymous mapping...")

		// 创建临时用户
		user, err := apiClient.CreateUser(CreateUserRequest{
			Username: "temp-user",
			Password: "temp123",
			Email:    "temp@test.com",
		})
		require.NoError(t, err)
		tempUserID = user.ID
		t.Logf("✅ Temp user created: %s", tempUserID)

		// 创建映射
		mapping, err := apiClient.CreateMapping(CreateMappingRequest{
			UserID:         tempUserID,
			SourceClientID: clientAID,
			TargetClientID: clientBID,
			Protocol:       "tcp",
			SourcePort:     8080,
			TargetHost:     "target-nginx",
			TargetPort:     80,
			MappingName:    "anonymous-test-mapping",
		})
		require.NoError(t, err)
		mappingID1 = mapping.ID
		t.Logf("✅ Mapping created: %s", mappingID1)
		t.Logf("   Source: Client %d:8080", clientAID)
		t.Logf("   Target: target-nginx:80 via Client %d", clientBID)
	})

	t.Run("1.1 测试匿名映射", func(t *testing.T) {
		t.Log("📋 Step 1.1: Testing anonymous mapping...")
		
		// 等待配置推送和端口监听启动
		t.Log("Waiting for mapping to be active (10 seconds)...")
		time.Sleep(10 * time.Second)

		success := testTunnelConnection(t, "localhost:18080", 10)
		require.True(t, success, "Anonymous mapping should work")
		t.Log("✅ Anonymous mapping works!")
	})

	t.Run("2. 移除映射", func(t *testing.T) {
		t.Log("📋 Step 2: Removing mapping...")

		err := apiClient.DeleteMapping(mappingID1)
		require.NoError(t, err)
		t.Logf("✅ Mapping %s deleted", mappingID1)

		// 等待配置推送
		time.Sleep(5 * time.Second)

		// 验证映射不可用
		t.Log("Verifying mapping is removed...")
		success := testTunnelConnection(t, "localhost:18080", 3)
		assert.False(t, success, "Mapping should be removed")
		t.Log("✅ Mapping successfully removed")
	})

	var finalUserID string
	t.Run("3. 创建正式用户", func(t *testing.T) {
		t.Log("📋 Step 3: Creating permanent user...")

		user, err := apiClient.CreateUser(CreateUserRequest{
			Username: "lifecycle-user",
			Password: "user123",
			Email:    "lifecycle@test.com",
		})
		require.NoError(t, err)
		finalUserID = user.ID
		t.Logf("✅ User created: %s", finalUserID)
	})

	t.Run("4. 关联匿名客户端", func(t *testing.T) {
		t.Log("📋 Step 4: Claiming anonymous clients...")

		// 关联 Client A
		resultA, err := apiClient.ClaimClient(clientAID, finalUserID, "claimed-client-a")
		require.NoError(t, err)
		t.Logf("✅ Client A claimed: %v", resultA)

		// 关联 Client B
		resultB, err := apiClient.ClaimClient(clientBID, finalUserID, "claimed-client-b")
		require.NoError(t, err)
		t.Logf("✅ Client B claimed: %v", resultB)

		// 验证客户端已关联
		clients, err := apiClient.ListClients()
		require.NoError(t, err)

		claimedCount := 0
		for _, client := range clients {
			if client.UserID == finalUserID {
				claimedCount++
				t.Logf("  Found claimed client: ID=%d, Name=%s", client.ID, client.Name)
			}
		}
		assert.GreaterOrEqual(t, claimedCount, 2, "Should have at least 2 claimed clients")
	})

	var mappingID2 string
	t.Run("5. 创建新映射（已关联客户端）", func(t *testing.T) {
		t.Log("📋 Step 5: Creating new mapping with claimed clients...")

		mapping, err := apiClient.CreateMapping(CreateMappingRequest{
			UserID:         finalUserID,
			SourceClientID: clientAID,
			TargetClientID: clientBID,
			Protocol:       "tcp",
			SourcePort:     8080,
			TargetHost:     "target-nginx",
			TargetPort:     80,
			MappingName:    "claimed-test-mapping",
		})
		require.NoError(t, err)
		mappingID2 = mapping.ID
		t.Logf("✅ New mapping created: %s", mappingID2)
	})

	t.Run("5.1 测试新映射", func(t *testing.T) {
		t.Log("📋 Step 5.1: Testing new mapping...")

		// 等待配置推送
		t.Log("Waiting for new mapping to be active (10 seconds)...")
		time.Sleep(10 * time.Second)

		success := testTunnelConnection(t, "localhost:18080", 10)
		require.True(t, success, "New mapping should work")
		t.Log("✅ New mapping works!")
	})

	t.Log("✅ Complete lifecycle test finished successfully!")
}

// testTunnelConnection 测试隧道连接
// 返回 true 如果连接成功，false 如果失败
func testTunnelConnection(t *testing.T, address string, maxRetries int) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < maxRetries; i++ {
		t.Logf("  Attempt %d/%d: Testing connection to %s...", i+1, maxRetries, address)

		resp, err := client.Get(fmt.Sprintf("http://%s/", address))
		if err != nil {
			t.Logf("    ❌ Request failed: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			t.Logf("    ❌ Failed to read response: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode == 200 {
			t.Logf("    ✅ Success! Status: %d, Size: %d bytes", resp.StatusCode, len(body))
			return true
		}

		t.Logf("    ❌ Unexpected status: %d", resp.StatusCode)
		time.Sleep(2 * time.Second)
	}

	t.Logf("  ❌ Failed after %d attempts", maxRetries)
	return false
}

