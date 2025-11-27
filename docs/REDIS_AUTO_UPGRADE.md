# Redis 自动共享机制

## 问题背景

在多节点部署时，如果每个节点使用独立的内存缓存，会导致：

1. **运行时数据不一致**：
   - 会话信息不共享
   - 连接状态不同步
   - ID 生成可能重复

2. **分布式功能失效**：
   - 消息队列无法跨节点传递
   - 节点间无法协调工作
   - 负载均衡无法正确路由

## 解决方案

### Redis 自动共享机制

系统支持 **双向自动共享** Redis 配置，确保多节点部署的一致性：

#### 规则 1: 存储 Redis → 消息队列 Redis

如果配置了 `storage.redis`，但 `message_broker` 未配置或为 `memory`，自动使用 Redis 作为消息队列：

```yaml
# 用户配置
storage:
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0
  hybrid:
    cache_type: memory  # ⚠️ 会自动升级为 redis

# message_broker 未配置或为 memory
```

**自动行为**：
```
✅ Storage Redis detected, auto-configuring message_broker to use redis for multi-node support
✅ Redis detected, auto-upgrading cache from 'memory' to 'redis' for multi-node support
```

**结果**：
```yaml
storage:
  redis:
    addr: "localhost:6379"
  hybrid:
    cache_type: redis  # ✅ 自动升级

message_broker:
  type: redis  # ✅ 自动配置
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0
    channel: "tunnox:messages"
```

#### 规则 2: 消息队列 Redis → 存储 Redis

如果配置了 `message_broker.redis`，但 `storage.redis` 未配置，自动使用消息队列的 Redis 配置：

```yaml
# 用户配置
message_broker:
  type: redis
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0

storage:
  type: hybrid
  # redis 未配置
```

**自动行为**：
```
✅ MessageBroker Redis detected, auto-configuring storage.redis for distributed cache
✅ Redis detected, auto-upgrading cache from 'memory' to 'redis' for multi-node support
```

**结果**：
```yaml
message_broker:
  type: redis
  redis:
    addr: "localhost:6379"

storage:
  redis:  # ✅ 自动配置
    addr: "localhost:6379"
    password: ""
    db: 0
    pool_size: 10
  hybrid:
    cache_type: redis  # ✅ 自动升级
```

### 配置示例

#### 单节点部署（默认）

```yaml
storage:
  type: hybrid
  hybrid:
    cache_type: memory  # ✅ 单节点使用内存缓存即可
    enable_persistent: true
    json:
      file_path: "data/tunnox-data.json"
      auto_save: true
      save_interval: 30
```

**特性**：
- ✅ 零依赖，不需要 Redis
- ✅ 高性能，内存访问速度快
- ✅ 简单部署，单机开箱即用

#### 多节点部署（自动升级）

```yaml
storage:
  type: hybrid
  redis:
    addr: "redis.example.com:6379"
    password: "your-redis-password"
    db: 0
    pool_size: 10
  hybrid:
    cache_type: memory  # ✅ 自动升级为 redis
    enable_persistent: true
    json:
      file_path: "data/tunnox-data.json"
      auto_save: true
      save_interval: 30

message_broker:
  type: redis
  redis:
    addr: "redis.example.com:6379"
    password: "your-redis-password"
    channel: "tunnox:messages"
```

**自动行为**：
1. 检测到 `storage.redis.addr` 已配置
2. 自动升级 `cache_type` 从 `memory` 到 `redis`
3. 使用同一个 Redis 配置
4. 多节点共享运行时数据

**特性**：
- ✅ 多节点负载均衡
- ✅ 会话共享
- ✅ 连接状态同步
- ✅ 分布式消息队列
- ✅ ID 生成唯一性保证

#### 显式配置（跳过自动升级）

```yaml
storage:
  type: hybrid
  redis:
    addr: "redis.example.com:6379"
  hybrid:
    cache_type: redis  # ✅ 显式设置为 redis，跳过自动升级提示
    enable_persistent: true
```

**特性**：
- 不会打印自动升级提示
- 直接使用 Redis 缓存
- 适合生产环境

## 自动共享规则

### 规则 1: 存储 Redis → 消息队列

**触发条件**：
1. `storage.redis.addr` 已配置
2. `message_broker.type` 为空或为 `memory`

**自动行为**：
```
message_broker.type: memory → redis
message_broker.redis.addr: 复制自 storage.redis.addr
message_broker.redis.password: 复制自 storage.redis.password
message_broker.redis.db: 复制自 storage.redis.db
```

### 规则 2: 消息队列 Redis → 存储

**触发条件**：
1. `message_broker.type` 为 `redis`
2. `message_broker.redis.addr` 已配置
3. `storage.redis.addr` 未配置

**自动行为**：
```
storage.redis.addr: 复制自 message_broker.redis.addr
storage.redis.password: 复制自 message_broker.redis.password
storage.redis.db: 复制自 message_broker.redis.db
```

### 规则 3: 缓存自动升级

**触发条件**：
1. `storage.type` 为 `hybrid`
2. `storage.hybrid.cache_type` 为 `memory` 或未设置
3. `storage.redis.addr` 已配置（可能是规则1或2自动配置的）

**自动行为**：
```
storage.hybrid.cache_type: memory → redis
```

### 验证逻辑

