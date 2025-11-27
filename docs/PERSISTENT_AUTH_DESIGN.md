# Tunnox 持久化与认证系统设计

## 1. 服务端持久化

### 1.1 默认配置
```yaml
storage:
  type: hybrid
  hybrid:
    cache_type: memory
    enable_persistent: true
    json:
      file_path: "data/tunnox-data.json"  # 默认路径
      auto_save: true
      save_interval: 30
```

**关键点**：
- ✅ 无配置或默认配置下，使用 JSON 文件持久化
- ✅ 默认路径：`data/tunnox-data.json`
- ✅ 自动保存，间隔 30 秒
- ✅ 关闭时强制保存

### 1.2 需要持久化的数据

**持久化数据** (前缀: `tunnox:*:`):
- `tunnox:user:*` - 用户信息
- `tunnox:client:*` - 客户端注册信息
- `tunnox:port_mapping:*` - 端口映射配置
- `tunnox:device:*` - 设备信息

**运行时数据** (前缀: `tunnox:runtime:*`):
- `tunnox:runtime:*` - 加密密钥
- `tunnox:session:*` - 会话信息
- `tunnox:id:used:*` - ID 使用记录

---

## 2. 关键 API 持久化需求

### 2.1 注册用户 API

**接口**: `POST /api/v1/users`

**请求**:
```json
{
  "username": "alice",
  "password": "secure_password",
  "email": "alice@example.com"
}
```

**持久化内容**:
```
tunnox:user:{user_id} = {
  "id": "user_123",
  "username": "alice",
  "password_hash": "bcrypt_hash",
  "email": "alice@example.com",
  "created_at": "2025-11-27T10:00:00Z",
  "quota": {...}
}
```

**实现检查**:
- [ ] CloudControl.CreateUser() 是否调用 storage.Set()
- [ ] key 前缀是否为 `tunnox:user:`
- [ ] 数据是否在服务重启后恢复

---

### 2.2 认领匿名客户端 API

**接口**: `POST /api/v1/clients/claim`

**请求**:
```json
{
  "user_id": "user_123",
  "device_id": "anonymous-device-001"
}
```

**服务端持久化**:
```
tunnox:client:{client_id} = {
  "client_id": 600000001,
  "user_id": "user_123",
  "device_id": "anonymous-device-001",
  "auth_token": "jwt_token_here",
  "claimed_at": "2025-11-27T10:00:00Z"
}
```

**客户端配置写入**:

**写入优先级**:
1. `{executable_dir}/client-config.yaml`
2. `{working_dir}/client-config.yaml`
3. `~/.tunnox/client-config.yaml`

**写入逻辑**:
```go
func (c *Client) SaveConfig(config *ClientConfig) error {
    paths := []string{
        filepath.Join(getExecutableDir(), "client-config.yaml"),
        filepath.Join(getWorkingDir(), "client-config.yaml"),
        filepath.Join(getUserHomeDir(), ".tunnox", "client-config.yaml"),
    }
    
    for _, path := range paths {
        if err := tryWriteConfig(path, config); err == nil {
            log.Infof("Config saved to %s", path)
            return nil
        }
        log.Warnf("Failed to save config to %s: %v, trying next...", path, err)
    }
    
    return errors.New("failed to save config to any location")
}
```

**配置内容**:
```yaml
# 认证信息（从匿名升级为注册客户端）
client_id: 600000001
auth_token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 服务器配置
server:
  address: "server.example.com:7001"
  protocol: "tcp"
```

**实现检查**:
- [ ] ClaimClient API 是否返回 client_id 和 auth_token
- [ ] 客户端收到响应后是否尝试保存配置
- [ ] 是否按优先级尝试多个路径
- [ ] 权限不足时是否正确降级到下一个路径

---

## 3. 客户端启动流程

### 3.1 配置加载优先级

**加载顺序**:
1. 命令行参数 `-config` 指定的路径
2. `{executable_dir}/client-config.yaml`
3. `{working_dir}/client-config.yaml`
4. `~/.tunnox/client-config.yaml`
5. 默认配置（匿名模式）

**实现逻辑**:
```go
func LoadClientConfig(cmdConfigPath string) (*ClientConfig, error) {
    // 1. 命令行指定
    if cmdConfigPath != "" {
        return loadConfigFromFile(cmdConfigPath)
    }
    
    // 2. 尝试标准路径
    searchPaths := []string{
        filepath.Join(getExecutableDir(), "client-config.yaml"),
        filepath.Join(getWorkingDir(), "client-config.yaml"),
        filepath.Join(getUserHomeDir(), ".tunnox", "client-config.yaml"),
    }
    
    for _, path := range searchPaths {
        if config, err := loadConfigFromFile(path); err == nil {
            log.Infof("Loaded config from %s", path)
            return config, nil
        }
    }
    
    // 3. 使用默认配置（匿名模式）
    log.Infof("No config file found, using default anonymous mode")
    return getDefaultConfig(), nil
}
```

### 3.2 身份认证

