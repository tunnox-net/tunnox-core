# Tunnox 测试构建计划

**创建时间**: 2025-11-27  
**目标**: 从单元测试到跨节点E2E的完整测试体系  
**当前状态**: 初步测试覆盖（~30%）  
**目标状态**: 75%+ 覆盖，完整E2E测试  

---

## 📊 当前测试覆盖情况分析

### 测试文件统计

**总测试文件数**: 44个

**分类统计**:
```
已有测试模块:
├── bridge/              ✅ 3个文件 - 59.4%覆盖
├── broker/              ✅ 2个文件 - 73.5%覆盖
├── command/             ⚠️  10个文件 - 编译失败
├── cloud/managers/      ✅ 2个文件 - 26.2%覆盖
├── cloud/services/      ⚠️  6个文件 - 编译失败
├── cloud/distributed/   ✅ 1个文件 - 25.7%覆盖
├── core/events/         ✅ 1个文件 - 53.0%覆盖
├── core/idgen/          ✅ 2个文件 - 59.6%覆盖
├── core/storage/        ✅ 3个文件 - 29.5%覆盖
├── protocol/adapter/    ⚠️  4个文件 - 部分失败
├── protocol/session/    ✅ 1个文件
├── protocol/udp/        ✅ 1个文件 - 新增✓
├── stream/              ✅ 3个文件 - 部分失败
├── utils/monitor/       ✅ 1个文件
└── utils/              ✅ 1个文件

无测试模块:
├── api/                 ❌ 0个文件 - 0%覆盖 (47个端点)
├── client/              ❌ 0个文件 - 0%覆盖 (核心功能)
├── server/              ❌ 0个文件 - 0%覆盖 (主程序)
├── packet/              ❌ 0个文件 - 0%覆盖 (协议核心)
├── cloud/repos/         ❌ 0个文件 - 0%覆盖 (数据访问)
└── cloud/models/        ❌ 0个文件 - 0%覆盖 (数据模型)
```

### 当前问题

#### 🔴 P0 - 紧急问题（阻塞测试）

1. **编译错误** - `internal/command` 包
   - 错误：`GetActiveChannels` 方法缺失
   - 影响：10个测试文件无法运行
   - 优先级：立即修复

2. **编译错误** - `internal/cloud/services` 包
   - 错误：`non-constant format string`
   - 影响：6个测试文件无法运行
   - 优先级：立即修复

3. **编译错误** - `internal/stream/transform` 包
   - 错误：配置字段不匹配
   - 影响：集成测试失败
   - 优先级：立即修复

4. **编译警告** - `cmd/client/main.go`
   - 错误：`redundant newline`
   - 影响：客户端编译失败
   - 优先级：立即修复

#### 🟡 P1 - 重要缺失（核心功能无测试）

5. **Management API** - 0%覆盖
   - 缺失：47个端点的测试
   - 影响：API功能未验证
   - 优先级：高

6. **Client** - 0%覆盖
   - 缺失：客户端核心逻辑测试
   - 影响：客户端功能未验证
   - 优先级：高

7. **Packet** - 0%覆盖
   - 缺失：协议序列化/反序列化测试
   - 影响：协议正确性未验证
   - 优先级：高

8. **Cloud Repos** - 0%覆盖
   - 缺失：数据访问层测试
   - 影响：数据操作未验证
   - 优先级：中

#### 🟢 P2 - 覆盖不足（需要提升）

9. **core/storage** - 29.5%覆盖
   - 需要：补充边界测试
   - 优先级：中

10. **cloud/managers** - 26.2%覆盖
    - 需要：补充业务逻辑测试
    - 优先级：中

11. **cloud/distributed** - 25.7%覆盖
    - 需要：补充分布式场景测试
    - 优先级：低

---

## 🎯 测试构建计划（分6个阶段）

### 阶段0: 修复现有测试（立即，1天）⚡

**目标**: 所有现有测试可以运行

**优先级**: 🔴 P0

**任务清单**:

1. **修复 command 包编译错误**
   - [ ] 添加 `GetActiveChannels()` 方法到 Session 接口
   - [ ] 更新 Mock Session 实现
   - [ ] 验证所有 command 测试通过

2. **修复 cloud/services 包编译错误**
   - [ ] 修正 LogCreated/LogUpdated/LogDeleted 调用
   - [ ] 使用常量格式字符串或关闭 lint 检查
   - [ ] 验证所有 services 测试通过

3. **修复 stream/transform 包测试**
   - [ ] 更新 TransformConfig 字段引用
   - [ ] 验证集成测试通过

4. **修复客户端编译警告**
   - [ ] 移除 main.go 中的冗余换行符
   - [ ] 验证客户端编译成功

**验收标准**:
```bash
go test ./... 
# 所有测试包都能编译
# 现有测试全部通过
```

**工作量**: 4-6小时

---

### 阶段1: 单元测试基础层（1-2天）🔧

**目标**: 核心模块达到80%+覆盖

**优先级**: 🔴 P0

#### 1.1 数据结构测试（最高优先级）

**文件**: `tests/unit/packet_test.go`

