# Tunnox 配置系统架构分析报告

**审查人**: 通信架构师
**审查日期**: 2025-12-29
**项目**: tunnox-core

---

## 1. 现有配置代码扫描结果

### 1.1 服务端配置 (Server)

#### 主入口: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/cmd/server/main.go`

**配置加载方式**:
- 使用 `flag` 包解析命令行参数
- 支持 `-config` 指定配置文件路径 (默认: `config.yaml`)
- 支持 `-log` 覆盖日志文件路径
- 支持 `-export-config` 导出配置模板

**配置优先级**: 命令行参数 > 配置文件

#### 核心配置结构: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/config.go`

```go
type Config struct {
    Server      ServerConfig      `yaml:"server"`
    Management  ManagementConfig  `yaml:"management"`
    Log         LogConfig         `yaml:"log"`
    Redis       RedisConfig       `yaml:"redis"`
    Persistence PersistenceConfig `yaml:"persistence"`
    Storage     StorageConfig     `yaml:"storage"`
    Platform    PlatformConfig    `yaml:"platform"`
}
```

**配置结构分析**:

| 配置组 | 文件位置 | 主要字段 |
|--------|----------|----------|
| `ServerConfig` | config.go:22 | Protocols (map) |
| `ProtocolConfig` | config.go:15 | Enabled, Port, Host |
| `ManagementConfig` | config.go:27 | Listen, Auth, PProf |
| `LogConfig` | config.go:48 | Level, File, Rotation |
| `RedisConfig` | config.go:63 | Enabled, Addr, Password, DB |
| `PersistenceConfig` | config.go:71 | Enabled, File, AutoSave, SaveInterval |
| `StorageConfig` | config.go:79 | Enabled, URL, Token, Timeout |
| `PlatformConfig` | config.go:87 | Enabled, URL, Token, Timeout |

#### 环境变量支持: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/config_env.go`

**已支持的环境变量**:

| 环境变量 | 对应配置 |
|----------|----------|
| `REDIS_ENABLED` | redis.enabled |
| `REDIS_ADDR` | redis.addr |
| `REDIS_PASSWORD` | redis.password |
| `REDIS_DB` | redis.db |
| `PERSISTENCE_ENABLED` | persistence.enabled |
| `PERSISTENCE_FILE` | persistence.file |
| `STORAGE_ENABLED` | storage.enabled |
| `STORAGE_URL` | storage.url |
| `STORAGE_TOKEN` | storage.token |
| `PLATFORM_ENABLED` | platform.enabled |
| `PLATFORM_URL` | platform.url |
| `LOG_LEVEL` | log.level |
| `LOG_FILE` | log.file |
| `SERVER_TCP_PORT` | server.protocols.tcp.port |
| `SERVER_KCP_PORT` | server.protocols.kcp.port |
| `SERVER_QUIC_PORT` | server.protocols.quic.port |
| `MANAGEMENT_LISTEN` | management.listen |
| `MANAGEMENT_AUTH_TOKEN` | management.auth.token |

### 1.2 客户端配置 (Client)

#### 主入口: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/cmd/client/main.go`

**配置加载方式**:
- 使用 `flag` 包解析命令行参数
- 命令行参数: `-config`, `-p` (protocol), `-s` (server), `-id`, `-device`, `-token`, `-anonymous`, `-log`, `-daemon`, `-interactive`
- 支持快捷命令: `tunnox http/tcp/udp/socks/code`

**配置优先级**: 命令行参数 > 配置文件 > 自动检测

#### 核心配置结构: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/config.go`

```go
type ClientConfig struct {
    ClientID  int64  `yaml:"client_id"`
    AuthToken string `yaml:"auth_token"`
    Anonymous bool   `yaml:"anonymous"`
    DeviceID  string `yaml:"device_id"`
    SecretKey string `yaml:"secret_key"`
    Server    struct {
        Address  string `yaml:"address"`
        Protocol string `yaml:"protocol"`
    } `yaml:"server"`
    Log LogConfig `yaml:"log"`
}
```

#### 配置管理器: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/config_manager.go`

**配置文件搜索路径** (按优先级):
1. 命令行指定路径
2. 可执行文件目录: `{exec}/client-config.yaml`
3. 工作目录: `{cwd}/client-config.yaml`
4. 用户主目录: `~/.tunnox/client-config.yaml`

**特点**:
- 原子写入 (先写临时文件再 rename)
- 保留服务器配置 (SaveConfigWithOptions)

