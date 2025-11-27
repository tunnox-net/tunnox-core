# E2E 负载均衡器测试计划

**创建时间**: 2025-11-27  
**场景**: Nginx负载均衡 + 多Server实例 + 共享Redis  
**核心挑战**: 命令通道和映射连接可能落在不同Server上  

---

## 🎯 测试场景概述

### 架构拓扑

```
                         ┌──────────────┐
                         │   Nginx LB   │  (负载均衡器)
                         │ :7000 (TCP)  │
                         │ :7001 (WS)   │
                         │ :7002 (UDP)  │
                         │ :7003 (QUIC) │
                         └──────┬───────┘
                                │
                ┌───────────────┼───────────────┐
                │               │               │
        ┌───────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
        │ Server-1     │ │ Server-2   │ │ Server-3   │
        │ (node-1)     │ │ (node-2)   │ │ (node-3)   │
        └──────┬───────┘ └─────┬──────┘ └─────┬──────┘
               │               │               │
               └───────────────┼───────────────┘
                               │
                        ┌──────▼───────┐
                        │    Redis     │  (共享存储+MQ)
                        │   :6379      │
                        └──────────────┘
                               ▲
                               │
                        ┌──────┴───────┐
                        │  RabbitMQ    │  (可选，高级MQ)
                        │   :5672      │
                        └──────────────┘

客户端A (控制连接) ──→ Nginx ──→ 可能到 Server-1
客户端A (隧道1)    ──→ Nginx ──→ 可能到 Server-2
客户端A (隧道2)    ──→ Nginx ──→ 可能到 Server-3
客户端B (控制连接) ──→ Nginx ──→ 可能到 Server-2
客户端B (隧道1)    ──→ Nginx ──→ 可能到 Server-1
```

### 核心挑战

1. **控制连接和隧道连接分离**
   - 客户端A的控制连接在Server-1
   - 客户端A的隧道连接可能在Server-2
   - 客户端B的控制连接在Server-3
   - A→B的隧道桥接需要跨Server通信

2. **会话状态共享**
   - Session信息存储在Redis
   - 需要所有Server能查询到所有客户端
   - 需要分布式锁保证一致性

3. **消息路由**
   - 配置推送需要通过消息队列
   - TunnelOpen请求需要跨Server路由
   - 心跳检测需要全局协调

4. **负载均衡策略**
   - Round-robin（轮询）
   - Least connections（最少连接）
   - IP Hash（源IP哈希）
   - 不同策略对测试的影响

---

## 📋 测试计划

### 阶段6.1: 负载均衡基础测试 (1天)

#### 测试目标
验证基本的负载均衡功能和跨Server通信

#### Docker Compose 配置

**文件**: `tests/e2e/docker-compose.load-balancer.yml`