**流程**:
```
1. 加载配置
   ├─ 有 client_id + auth_token → 注册客户端模式
   └─ 无认证信息 → 匿名模式

2. 连接服务器
   ├─ 发送 HandshakeRequest
   │  ├─ 注册模式: client_id + auth_token
   │  └─ 匿名模式: device_id
   
3. 服务器验证
   ├─ 注册模式: 验证 JWT token
   │  ├─ 成功 → 返回 client_id
   │  └─ 失败 → 拒绝连接
   └─ 匿名模式: 分配临时 client_id

4. 下载映射配置
   └─ 服务器推送 ConfigSet 命令
```

**实现检查**:
- [ ] 客户端是否正确识别注册/匿名模式
- [ ] HandshakeRequest 是否包含正确的认证信息
- [ ] 服务器是否正确验证 JWT
- [ ] 认证成功后是否推送映射配置

---

## 4. 多客户端登录控制

### 4.1 踢下线机制

**场景**: 同一 client_id 多次登录

**服务端行为**:
```go
type SessionManager struct {
    sessions map[int64]*Session  // client_id -> session
    mu       sync.RWMutex
}

func (s *SessionManager) RegisterSession(clientID int64, newSession *Session) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // 检查是否已有会话
    if oldSession, exists := s.sessions[clientID]; exists {
        log.Warnf("Client %d already connected, kicking old session", clientID)
        
        // 发送 Kick 命令
        kickCmd := &packet.JsonCommand{
            Type: packet.CommandTypeKick,
            Body: json.Marshal(KickReason{
                Reason: "Another client logged in with the same ID",
                Code:   "DUPLICATE_LOGIN",
            }),
        }
        oldSession.SendCommand(kickCmd)
        
        // 标记旧会话为 kicked
        oldSession.kicked = true
        oldSession.Close()
    }
    
    // 注册新会话
    s.sessions[clientID] = newSession
}
```

**客户端行为**:
```go
func (c *Client) handleKickCommand(cmd *packet.JsonCommand) {
    var reason KickReason
    json.Unmarshal(cmd.Body, &reason)
    
    log.Errorf("Client kicked: %s (%s)", reason.Reason, reason.Code)
    
    // 标记为被踢下线，禁止重连
    c.kicked = true
    c.Stop()
}

func (c *Client) shouldReconnect() bool {
    // 被踢下线不重连
    if c.kicked {
        return false
    }
    
    // 其他情况可以重连
    return true
}
```

**实现检查**:
- [ ] SessionManager 是否维护 client_id -> session 映射
- [ ] 新连接是否踢掉旧连接
- [ ] 旧连接是否收到 Kick 命令
- [ ] 旧连接是否正确停止重连

---

## 5. 断线重连机制

### 5.1 重连策略

**触发条件**:
- ✅ 网络断开
- ✅ 服务器重启
- ✅ 读写超时
- ❌ 被踢下线（不重连）
- ❌ 认证失败（不重连）

**重连参数**:
```go
type ReconnectConfig struct {
    Enabled      bool          // 是否启用重连
    InitialDelay time.Duration // 初始延迟（1秒）
    MaxDelay     time.Duration // 最大延迟（60秒）
    MaxAttempts  int           // 最大尝试次数（0=无限）
    Backoff      float64       // 退避因子（2.0=指数退避）
}

// 默认配置
var DefaultReconnectConfig = ReconnectConfig{
    Enabled:      true,
    InitialDelay: 1 * time.Second,
    MaxDelay:     60 * time.Second,
    MaxAttempts:  0,  // 无限重试
    Backoff:      2.0,
}
```

**重连逻辑**:
```go
func (c *Client) reconnectLoop() {
    delay := c.reconnectConfig.InitialDelay
    attempts := 0
    
    for {
        // 检查是否应该重连
        if !c.shouldReconnect() {
            log.Infof("Client should not reconnect, stopping...")
            return
        }
        
        // 检查最大尝试次数
        if c.reconnectConfig.MaxAttempts > 0 && attempts >= c.reconnectConfig.MaxAttempts {
            log.Errorf("Max reconnect attempts reached, giving up")
            return
        }
        
        log.Infof("Reconnecting in %v (attempt %d)...", delay, attempts+1)
        time.Sleep(delay)
        
        // 尝试重连
        if err := c.Connect(); err == nil {
            log.Infof("Reconnected successfully")
            return
        }
        
        // 增加延迟（指数退避）
        delay = time.Duration(float64(delay) * c.reconnectConfig.Backoff)
        if delay > c.reconnectConfig.MaxDelay {
            delay = c.reconnectConfig.MaxDelay
        }
        attempts++
    }
}
```

**实现检查**:
- [ ] 客户端是否实现重连逻辑
- [ ] 是否使用指数退避
- [ ] 被踢下线是否禁止重连
- [ ] 认证失败是否禁止重连
- [ ] 网络错误是否触发重连

---

## 6. 实现清单

### 6.1 服务端

- [ ] **JSON 持久化默认启用**
  - [ ] GetDefaultConfig() 返回 enable_persistent: true
  - [ ] 默认 JSON 路径 "data/tunnox-data.json"
  - [ ] 验证持久化前缀配置