```go
package unit

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "tunnox-core/internal/packet"
)

func TestTransferPacket_Serialize(t *testing.T) {
    tests := []struct {
        name    string
        packet  *packet.TransferPacket
        wantErr bool
    }{
        {
            name: "handshake packet",
            packet: &packet.TransferPacket{
                PacketType: packet.Handshake,
                Payload:    []byte("test"),
            },
            wantErr: false,
        },
        // ... 更多测试用例
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            data, err := tt.packet.Serialize()
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.NotEmpty(t, data)
                
                // 反序列化验证
                parsed, err := packet.Deserialize(data)
                assert.NoError(t, err)
                assert.Equal(t, tt.packet.PacketType, parsed.PacketType)
            }
        })
    }
}
```

**测试清单**:
- [ ] TransferPacket 序列化/反序列化
- [ ] HandshakeRequest/Response
- [ ] CommandPacket 序列化
- [ ] TunnelOpenRequest/Response
- [ ] 边界条件（空payload、超大包等）
- [ ] 错误处理

**目标覆盖率**: 95%+

#### 1.2 配置验证测试

**文件**: `tests/unit/config_test.go`

```go
package unit

func TestServerConfig_Validate(t *testing.T) {
    tests := []struct {
        name    string
        config  *server.Config
        wantErr bool
    }{
        {"valid config", validConfig(), false},
        {"missing listen addr", &server.Config{}, true},
        {"invalid port", invalidPortConfig(), true},
        // ...
    }
    // ...
}

func TestClientConfig_Validate(t *testing.T) {
    // ...
}

func TestMappingConfig_Validate(t *testing.T) {
    // ...
}
```

**测试清单**:
- [ ] ServerConfig 验证
- [ ] ClientConfig 验证
- [ ] MappingConfig 验证
- [ ] 默认值测试
- [ ] 边界值测试

**目标覆盖率**: 90%+

#### 1.3 Storage 核心测试

**文件**: `tests/unit/storage_comprehensive_test.go`

```go
package unit

func TestStorage_CRUD_Operations(t *testing.T) {
    storages := []struct{
        name string
        storage storage.Storage
    }{
        {"memory", storage.NewMemoryStorage()},
        {"json", createTestJSONStorage(t)},
        // Redis需要集成测试环境
    }
    
    for _, s := range storages {
        t.Run(s.name, func(t *testing.T) {
            testStorageSet(t, s.storage)
            testStorageGet(t, s.storage)
            testStorageDelete(t, s.storage)
            testStorageExists(t, s.storage)
            // ...
        })
    }
}

func TestTypedStorage_TypeSafety(t *testing.T) {
    // 已有，补充边界测试
}

func TestHybridStorage_KeyRouting(t *testing.T) {
    // 测试key前缀路由逻辑
}
```

**测试清单**:
- [ ] 基础CRUD操作
- [ ] List/Hash操作
- [ ] TTL/过期测试
- [ ] 并发安全测试
- [ ] HybridStorage路由测试
- [ ] TypedStorage类型安全测试

**目标覆盖率**: 85%+

#### 1.4 Models 序列化测试

**文件**: `tests/unit/models_test.go`

```go
package unit

func TestUser_JSON(t *testing.T) {
    user := &models.User{
        ID: "user1",
        Username: "test",
        // ...
    }
    
    // 序列化
    data, err := json.Marshal(user)
    assert.NoError(t, err)
    
    // 反序列化
    var parsed models.User
    err = json.Unmarshal(data, &parsed)
    assert.NoError(t, err)
    assert.Equal(t, user.ID, parsed.ID)
}

func TestClient_JSON(t *testing.T) {
    // ...
}

func TestPortMapping_JSON(t *testing.T) {
    // ...
}
```

**测试清单**:
- [ ] User JSON序列化
- [ ] Client JSON序列化
- [ ] PortMapping JSON序列化
- [ ] Node JSON序列化
- [ ] 嵌套结构测试
- [ ] nil值处理测试

**目标覆盖率**: 95%+

**阶段1总结**:
- 新增测试文件: ~10个
- 新增测试用例: ~150-200个
- 预期覆盖率提升: +25%
- 工作量: 1-2天

---

### 阶段2: Management API 测试（2-3天）🌐

**目标**: API层达到85%+覆盖

**优先级**: 🔴 P0

#### 2.1 测试基础设施

**文件**: `tests/helpers/api_test_server.go`

```go
package helpers

type TestAPIServer struct {
    Server       *api.ManagementAPIServer
    CloudControl *managers.CloudControl
    Storage      storage.Storage
    Router       http.Handler
    TestServer   *httptest.Server
}

func NewTestAPIServer(t *testing.T) *TestAPIServer {
    storage := storage.NewMemoryStorage()
    
    cloudControl := managers.NewCloudControl(
        getTestConfig(),
        storage,
    )
    
    apiConfig := &api.APIConfig{
        Auth: api.AuthConfig{Type: "none"},
        CORS: api.CORSConfig{Enabled: false},
    }
    
    server := api.NewManagementAPIServer(
        context.Background(),
        apiConfig,
        cloudControl,
    )
    
    testServer := httptest.NewServer(server.GetRouter())
    
    return &TestAPIServer{
        Server:       server,
        CloudControl: cloudControl,
        Storage:      storage,
        TestServer:   testServer,
    }
}

func (s *TestAPIServer) Close() {
    s.TestServer.Close()
    s.CloudControl.Close()
}

func (s *TestAPIServer) POST(path string, body interface{}) (*http.Response, error) {
    data, _ := json.Marshal(body)
    return http.Post(
        s.TestServer.URL+path,
        "application/json",
        bytes.NewReader(data),
    )
}

func (s *TestAPIServer) GET(path string) (*http.Response, error) {
    return http.Get(s.TestServer.URL + path)
}
```

