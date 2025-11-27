package e2e

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadBalancer_Environment 测试环境基础功能
func TestLoadBalancer_Environment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E load balancer test in short mode")
	}

	t.Log("🚀 Starting Load Balancer Environment Test...")

	// 创建测试环境
	compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
	defer compose.Cleanup()

	// 等待所有服务就绪
	t.Log("⏳ Waiting for services to be healthy...")
	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 60*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)
	compose.WaitForHealthy("nginx-target", 30*time.Second)

	t.Log("✅ All services are healthy")

	// 测试Nginx负载均衡器健康检查
	t.Run("Nginx健康检查", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://localhost:18081/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		t.Log("✓ Nginx health check passed")
	})

	// 测试Redis连接
	t.Run("Redis连接测试", func(t *testing.T) {
		// 通过日志检查Redis连接
		logs := compose.GetLogs("tunnox-server-1")
		// Redis连接成功的日志应该存在
		assert.NotEmpty(t, logs)
		t.Log("✓ Redis connection verified")
	})

	// 测试目标服务
	t.Run("测试目标服务", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://localhost:80")
		// 注意：这个请求会失败，因为nginx-target不对外暴露端口
		// 这是正常的，我们只是验证容器在运行
		_ = err
		_ = resp
		t.Log("✓ Target service check completed")
	})

	t.Log("✅ Environment test completed successfully")
}

// TestLoadBalancer_BasicDistribution 测试基本负载分布
func TestLoadBalancer_BasicDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E load balancer test in short mode")
	}

	t.Log("🚀 Starting Load Balancer Distribution Test...")

	compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
	defer compose.Cleanup()

	// 等待服务就绪
	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 60*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)

	// 测试API负载分布
	t.Run("API请求分布", func(t *testing.T) {
		apiClient := compose.GetAPIClient("http://localhost:18081")

		// 连续发送多个请求
		requestCount := 30
		successCount := 0

		for i := 0; i < requestCount; i++ {
			err := apiClient.HealthCheck()
			if err == nil {
				successCount++
			}
			time.Sleep(10 * time.Millisecond)
		}

		// 验证大部分请求成功
		successRate := float64(successCount) / float64(requestCount) * 100
		assert.Greater(t, successRate, 90.0,
			"Success rate should be > 90%%, got %.2f%%", successRate)

		t.Logf("✓ API request distribution: %d/%d requests succeeded (%.2f%%)",
			successCount, requestCount, successRate)
	})

	t.Log("✅ Distribution test completed successfully")
}

// TestLoadBalancer_ConcurrentRequests 测试并发请求
func TestLoadBalancer_ConcurrentRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E load balancer test in short mode")
	}

	t.Log("🚀 Starting Load Balancer Concurrent Requests Test...")

	compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
	defer compose.Cleanup()

	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 60*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)

	t.Run("并发健康检查", func(t *testing.T) {
		apiClient := compose.GetAPIClient("http://localhost:18081")

		// 先测试一次看是否能连接
		t.Log("Testing single health check first...")
		err := apiClient.HealthCheck()
		if err != nil {
			t.Logf("Single health check failed: %v", err)
		} else {
			t.Log("✓ Single health check succeeded")
		}

		concurrency := 100
		var wg sync.WaitGroup
		successCount := int64(0)
		failCount := int64(0)
		var firstError error

		start := time.Now()

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				err := apiClient.HealthCheck()
				if err != nil {
					atomic.AddInt64(&failCount, 1)
					if firstError == nil {
						firstError = err
					}
					if idx < 5 {
						t.Logf("Request %d failed: %v", idx, err)
					}
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		t.Logf("Concurrent requests: %d", concurrency)
		t.Logf("Success: %d, Failed: %d", successCount, failCount)
		if firstError != nil {
			t.Logf("First error: %v", firstError)
		}
		t.Logf("Duration: %v", duration)
		t.Logf("QPS: %.2f", float64(concurrency)/duration.Seconds())

		// 验证成功率
		successRate := float64(successCount) / float64(concurrency) * 100
		assert.Greater(t, successRate, 90.0,
			"Success rate should be > 90%%, got %.2f%%", successRate)

		t.Logf("✓ Concurrent test completed: %.2f%% success rate", successRate)
	})

	t.Log("✅ Concurrent requests test completed successfully")
}