```yaml
version: '3.8'

services:
  # Redis (共享存储和消息队列)
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  # RabbitMQ (可选，高级消息队列)
  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "5672:5672"
      - "15672:15672"
    environment:
      RABBITMQ_DEFAULT_USER: tunnox
      RABBITMQ_DEFAULT_PASS: tunnox123
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Tunnox Server 实例1
  tunnox-server-1:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    environment:
      - NODE_ID=node-1
      - NODE_NAME=Server-1
      - LISTEN_ADDR=0.0.0.0:7000
      - WS_ADDR=0.0.0.0:7001
      - UDP_ADDR=0.0.0.0:7002
      - QUIC_ADDR=0.0.0.0:7003
      - API_ADDR=0.0.0.0:8080
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
      - LOG_LEVEL=info
    depends_on:
      redis:
        condition: service_healthy
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  # Tunnox Server 实例2
  tunnox-server-2:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    environment:
      - NODE_ID=node-2
      - NODE_NAME=Server-2
      - LISTEN_ADDR=0.0.0.0:7000
      - WS_ADDR=0.0.0.0:7001
      - UDP_ADDR=0.0.0.0:7002
      - QUIC_ADDR=0.0.0.0:7003
      - API_ADDR=0.0.0.0:8080
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
      - LOG_LEVEL=info
    depends_on:
      redis:
        condition: service_healthy
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  # Tunnox Server 实例3
  tunnox-server-3:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    environment:
      - NODE_ID=node-3
      - NODE_NAME=Server-3
      - LISTEN_ADDR=0.0.0.0:7000
      - WS_ADDR=0.0.0.0:7001
      - UDP_ADDR=0.0.0.0:7002
      - QUIC_ADDR=0.0.0.0:7003
      - API_ADDR=0.0.0.0:8080
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
      - LOG_LEVEL=info
    depends_on:
      redis:
        condition: service_healthy
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  # Tunnox Server 实例4 (可选，用于扩容测试)
  tunnox-server-4:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    environment:
      - NODE_ID=node-4
      - NODE_NAME=Server-4
      - LISTEN_ADDR=0.0.0.0:7000
      - WS_ADDR=0.0.0.0:7001
      - UDP_ADDR=0.0.0.0:7002
      - QUIC_ADDR=0.0.0.0:7003
      - API_ADDR=0.0.0.0:8080
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
      - LOG_LEVEL=info
    depends_on:
      redis:
        condition: service_healthy
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
    profiles:
      - scale  # 仅在扩容测试时启动

  # Tunnox Server 实例5 (可选，用于扩容测试)
  tunnox-server-5:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    environment:
      - NODE_ID=node-5
      - NODE_NAME=Server-5
      - LISTEN_ADDR=0.0.0.0:7000
      - WS_ADDR=0.0.0.0:7001
      - UDP_ADDR=0.0.0.0:7002
      - QUIC_ADDR=0.0.0.0:7003
      - API_ADDR=0.0.0.0:8080
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
      - LOG_LEVEL=info
    depends_on:
      redis:
        condition: service_healthy
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
    profiles:
      - scale  # 仅在扩容测试时启动

  # Nginx 负载均衡器
  nginx:
    image: nginx:alpine
    ports:
      - "7000:7000"   # TCP
      - "7001:7001"   # WebSocket
      - "7002:7002/udp"   # UDP
      - "7003:7003/udp"   # QUIC
      - "8080:8080"   # Management API
    volumes:
      - ./nginx/load-balancer.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      tunnox-server-1:
        condition: service_healthy
      tunnox-server-2:
        condition: service_healthy
      tunnox-server-3:
        condition: service_healthy
    networks:
      - tunnox-net

  # 测试目标服务 - Nginx (用于验证隧道转发)
  nginx-target:
    image: nginx:alpine
    volumes:
      - ./nginx/html:/usr/share/nginx/html:ro
    networks:
      - tunnox-net

  # 测试目标服务 - PostgreSQL
  postgres-target:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: admin
      POSTGRES_PASSWORD: dtcpay
      POSTGRES_DB: testdb
    networks:
      - tunnox-net

networks:
  tunnox-net:
    driver: bridge
```

#### Nginx 负载均衡配置

**文件**: `tests/e2e/nginx/load-balancer.conf`