#### 2.2 用户管理 API 测试

**文件**: `tests/api/user_api_test.go`

```go
package api_test

func TestUserAPI_CreateUser(t *testing.T) {
    server := helpers.NewTestAPIServer(t)
    defer server.Close()
    
    // 测试创建用户
    resp, err := server.POST("/api/v1/users", map[string]string{
        "username": "testuser",
        "email":    "test@example.com",
    })
    
    assert.NoError(t, err)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    assert.NotEmpty(t, result["id"])
}

func TestUserAPI_GetUser(t *testing.T) {
    // ...
}

func TestUserAPI_UpdateUser(t *testing.T) {
    // ...
}

func TestUserAPI_DeleteUser(t *testing.T) {
    // ...
}

func TestUserAPI_ListUsers(t *testing.T) {
    // ...
}

func TestUserAPI_Quota(t *testing.T) {
    // 测试配额管理
}
```

**测试清单** (7个端点 × 3-5个测试 = ~30个测试):
- [ ] POST /api/v1/users - 创建用户
- [ ] GET /api/v1/users/{id} - 获取用户
- [ ] PUT /api/v1/users/{id} - 更新用户
- [ ] DELETE /api/v1/users/{id} - 删除用户
- [ ] GET /api/v1/users - 列出用户
- [ ] GET /api/v1/users/{id}/quota - 获取配额
- [ ] PUT /api/v1/users/{id}/quota - 更新配额

#### 2.3 客户端管理 API 测试

**文件**: `tests/api/client_api_test.go`

**测试清单** (10个端点 × 3-5个测试 = ~40个测试):
- [ ] GET /api/v1/clients - 列出所有客户端
- [ ] POST /api/v1/clients - 创建客户端
- [ ] GET /api/v1/clients/{id} - 获取客户端
- [ ] PUT /api/v1/clients/{id} - 更新客户端
- [ ] DELETE /api/v1/clients/{id} - 删除客户端
- [ ] POST /api/v1/clients/{id}/disconnect - 强制下线
- [ ] POST /api/v1/clients/claim - 认领客户端
- [ ] GET /api/v1/clients/{id}/connections - 客户端连接
- [ ] GET /api/v1/clients/{id}/mappings - 客户端映射
- [ ] POST /api/v1/clients/batch/disconnect - 批量下线

#### 2.4 映射管理 API 测试

**文件**: `tests/api/mapping_api_test.go`

**测试清单** (8个端点 × 3-5个测试 = ~35个测试):
- [ ] GET /api/v1/mappings - 列出所有映射
- [ ] POST /api/v1/mappings - 创建映射
- [ ] GET /api/v1/mappings/{id} - 获取映射
- [ ] PUT /api/v1/mappings/{id} - 更新映射
- [ ] DELETE /api/v1/mappings/{id} - 删除映射
- [ ] GET /api/v1/mappings/{id}/connections - 映射连接
- [ ] POST /api/v1/mappings/batch/delete - 批量删除
- [ ] POST /api/v1/mappings/batch/update - 批量更新

#### 2.5 其他 API 测试

**文件**: 
- `tests/api/auth_api_test.go` - 认证API（4个端点）
- `tests/api/search_api_test.go` - 搜索API（3个端点）
- `tests/api/stats_api_test.go` - 统计API（5个端点）
- `tests/api/connection_api_test.go` - 连接管理API（2个端点）
- `tests/api/node_api_test.go` - 节点管理API（2个端点）

**阶段2总结**:
- 新增测试文件: ~10个
- 新增测试用例: ~200-250个
- 覆盖47个API端点
- 预期API覆盖率: 85%+
- 工作量: 2-3天

---

### 阶段3: 集成测试 - 本地隧道（3-4天）🔗

**目标**: 验证核心隧道功能

**优先级**: 🟡 P1

#### 3.1 测试辅助工具

**文件**: `tests/helpers/tunnel_test_harness.go`

```go
package helpers

type TunnelTestHarness struct {
    Server       *server.Server
    SourceClient *client.TunnoxClient
    TargetClient *client.TunnoxClient
    TargetService net.Listener
    ServerPort   int
}

func NewTunnelTestHarness(t *testing.T) *TunnelTestHarness {
    // 1. 启动目标服务（HTTP/TCP）
    targetListener, _ := net.Listen("tcp", "127.0.0.1:0")
    targetPort := targetListener.Addr().(*net.TCPAddr).Port
    
    // 2. 启动 Tunnox Server
    serverPort := getFreePort(t)
    serverConfig := getTestServerConfig(serverPort)
    tunnoxServer := server.NewServer(serverConfig)
    go tunnoxServer.Start()
    time.Sleep(200 * time.Millisecond)
    
    // 3. 启动源客户端
    sourceConfig := getSourceClientConfig(serverPort)
    sourceClient := client.NewClient(context.Background(), sourceConfig)
    go sourceClient.Connect()
    time.Sleep(200 * time.Millisecond)
    
    // 4. 启动目标客户端
    targetConfig := getTargetClientConfig(serverPort)
    targetClient := client.NewClient(context.Background(), targetConfig)
    go targetClient.Connect()
    time.Sleep(200 * time.Millisecond)
    
    return &TunnelTestHarness{
        Server:        tunnoxServer,
        SourceClient:  sourceClient,
        TargetClient:  targetClient,
        TargetService: targetListener,
        ServerPort:    serverPort,
    }
}

func (h *TunnelTestHarness) CreateMapping(protocol string, targetPort int) *models.PortMapping {
    // 通过API创建映射
}

func (h *TunnelTestHarness) Close() {
    // 清理资源
}
```