```go
// 自动检测缓存类型
if config.Hybrid.CacheType == "" || config.Hybrid.CacheType == "memory" {
    if config.Redis.Addr != "" {
        utils.Infof("✅ Redis detected, auto-upgrading cache from 'memory' to 'redis' for multi-node support")
        config.Hybrid.CacheType = "redis"
    }
}

// 验证 Redis 配置
if config.Hybrid.CacheType == "redis" {
    if config.Redis.Addr == "" {
        return fmt.Errorf("redis cache enabled but storage.redis.addr not configured")
    }
}
```

## 数据分类

### 运行时数据（缓存）

存储在 Redis 缓存中，多节点共享：

- `tunnox:runtime:*` - 运行时临时数据
- `tunnox:session:*` - 会话信息
- `tunnox:connection:*` - 连接状态
- `tunnox:id:used:*` - ID 使用记录

**TTL**: 默认 24 小时，自动过期

### 持久化数据（数据库）

存储在持久化存储中（JSON/远程数据库）：

- `tunnox:user:*` - 用户信息
- `tunnox:client:*` - 客户端信息
- `tunnox:mapping:*` - 端口映射配置
- `tunnox:quota:*` - 配额信息

**TTL**: 永久保存，直到手动删除

## 多节点架构

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Node 1     │     │  Node 2     │     │  Node 3     │
│             │     │             │     │             │
│  ┌───────┐  │     │  ┌───────┐  │     │  ┌───────┐  │
│  │ Cache │◄─┼─────┼─►│ Cache │◄─┼─────┼─►│ Cache │  │
│  └───────┘  │     │  └───────┘  │     │  └───────┘  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       │         ┌─────────▼─────────┐         │
       └────────►│   Redis Cluster   │◄────────┘
                 │                   │
                 │  - 运行时缓存     │
                 │  - 会话共享       │
                 │  - 消息队列       │
                 └───────────────────┘
                           │
                 ┌─────────▼─────────┐
                 │  Persistent DB    │
                 │                   │
                 │  - 用户数据       │
                 │  - 映射配置       │
                 │  - 配额信息       │
                 └───────────────────┘
```

## 配置建议

### 开发环境

```yaml
storage:
  type: hybrid
  hybrid:
    cache_type: memory  # 单机内存缓存
    enable_persistent: false  # 不持久化，重启清空
```

### 测试环境

```yaml
storage:
  type: hybrid
  hybrid:
    cache_type: memory
    enable_persistent: true
    json:
      file_path: "data/tunnox-data.json"
      auto_save: true
      save_interval: 30
```

### 生产环境（单节点）

```yaml
storage:
  type: hybrid
  hybrid:
    cache_type: memory
    enable_persistent: true
    remote:
      type: grpc
      grpc:
        address: "storage-service:50051"
```

### 生产环境（多节点）

```yaml
storage:
  type: hybrid
  redis:
    addr: "redis-cluster:6379"
    password: "${REDIS_PASSWORD}"
    db: 0
    pool_size: 50
  hybrid:
    cache_type: redis  # 显式设置
    enable_persistent: true
    remote:
      type: grpc
      grpc:
        address: "storage-service:50051"

message_broker:
  type: redis
  redis:
    addr: "redis-cluster:6379"
    password: "${REDIS_PASSWORD}"
    channel: "tunnox:messages"
```

## 常见问题

### Q1: 为什么需要自动升级？

**A**: 避免配置错误。如果配置了 Redis 但缓存仍使用 memory，会导致多节点数据不一致，难以排查。

### Q2: 如何禁用自动升级？

**A**: 显式设置 `cache_type: redis` 或不配置 `storage.redis.addr`。

### Q3: 自动升级会影响性能吗？

**A**: 
- **单节点**: 内存缓存略快于 Redis，但差异不大（亚毫秒级）
- **多节点**: Redis 是唯一选择，性能损失可忽略不计

### Q4: 可以单独配置缓存和持久化的 Redis 吗？

**A**: 可以。使用不同的 `db` 编号：

```yaml
storage:
  redis:
    db: 0  # 缓存使用 db 0
  hybrid:
    remote:
      type: grpc
      grpc:
        address: "..."  # 持久化使用远程服务
```

### Q5: 消息队列和缓存可以用不同的 Redis 吗？

**A**: 当前版本共用同一个 Redis 配置。未来版本可以分离：

```yaml
# 未来规划
storage:
  redis:
    addr: "redis-cache:6379"  # 缓存专用

message_broker:
  redis:
    addr: "redis-mq:6379"  # 消息队列专用
```

## 监控指标

### Redis 缓存状态

```bash
# 查看 Redis 中的缓存数据
redis-cli -h redis.example.com -a password

# 查看运行时数据
KEYS tunnox:runtime:*
KEYS tunnox:session:*
KEYS tunnox:connection:*

# 查看内存使用
INFO memory

# 查看连接数
INFO clients
```

### 缓存命中率

```bash
# 通过管理 API 查询
curl http://localhost:9000/api/v1/stats/system

# 响应示例
{
  "cache": {
    "type": "redis",
    "hit_rate": 0.95,
    "total_hits": 10000,
    "total_misses": 500
  }
}
```

## 总结

自动升级机制确保：

1. ✅ **零配置错误**：检测到 Redis 自动启用多节点支持
2. ✅ **向后兼容**：单节点仍然可以使用内存缓存
3. ✅ **简化配置**：不需要手动设置多个相关配置项
4. ✅ **提示友好**：清晰的日志提示自动升级行为

**推荐配置**：
- 单节点：不配置 Redis，使用内存缓存
- 多节点：配置 Redis，自动升级为 Redis 缓存