```nginx
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 10000;
    use epoll;
}

stream {
    # TCP 负载均衡 (端口 7000)
    upstream tunnox_tcp {
        # 负载均衡策略: least_conn (最少连接)
        least_conn;
        
        server tunnox-server-1:7000 max_fails=3 fail_timeout=30s;
        server tunnox-server-2:7000 max_fails=3 fail_timeout=30s;
        server tunnox-server-3:7000 max_fails=3 fail_timeout=30s;
    }

    server {
        listen 7000;
        proxy_pass tunnox_tcp;
        proxy_timeout 3600s;
        proxy_connect_timeout 10s;
    }

    # UDP 负载均衡 (端口 7002)
    upstream tunnox_udp {
        # UDP 使用 hash 策略保证同一客户端到同一后端
        hash $remote_addr consistent;
        
        server tunnox-server-1:7002 max_fails=3 fail_timeout=30s;
        server tunnox-server-2:7002 max_fails=3 fail_timeout=30s;
        server tunnox-server-3:7002 max_fails=3 fail_timeout=30s;
    }

    server {
        listen 7002 udp;
        proxy_pass tunnox_udp;
        proxy_timeout 30s;
        proxy_responses 1;
    }

    # QUIC 负载均衡 (端口 7003)
    upstream tunnox_quic {
        hash $remote_addr consistent;
        
        server tunnox-server-1:7003 max_fails=3 fail_timeout=30s;
        server tunnox-server-2:7003 max_fails=3 fail_timeout=30s;
        server tunnox-server-3:7003 max_fails=3 fail_timeout=30s;
    }

    server {
        listen 7003 udp;
        proxy_pass tunnox_quic;
        proxy_timeout 30s;
        proxy_responses 1;
    }
}

http {
    # WebSocket 负载均衡 (端口 7001)
    upstream tunnox_ws {
        # WebSocket 需要保持连接，使用 ip_hash
        ip_hash;
        
        server tunnox-server-1:7001 max_fails=3 fail_timeout=30s;
        server tunnox-server-2:7001 max_fails=3 fail_timeout=30s;
        server tunnox-server-3:7001 max_fails=3 fail_timeout=30s;
    }

    server {
        listen 7001;
        
        location / {
            proxy_pass http://tunnox_ws;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_read_timeout 3600s;
        }
    }

    # Management API 负载均衡 (端口 8080)
    upstream tunnox_api {
        # API 使用轮询
        least_conn;
        
        server tunnox-server-1:8080 max_fails=3 fail_timeout=30s;
        server tunnox-server-2:8080 max_fails=3 fail_timeout=30s;
        server tunnox-server-3:8080 max_fails=3 fail_timeout=30s;
    }

    server {
        listen 8080;
        
        location / {
            proxy_pass http://tunnox_api;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        # 健康检查端点不走负载均衡，直接返回
        location /health {
            access_log off;
            return 200 "healthy\n";
            add_header Content-Type text/plain;
        }
    }
}
```

#### 测试用例

**文件**: `tests/e2e/load_balancer_test.go`