### 1.3 云控配置 (Cloud)

#### 核心配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/cloud/configs/configs.go`

```go
type ControlConfig struct {
    APIEndpoint       string        `json:"api_endpoint"`
    APIKey            string        `json:"api_key,omitempty"`
    APISecret         string        `json:"api_secret,omitempty"`
    Timeout           time.Duration `json:"timeout"`
    NodeID            string        `json:"node_id,omitempty"`
    NodeName          string        `json:"node_name,omitempty"`
    UseBuiltIn        bool          `json:"use_built_in"`
    JWTSecretKey      string        `json:"jwt_secret_key"`
    JWTExpiration     time.Duration `json:"jwt_expiration"`
    RefreshExpiration time.Duration `json:"refresh_expiration"`
    JWTIssuer         string        `json:"jwt_issuer"`
}
```

**其他配置类型**:
- `ClientConfig`: 客户端运行时配置 (压缩、带宽、连接数)
- `MappingConfig`: 端口映射配置 (压缩、加密、超时)
- `NodeConfig`: 节点配置 (连接数、心跳、带宽)

### 1.4 存储配置 (Storage)

#### 工厂配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/factory.go`

**存储类型**:
- `memory`: 内存存储
- `redis`: Redis 存储
- `hybrid`: 混合存储 (缓存 + 持久化)

#### Redis 配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/redis_storage.go`

```go
type RedisConfig struct {
    Addr     string `json:"addr" yaml:"addr"`
    Password string `json:"password" yaml:"password"`
    DB       int    `json:"db" yaml:"db"`
    PoolSize int    `json:"pool_size" yaml:"pool_size"`
}
```

#### JSON 存储配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/json_storage.go`

```go
type JSONStorageConfig struct {
    FilePath     string
    AutoSave     bool
    SaveInterval time.Duration
}
```

#### 远程存储配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/remote_storage.go`

```go
type RemoteStorageConfig struct {
    GRPCAddress string
    Timeout     time.Duration
    MaxRetries  int
    TLSEnabled  bool
    TLSCertFile string
    TLSKeyFile  string
}
```

#### 混合存储配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/hybrid_config.go`

```go
type HybridConfig struct {
    PersistentPrefixes       []string
    SharedPrefixes           []string
    SharedPersistentPrefixes []string
    DefaultCacheTTL          time.Duration
    PersistentCacheTTL       time.Duration
    SharedCacheTTL           time.Duration
    EnablePersistent         bool
}
```

### 1.5 其他配置模块

#### HTTP 服务配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/httpservice/config.go`

**模块化配置结构**:
- `HTTPServiceConfig`: 顶层配置
- `ModulesConfig`: 子模块配置
  - `ManagementAPI`: 管理 API
  - `WebSocket`: WebSocket 传输
  - `DomainProxy`: 域名代理
  - `WebSocketProxy`: WebSocket 代理
- `CORSConfig`: 跨域配置
- `RateLimitConfig`: 限流配置

#### Session 配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/protocol/session/config.go`

```go
type SessionManagerConfig struct {
    IDManager             *idgen.IDManager
    Logger                corelog.Logger
    HeartbeatTimeout      time.Duration
    CleanupInterval       time.Duration
    MaxConnections        int
    MaxControlConnections int
}
```

#### 消息代理配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/broker/factory.go`

```go
type BrokerConfig struct {
    Type   BrokerType
    NodeID string
    Redis  *RedisBrokerConfig
    NATS   interface{}  // 未来扩展
}
```

#### 安全配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/security/rate_limiter.go`

```go
type RateLimitConfig struct {
    Rate  int           // 速率（每秒令牌数）
    Burst int           // 突发容量
    TTL   time.Duration // Bucket过期时间
}
```

#### 流处理配置: `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/stream/factory.go`

```go
type StreamFactoryConfig struct {
    EnableCompression bool
    EnableEncryption  bool
    EnableRateLimit   bool
    CompressionLevel  int
    EncryptionKey     []byte
    RateLimitBytes    int64
    BufferSize        int
}
```

---

## 2. 配置架构问题识别

### 2.1 配置加载机制分析

**当前实现**:
```
命令行参数 (flag) -> YAML 配置文件 (gopkg.in/yaml.v3) -> 环境变量覆盖 (ApplyEnvOverrides)
```

**问题**:

| 问题 | 严重程度 | 描述 |
|------|----------|------|
| 加载逻辑分散 | 中 | 服务端和客户端各自实现加载逻辑，无统一入口 |
| 环境变量手动解析 | 中 | ApplyEnvOverrides 使用硬编码的 os.Getenv，扩展性差 |
| 无 .env 文件支持 | 低 | 开发环境需要手动设置环境变量 |
| 配置验证分散 | 中 | ValidateConfig 与默认值设置混合，逻辑不清晰 |

### 2.2 配置分散程度评估

**问题**: 配置结构散落在多个包中

| 位置 | 配置类型数量 | 备注 |
|------|-------------|------|
| `internal/app/server/config.go` | 10 | 服务端主配置 |
| `internal/client/config.go` | 2 | 客户端主配置 |
| `internal/cloud/configs/configs.go` | 4 | 云控配置 |
| `internal/core/storage/*.go` | 5 | 存储配置 |
| `internal/httpservice/config.go` | 9 | HTTP 服务配置 |
| `internal/protocol/session/config.go` | 2 | 会话配置 |
| `internal/broker/factory.go` | 2 | 消息代理配置 |
| `internal/security/*.go` | 4+ | 安全配置 |
| `internal/stream/*.go` | 3+ | 流处理配置 |

**总计**: 约 40+ 个配置结构体，分布在 15+ 个文件中

### 2.3 配置类型安全评估

**优点**:
- 所有配置使用强类型结构体
- YAML/JSON tag 明确定义序列化行为
- 遵循项目禁止 `interface{}/any` 的规范

**问题**:

| 问题 | 位置 | 描述 |
|------|------|------|
| tag 不一致 | 多处 | 部分使用 `json:` tag，部分使用 `yaml:` tag |
| 敏感字段暴露 | config.go | Token、Password 等字段可能被日志打印 |
| 嵌套结构命名 | client/config.go | `Server struct {}` 匿名嵌套，不利于复用 |

### 2.4 默认值管理现状

**当前方式**:

| 模块 | 默认值管理方式 | 文件 |
|------|---------------|------|
| Server | `GetDefaultConfig()` 函数 | config.go:268 |
| Client | `getDefaultConfig()` 私有函数 | config_manager.go:219 |
| Cloud | `DefaultControlConfig()` 函数 | configs.go:26 |
| Storage | 各自 `DefaultXxxConfig()` 函数 | factory.go, json_storage.go |
| HTTP Service | `DefaultHTTPServiceConfig()` 函数 | httpservice/config.go:90 |
| Security | `DefaultIPRateLimitConfig()` 等 | rate_limiter.go:53 |

**问题**:
- 默认值分散在各个包中
- 部分在 Validate 函数中设置默认值（如 ValidateConfig）
- 默认值与验证逻辑混合

---

## 3. 技术需求分析

### 3.1 环境变量支持现状

**已实现** (服务端):
- 通过 `ApplyEnvOverrides` 函数支持环境变量覆盖
- 支持 30+ 个环境变量
- 环境变量优先级高于配置文件

**未实现**:
- 客户端无环境变量支持
- 无自动环境变量绑定（需手动维护映射关系）
- 无嵌套环境变量支持（如 `SERVER_PROTOCOLS_TCP_PORT`）

### 3.2 .env 文件支持需求

**当前状态**: 不支持

**需求场景**:
- 开发环境快速配置
- Docker Compose 环境变量注入
- CI/CD 敏感信息管理

**推荐方案**:
- 引入 `github.com/joho/godotenv` 或使用 Viper 内置支持
- 支持 `.env`, `.env.local`, `.env.{environment}` 层级

### 3.3 YAML 配置解析能力

**当前实现**:
- 使用 `gopkg.in/yaml.v3`
- 支持嵌套结构、数组、map
- 手动实现路径展开（`utils.ExpandPath`）

**缺失能力**:
- 无配置文件合并能力
- 无环境变量插值（`${ENV_VAR}`）
- 无 include/import 支持

### 3.4 配置优先级实现方案

**当前优先级** (服务端):
```
命令行参数 > 环境变量 > YAML 配置文件 > 默认值
```

**推荐优先级** (符合 12-Factor App):
```
1. 命令行参数 (最高)
2. 环境变量
3. .env 文件
4. 配置文件 (config.yaml, config.local.yaml)
5. 代码默认值 (最低)
```

---

## 4. 业界最佳实践参考

### 4.1 Viper 配置库特性