#### 3.2 TCP 隧道测试

**文件**: `tests/integration/tunnel_tcp_test.go`

```go
package integration

func TestTCPTunnel_BasicForwarding(t *testing.T) {
    harness := helpers.NewTunnelTestHarness(t)
    defer harness.Close()
    
    // 启动目标HTTP服务器
    go http.Serve(harness.TargetService, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello from target"))
    }))
    
    // 创建映射
    mapping := harness.CreateMapping("tcp", harness.TargetService.Addr().(*net.TCPAddr).Port)
    
    // 等待映射生效
    time.Sleep(100 * time.Millisecond)
    
    // 通过源端口访问
    resp, err := http.Get(fmt.Sprintf("http://localhost:%d", mapping.SourcePort))
    assert.NoError(t, err)
    
    body, _ := io.ReadAll(resp.Body)
    assert.Equal(t, "Hello from target", string(body))
}

func TestTCPTunnel_LargeDataTransfer(t *testing.T) {
    // 测试大文件传输
}

func TestTCPTunnel_Concurrency(t *testing.T) {
    // 测试并发连接
}

func TestTCPTunnel_Reconnect(t *testing.T) {
    // 测试断线重连
}
```

**测试清单**:
- [ ] 基础转发
- [ ] 大数据传输（10MB+）
- [ ] 并发连接（100+）
- [ ] 断线重连
- [ ] 客户端崩溃恢复

#### 3.3 UDP 隧道测试

**文件**: `tests/integration/tunnel_udp_test.go`

**测试清单**:
- [ ] UDP数据包转发
- [ ] 大数据包处理
- [ ] 数据包顺序
- [ ] 超时处理

#### 3.4 压缩/加密测试

**文件**: `tests/integration/tunnel_transform_test.go`

```go
func TestTunnel_Compression(t *testing.T) {
    // 测试启用压缩的隧道
}

func TestTunnel_Encryption(t *testing.T) {
    // 测试启用加密的隧道
}

func TestTunnel_CompressionAndEncryption(t *testing.T) {
    // 测试同时启用压缩和加密
}

func TestTunnel_RateLimit(t *testing.T) {
    // 测试带宽限速
}
```

**测试清单**:
- [ ] Gzip压缩测试
- [ ] AES-256-GCM加密测试
- [ ] 压缩+加密组合测试
- [ ] 带宽限速测试
- [ ] 性能对比测试

#### 3.5 SOCKS5 测试

**文件**: `tests/integration/tunnel_socks5_test.go`

**测试清单**:
- [ ] SOCKS5代理连接
- [ ] 认证测试
- [ ] 多目标访问

**阶段3总结**:
- 新增测试文件: ~6个
- 新增测试用例: ~50-80个
- 覆盖所有协议类型
- 预期集成测试覆盖: 60%+
- 工作量: 3-4天

---

### 阶段4: 集成测试 - 客户端-服务器交互（2-3天）🔄

**目标**: 验证客户端生命周期

**优先级**: 🟡 P1

#### 4.1 握手和认证测试

**文件**: `tests/integration/client_handshake_test.go`

```go
func TestClientHandshake_TCP(t *testing.T) {
    // TCP协议握手
}

func TestClientHandshake_UDP(t *testing.T) {
    // UDP协议握手
}

func TestClientHandshake_WebSocket(t *testing.T) {
    // WebSocket协议握手
}

func TestClientHandshake_QUIC(t *testing.T) {
    // QUIC协议握手
}

func TestClientHandshake_InvalidToken(t *testing.T) {
    // 无效token
}
```

**测试清单**:
- [ ] TCP握手
- [ ] UDP握手
- [ ] WebSocket握手
- [ ] QUIC握手
- [ ] 匿名客户端
- [ ] 注册客户端
- [ ] Token验证
- [ ] 认证失败

#### 4.2 心跳和保活测试

**文件**: `tests/integration/client_heartbeat_test.go`

```go
func TestClientHeartbeat_Normal(t *testing.T) {
    // 正常心跳
}

func TestClientHeartbeat_Timeout(t *testing.T) {
    // 心跳超时
}

func TestClientHeartbeat_ServerRestart(t *testing.T) {
    // 服务器重启
}
```

**测试清单**:
- [ ] 正常心跳
- [ ] 心跳超时
- [ ] 服务器重启
- [ ] 网络抖动

#### 4.3 重连测试

**文件**: `tests/integration/client_reconnect_test.go`

