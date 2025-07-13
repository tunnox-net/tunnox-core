# RedisStorage 使用指南

## 概述

RedisStorage 是 Tunnox Core 项目中的 Redis 存储实现，提供了完整的 Storage 接口实现，支持 Redis 的所有核心功能。

## 特性

- ✅ 完整的 Storage 接口实现
- ✅ 支持基本键值操作（Set、Get、Delete、Exists）
- ✅ 支持列表操作（SetList、GetList、AppendToList、RemoveFromList）
- ✅ 支持哈希操作（SetHash、GetHash、GetAllHash、DeleteHash）
- ✅ 支持计数器操作（Incr、IncrBy）
- ✅ 支持过期时间管理（SetExpiration、GetExpiration）
- ✅ 支持原子操作（SetNX、CompareAndSwap）
- ✅ 自动 JSON 序列化/反序列化
- ✅ 连接池管理
- ✅ 超时控制
- ✅ 错误处理和日志记录

## 安装依赖

在使用 RedisStorage 之前，需要安装 Redis Go 客户端：

```bash
go get github.com/redis/go-redis/v9
```

## 基本使用

### 1. 创建 RedisStorage 实例

```go
package main

import (
    "context"
    "time"
    "tunnox-core/internal/cloud/storages"
)

func main() {
    ctx := context.Background()
    
    // 创建 Redis 配置
    config := &storages.RedisConfig{
        Addr:     "localhost:6379", // Redis 服务器地址
        Password: "",               // Redis 密码（如果有）
        DB:       0,                // 数据库编号
        PoolSize: 10,               // 连接池大小
    }
    
    // 创建 RedisStorage 实例
    storage, err := storages.NewRedisStorage(ctx, config)
    if err != nil {
        panic(err)
    }
    defer storage.Close()
    
    // 使用存储...
}
```

### 2. 基本操作

```go
// 设置值
err := storage.Set("user:123", map[string]interface{}{
    "name": "John Doe",
    "age":  30,
}, 30*time.Minute)

// 获取值
value, err := storage.Get("user:123")

// 检查键是否存在
exists, err := storage.Exists("user:123")

// 删除键
err := storage.Delete("user:123")
```

### 3. 列表操作

```go
// 设置列表
users := []interface{}{"user1", "user2", "user3"}
err := storage.SetList("online:users", users, 10*time.Minute)

// 获取列表
userList, err := storage.GetList("online:users")

// 追加到列表
err := storage.AppendToList("online:users", "user4")

// 从列表中移除
err := storage.RemoveFromList("online:users", "user2")
```

### 4. 哈希操作

```go
// 设置哈希字段
err := storage.SetHash("user:profile:123", "name", "John Doe")
err = storage.SetHash("user:profile:123", "email", "john@example.com")

// 获取单个字段
name, err := storage.GetHash("user:profile:123", "name")

// 获取所有字段
profile, err := storage.GetAllHash("user:profile:123")

// 删除字段
err := storage.DeleteHash("user:profile:123", "email")
```

### 5. 计数器操作

```go
// 递增计数器
views, err := storage.Incr("page:views")

// 按值递增
views, err := storage.IncrBy("page:views", 5)
```

### 6. 原子操作

```go
// 原子设置（仅当键不存在时）
acquired, err := storage.SetNX("lock:resource", "locked", 30*time.Second)

// 比较并交换
swapped, err := storage.CompareAndSwap("config:version", "v1.0", "v2.0", 1*time.Hour)
```

### 7. 过期时间管理

```go
// 设置过期时间
err := storage.SetExpiration("temp:data", 60*time.Second)

// 获取过期时间
ttl, err := storage.GetExpiration("temp:data")
```

## 使用存储工厂

存储工厂提供了统一的存储创建接口：

```go
// 创建存储工厂
factory := storages.NewStorageFactory(ctx)

// 通过配置创建 Redis 存储
config := map[string]interface{}{
    "type":      "redis",
    "addr":      "localhost:6379",
    "password":  "",
    "db":        0,
    "pool_size": 10,
}

storage, err := factory.CreateStorageWithConfig(config)
if err != nil {
    panic(err)
}
defer storage.Close()
```

## 高级功能

### 1. 连接测试

```go
// 测试 Redis 连接
err := storage.Ping()
if err != nil {
    log.Printf("Redis connection failed: %v", err)
}
```