```go
package e2e

import (
    "context"
    "fmt"
    "net/http"
    "sync"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestLoadBalancer_ControlAndTunnelSeparation 测试控制连接和隧道连接分离
func TestLoadBalancer_ControlAndTunnelSeparation(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E load balancer test in short mode")
    }
    
    // 1. 启动环境 (3个Server + Nginx LB)
    compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
    defer compose.Cleanup()
    
    // 2. 等待所有Server就绪
    compose.WaitForHealthy("tunnox-server-1", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-2", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-3", 30*time.Second)
    compose.WaitForHealthy("nginx", 10*time.Second)
    
    // 3. 创建多个客户端连接
    clients := make([]*TestClient, 10)
    for i := 0; i < 10; i++ {
        client := NewTestClient(t, &ClientConfig{
            Protocol: "tcp",
            ServerAddr: "localhost:7000", // 通过Nginx负载均衡
        })
        clients[i] = client
        require.NoError(t, client.Connect())
    }
    
    // 4. 查询每个客户端的控制连接所在Server
    apiClient := compose.GetAPIClient("http://localhost:8080")
    
    serverDistribution := make(map[string]int)
    for i, client := range clients {
        clientInfo, err := apiClient.GetClient(client.ID)
        require.NoError(t, err)
        
        nodeID := clientInfo.NodeID
        serverDistribution[nodeID]++
        
        t.Logf("Client %d: control connection on %s", i, nodeID)
    }
    
    // 5. 验证负载分布（至少2个Server有连接）
    assert.GreaterOrEqual(t, len(serverDistribution), 2, 
        "Control connections should be distributed across at least 2 servers")
    
    // 6. 创建映射（客户端0 -> 客户端5）
    mapping, err := apiClient.CreateMapping(&MappingRequest{
        SourceClientID: clients[0].ID,
        TargetClientID: clients[5].ID,
        Protocol:       "tcp",
        TargetHost:     "nginx-target",
        TargetPort:     80,
    })
    require.NoError(t, err)
    
    // 7. 等待映射生效
    time.Sleep(2 * time.Second)
    
    // 8. 建立多个隧道连接
    var wg sync.WaitGroup
    tunnelCount := 20
    successCount := 0
    var mu sync.Mutex
    
    for i := 0; i < tunnelCount; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            
            // 通过映射的源端口访问
            resp, err := http.Get(fmt.Sprintf("http://localhost:%d", mapping.SourcePort))
            if err != nil {
                t.Logf("Tunnel %d: failed to connect: %v", idx, err)
                return
            }
            defer resp.Body.Close()
            
            if resp.StatusCode == 200 {
                mu.Lock()
                successCount++
                mu.Unlock()
            }
        }(i)
    }
    
    wg.Wait()
    
    // 9. 验证所有隧道连接成功
    assert.Equal(t, tunnelCount, successCount, 
        "All tunnel connections should succeed even when distributed across servers")
    
    // 10. 查询隧道连接分布
    connections, err := apiClient.GetMappingConnections(mapping.ID)
    require.NoError(t, err)
    
    tunnelServerDist := make(map[string]int)
    for _, conn := range connections {
        tunnelServerDist[conn.NodeID]++
    }
    
    t.Logf("Tunnel distribution: %v", tunnelServerDist)
    
    // 11. 验证隧道连接分布到了多个Server
    assert.GreaterOrEqual(t, len(tunnelServerDist), 2, 
        "Tunnel connections should be distributed across multiple servers")
}

// TestLoadBalancer_CrossServerMessaging 测试跨Server消息传递
func TestLoadBalancer_CrossServerMessaging(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E load balancer test in short mode")
    }
    
    compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
    defer compose.Cleanup()
    
    compose.WaitForHealthy("tunnox-server-1", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-2", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-3", 30*time.Second)
    
    apiClient := compose.GetAPIClient("http://localhost:8080")
    
    // 创建用户和客户端
    user, err := apiClient.CreateUser("testuser", "test@example.com")
    require.NoError(t, err)
    
    // 创建映射
    mapping, err := apiClient.CreateMapping(&MappingRequest{
        UserID:         user.ID,
        SourceClientID: 1,
        TargetClientID: 2,
        Protocol:       "tcp",
        TargetHost:     "nginx-target",
        TargetPort:     80,
    })
    require.NoError(t, err)
    
    // 等待配置推送通过消息队列传递到所有Server
    time.Sleep(3 * time.Second)
    
    // 从不同的Server查询映射信息，应该都能查到
    for i := 1; i <= 3; i++ {
        serverAPI := compose.GetAPIClient(fmt.Sprintf("http://tunnox-server-%d:8080", i))
        
        fetchedMapping, err := serverAPI.GetMapping(mapping.ID)
        require.NoError(t, err, "Server %d should have the mapping", i)
        assert.Equal(t, mapping.ID, fetchedMapping.ID)
        
        t.Logf("✓ Server-%d has mapping %s", i, mapping.ID)
    }
    
    // 更新映射
    mapping.Status = "disabled"
    err = apiClient.UpdateMapping(mapping)
    require.NoError(t, err)
    
    // 等待更新传播
    time.Sleep(2 * time.Second)
    
    // 验证所有Server都收到了更新
    for i := 1; i <= 3; i++ {
        serverAPI := compose.GetAPIClient(fmt.Sprintf("http://tunnox-server-%d:8080", i))
        
        fetchedMapping, err := serverAPI.GetMapping(mapping.ID)
        require.NoError(t, err)
        assert.Equal(t, "disabled", string(fetchedMapping.Status))
        
        t.Logf("✓ Server-%d received mapping update", i)
    }
}

// TestLoadBalancer_NodeFailover 测试节点故障转移
func TestLoadBalancer_NodeFailover(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E load balancer test in short mode")
    }
    
    compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
    defer compose.Cleanup()
    
    compose.WaitForHealthy("tunnox-server-1", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-2", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-3", 30*time.Second)
    
    // 创建客户端
    client := NewTestClient(t, &ClientConfig{
        Protocol:   "tcp",
        ServerAddr: "localhost:7000",
        AutoReconnect: true,
    })
    require.NoError(t, client.Connect())
    
    apiClient := compose.GetAPIClient("http://localhost:8080")
    
    // 查询客户端连接的Server
    clientInfo, err := apiClient.GetClient(client.ID)
    require.NoError(t, err)
    originalNode := clientInfo.NodeID
    
    t.Logf("Client initially connected to %s", originalNode)
    
    // 创建映射并验证工作正常
    mapping, err := apiClient.CreateMapping(&MappingRequest{
        SourceClientID: client.ID,
        TargetClientID: 99999, // 假设存在
        Protocol:       "tcp",
        TargetHost:     "nginx-target",
        TargetPort:     80,
    })
    require.NoError(t, err)
    
    // 停止客户端连接的Server
    t.Logf("Stopping %s...", originalNode)
    compose.StopService(originalNode)
    
    // 等待客户端重连到其他Server
    time.Sleep(5 * time.Second)
    
    // 验证客户端已重连
    clientInfo, err = apiClient.GetClient(client.ID)
    require.NoError(t, err)
    newNode := clientInfo.NodeID
    
    assert.NotEqual(t, originalNode, newNode, "Client should reconnect to a different server")
    assert.Equal(t, "online", clientInfo.Status, "Client should be online after reconnection")
    
    t.Logf("✓ Client reconnected to %s", newNode)
    
    // 验证映射仍然存在且可访问
    fetchedMapping, err := apiClient.GetMapping(mapping.ID)
    require.NoError(t, err)
    assert.Equal(t, mapping.ID, fetchedMapping.ID)
    
    t.Logf("✓ Mapping still accessible after node failure")
}

// TestLoadBalancer_DifferentStrategies 测试不同的负载均衡策略
func TestLoadBalancer_DifferentStrategies(t *testing.T) {
    strategies := []struct {
        name           string
        nginxConfig    string
        expectedBehavior string
    }{
        {
            name:           "round-robin",
            nginxConfig:    "round-robin.conf",
            expectedBehavior: "均匀分布",
        },
        {
            name:           "least-conn",
            nginxConfig:    "least-conn.conf",
            expectedBehavior: "倾向连接数少的Server",
        },
        {
            name:           "ip-hash",
            nginxConfig:    "ip-hash.conf",
            expectedBehavior: "相同IP到相同Server",
        },
    }
    
    for _, strategy := range strategies {
        t.Run(strategy.name, func(t *testing.T) {
            // 为每个策略启动独立的测试环境
            compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
            defer compose.Cleanup()
            
            // 更新Nginx配置
            compose.UpdateNginxConfig(strategy.nginxConfig)
            
            // 创建大量客户端
            clientCount := 100
            clients := make([]*TestClient, clientCount)
            
            for i := 0; i < clientCount; i++ {
                client := NewTestClient(t, &ClientConfig{
                    Protocol:   "tcp",
                    ServerAddr: "localhost:7000",
                })
                require.NoError(t, client.Connect())
                clients[i] = client
            }
            
            // 统计分布
            apiClient := compose.GetAPIClient("http://localhost:8080")
            distribution := make(map[string]int)
            
            for _, client := range clients {
                clientInfo, err := apiClient.GetClient(client.ID)
                require.NoError(t, err)
                distribution[clientInfo.NodeID]++
            }
            
            t.Logf("Strategy: %s", strategy.name)
            t.Logf("Distribution: %v", distribution)
            t.Logf("Expected behavior: %s", strategy.expectedBehavior)
            
            // 验证所有Server都有连接
            assert.Equal(t, 3, len(distribution), 
                "All 3 servers should have connections")
        })
    }
}

// TestLoadBalancer_HighConcurrency 测试高并发场景
func TestLoadBalancer_HighConcurrency(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E load balancer test in short mode")
    }
    
    compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
    defer compose.Cleanup()
    
    compose.WaitForHealthy("tunnox-server-1", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-2", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-3", 30*time.Second)
    
    apiClient := compose.GetAPIClient("http://localhost:8080")
    
    // 创建映射
    mapping, err := apiClient.CreateMapping(&MappingRequest{
        SourceClientID: 1,
        TargetClientID: 2,
        Protocol:       "tcp",
        TargetHost:     "nginx-target",
        TargetPort:     80,
    })
    require.NoError(t, err)
    
    time.Sleep(2 * time.Second)
    
    // 并发建立1000个连接
    concurrentConns := 1000
    var wg sync.WaitGroup
    successCount := int64(0)
    failCount := int64(0)
    
    start := time.Now()
    
    for i := 0; i < concurrentConns; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            resp, err := http.Get(fmt.Sprintf("http://localhost:%d", mapping.SourcePort))
            if err != nil {
                atomic.AddInt64(&failCount, 1)
                return
            }
            defer resp.Body.Close()
            
            if resp.StatusCode == 200 {
                atomic.AddInt64(&successCount, 1)
            } else {
                atomic.AddInt64(&failCount, 1)
            }
        }()
    }
    
    wg.Wait()
    duration := time.Since(start)
    
    t.Logf("Concurrent connections: %d", concurrentConns)
    t.Logf("Success: %d, Failed: %d", successCount, failCount)
    t.Logf("Duration: %v", duration)
    t.Logf("QPS: %.2f", float64(concurrentConns)/duration.Seconds())
    
    // 验证成功率 > 95%
    successRate := float64(successCount) / float64(concurrentConns) * 100
    assert.Greater(t, successRate, 95.0, 
        "Success rate should be > 95% in load balanced environment")
}

// TestLoadBalancer_DynamicScaling 测试动态扩缩容
func TestLoadBalancer_DynamicScaling(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E load balancer test in short mode")
    }
    
    compose := NewDockerComposeEnv(t, "docker-compose.load-balancer.yml")
    defer compose.Cleanup()
    
    // 初始只启动2个Server
    compose.WaitForHealthy("tunnox-server-1", 30*time.Second)
    compose.WaitForHealthy("tunnox-server-2", 30*time.Second)
    
    apiClient := compose.GetAPIClient("http://localhost:8080")
    
    // 创建50个客户端
    clients := createClients(t, 50, "localhost:7000")
    
    // 统计初始分布（应该分布在2个Server上）
    dist1 := getClientDistribution(t, apiClient, clients)
    assert.Equal(t, 2, len(dist1), "Should distribute across 2 servers initially")
    t.Logf("Initial distribution (2 servers): %v", dist1)
    
    // 扩容：启动第3个Server
    t.Log("Scaling up: starting server-3...")
    compose.StartService("tunnox-server-3")
    compose.WaitForHealthy("tunnox-server-3", 30*time.Second)
    
    // 更新Nginx配置以包含新Server（或等待Nginx自动检测）
    compose.ReloadNginx()
    
    // 创建更多客户端
    newClients := createClients(t, 50, "localhost:7000")
    clients = append(clients, newClients...)
    
    // 统计扩容后分布（应该分布在3个Server上）
    dist2 := getClientDistribution(t, apiClient, clients)
    assert.Equal(t, 3, len(dist2), "Should distribute across 3 servers after scaling")
    t.Logf("After scaling up (3 servers): %v", dist2)
    
    // 缩容：停止Server-1
    t.Log("Scaling down: stopping server-1...")
    compose.StopService("tunnox-server-1")
    
    // 等待客户端重连
    time.Sleep(10 * time.Second)
    
    // 统计缩容后分布（应该分布在2个Server上）
    dist3 := getClientDistribution(t, apiClient, clients)
    assert.Equal(t, 2, len(dist3), "Should distribute across 2 servers after scaling down")
    assert.NotContains(t, dist3, "node-1", "Should not have clients on stopped server")
    t.Logf("After scaling down (2 servers): %v", dist3)
}

// 辅助函数
func createClients(t *testing.T, count int, serverAddr string) []*TestClient {
    clients := make([]*TestClient, count)
    for i := 0; i < count; i++ {
        client := NewTestClient(t, &ClientConfig{
            Protocol:   "tcp",
            ServerAddr: serverAddr,
        })
        require.NoError(t, client.Connect())
        clients[i] = client
    }
    return clients
}

func getClientDistribution(t *testing.T, apiClient *APIClient, clients []*TestClient) map[string]int {
    dist := make(map[string]int)
    for _, client := range clients {
        clientInfo, err := apiClient.GetClient(client.ID)
        if err != nil {
            continue // 客户端可能已断开
        }
        dist[clientInfo.NodeID]++
    }
    return dist
}
```