```go
func TestClientReconnect_NetworkInterruption(t *testing.T) {
    harness := helpers.NewTunnelTestHarness(t)
    defer harness.Close()
    
    // 验证初始连接
    assert.True(t, harness.SourceClient.IsConnected())
    
    // 模拟网络中断
    harness.Server.Stop()
    time.Sleep(500 * time.Millisecond)
    
    // 重启服务器
    harness.Server.Start()
    
    // 验证自动重连
    time.Sleep(2 * time.Second)
    assert.True(t, harness.SourceClient.IsConnected())
}

func TestClientReconnect_ExponentialBackoff(t *testing.T) {
    // 测试指数退避
}

func TestClientReconnect_MaxAttempts(t *testing.T) {
    // 测试最大重试次数
}

func TestClientReconnect_MappingRestore(t *testing.T) {
    // 测试重连后映射恢复
}
```

**测试清单**:
- [ ] 网络中断重连
- [ ] 指数退避算法
- [ ] 最大重试次数
- [ ] 重连后映射恢复
- [ ] 重连失败处理

#### 4.4 配置推送测试

**文件**: `tests/integration/config_push_test.go`

```go
func TestConfigPush_NewMapping(t *testing.T) {
    // 测试新建映射后的配置推送
}

func TestConfigPush_UpdateMapping(t *testing.T) {
    // 测试更新映射后的配置推送
}

func TestConfigPush_DeleteMapping(t *testing.T) {
    // 测试删除映射后的配置推送
}

func TestConfigPush_ClientOffline(t *testing.T) {
    // 测试客户端离线时的配置推送
}
```

**测试清单**:
- [ ] 新建映射推送
- [ ] 更新映射推送
- [ ] 删除映射推送
- [ ] 客户端离线处理
- [ ] 推送超时处理

**阶段4总结**:
- 新增测试文件: ~4个
- 新增测试用例: ~40-50个
- 覆盖客户端生命周期
- 工作量: 2-3天

---

### 阶段5: E2E测试 - 单节点完整场景（3-4天）🎯

**目标**: 端到端验证核心用例

**优先级**: 🟡 P1

#### 5.1 Docker 测试环境

**文件**: `tests/e2e/docker-compose.yml`

```yaml
version: '3.8'

services:
  # Tunnox 服务器
  tunnox-server:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    ports:
      - "7000:7000"    # TCP
      - "7001:7001"    # WebSocket
      - "7002:7002"    # UDP
      - "7003:7003"    # QUIC
      - "8080:8080"    # Management API
    environment:
      - STORAGE_TYPE=json
      - STORAGE_JSON_PATH=/data/tunnox.json
      - MESSAGE_BROKER_TYPE=memory
      - LOG_LEVEL=info
    volumes:
      - ./data:/data
    networks:
      - tunnox-net
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  # 源客户端（匿名）
  client-source-anon:
    build:
      context: ../..
      dockerfile: Dockerfile.client
    command: ["./client", "-p", "tcp", "-s", "tunnox-server:7000", "--anonymous"]
    depends_on:
      tunnox-server:
        condition: service_healthy
    networks:
      - tunnox-net

  # 目标客户端（注册）
  client-target:
    build:
      context: ../..
      dockerfile: Dockerfile.client
    command: ["./client", "-p", "tcp", "-s", "tunnox-server:7000"]
    environment:
      - CLIENT_ID=12345
      - AUTH_TOKEN=test-token-123
    depends_on:
      tunnox-server:
        condition: service_healthy
    networks:
      - tunnox-net

  # 测试目标服务 - Nginx
  nginx-target:
    image: nginx:alpine
    ports:
      - "9080:80"
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
    ports:
      - "5432:5432"
    networks:
      - tunnox-net

networks:
  tunnox-net:
    driver: bridge
```

#### 5.2 E2E测试脚本

**文件**: `tests/e2e/run_e2e_tests.sh`

```bash
#!/bin/bash
set -e

echo "🚀 Starting E2E Tests..."

# 1. 启动环境
echo "📦 Starting Docker environment..."
docker-compose -f docker-compose.yml up -d
sleep 10

# 2. 等待服务就绪
echo "⏳ Waiting for services to be ready..."
timeout 30 bash -c 'until curl -f http://localhost:8080/health; do sleep 2; done'

# 3. 运行测试用例
echo "🧪 Running test cases..."

# 测试1: 创建用户
echo "Test 1: Create User"
USER_RESP=$(curl -s -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","email":"test@example.com"}')
USER_ID=$(echo $USER_RESP | jq -r '.id')
echo "✓ Created user: $USER_ID"

# 测试2: 创建客户端
echo "Test 2: Create Client"
CLIENT_RESP=$(curl -s -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d "{\"user_id\":\"$USER_ID\",\"client_name\":\"test-client\"}")
CLIENT_ID=$(echo $CLIENT_RESP | jq -r '.id')
echo "✓ Created client: $CLIENT_ID"

# 测试3: 创建映射（Nginx）
echo "Test 3: Create Mapping (Nginx)"
MAPPING_RESP=$(curl -s -X POST http://localhost:8080/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\":\"$USER_ID\",
    \"source_client_id\":1,
    \"target_client_id\":2,
    \"protocol\":\"tcp\",
    \"target_host\":\"nginx-target\",
    \"target_port\":80,
    \"enable_compression\":true
  }")
MAPPING_ID=$(echo $MAPPING_RESP | jq -r '.id')
SOURCE_PORT=$(echo $MAPPING_RESP | jq -r '.source_port')
echo "✓ Created mapping: $MAPPING_ID (port $SOURCE_PORT)"

# 测试4: 等待映射生效
echo "Test 4: Wait for mapping to be active"
sleep 5

# 测试5: 测试HTTP转发
echo "Test 5: Test HTTP forwarding"
HTTP_RESP=$(curl -s http://localhost:$SOURCE_PORT)
if [[ $HTTP_RESP == *"nginx"* ]]; then
  echo "✓ HTTP forwarding works"
else
  echo "✗ HTTP forwarding failed"
  exit 1
fi

# 测试6: 测试压缩（检查响应头）
echo "Test 6: Test compression"
# ...

# 测试7: 测试统计
echo "Test 7: Check statistics"
STATS=$(curl -s http://localhost:8080/api/v1/stats/system)
echo "✓ System stats: $STATS"

# 4. 清理环境
echo "🧹 Cleaning up..."
docker-compose -f docker-compose.yml down -v

echo "✅ All E2E tests passed!"
```