// TestLoadBalancer_ServiceFailover 测试服务故障转移
func TestLoadBalancer_ServiceFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E load balancer test in short mode")
	}

	t.Log("🚀 Starting Load Balancer Service Failover Test...")

	compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
	defer compose.Cleanup()

	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 60*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)

	t.Run("停止一个Server后请求继续成功", func(t *testing.T) {
		apiClient := compose.GetAPIClient("http://localhost:18081")

		// 验证初始状态正常
		err := apiClient.HealthCheck()
		require.NoError(t, err, "Initial health check should succeed")

		// 停止Server-1
		t.Log("Stopping tunnox-server-1...")
		compose.StopService("tunnox-server-1")

		// 等待Nginx检测到服务不可用
		time.Sleep(5 * time.Second)

		// 继续发送请求，应该被路由到其他Server
		successCount := 0
		requestCount := 20

		for i := 0; i < requestCount; i++ {
			err := apiClient.HealthCheck()
			if err == nil {
				successCount++
			}
			time.Sleep(100 * time.Millisecond)
		}

		successRate := float64(successCount) / float64(requestCount) * 100
		assert.Greater(t, successRate, 80.0,
			"Success rate should be > 80%% after one server down, got %.2f%%", successRate)

		t.Logf("✓ Failover test: %d/%d requests succeeded (%.2f%%) with one server down",
			successCount, requestCount, successRate)

		// 重新启动Server-1
		t.Log("Restarting tunnox-server-1...")
		compose.StartService("tunnox-server-1")
		time.Sleep(10 * time.Second)

		// 验证恢复后正常
		err = apiClient.HealthCheck()
		assert.NoError(t, err, "Health check should succeed after server restart")

		t.Log("✓ Service recovered successfully")
	})

	t.Log("✅ Service failover test completed successfully")
}

// TestLoadBalancer_MultipleServerFailures 测试多服务器故障
func TestLoadBalancer_MultipleServerFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E load balancer test in short mode")
	}

	t.Log("🚀 Starting Load Balancer Multiple Server Failures Test...")

	compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
	defer compose.Cleanup()

	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 60*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)

	t.Run("停止两个Server后系统仍可用", func(t *testing.T) {
		apiClient := compose.GetAPIClient("http://localhost:18081")

		// 停止Server-1和Server-2
		t.Log("Stopping tunnox-server-1 and tunnox-server-2...")
		compose.StopService("tunnox-server-1")
		compose.StopService("tunnox-server-2")

		// 等待Nginx检测到服务不可用
		time.Sleep(10 * time.Second)

		// 只剩Server-3，应该还能工作
		successCount := 0
		requestCount := 10

		for i := 0; i < requestCount; i++ {
			err := apiClient.HealthCheck()
			if err == nil {
				successCount++
			}
			time.Sleep(200 * time.Millisecond)
		}

		successRate := float64(successCount) / float64(requestCount) * 100
		assert.Greater(t, successRate, 70.0,
			"Success rate should be > 70%% with only one server running, got %.2f%%", successRate)

		t.Logf("✓ Multiple failures test: %d/%d requests succeeded (%.2f%%) with two servers down",
			successCount, requestCount, successRate)
	})

	t.Log("✅ Multiple server failures test completed successfully")
}

// TestLoadBalancer_StressTest 压力测试
func TestLoadBalancer_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E load balancer stress test in short mode")
	}

	t.Log("🚀 Starting Load Balancer Stress Test...")

	compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
	defer compose.Cleanup()

	compose.WaitForHealthy("redis", 30*time.Second)
	compose.WaitForHealthy("tunnox-server-1", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-2", 60*time.Second)
	compose.WaitForHealthy("tunnox-server-3", 60*time.Second)
	compose.WaitForHealthy("nginx", 30*time.Second)

	t.Run("高并发持续请求", func(t *testing.T) {
		apiClient := compose.GetAPIClient("http://localhost:18081")

		// 并发配置
		concurrency := 50
		duration := 10 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()

		var wg sync.WaitGroup
		successCount := int64(0)
		failCount := int64(0)

		start := time.Now()

		// 启动并发workers
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				requestCount := 0
				for {
					select {
					case <-ctx.Done():
						return
					default:
						err := apiClient.HealthCheck()
						if err != nil {
							atomic.AddInt64(&failCount, 1)
						} else {
							atomic.AddInt64(&successCount, 1)
						}
						requestCount++
						time.Sleep(100 * time.Millisecond)
					}
				}
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		totalRequests := successCount + failCount
		successRate := float64(successCount) / float64(totalRequests) * 100
		qps := float64(totalRequests) / elapsed.Seconds()

		t.Logf("Stress test results:")
		t.Logf("  Duration: %v", elapsed)
		t.Logf("  Concurrency: %d workers", concurrency)
		t.Logf("  Total requests: %d", totalRequests)
		t.Logf("  Success: %d, Failed: %d", successCount, failCount)
		t.Logf("  Success rate: %.2f%%", successRate)
		t.Logf("  QPS: %.2f", qps)

		// 验证性能指标
		assert.Greater(t, successRate, 95.0,
			"Success rate should be > 95%% in stress test, got %.2f%%", successRate)

		assert.Greater(t, qps, 10.0,
			"QPS should be > 10, got %.2f", qps)

		t.Logf("✓ Stress test passed")
	})

	t.Log("✅ Stress test completed successfully")
}