---

## 🎯 关键测试场景

### 1. 控制连接和隧道连接分离 ⭐⭐⭐⭐⭐

**场景**:
```
客户端A的控制连接 → Nginx → Server-1
客户端B的控制连接 → Nginx → Server-2
A→B的隧道连接     → Nginx → Server-3
```

**验证点**:
- ✅ Server-3需要能查询到Server-1的客户端A信息（通过Redis）
- ✅ Server-3需要能查询到Server-2的客户端B信息（通过Redis）
- ✅ TunnelOpen请求需要通过消息队列路由到Server-2
- ✅ 数据转发正常工作

**预期行为**:
- Server-3收到TunnelOpen请求后，从Redis查询目标客户端B的控制连接所在节点
- Server-3通过消息队列发送TunnelOpen到Server-2
- Server-2通过客户端B的控制连接发送TunnelOpen命令
- 客户端B建立隧道连接，可能到Server-1/2/3中的任意一个
- 数据通过Server间的桥接正常转发

### 2. 配置推送跨Server ⭐⭐⭐⭐

**场景**:
```
管理员通过API（到Server-1）创建映射
客户端A连接在Server-2
客户端B连接在Server-3
```

**验证点**:
- ✅ Server-1将映射配置写入Redis
- ✅ Server-1通过消息队列广播配置更新
- ✅ Server-2和Server-3收到消息并推送给各自的客户端
- ✅ 所有客户端都收到最新配置