#### 5.3 Go E2E测试

**文件**: `tests/e2e/e2e_basic_test.go`

```go
package e2e

func TestE2E_CompleteWorkflow(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E test in short mode")
    }
    
    // 1. 启动Docker环境
    compose := NewDockerComposeEnv(t)
    defer compose.Cleanup()
    
    // 2. 等待服务就绪
    compose.WaitForHealthy("tunnox-server", 30*time.Second)
    
    // 3. 创建用户
    apiClient := compose.GetAPIClient()
    user, err := apiClient.CreateUser("testuser", "test@example.com")
    require.NoError(t, err)
    
    // 4. 创建客户端
    client, err := apiClient.CreateClient(user.ID, "test-client")
    require.NoError(t, err)
    
    // 5. 创建映射
    mapping, err := apiClient.CreateMapping(&MappingRequest{
        UserID:         user.ID,
        SourceClientID: 1,
        TargetClientID: 2,
        Protocol:       "tcp",
        TargetHost:     "nginx-target",
        TargetPort:     80,
    })
    require.NoError(t, err)
    
    // 6. 测试转发
    resp, err := http.Get(fmt.Sprintf("http://localhost:%d", mapping.SourcePort))
    require.NoError(t, err)
    body, _ := io.ReadAll(resp.Body)
    assert.Contains(t, string(body), "nginx")
    
    // 7. 验证统计
    stats, err := apiClient.GetSystemStats()
    require.NoError(t, err)
    assert.Greater(t, stats.ActiveMappings, 0)
}
```

**测试清单**:
- [ ] 完整工作流（用户→客户端→映射→转发）
- [ ] Nginx转发测试
- [ ] PostgreSQL转发测试
- [ ] 压缩功能测试
- [ ] 加密功能测试
- [ ] 多协议测试（TCP/UDP/WebSocket/QUIC）
- [ ] 性能基准测试

**阶段5总结**:
- 新增测试文件: ~5个
- 新增测试用例: ~15-20个
- 完整E2E场景覆盖
- 工作量: 3-4天

---

### 阶段6: E2E测试 - 跨节点多实例+负载均衡（5-6天）🌍

**目标**: 验证分布式部署和负载均衡场景

**优先级**: 🟢 P2

**特别说明**: 此阶段包含负载均衡器测试，详见 `E2E_LOAD_BALANCER_TEST_PLAN.md`

#### 6.1 多节点架构

**文件**: `tests/e2e/docker-compose.multi-node.yml`

```yaml
version: '3.8'

services:
  # Redis（共享存储和消息队列）
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - tunnox-net

  # Tunnox 服务器节点1
  tunnox-server-1:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    ports:
      - "7000:7000"
      - "8080:8080"
    environment:
      - NODE_ID=node-1
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
    depends_on:
      - redis
    networks:
      - tunnox-net

  # Tunnox 服务器节点2
  tunnox-server-2:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    ports:
      - "7010:7000"
      - "8081:8080"
    environment:
      - NODE_ID=node-2
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
    depends_on:
      - redis
    networks:
      - tunnox-net

  # Tunnox 服务器节点3
  tunnox-server-3:
    build:
      context: ../..
      dockerfile: Dockerfile.server
    ports:
      - "7020:7000"
      - "8082:8080"
    environment:
      - NODE_ID=node-3
      - STORAGE_TYPE=redis
      - STORAGE_REDIS_ADDR=redis:6379
      - MESSAGE_BROKER_TYPE=redis
      - MESSAGE_BROKER_REDIS_ADDR=redis:6379
    depends_on:
      - redis
    networks:
      - tunnox-net

  # Nginx 负载均衡器
  nginx-lb:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx/lb.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      - tunnox-server-1
      - tunnox-server-2
      - tunnox-server-3
    networks:
      - tunnox-net

  # 源客户端（连接到节点1）
  client-source:
    build:
      context: ../..
      dockerfile: Dockerfile.client
    command: ["./client", "-p", "tcp", "-s", "tunnox-server-1:7000"]
    depends_on:
      - tunnox-server-1
    networks:
      - tunnox-net

  # 目标客户端（连接到节点2）
  client-target:
    build:
      context: ../..
      dockerfile: Dockerfile.client
    command: ["./client", "-p", "tcp", "-s", "tunnox-server-2:7000"]
    depends_on:
      - tunnox-server-2
    networks:
      - tunnox-net

  # 测试目标服务
  nginx-target:
    image: nginx:alpine
    networks:
      - tunnox-net

networks:
  tunnox-net:
    driver: bridge
```

#### 6.2 跨节点测试