- [ ] **注册用户 API**
  - [ ] CloudControl.CreateUser() 调用 storage.Set()
  - [ ] key 使用 "tunnox:user:{user_id}"
  - [ ] 测试重启后数据恢复

- [ ] **认领客户端 API**
  - [ ] CloudControl.ClaimClient() 实现
  - [ ] 返回 client_id + auth_token
  - [ ] 更新客户端信息到持久化存储
  - [ ] 测试重启后客户端信息恢复

- [ ] **多客户端登录控制**
  - [ ] SessionManager 维护 client_id -> session 映射
  - [ ] 新连接踢掉旧连接
  - [ ] 发送 Kick 命令到旧连接
  - [ ] 测试踢下线流程

### 6.2 客户端

- [ ] **配置文件管理**
  - [ ] 实现多路径配置加载（可执行文件目录/工作目录/~/.tunnox）
  - [ ] 实现配置保存（按优先级尝试写入）
  - [ ] 权限不足时降级到下一个路径
  - [ ] 测试各路径权限场景

- [ ] **认领客户端流程**
  - [ ] 实现 ClaimClient API 调用
  - [ ] 收到响应后保存配置
  - [ ] 使用新认证信息重新连接
  - [ ] 测试完整认领流程

- [ ] **启动流程优化**
  - [ ] 按优先级加载配置
  - [ ] 识别注册/匿名模式
  - [ ] 连接后下载映射配置
  - [ ] 测试各种启动场景

- [ ] **断线重连机制**
  - [ ] 实现指数退避重连
  - [ ] Kick 命令处理（禁止重连）
  - [ ] shouldReconnect() 逻辑
  - [ ] 测试网络断开、被踢、认证失败等场景

---

## 7. 测试场景

### 7.1 服务端持久化测试

```bash
# 1. 启动服务器（默认配置）
./tunnox-server

# 2. 创建用户
curl -X POST http://localhost:9000/api/v1/users \
  -H "Authorization: Bearer token" \
  -d '{"username":"alice","password":"pass123"}'

# 3. 创建映射
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Authorization: Bearer token" \
  -d '{...}'

# 4. 查看 JSON 文件
cat data/tunnox-data.json

# 5. 重启服务器
pkill tunnox-server
./tunnox-server

# 6. 验证数据恢复
curl -X GET http://localhost:9000/api/v1/users/alice
```

### 7.2 客户端认领测试

```bash
# 1. 匿名启动客户端
./tunnox-client -p tcp -s localhost:7001 -anonymous -device test-device

# 2. 服务端认领客户端
curl -X POST http://localhost:9000/api/v1/clients/claim \
  -H "Authorization: Bearer token" \
  -d '{"user_id":"user_123","device_id":"test-device"}'

# 3. 客户端自动保存配置并重连

# 4. 重启客户端（使用保存的配置）
./tunnox-client

# 5. 验证使用注册身份连接
```

### 7.3 踢下线测试

```bash
# 1. 启动客户端 A
./tunnox-client -config config.yaml &

# 2. 启动客户端 B（同一 client_id）
./tunnox-client -config config.yaml &

# 3. 验证客户端 A 被踢下线
# 4. 验证客户端 A 不重连
# 5. 验证客户端 B 正常工作
```

### 7.4 断线重连测试

```bash
# 1. 启动客户端
./tunnox-client -p tcp -s localhost:7001 -anonymous

# 2. 停止服务器
pkill tunnox-server

# 3. 观察客户端重连行为
# 4. 重启服务器
./tunnox-server

# 5. 验证客户端自动重连成功
```

---

## 8. 配置示例

### 8.1 服务端配置（默认）

```yaml
# 存储配置（默认）
storage:
  type: hybrid
  hybrid:
    cache_type: memory
    enable_persistent: true
    json:
      file_path: "data/tunnox-data.json"
      auto_save: true
      save_interval: 30
    persistent_prefixes:
      - "tunnox:user:"
      - "tunnox:client:"
      - "tunnox:port_mapping:"
    runtime_prefixes:
      - "tunnox:runtime:"
      - "tunnox:session:"
```

### 8.2 客户端配置（注册后）

```yaml
# 认证信息（认领后自动保存）
client_id: 600000001
auth_token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 服务器配置
server:
  address: "server.example.com:7001"
  protocol: "tcp"

# 重连配置（可选）
reconnect:
  enabled: true
  initial_delay: 1
  max_delay: 60
  max_attempts: 0
  backoff: 2.0
```

---

## 9. 时间规划

| 任务 | 预估时间 | 优先级 |
|------|---------|--------|
| 服务端 JSON 持久化默认启用 | 0.5h | P0 |
| 配置文件多路径加载/保存 | 2h | P0 |
| 认领客户端 API 实现 | 2h | P0 |
| 客户端配置自动保存 | 1h | P0 |
| 踢下线机制实现 | 1.5h | P1 |
| 断线重连机制完善 | 2h | P1 |
| 测试和调试 | 3h | P0 |

**总计**: 约 12 小时