### 3. 节点故障转移 ⭐⭐⭐⭐⭐

**场景**:
```
客户端A连接在Server-1
Server-1崩溃
```

**验证点**:
- ✅ Nginx检测到Server-1不健康，停止转发请求到Server-1
- ✅ 客户端A自动重连到Server-2或Server-3
- ✅ 客户端A的映射配置从Redis恢复
- ✅ 现有的隧道连接不受影响（如果不在Server-1上）

### 4. 负载均衡策略差异 ⭐⭐⭐

**策略对比**:

| 策略 | 优点 | 缺点 | 适用场景 |
|------|------|------|---------|
| **Round-robin** | 均匀分布，简单 | 不考虑Server负载 | 同质化Server |
| **Least-conn** | 负载更均衡 | 需要维护连接数状态 | 连接生命周期长的场景 |
| **IP-hash** | 会话保持 | 分布可能不均 | 需要会话粘性 |
| **Hash (一致性哈希)** | 扩缩容影响小 | 实现复杂 | 动态扩缩容场景 |

**测试验证**:
- ✅ 每种策略下客户端分布符合预期
- ✅ 映射连接能正常建立和转发
- ✅ 配置推送正常工作

### 5. 动态扩缩容 ⭐⭐⭐⭐

**扩容场景**:
```
初始: 3个Server
扩容: 新增2个Server (总共5个)
```