**文件**: `tests/e2e/multi_node_test.go`

```go
package e2e

func TestMultiNode_CrossNodeTunnel(t *testing.T) {
    // 客户端A连接到节点1，客户端B连接到节点2
    // 验证跨节点隧道建立和数据转发
}

func TestMultiNode_ConfigSync(t *testing.T) {
    // 在节点1创建映射
    // 验证节点2和节点3能获取到配置
}

func TestMultiNode_NodeFailover(t *testing.T) {
    // 停止节点1
    // 验证客户端自动切换到节点2
    // 验证映射继续工作
}

func TestMultiNode_LoadBalancing(t *testing.T) {
    // 启动多个客户端
    // 验证负载均衡到不同节点
}

func TestMultiNode_MessageBroker(t *testing.T) {
    // 测试Redis消息队列
    // 验证跨节点消息传递
}

func TestMultiNode_DistributedStorage(t *testing.T) {
    // 测试Redis存储
    // 验证数据一致性
}
```

**测试清单**:
- [ ] 跨节点隧道建立
- [ ] 跨节点配置同步
- [ ] 节点故障转移
- [ ] 负载均衡
- [ ] 消息队列测试
- [ ] 分布式存储一致性
- [ ] 分布式锁测试
- [ ] 集群扩缩容
- [ ] 网络分区恢复

#### 6.3 性能和压力测试

**文件**: `tests/e2e/performance_test.go`

```go
func BenchmarkMultiNode_Throughput(b *testing.B) {
    // 测试吞吐量
}

func BenchmarkMultiNode_Latency(b *testing.B) {
    // 测试延迟
}

func TestMultiNode_StressTest(t *testing.T) {
    // 1000+并发连接
    // 10000+映射
    // 持续24小时稳定性测试
}
```

**测试清单**:
- [ ] 吞吐量测试
- [ ] 延迟测试
- [ ] 并发连接测试（1000+）
- [ ] 大量映射测试（10000+）
- [ ] 长时间稳定性测试
- [ ] 资源占用测试

**阶段6总结**:
- 新增测试文件: ~6个
- 新增测试用例: ~30-40个
- 完整分布式场景覆盖
- 工作量: 4-5天

---

## 📊 测试覆盖率目标总结

### 阶段性目标

| 阶段 | 完成时间 | 新增测试 | 累计覆盖率 | 状态 |
|------|---------|---------|-----------|------|
| **阶段0** | 1天 | 修复现有 | ~30% | 🔴 待开始 |
| **阶段1** | +2天 | ~200个 | ~55% | 🔴 待开始 |
| **阶段2** | +3天 | ~250个 | ~70% | 🔴 待开始 |
| **阶段3** | +4天 | ~80个 | ~75% | 🔴 待开始 |
| **阶段4** | +3天 | ~50个 | ~78% | 🔴 待开始 |
| **阶段5** | +4天 | ~20个 | ~80% | 🔴 待开始 |
| **阶段6** | +5天 | ~40个 | ~82% | 🔴 待开始 |
| **总计** | **22天** | **~640个** | **82%** | - |

### 最终覆盖率分布

```
业务逻辑层:     90%+ (CloudControl, Managers, Services)
数据处理层:     95%+ (Packet, Storage, Models)
API层:          85%+ (47个端点全覆盖)
网络IO层:       60%+ (集成测试覆盖)
客户端层:       75%+ (核心功能全覆盖)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总体覆盖率:     82%+ ⭐⭐⭐⭐⭐
```

---

## 🛠️ 测试基础设施

### CI/CD 集成

**文件**: `.github/workflows/test.yml`

```yaml
name: Tests

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run Unit Tests
        run: go test -v -race -coverprofile=coverage.txt ./tests/unit/...
      
      - name: Upload Coverage
        uses: codecov/codecov-action@v3

  api-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run API Tests
        run: go test -v ./tests/api/...

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run Integration Tests
        run: go test -v -timeout 10m ./tests/integration/...

  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Start Docker Compose
        run: docker-compose -f tests/e2e/docker-compose.yml up -d
      
      - name: Run E2E Tests
        run: go test -v -timeout 20m ./tests/e2e/...
      
      - name: Cleanup
        if: always()
        run: docker-compose -f tests/e2e/docker-compose.yml down -v
```

### 测试辅助工具集

**目录结构**:
```
tests/
├── helpers/
│   ├── api_client.go          # API测试客户端
│   ├── test_server.go         # 测试服务器工具
│   ├── tunnel_harness.go      # 隧道测试工具
│   ├── docker_compose.go      # Docker Compose封装
│   ├── assertions.go          # 自定义断言
│   └── fixtures.go            # 测试数据生成器
├── fixtures/
│   ├── users.json             # 用户测试数据
│   ├── clients.json           # 客户端测试数据
│   └── mappings.json          # 映射测试数据
└── mocks/
    ├── mock_storage.go        # Storage Mock
    ├── mock_broker.go         # Broker Mock
    └── mock_session.go        # Session Mock
```

---

## 📈 进度跟踪

### 任务清单

#### 🔴 阶段0: 修复现有测试（1天）
- [ ] 修复 command 包编译错误
- [ ] 修复 cloud/services 包编译错误
- [ ] 修复 stream/transform 包测试
- [ ] 修复 cmd/client 编译警告
- [ ] 验证所有现有测试通过

