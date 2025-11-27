// +build manual

package e2e

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestManual_FullTunnel 手动测试完整隧道（不自动清理）
// 运行方式: go test -v ./tests/e2e/... -tags manual -run TestManual_FullTunnel
func TestManual_FullTunnel(t *testing.T) {
	t.Log("🚀 Starting Manual Full Tunnel Test (no auto-cleanup)...")

	compose := SetupE2EEnvironment(t, "docker-compose.full-tunnel.yml")

	// 等待服务
	t.Log("⏳ Waiting for services...")
	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 90*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 90*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 90*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)
	compose.WaitForHealthy("target-nginx", 30*time.Second)
	compose.WaitForHealthy("target-postgres", 60*time.Second)

	t.Log("✅ All services healthy")

	// 获取API客户端
	apiClient := compose.GetAPIClient("http://localhost:19000")

	// 等待客户端连接
	t.Log("⏳ Waiting for clients to connect (15s)...")
	time.Sleep(15 * time.Second)

	// 列出客户端
	allClients, err := apiClient.ListClients()
	require.NoError(t, err)
	t.Logf("Found %d total clients", len(allClients))

	// 过滤在线匿名客户端
	onlineClientsMap := make(map[int64]ClientResponse)
	for _, client := range allClients {
		if client.Status == "online" && client.Type == "anonymous" {
			onlineClientsMap[client.ID] = client
		}
	}

	onlineClients := make([]ClientResponse, 0, len(onlineClientsMap))
	for _, client := range onlineClientsMap {
		onlineClients = append(onlineClients, client)
	}

	t.Logf("Found %d online anonymous clients", len(onlineClients))
	for i, client := range onlineClients {
		t.Logf("  Client[%d]: ID=%d, Name=%s", i, client.ID, client.Name)
	}

	require.GreaterOrEqual(t, len(onlineClients), 2, "Need at least 2 online clients")

	clientAID := onlineClients[0].ID
	clientBID := onlineClients[1].ID
	t.Logf("Using ClientA=%d, ClientB=%d", clientAID, clientBID)

	// 创建用户
	user, err := apiClient.CreateUser(CreateUserRequest{
		Username: "manual-test",
		Password: "test123",
		Email:    "manual@test.com",
	})
	require.NoError(t, err)
	t.Logf("✅ User created: %s", user.ID)

	// 创建映射
	mapping, err := apiClient.CreateMapping(CreateMappingRequest{
		UserID:         user.ID,
		SourceClientID: clientAID,
		TargetClientID: clientBID,
		Protocol:       "tcp",
		SourcePort:     8080,
		TargetHost:     "target-nginx",
		TargetPort:     80,
		MappingName:    "manual-test-mapping",
	})
	require.NoError(t, err)
	t.Logf("✅ Mapping created: %s", mapping.ID)
	t.Logf("   Source: Client%d:8080", clientAID)
	t.Logf("   Target: target-nginx:80 via Client%d", clientBID)

	// 等待配置推送
	t.Log("⏳ Waiting for config push (30s)...")
	time.Sleep(30 * time.Second)

	// 测试连接
	t.Log("📋 Testing connection through tunnel...")
	for i := 0; i < 5; i++ {
		t.Logf("Attempt %d/5...", i+1)

		conn, err := net.DialTimeout("tcp", "localhost:18080", 3*time.Second)
		if err != nil {
			t.Logf("  ❌ Connection failed: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		t.Log("  ✓ TCP connected")

		request := "GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
		n, err := conn.Write([]byte(request))
		if err != nil {
			conn.Close()
			t.Logf("  ❌ Write failed: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		t.Logf("  ✓ Wrote %d bytes", n)

		response := make([]byte, 4096)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		totalRead := 0
		for totalRead < len(response) {
			n, err := conn.Read(response[totalRead:])
			if n > 0 {
				totalRead += n
			}
			if err != nil {
				break
			}
		}
		conn.Close()

		t.Logf("  Read %d bytes", totalRead)
		if totalRead > 0 {
			t.Logf("  Response: %s", string(response[:min(200, totalRead)]))
			t.Log("✅ SUCCESS!")
			break
		}

		t.Log("  ❌ Read 0 bytes")
		time.Sleep(3 * time.Second)
	}

	t.Log("")
	t.Log("=== Environment is still running ===")
	t.Log("Check client logs with:")
	t.Log("  docker logs $(docker ps --filter 'name=client-a' --format '{{.ID}}' | head -1) 2>&1 | tail -100")
	t.Log("  docker logs $(docker ps --filter 'name=client-b' --format '{{.ID}}' | head -1) 2>&1 | tail -100")
	t.Log("")
	t.Log("Cleanup with:")
	t.Log("  docker-compose -f tests/e2e/docker-compose.full-tunnel.yml down")
	t.Log("")
	t.Log("Press Ctrl+C to stop (environment will remain running)")
	
	// 保持测试运行，不自动清理
	select {}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