**验证点**:
- ✅ 新Server加入后，新连接开始分配到新Server
- ✅ 现有连接不受影响
- ✅ 配置同步到新Server
- ✅ 新Server能正常处理隧道请求

**缩容场景**:
```
初始: 5个Server
缩容: 停止2个Server (剩余3个)
```

**验证点**:
- ✅ 停止Server上的客户端自动重连到其他Server
- ✅ Nginx停止转发到已停止的Server
- ✅ 隧道连接迁移或重建
- ✅ 无数据丢失

### 6. 高并发压力测试 ⭐⭐⭐⭐⭐

**场景**:
```
1000个客户端同时连接
每个客户端建立10个隧道
总计10000个并发隧道连接
```

**验证点**:
- ✅ 成功率 > 95%
- ✅ 平均延迟 < 20ms
- ✅ P99延迟 < 100ms
- ✅ 吞吐量 > 500MB/s
- ✅ 负载均匀分布
- ✅ 无内存泄漏
- ✅ 无死锁

---

## 📊 性能基准

### 目标性能指标

| 指标 | 单节点 | 3节点集群 | 5节点集群 |
|------|--------|----------|----------|
| **最大并发连接** | 10,000 | 30,000 | 50,000 |
| **最大隧道数** | 5,000 | 15,000 | 25,000 |
| **平均延迟** | <10ms | <15ms | <20ms |
| **P99延迟** | <50ms | <80ms | <100ms |
| **吞吐量** | 200MB/s | 500MB/s | 800MB/s |
| **连接建立速率** | 1000/s | 2500/s | 4000/s |

### 性能测试脚本

**文件**: `tests/e2e/benchmark_load_balancer.sh`