#### 🔴 阶段1: 单元测试基础层（2天）
- [ ] Packet 序列化测试（~30个）
- [ ] Config 验证测试（~25个）
- [ ] Storage 核心测试（~50个）
- [ ] Models 序列化测试（~30个）
- [ ] 覆盖率达到55%+

#### 🔴 阶段2: Management API测试（3天）
- [ ] 测试基础设施（helpers）
- [ ] 用户管理API测试（~30个）
- [ ] 客户端管理API测试（~40个）
- [ ] 映射管理API测试（~35个）
- [ ] 其他API测试（~40个）
- [ ] 覆盖率达到70%+

#### 🟡 阶段3: 集成测试-隧道（4天）
- [ ] 测试工具harness
- [ ] TCP隧道测试（~20个）
- [ ] UDP隧道测试（~15个）
- [ ] 压缩/加密测试（~15个）
- [ ] SOCKS5测试（~10个）
- [ ] 覆盖率达到75%+

#### 🟡 阶段4: 集成测试-客户端（3天）
- [ ] 握手和认证测试（~15个）
- [ ] 心跳测试（~8个）
- [ ] 重连测试（~12个）
- [ ] 配置推送测试（~10个）
- [ ] 覆盖率达到78%+

#### 🟡 阶段5: E2E-单节点（4天）
- [ ] Docker环境搭建
- [ ] E2E测试脚本
- [ ] Go E2E测试（~15个）
- [ ] 覆盖率达到80%+

#### 🟢 阶段6: E2E-多节点（5天）
- [ ] 多节点Docker环境
- [ ] 跨节点测试（~20个）
- [ ] 性能压力测试（~15个）
- [ ] 覆盖率达到82%+

---

## 🎯 成功标准

### 质量门禁

**必须达标**:
- ✅ 所有测试通过率 100%
- ✅ 单元测试覆盖率 ≥ 80%
- ✅ API测试覆盖率 ≥ 85%
- ✅ 总体覆盖率 ≥ 75%
- ✅ 无编译错误和警告
- ✅ 无竞态条件（go test -race）

**E2E验证**:
- ✅ 所有协议正常工作（TCP/UDP/WS/QUIC）
- ✅ 压缩/加密功能正常
- ✅ 多节点配置同步
- ✅ 故障转移正常
- ✅ 性能达标（延迟<10ms，吞吐>100MB/s）

### 文档要求

**必须提供**:
- ✅ 测试运行指南（README.md）
- ✅ 测试覆盖率报告（自动生成）
- ✅ E2E测试环境说明
- ✅ CI/CD配置文档

---

## 📅 时间线

```
Week 1: 阶段0 + 阶段1 (3天)
  Day 1: 修复现有测试
  Day 2-3: 单元测试基础层

Week 2: 阶段2 (3天) + 阶段3开始
  Day 4-6: Management API测试
  Day 7: 集成测试工具开发

Week 3: 阶段3完成 + 阶段4
  Day 8-10: 隧道测试
  Day 11-13: 客户端测试

Week 4: 阶段5 + 阶段6
  Day 14-17: E2E单节点测试
  Day 18-22: E2E多节点测试
```

**总计**: 4-5周完成完整测试体系

---

## 💡 最佳实践

### 测试编写原则

1. **独立性**: 每个测试独立运行
2. **可重复性**: 相同输入产生相同输出
3. **快速性**: 单元测试<100ms，集成测试<5s
4. **清晰性**: 测试名称描述意图
5. **覆盖性**: 正常路径 + 异常路径 + 边界条件

### 命名规范

```go
// 单元测试
func TestPacket_Serialize_ValidData(t *testing.T) {}
func TestConfig_Validate_MissingAddr(t *testing.T) {}

// 集成测试
func TestTunnel_TCP_BasicForwarding(t *testing.T) {}
func TestClient_Reconnect_NetworkInterruption(t *testing.T) {}

// E2E测试
func TestE2E_CompleteWorkflow(t *testing.T) {}
func TestMultiNode_CrossNodeTunnel(t *testing.T) {}
```

### 表驱动测试

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {"case1", input1, output1, false},
        {"case2", input2, output2, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Function(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

---

## 🚀 下一步行动

### 立即开始（今天）

1. **修复编译错误** - 阶段0
   ```bash
   cd /Users/roger.tong/GolandProjects/tunnox-core
   
   # 修复 command 包
   # 修复 cloud/services 包
   # 修复 stream/transform 包
   # 修复 cmd/client 警告
   
   # 验证
   go test ./...
   ```

2. **创建测试目录结构**
   ```bash
   mkdir -p tests/{unit,api,integration,e2e,helpers,fixtures,mocks}
   touch tests/unit/.gitkeep
   touch tests/api/.gitkeep
   # ...
   ```

3. **编写第一个测试** - Packet测试
   ```bash
   # 创建 tests/unit/packet_test.go
   # 运行测试
   go test -v ./tests/unit/
   ```

### 本周目标

- ✅ 完成阶段0（修复现有测试）
- ✅ 完成阶段1（单元测试基础层）
- ✅ 覆盖率达到55%+

---

**结论**: 这是一个完整、可执行的测试构建计划，从单元测试到跨节点E2E，预计22个工作日完成，最终达到82%+覆盖率和完整的分布式E2E验证。

**准备好开始了吗？** 🚀