| 特性 | 描述 | Tunnox 需求 |
|------|------|-------------|
| 多格式支持 | JSON, YAML, TOML, HCL, envfile | 需要 YAML, ENV |
| 环境变量自动绑定 | `viper.AutomaticEnv()` | 需要 |
| 配置文件搜索 | 多路径搜索 | 需要 |
| 配置热重载 | `viper.WatchConfig()` | 可选 |
| 默认值管理 | `viper.SetDefault()` | 需要 |
| 嵌套 key 访问 | `viper.Get("server.protocols.tcp.port")` | 需要 |
| 环境变量前缀 | `viper.SetEnvPrefix("TUNNOX")` | 推荐 |
| 配置合并 | 多配置文件合并 | 可选 |

### 4.2 12-Factor App 配置原则

| 原则 | 描述 | Tunnox 现状 | 建议 |
|------|------|-------------|------|
| 配置与代码分离 | 配置存储在环境变量中 | 部分实现 | 全面支持 |
| 严格分离 | 代码不区分环境 | 已实现 | 保持 |
| 不在版本控制中存储配置 | 敏感配置不入库 | 未强制 | 添加 .gitignore |
| 环境变量作为配置源 | 主要配置通过环境变量 | 服务端实现 | 客户端也需支持 |

### 4.3 多层级配置覆盖模式

**推荐架构**:

```
┌─────────────────────────────────────────────────────────────┐
│                     ConfigManager                            │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ CLI Flags   │ >│ Environment │ >│ Config Files       │ │
│  │ (最高优先级)│  │ Variables   │  │ (.yaml, .env)      │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
│                            ↓                                 │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                 Merged Configuration                     ││
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐       ││
│  │  │ Server  │ │ Client  │ │ Storage │ │Security │ ...   ││
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘       ││
│  └─────────────────────────────────────────────────────────┘│
│                            ↓                                 │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              Validation & Default Values                 ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

---

## 5. 配置项完整清单

### 5.1 Server 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 描述 |
|--------|------|------|--------|----------|------|
| `server.protocols.tcp.enabled` | bool | 否 | true | - | 启用 TCP 协议 |
| `server.protocols.tcp.port` | int | 否 | 8000 | `SERVER_TCP_PORT` | TCP 监听端口 |
| `server.protocols.tcp.host` | string | 否 | "0.0.0.0" | - | TCP 监听地址 |
| `server.protocols.kcp.enabled` | bool | 否 | true | - | 启用 KCP 协议 |
| `server.protocols.kcp.port` | int | 否 | 8000 | `SERVER_KCP_PORT` | KCP 监听端口 |
| `server.protocols.quic.enabled` | bool | 否 | true | - | 启用 QUIC 协议 |
| `server.protocols.quic.port` | int | 否 | 8443 | `SERVER_QUIC_PORT` | QUIC 监听端口 |
| `server.protocols.websocket.enabled` | bool | 否 | true | - | 启用 WebSocket |
| `management.listen` | string | 否 | "0.0.0.0:9000" | `MANAGEMENT_LISTEN` | 管理 API 地址 |
| `management.auth.type` | string | 否 | "bearer" | `MANAGEMENT_AUTH_TYPE` | 认证类型 |
| `management.auth.token` | string | 否 | "" | `MANAGEMENT_AUTH_TOKEN` | API Token |
| `management.pprof.enabled` | bool | 否 | true | `MANAGEMENT_PPROF_ENABLED` | 启用 PProf |
| `management.pprof.data_dir` | string | 否 | "logs/pprof" | - | PProf 数据目录 |
| `log.level` | string | 否 | "info" | `LOG_LEVEL` | 日志级别 |
| `log.file` | string | 否 | "logs/server.log" | `LOG_FILE` | 日志文件路径 |
| `log.rotation.max_size` | int | 否 | 100 | - | 单文件最大 MB |
| `log.rotation.max_backups` | int | 否 | 10 | - | 保留文件数 |
| `log.rotation.max_age` | int | 否 | 30 | - | 保留天数 |
| `log.rotation.compress` | bool | 否 | false | - | 是否压缩 |
| `redis.enabled` | bool | 否 | false | `REDIS_ENABLED` | 启用 Redis |
| `redis.addr` | string | 条件必填 | "redis:6379" | `REDIS_ADDR` | Redis 地址 |
| `redis.password` | string | 否 | "" | `REDIS_PASSWORD` | Redis 密码 |
| `redis.db` | int | 否 | 0 | `REDIS_DB` | Redis DB |
| `persistence.enabled` | bool | 否 | true | `PERSISTENCE_ENABLED` | 启用持久化 |
| `persistence.file` | string | 否 | "data/tunnox.json" | `PERSISTENCE_FILE` | 持久化文件 |
| `persistence.auto_save` | bool | 否 | true | `PERSISTENCE_AUTO_SAVE` | 自动保存 |
| `persistence.save_interval` | int | 否 | 30 | `PERSISTENCE_SAVE_INTERVAL` | 保存间隔(秒) |
| `storage.enabled` | bool | 否 | false | `STORAGE_ENABLED` | 启用远程存储 |
| `storage.url` | string | 条件必填 | - | `STORAGE_URL` | 存储服务 URL |
| `storage.token` | string | 否 | "" | `STORAGE_TOKEN` | 存储服务 Token |
| `storage.timeout` | int | 否 | 10 | `STORAGE_TIMEOUT` | 超时(秒) |
| `platform.enabled` | bool | 否 | false | `PLATFORM_ENABLED` | 启用云控平台 |
| `platform.url` | string | 条件必填 | - | `PLATFORM_URL` | 平台 URL |
| `platform.token` | string | 否 | "" | `PLATFORM_TOKEN` | 平台 Token |
| `platform.timeout` | int | 否 | 10 | `PLATFORM_TIMEOUT` | 超时(秒) |

### 5.2 Client 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 命令行参数 | 描述 |
|--------|------|------|--------|------------|------|
| `client_id` | int64 | 条件必填 | 0 | `-id` | 客户端 ID |
| `auth_token` | string | 条件必填 | "" | `-token` | 认证 Token |
| `anonymous` | bool | 否 | true | `-anonymous` | 匿名模式 |
| `device_id` | string | 否 | "anonymous-device" | `-device` | 设备 ID |
| `secret_key` | string | 否 | "" | - | 匿名密钥 |
| `server.address` | string | 否 | "https://gw.tunnox.net/_tunnox" | `-s` | 服务器地址 |
| `server.protocol` | string | 否 | "websocket" | `-p` | 协议类型 |
| `log.level` | string | 否 | "info" | - | 日志级别 |
| `log.format` | string | 否 | "text" | - | 日志格式 |
| `log.file` | string | 否 | (自动检测) | `-log` | 日志文件 |

### 5.3 Storage 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 描述 |
|--------|------|------|--------|------|
| `storage.type` | string | 是 | "memory" | 存储类型: memory/redis/hybrid |
| `redis.addr` | string | 条件必填 | "localhost:6379" | Redis 地址 |
| `redis.password` | string | 否 | "" | Redis 密码 |
| `redis.db` | int | 否 | 0 | Redis DB |
| `redis.pool_size` | int | 否 | 10 | 连接池大小 |
| `json.file_path` | string | 否 | "data/tunnox-data.json" | JSON 文件路径 |
| `json.auto_save` | bool | 否 | true | 自动保存 |
| `json.save_interval` | duration | 否 | 30s | 保存间隔 |
| `remote.grpc_address` | string | 条件必填 | - | gRPC 地址 |
| `remote.timeout` | duration | 否 | 5s | 连接超时 |
| `remote.max_retries` | int | 否 | 3 | 最大重试次数 |
| `remote.tls_enabled` | bool | 否 | false | 启用 TLS |
| `hybrid.cache_type` | string | 否 | "memory" | 缓存类型 |
| `hybrid.enable_persistent` | bool | 否 | false | 启用持久化 |
| `hybrid.default_cache_ttl` | duration | 否 | 1h | 默认缓存 TTL |

### 5.4 Protocol 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 描述 |
|--------|------|------|--------|------|
| `session.heartbeat_timeout` | duration | 否 | 60s | 心跳超时 |
| `session.cleanup_interval` | duration | 否 | 15s | 清理间隔 |
| `session.max_connections` | int | 否 | 10000 | 最大连接数 |
| `session.max_control_connections` | int | 否 | 5000 | 最大控制连接数 |

### 5.5 Security 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 描述 |
|--------|------|------|--------|------|
| `rate_limit.ip.rate` | int | 否 | 10 | IP 速率(每秒) |
| `rate_limit.ip.burst` | int | 否 | 20 | IP 突发容量 |
| `rate_limit.ip.ttl` | duration | 否 | 5m | Bucket TTL |
| `rate_limit.tunnel.rate` | int | 否 | 1048576 | 隧道速率(字节/秒) |
| `rate_limit.tunnel.burst` | int | 否 | 10485760 | 隧道突发(10MB) |
| `jwt.secret_key` | string | 是 | - | JWT 签名密钥 |
| `jwt.expiration` | duration | 否 | 24h | Token 过期时间 |
| `jwt.issuer` | string | 否 | "tunnox" | JWT 签发者 |

### 5.6 Stream 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 描述 |
|--------|------|------|--------|------|
| `stream.enable_compression` | bool | 否 | false | 启用压缩 |
| `stream.compression_level` | int | 否 | 6 | 压缩级别(1-9) |
| `stream.enable_encryption` | bool | 否 | false | 启用加密 |
| `stream.encryption_key` | bytes | 条件必填 | - | 加密密钥 |
| `stream.enable_rate_limit` | bool | 否 | false | 启用限流 |
| `stream.rate_limit_bytes` | int64 | 否 | 1048576 | 限流(字节/秒) |
| `stream.buffer_size` | int | 否 | 4096 | 缓冲区大小 |

### 5.7 HTTP Service 模块配置

| 配置项 | 类型 | 必填 | 默认值 | 描述 |
|--------|------|------|--------|------|
| `http.enabled` | bool | 否 | true | 启用 HTTP 服务 |
| `http.listen_addr` | string | 否 | "0.0.0.0:9000" | 监听地址 |
| `http.modules.management_api.enabled` | bool | 否 | true | 启用管理 API |
| `http.modules.websocket.enabled` | bool | 否 | true | 启用 WebSocket |
| `http.modules.domain_proxy.enabled` | bool | 否 | false | 启用域名代理 |
| `http.modules.domain_proxy.base_domains` | []string | 否 | [] | 基础域名列表 |
| `http.cors.enabled` | bool | 否 | true | 启用 CORS |
| `http.cors.allowed_origins` | []string | 否 | ["*"] | 允许的源 |
| `http.rate_limit.enabled` | bool | 否 | false | 启用限流 |
| `http.rate_limit.requests_per_second` | int | 否 | 100 | 请求数/秒 |

---

## 6. 改进建议

### 6.1 短期改进 (1-2 周)

1. **统一环境变量前缀**
   - 使用 `TUNNOX_` 前缀
   - 例如: `TUNNOX_SERVER_TCP_PORT`, `TUNNOX_REDIS_ADDR`

2. **添加 .env 文件支持**
   - 引入 `github.com/joho/godotenv`
   - 支持 `.env`, `.env.local`

3. **统一 tag 风格**
   - 所有配置结构体统一使用 `yaml:` 和 `json:` 双 tag

### 6.2 中期改进 (1-2 月)

1. **引入 Viper 配置库**
   - 统一配置加载入口
   - 自动环境变量绑定
   - 配置热重载支持

2. **配置结构重组**
   - 创建 `internal/config` 统一包
   - 按模块组织子配置
   - 提供统一的 `Config` 根结构

3. **敏感信息处理**
   - 添加 `Secret` 类型包装敏感字段
   - 实现 `String()` 方法隐藏敏感值
   - 日志输出时自动脱敏

### 6.3 长期改进 (3-6 月)

1. **配置验证框架**
   - 使用 `go-playground/validator`
   - 声明式验证规则
   - 友好的错误消息

2. **配置文档生成**
   - 从结构体自动生成配置文档
   - 生成配置模板

3. **配置管理 API**
   - 运行时查看配置
   - 动态修改部分配置
   - 配置变更审计

---

## 7. 文件路径索引

| 文件 | 主要内容 |
|------|----------|
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/cmd/server/main.go` | 服务端入口 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/cmd/client/main.go` | 客户端入口 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/config.go` | 服务端配置结构 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/config_env.go` | 环境变量覆盖 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/config.go` | 客户端配置结构 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/config_manager.go` | 客户端配置管理 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/cloud/configs/configs.go` | 云控配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/factory.go` | 存储工厂配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/redis_storage.go` | Redis 配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/json_storage.go` | JSON 存储配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/remote_storage.go` | 远程存储配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/hybrid_config.go` | 混合存储配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/httpservice/config.go` | HTTP 服务配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/protocol/session/config.go` | 会话配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/broker/factory.go` | 消息代理配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/security/rate_limiter.go` | 速率限制配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/stream/factory.go` | 流工厂配置 |
| `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/stream/config.go` | 流配置模板 |

---

**报告完成**