```bash
#!/bin/bash

echo "🚀 Starting Load Balancer Performance Benchmark..."

# 启动环境
docker-compose -f docker-compose.load-balancer.yml up -d
sleep 30

# 等待所有服务就绪
for i in 1 2 3; do
    until curl -f http://localhost:8080/health; do
        echo "Waiting for server $i..."
        sleep 2
    done
done

echo "✅ All servers ready"

# 性能测试1: 连接建立速率
echo "📊 Test 1: Connection Establishment Rate"
wrk -t10 -c1000 -d30s --latency \
    -s benchmark/connect.lua \
    http://localhost:7000

# 性能测试2: 隧道吞吐量
echo "📊 Test 2: Tunnel Throughput"
iperf3 -c localhost -p 9000 -t 60 -P 100

# 性能测试3: API响应时间
echo "📊 Test 3: API Response Time"
wrk -t10 -c100 -d30s --latency \
    http://localhost:8080/api/v1/stats/system

# 性能测试4: 消息队列延迟
echo "📊 Test 4: Message Queue Latency"
go test -v -run=TestLoadBalancer_MessageLatency \
    -benchtime=30s ./tests/e2e/

# 清理
docker-compose -f docker-compose.load-balancer.yml down

echo "✅ Benchmark complete"
```

---

## 🔧 调试和监控

### 1. Redis监控

**查看所有Session**:
```bash
docker exec -it redis redis-cli
> KEYS tunnox:session:*
> HGETALL tunnox:session:12345
```

**查看消息队列**:
```bash
> LLEN tunnox:mq:tunnel_open
> LRANGE tunnox:mq:tunnel_open 0 -1
```

### 2. 查看Server日志

```bash
# 查看所有Server日志
docker-compose logs -f tunnox-server-1 tunnox-server-2 tunnox-server-3

# 查看特定Server的错误日志
docker-compose logs tunnox-server-1 | grep ERROR

# 查看Nginx访问日志
docker-compose logs nginx | grep "proxy"
```

### 3. 流量分析

**查看Nginx连接分布**:
```bash
# 进入Nginx容器
docker exec -it nginx sh

# 查看upstream状态（需要ngx_http_upstream_module）
curl http://localhost:8080/upstream_status
```

### 4. 性能分析

**查看Server资源使用**:
```bash
docker stats tunnox-server-1 tunnox-server-2 tunnox-server-3
```

**查看Redis性能**:
```bash
docker exec -it redis redis-cli INFO stats
docker exec -it redis redis-cli SLOWLOG GET 10
```

---

## 📋 测试检查清单

### 阶段6.1 任务清单

- [ ] 编写Docker Compose配置（3节点+Nginx+Redis）
- [ ] 编写Nginx负载均衡配置（4种策略）
- [ ] 实现 `TestLoadBalancer_ControlAndTunnelSeparation`
- [ ] 实现 `TestLoadBalancer_CrossServerMessaging`
- [ ] 实现 `TestLoadBalancer_NodeFailover`
- [ ] 实现 `TestLoadBalancer_DifferentStrategies`
- [ ] 实现 `TestLoadBalancer_HighConcurrency`
- [ ] 实现 `TestLoadBalancer_DynamicScaling`
- [ ] 编写性能基准测试脚本
- [ ] 验证所有测试通过

### 验收标准

- ✅ 所有负载均衡测试用例通过
- ✅ 控制连接和隧道连接分离场景正常工作
- ✅ 跨Server消息传递延迟 < 100ms
- ✅ 节点故障转移时间 < 10s
- ✅ 高并发成功率 > 95%
- ✅ 动态扩缩容无数据丢失
- ✅ 性能指标达到预期

---

## 🎯 总结

负载均衡场景是分布式部署的核心挑战，需要重点测试：

1. **跨Server路由** - 命令通道和隧道连接可能在不同Server
2. **配置同步** - 通过Redis+MQ保证配置一致性
3. **故障转移** - 节点故障时客户端自动重连
4. **负载均衡策略** - 不同策略的适用场景
5. **动态扩缩容** - 无缝添加/移除Server
6. **性能压力** - 高并发下的稳定性

这些测试确保Tunnox在生产环境的多节点部署下能够稳定、高效地工作！