### 2. 获取 Redis 客户端

```go
// 获取底层 Redis 客户端（用于高级操作）
client := storage.GetClient()

// 使用客户端执行自定义命令
result := client.Eval(ctx, "return redis.call('INFO')", nil)
```

### 3. 数据库管理

```go
// 清空当前数据库
err := storage.FlushDB()

// 获取键数量
count, err := storage.GetKeyCount()

// 获取匹配模式的键
keys, err := storage.GetKeys("user:*")
```

## 配置选项

### RedisConfig 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| Addr | string | "localhost:6379" | Redis 服务器地址 |
| Password | string | "" | Redis 密码 |
| DB | int | 0 | 数据库编号 |
| PoolSize | int | 10 | 连接池大小 |

### 配置示例

```go
// 本地 Redis
config := &storages.RedisConfig{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
    PoolSize: 10,
}

// 远程 Redis（带密码）
config := &storages.RedisConfig{
    Addr:     "redis.example.com:6379",
    Password: "your_password",
    DB:       1,
    PoolSize: 20,
}

// Redis Cluster
config := &storages.RedisConfig{
    Addr:     "redis-cluster.example.com:7000",
    Password: "cluster_password",
    DB:       0,
    PoolSize: 50,
}
```

## 错误处理

RedisStorage 会返回标准的错误类型：

```go
// 键不存在错误
if err == storages.ErrKeyNotFound {
    // 处理键不存在的情况
}

// 类型错误
if err == storages.ErrInvalidType {
    // 处理类型不匹配的情况
}

// Redis 连接错误
if err != nil {
    // 处理其他错误
    log.Printf("Redis operation failed: %v", err)
}
```

## 性能优化

### 1. 连接池配置

```go
config := &storages.RedisConfig{
    Addr:     "localhost:6379",
    PoolSize: 50, // 根据并发需求调整
}
```

### 2. 批量操作

对于大量操作，建议使用 Redis 的管道功能：

```go
client := storage.GetClient()
pipe := client.Pipeline()

// 批量设置
for i := 0; i < 1000; i++ {
    pipe.Set(ctx, fmt.Sprintf("key:%d", i), fmt.Sprintf("value:%d", i), time.Hour)
}

// 执行批量操作
_, err := pipe.Exec(ctx)
```

### 3. 序列化优化

RedisStorage 使用 JSON 序列化，对于大型对象，考虑使用更高效的序列化方式：

```go
// 对于大型对象，考虑压缩或使用更高效的序列化
import "github.com/golang/snappy"

// 压缩数据
data := []byte("large data...")
compressed := snappy.Encode(nil, data)
err := storage.Set("large:data", compressed, time.Hour)
```

## 监控和日志

RedisStorage 提供了详细的日志记录：

```go
// 日志级别可以通过 utils 包配置
// 所有操作都会记录详细的日志信息
```

## 测试

运行 RedisStorage 测试：

```bash
# 需要 Redis 服务器运行在 localhost:6379
go test ./tests -run TestRedisStorage

# 跳过 Redis 测试（如果没有 Redis 服务器）
go test ./tests -run TestRedisStorage -skip-redis
```

## 示例代码

完整的示例代码请参考：`examples/redis_storage_example.go`

## 注意事项

1. **Redis 服务器要求**：确保 Redis 服务器正在运行且可访问
2. **网络超时**：所有操作都有 5-10 秒的超时限制
3. **序列化限制**：只支持可 JSON 序列化的数据类型
4. **内存使用**：大型对象会占用更多内存
5. **连接管理**：使用完毕后记得调用 `Close()` 方法

## 故障排除

### 常见问题

1. **连接失败**
   ```
   Error: dial tcp localhost:6379: connect: connection refused
   ```
   解决：确保 Redis 服务器正在运行

2. **认证失败**
   ```
   Error: NOAUTH Authentication required
   ```
   解决：检查 Redis 密码配置

3. **超时错误**
   ```
   Error: context deadline exceeded
   ```
   解决：检查网络连接或增加超时时间

4. **序列化错误**
   ```
   Error: json: unsupported type
   ```
   解决：确保数据类型支持 JSON 序列化

### 调试模式

启用详细日志：

```go
// 在应用启动时设置日志级别
utils.SetLogLevel(utils.LogLevelDebug)
``` 