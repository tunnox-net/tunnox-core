# API 测试辅助工具

本目录包含用于 Management API 测试的辅助工具和测试基础设施。

## 文件说明

### api_test_server.go
测试服务器封装，提供便捷的方法来创建和管理用于测试的 API 服务器。

**主要功能：**
- 自动创建内存存储（无需外部依赖）
- 支持自定义配置（认证、CORS等）
- 遵循 dispose 模式，自动清理资源
- 提供便捷的访问方法

**使用示例：**
```go
func TestExample(t *testing.T) {
    ctx := context.Background()
    
    // 创建测试服务器（使用默认配置）
    server, err := NewTestAPIServer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer server.Stop()
    
    // 启动服务器
    if err := server.Start(); err != nil {
        t.Fatal(err)
    }
    
    // 获取服务器URL
    apiURL := server.GetAPIURL()
    
    // 进行API测试...
}
```

**自定义配置：**
```go
cfg := &TestAPIServerConfig{
    ListenAddr: "127.0.0.1:8888",
    AuthType:   "api_key",
    APISecret:  "test-secret",
    EnableCORS: true,
}
server, _ := NewTestAPIServer(ctx, cfg)
```

### api_client.go
API 客户端工具，提供类型安全的方法来调用 Management API 端点。

**主要功能：**
- 完整的 CRUD 操作（用户、客户端、映射）
- 统计查询接口
- 搜索接口
- 健康检查
- 自动处理请求/响应序列化
- 支持认证令牌

**使用示例：**
```go
func TestExample(t *testing.T) {
    ctx := context.Background()
    
    // 创建API客户端
    client := NewAPIClient(ctx, "http://localhost:8080/api/v1")
    defer client.Close()
    
    // 设置认证令牌（如果需要）
    client.SetAuthToken("your-token")
    
    // 创建用户
    user, err := client.CreateUser("alice", "alice@example.com")
    if err != nil {
        t.Fatal(err)
    }
    
    // 创建客户端
    clientInfo, err := client.CreateClient(user.ID, "Alice's Desktop")
    if err != nil {
        t.Fatal(err)
    }
    
    // 创建映射
    mapping := &models.PortMapping{
        UserID:         user.ID,
        SourceClientID: clientInfo.ID,
        TargetClientID: anotherClient.ID,
        Protocol:       models.ProtocolTCP,
        SourcePort:     8080,
        TargetPort:     80,
        // ... 其他字段
    }
    result, err := client.CreateMapping(mapping)
}
```

### api_test_server_test.go
测试服务器的单元测试。

**测试覆盖：**
- ✅ 创建默认配置的服务器
- ✅ 创建自定义配置的服务器
- ✅ 启动和停止服务器
- ✅ 服务器超时处理
- ✅ Getter 方法
- ✅ Dispose 模式验证
- ✅ 上下文取消自动清理
- ✅ 并发访问安全性

### api_client_test.go
API 客户端的单元测试。

**测试覆盖：**
- ✅ 创建客户端
- ✅ 设置认证令牌
- ✅ 健康检查
- ✅ 用户管理（CRUD）
- ✅ 客户端管理（CRUD）
- ✅ 映射管理（CRUD）
- ✅ 搜索操作
- ✅ Dispose 模式验证

## 测试数据 Fixtures

### tests/fixtures/
包含用于测试的 JSON 格式测试数据。

**文件列表：**
- `users.json` - 测试用户数据（4个用户，包含不同计划和状态）
- `clients.json` - 测试客户端数据（5个客户端，包含注册和匿名类型）
- `mappings.json` - 测试映射数据（5个映射，包含不同协议和状态）
- `fixtures.go` - 加载 fixture 数据的辅助函数

**使用示例：**
```go
import "tunnox-core/tests/fixtures"

func TestWithFixtures(t *testing.T) {
    // 加载测试用户
    users, err := fixtures.LoadUsers()
    if err != nil {
        t.Fatal(err)
    }
    
    // 加载测试客户端
    clients, err := fixtures.LoadClients()
    if err != nil {
        t.Fatal(err)
    }
    
    // 加载测试映射
    mappings, err := fixtures.LoadMappings()
    if err != nil {
        t.Fatal(err)
    }
    
    // 使用测试数据...
}
```

## 完整的集成测试示例

```go
package api_test

import (
    "context"
    "testing"
    
    "tunnox-core/tests/helpers"
    "tunnox-core/tests/fixtures"
)

func TestUserLifecycle(t *testing.T) {
    ctx := context.Background()
    
    // 1. 启动测试服务器
    server, err := helpers.NewTestAPIServer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer server.Stop()
    
    if err := server.Start(); err != nil {
        t.Fatal(err)
    }
    
    // 2. 创建API客户端
    client := helpers.NewAPIClient(ctx, server.GetAPIURL())
    defer client.Close()
    
    // 3. 执行测试
    t.Run("创建用户", func(t *testing.T) {
        user, err := client.CreateUser("testuser", "test@example.com")
        if err != nil {
            t.Fatal(err)
        }
        
        if user.Username != "testuser" {
            t.Errorf("expected username 'testuser', got '%s'", user.Username)
        }
    })
    
    t.Run("列出用户", func(t *testing.T) {
        users, err := client.ListUsers()
        if err != nil {
            t.Fatal(err)
        }
        
        if len(users) == 0 {
            t.Error("expected at least one user")
        }
    })
}
```

## 运行测试

```bash
# 运行所有测试
go test -v ./tests/helpers/...

# 运行特定测试
go test -v ./tests/helpers/... -run TestNewTestAPIServer

# 运行带覆盖率的测试
go test -v -coverprofile=coverage.out ./tests/helpers/...
go tool cover -html=coverage.out
```

## 设计原则

1. **Dispose 模式**：所有资源都遵循 dispose 模式，确保正确清理
2. **类型安全**：避免使用 `map[string]interface{}`，使用具体的结构体类型
3. **无外部依赖**：测试使用内存存储，无需外部数据库或服务
4. **并发安全**：所有组件都是并发安全的
5. **易于使用**：提供简单直观的 API
6. **完整覆盖**：测试覆盖所有主要功能

## 注意事项

1. **端口分配**：默认使用随机端口（`127.0.0.1:0`），避免端口冲突
2. **资源清理**：始终使用 `defer server.Stop()` 和 `defer client.Close()`
3. **上下文管理**：使用带超时的上下文避免测试挂起
4. **错误处理**：测试中应该正确处理和验证错误
5. **测试隔离**：每个测试应该独立，不依赖其他测试的状态

## 下一步

这些测试基础设施已经准备就绪，可以用于：
- 编写 API 层的集成测试
- 编写 API 端点的单元测试
- 进行性能和负载测试
- 验证 API 的行为和边界情况

