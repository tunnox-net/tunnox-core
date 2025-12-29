# Tunnox 配置 Schema 完整定义

**版本**: 1.0
**日期**: 2025-12-29
**作者**: 通信架构师

---

## 1. 配置结构总览

```yaml
# tunnox-core 完整配置结构
server:           # 服务端核心配置
  protocols:      # 传输协议配置
  session:        # 会话管理配置

client:           # 客户端配置
  server:         # 连接服务器配置
  log:            # 日志配置

management:       # 管理 API 配置
  auth:           # 认证配置
  pprof:          # 性能分析配置

http:             # HTTP 服务配置
  modules:        # 功能模块配置
  cors:           # 跨域配置
  rate_limit:     # 限流配置

storage:          # 存储配置
  redis:          # Redis 配置
  persistence:    # 本地持久化配置
  remote:         # 远程存储配置

security:         # 安全配置
  jwt:            # JWT 配置
  rate_limit:     # 速率限制配置

log:              # 日志配置
  rotation:       # 日志轮转配置

health:           # 健康检查配置 (新增)

platform:         # 云控平台配置
```

---

## 2. Server 配置模块

### 2.1 server.protocols - 传输协议配置

```yaml
server:
  protocols:
    tcp:
      enabled: true
      port: 8000
      host: "0.0.0.0"

    websocket:
      enabled: true
      # WebSocket 通过 HTTP 服务提供，无独立端口

    kcp:
      enabled: true
      port: 8000
      host: "0.0.0.0"
      # KCP 特有配置
      mode: "fast"              # normal/fast/fast2/fast3
      snd_wnd: 1024             # 发送窗口大小
      rcv_wnd: 1024             # 接收窗口大小
      mtu: 1400                 # 最大传输单元

    quic:
      enabled: true
      port: 8443
      host: "0.0.0.0"
      # QUIC 特有配置
      max_streams: 100          # 最大并发流数
      idle_timeout: "30s"       # 空闲超时
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `server.protocols.tcp.enabled` | bool | 否 | true | `TUNNOX_SERVER_TCP_ENABLED` | 启用 TCP 协议 |
| `server.protocols.tcp.port` | int | 否 | 8000 | `TUNNOX_SERVER_TCP_PORT` | TCP 监听端口 |
| `server.protocols.tcp.host` | string | 否 | "0.0.0.0" | `TUNNOX_SERVER_TCP_HOST` | TCP 监听地址 |
| `server.protocols.websocket.enabled` | bool | 否 | true | `TUNNOX_SERVER_WEBSOCKET_ENABLED` | 启用 WebSocket |
| `server.protocols.kcp.enabled` | bool | 否 | true | `TUNNOX_SERVER_KCP_ENABLED` | 启用 KCP 协议 |
| `server.protocols.kcp.port` | int | 否 | 8000 | `TUNNOX_SERVER_KCP_PORT` | KCP 监听端口 (UDP) |
| `server.protocols.kcp.host` | string | 否 | "0.0.0.0" | `TUNNOX_SERVER_KCP_HOST` | KCP 监听地址 |
| `server.protocols.kcp.mode` | string | 否 | "fast" | `TUNNOX_SERVER_KCP_MODE` | KCP 模式 |
| `server.protocols.quic.enabled` | bool | 否 | true | `TUNNOX_SERVER_QUIC_ENABLED` | 启用 QUIC 协议 |
| `server.protocols.quic.port` | int | 否 | 8443 | `TUNNOX_SERVER_QUIC_PORT` | QUIC 监听端口 |
| `server.protocols.quic.host` | string | 否 | "0.0.0.0" | `TUNNOX_SERVER_QUIC_HOST` | QUIC 监听地址 |

**验证规则**:
- `port`: 1-65535, 推荐 >= 1024
- `host`: 有效 IP 地址或 "0.0.0.0"
- `kcp.mode`: 枚举值 ["normal", "fast", "fast2", "fast3"]

### 2.2 server.session - 会话管理配置

```yaml
server:
  session:
    heartbeat_timeout: "60s"        # 心跳超时
    cleanup_interval: "15s"         # 清理间隔
    max_connections: 10000          # 最大连接数
    max_control_connections: 5000   # 最大控制连接数
    reconnect_window: "300s"        # 重连窗口期
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `server.session.heartbeat_timeout` | duration | 否 | 60s | `TUNNOX_SESSION_HEARTBEAT_TIMEOUT` | 心跳超时时间 |
| `server.session.cleanup_interval` | duration | 否 | 15s | `TUNNOX_SESSION_CLEANUP_INTERVAL` | 会话清理间隔 |
| `server.session.max_connections` | int | 否 | 10000 | `TUNNOX_SESSION_MAX_CONNECTIONS` | 最大连接数 |
| `server.session.max_control_connections` | int | 否 | 5000 | `TUNNOX_SESSION_MAX_CONTROL_CONNECTIONS` | 最大控制连接数 |
| `server.session.reconnect_window` | duration | 否 | 300s | `TUNNOX_SESSION_RECONNECT_WINDOW` | 重连窗口期 |

**验证规则**:
- `heartbeat_timeout`: >= 10s
- `cleanup_interval`: >= 5s, < heartbeat_timeout
- `max_connections`: >= 1
- `max_control_connections`: >= 1, <= max_connections

---

## 3. Client 配置模块

### 3.1 客户端完整配置

```yaml
client:
  # 身份认证
  client_id: 0                          # 客户端 ID (注册模式必填)
  auth_token: ""                        # 认证 Token (注册模式必填)
  anonymous: true                       # 匿名模式
  device_id: "auto"                     # 设备 ID ("auto" 自动生成)
  secret_key: ""                        # 匿名模式密钥

  # 服务器连接
  server:
    address: "https://gw.tunnox.net/_tunnox"   # 服务器地址
    protocol: "websocket"                       # 协议类型
    auto_reconnect: true                        # 自动重连
    reconnect_interval: "5s"                    # 重连间隔
    connect_timeout: "30s"                      # 连接超时

  # 日志配置
  log:
    level: "info"
    format: "text"                      # text/json
    file: ""                            # 空则自动检测

  # 流处理
  stream:
    enable_compression: false
    compression_level: 6
    buffer_size: 4096
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | CLI 参数 | 环境变量 | 说明 |
|--------|------|------|--------|----------|----------|------|
| `client.client_id` | int64 | 条件 | 0 | `-id` | `TUNNOX_CLIENT_ID` | 客户端 ID |
| `client.auth_token` | string | 条件 | "" | `-token` | `TUNNOX_CLIENT_TOKEN` | 认证 Token |
| `client.anonymous` | bool | 否 | true | `-anonymous` | `TUNNOX_CLIENT_ANONYMOUS` | 匿名模式 |
| `client.device_id` | string | 否 | "auto" | `-device` | `TUNNOX_CLIENT_DEVICE_ID` | 设备 ID |
| `client.secret_key` | secret | 否 | "" | - | `TUNNOX_CLIENT_SECRET_KEY` | 匿名密钥 |
| `client.server.address` | string | 否 | "https://gw.tunnox.net/_tunnox" | `-s` | `TUNNOX_SERVER_ADDRESS` | 服务器地址 |
| `client.server.protocol` | string | 否 | "websocket" | `-p` | `TUNNOX_SERVER_PROTOCOL` | 协议类型 |
| `client.server.auto_reconnect` | bool | 否 | true | - | `TUNNOX_SERVER_AUTO_RECONNECT` | 自动重连 |
| `client.server.reconnect_interval` | duration | 否 | 5s | - | `TUNNOX_SERVER_RECONNECT_INTERVAL` | 重连间隔 |
| `client.server.connect_timeout` | duration | 否 | 30s | - | `TUNNOX_SERVER_CONNECT_TIMEOUT` | 连接超时 |
| `client.log.level` | string | 否 | "info" | - | `TUNNOX_LOG_LEVEL` | 日志级别 |
| `client.log.format` | string | 否 | "text" | - | `TUNNOX_LOG_FORMAT` | 日志格式 |
| `client.log.file` | string | 否 | "" | `-log` | `TUNNOX_LOG_FILE` | 日志文件 |

**验证规则**:
- `client_id`: anonymous=false 时必填, > 0
- `auth_token`: anonymous=false 时必填
- `server.protocol`: 枚举值 ["tcp", "websocket", "kcp", "quic", "auto"]
- `log.level`: 枚举值 ["debug", "info", "warn", "error"]
- `log.format`: 枚举值 ["text", "json"]

---

## 4. Management 配置模块

### 4.1 管理 API 配置

```yaml
management:
  enabled: true
  listen: "0.0.0.0:9000"

  auth:
    type: "bearer"                      # none/bearer/basic
    token: ""                           # Bearer Token
    username: ""                        # Basic Auth 用户名
    password: ""                        # Basic Auth 密码

  pprof:
    enabled: true
    data_dir: "logs/pprof"

  cors:
    enabled: true
    allowed_origins:
      - "*"
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `management.enabled` | bool | 否 | true | `TUNNOX_MANAGEMENT_ENABLED` | 启用管理 API |
| `management.listen` | string | 否 | "0.0.0.0:9000" | `TUNNOX_MANAGEMENT_LISTEN` | 监听地址 |
| `management.auth.type` | string | 否 | "bearer" | `TUNNOX_MANAGEMENT_AUTH_TYPE` | 认证类型 |
| `management.auth.token` | secret | 条件 | "" | `TUNNOX_MANAGEMENT_AUTH_TOKEN` | API Token |
| `management.auth.username` | string | 条件 | "" | `TUNNOX_MANAGEMENT_AUTH_USERNAME` | Basic 用户名 |
| `management.auth.password` | secret | 条件 | "" | `TUNNOX_MANAGEMENT_AUTH_PASSWORD` | Basic 密码 |
| `management.pprof.enabled` | bool | 否 | true | `TUNNOX_MANAGEMENT_PPROF_ENABLED` | 启用 PProf |
| `management.pprof.data_dir` | string | 否 | "logs/pprof" | `TUNNOX_MANAGEMENT_PPROF_DATA_DIR` | PProf 数据目录 |

**验证规则**:
- `auth.type`: 枚举值 ["none", "bearer", "basic"]
- `auth.token`: auth.type="bearer" 时推荐配置
- `auth.username/password`: auth.type="basic" 时必填

---

## 5. HTTP 服务配置模块

### 5.1 HTTP 服务完整配置

```yaml
http:
  enabled: true
  listen: "0.0.0.0:9000"

  modules:
    management_api:
      enabled: true
      prefix: "/_api"

    websocket:
      enabled: true
      path: "/_tunnox"

    domain_proxy:
      enabled: false
      base_domains:                     # 基础域名列表 (启用时必填)
        - "localhost.tunnox.dev"        # 本地开发默认域名
      default_subdomain_length: 8       # 默认子域名长度
      ssl:
        enabled: false
        cert_path: ""
        key_path: ""
        auto_ssl: false                 # 自动申请证书

    websocket_proxy:
      enabled: false

  cors:
    enabled: true
    allowed_origins:
      - "*"
    allowed_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "OPTIONS"
    allowed_headers:
      - "*"
    max_age: 86400

  rate_limit:
    enabled: false
    requests_per_second: 100
    burst: 200
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `http.enabled` | bool | 否 | true | `TUNNOX_HTTP_ENABLED` | 启用 HTTP 服务 |
| `http.listen` | string | 否 | "0.0.0.0:9000" | `TUNNOX_HTTP_LISTEN` | HTTP 监听地址 |
| `http.modules.management_api.enabled` | bool | 否 | true | `TUNNOX_HTTP_MANAGEMENT_API_ENABLED` | 启用管理 API |
| `http.modules.management_api.prefix` | string | 否 | "/_api" | `TUNNOX_HTTP_MANAGEMENT_API_PREFIX` | API 路径前缀 |
| `http.modules.websocket.enabled` | bool | 否 | true | `TUNNOX_HTTP_WEBSOCKET_ENABLED` | 启用 WebSocket |
| `http.modules.websocket.path` | string | 否 | "/_tunnox" | `TUNNOX_HTTP_WEBSOCKET_PATH` | WebSocket 路径 |
| `http.modules.domain_proxy.enabled` | bool | 否 | false | `TUNNOX_HTTP_DOMAIN_PROXY_ENABLED` | 启用域名代理 |
| `http.modules.domain_proxy.base_domains` | []string | 条件 | [] | `TUNNOX_HTTP_BASE_DOMAINS` | 基础域名列表 |
| `http.modules.domain_proxy.default_subdomain_length` | int | 否 | 8 | `TUNNOX_HTTP_SUBDOMAIN_LENGTH` | 子域名长度 |
| `http.modules.domain_proxy.ssl.enabled` | bool | 否 | false | `TUNNOX_HTTP_SSL_ENABLED` | 启用 SSL |
| `http.modules.domain_proxy.ssl.cert_path` | string | 条件 | "" | `TUNNOX_HTTP_SSL_CERT_PATH` | 证书路径 |
| `http.modules.domain_proxy.ssl.key_path` | string | 条件 | "" | `TUNNOX_HTTP_SSL_KEY_PATH` | 私钥路径 |
| `http.cors.enabled` | bool | 否 | true | `TUNNOX_HTTP_CORS_ENABLED` | 启用 CORS |
| `http.cors.allowed_origins` | []string | 否 | ["*"] | `TUNNOX_HTTP_CORS_ORIGINS` | 允许的源 |
| `http.rate_limit.enabled` | bool | 否 | false | `TUNNOX_HTTP_RATE_LIMIT_ENABLED` | 启用限流 |
| `http.rate_limit.requests_per_second` | int | 否 | 100 | `TUNNOX_HTTP_RATE_LIMIT_RPS` | 每秒请求数 |

**验证规则**:
- `http.modules.domain_proxy.base_domains`: domain_proxy.enabled=true 时必填，至少一个域名
- `http.modules.domain_proxy.ssl.cert_path/key_path`: ssl.enabled=true 时必填
- `http.cors.max_age`: >= 0

**依赖关系**:
- `server.protocols.websocket.enabled=true` 依赖 `http.enabled=true` 和 `http.modules.websocket.enabled=true`

---

## 6. Storage 配置模块

### 6.1 存储配置

```yaml
storage:
  # 存储类型: memory/redis/hybrid
  type: "memory"

  # Redis 配置
  redis:
    enabled: false
    addr: "localhost:6379"
    password: ""
    db: 0
    pool_size: 10
    min_idle_conns: 5
    max_retries: 3
    dial_timeout: "5s"
    read_timeout: "3s"
    write_timeout: "3s"

  # 本地持久化配置
  persistence:
    enabled: true
    file: "data/tunnox.json"
    auto_save: true
    save_interval: "30s"

  # 远程 gRPC 存储配置
  remote:
    enabled: false
    grpc_address: ""
    timeout: "5s"
    max_retries: 3
    tls:
      enabled: false
      cert_file: ""
      key_file: ""
      ca_file: ""

  # 混合存储配置
  hybrid:
    cache_type: "memory"              # memory/redis
    enable_persistent: false
    default_cache_ttl: "1h"
    persistent_cache_ttl: "24h"
    shared_cache_ttl: "5m"
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `storage.type` | string | 否 | "memory" | `TUNNOX_STORAGE_TYPE` | 存储类型 |
| `storage.redis.enabled` | bool | 否 | false | `TUNNOX_REDIS_ENABLED` | 启用 Redis |
| `storage.redis.addr` | string | 条件 | "localhost:6379" | `TUNNOX_REDIS_ADDR` | Redis 地址 |
| `storage.redis.password` | secret | 否 | "" | `TUNNOX_REDIS_PASSWORD` | Redis 密码 |
| `storage.redis.db` | int | 否 | 0 | `TUNNOX_REDIS_DB` | Redis DB |
| `storage.redis.pool_size` | int | 否 | 10 | `TUNNOX_REDIS_POOL_SIZE` | 连接池大小 |
| `storage.persistence.enabled` | bool | 否 | true | `TUNNOX_PERSISTENCE_ENABLED` | 启用持久化 |
| `storage.persistence.file` | string | 否 | "data/tunnox.json" | `TUNNOX_PERSISTENCE_FILE` | 持久化文件 |
| `storage.persistence.auto_save` | bool | 否 | true | `TUNNOX_PERSISTENCE_AUTO_SAVE` | 自动保存 |
| `storage.persistence.save_interval` | duration | 否 | 30s | `TUNNOX_PERSISTENCE_SAVE_INTERVAL` | 保存间隔 |
| `storage.remote.enabled` | bool | 否 | false | `TUNNOX_STORAGE_REMOTE_ENABLED` | 启用远程存储 |
| `storage.remote.grpc_address` | string | 条件 | "" | `TUNNOX_STORAGE_GRPC_ADDRESS` | gRPC 地址 |
| `storage.remote.timeout` | duration | 否 | 5s | `TUNNOX_STORAGE_TIMEOUT` | 超时时间 |

**验证规则**:
- `storage.type`: 枚举值 ["memory", "redis", "hybrid"]
- `storage.redis.addr`: redis.enabled=true 时必填
- `storage.redis.db`: 0-15
- `storage.remote.grpc_address`: remote.enabled=true 时必填

**互斥规则**:
- `redis.enabled=true` 时，`persistence` 配置被忽略（集群模式不使用本地持久化）

---

## 7. Security 配置模块

### 7.1 安全配置

```yaml
security:
  # JWT 配置
  jwt:
    secret_key: ""                      # JWT 签名密钥 (生产环境必填)
    expiration: "24h"                   # Token 过期时间
    refresh_expiration: "168h"          # Refresh Token 过期时间 (7天)
    issuer: "tunnox"

  # 速率限制配置
  rate_limit:
    # IP 级别限流
    ip:
      enabled: true
      rate: 10                          # 每秒请求数
      burst: 20                         # 突发容量
      ttl: "5m"                         # Bucket TTL

    # 隧道级别限流
    tunnel:
      enabled: false
      rate: 1048576                     # 字节/秒 (1MB/s)
      burst: 10485760                   # 突发容量 (10MB)

    # 客户端级别限流
    client:
      enabled: false
      rate: 100                         # 请求/秒
      burst: 200

  # 黑白名单
  access_control:
    enabled: false
    whitelist:
      ips: []
      cidrs: []
    blacklist:
      ips: []
      cidrs: []
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `security.jwt.secret_key` | secret | 推荐 | "" | `TUNNOX_JWT_SECRET_KEY` | JWT 签名密钥 |
| `security.jwt.expiration` | duration | 否 | 24h | `TUNNOX_JWT_EXPIRATION` | Token 过期时间 |
| `security.jwt.refresh_expiration` | duration | 否 | 168h | `TUNNOX_JWT_REFRESH_EXPIRATION` | Refresh Token 过期时间 |
| `security.jwt.issuer` | string | 否 | "tunnox" | `TUNNOX_JWT_ISSUER` | JWT 签发者 |
| `security.rate_limit.ip.enabled` | bool | 否 | true | `TUNNOX_RATE_LIMIT_IP_ENABLED` | 启用 IP 限流 |
| `security.rate_limit.ip.rate` | int | 否 | 10 | `TUNNOX_RATE_LIMIT_IP_RATE` | IP 请求速率 |
| `security.rate_limit.ip.burst` | int | 否 | 20 | `TUNNOX_RATE_LIMIT_IP_BURST` | IP 突发容量 |
| `security.rate_limit.tunnel.enabled` | bool | 否 | false | `TUNNOX_RATE_LIMIT_TUNNEL_ENABLED` | 启用隧道限流 |
| `security.rate_limit.tunnel.rate` | int64 | 否 | 1048576 | `TUNNOX_RATE_LIMIT_TUNNEL_RATE` | 隧道流量速率 |

**验证规则**:
- `jwt.secret_key`: 生产环境强烈推荐配置，至少 32 字符
- `rate_limit.*.rate`: > 0
- `rate_limit.*.burst`: >= rate

---

## 8. Log 配置模块

### 8.1 日志配置

```yaml
log:
  level: "info"                         # debug/info/warn/error
  format: "text"                        # text/json
  file: "logs/server.log"               # 日志文件路径
  console: true                         # 同时输出到控制台

  rotation:
    enabled: true
    max_size: 100                       # 单文件最大 MB
    max_backups: 10                     # 保留文件数
    max_age: 30                         # 保留天数
    compress: false                     # 压缩旧日志
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `log.level` | string | 否 | "info" | `TUNNOX_LOG_LEVEL` | 日志级别 |
| `log.format` | string | 否 | "text" | `TUNNOX_LOG_FORMAT` | 日志格式 |
| `log.file` | string | 否 | "logs/server.log" | `TUNNOX_LOG_FILE` | 日志文件 |
| `log.console` | bool | 否 | true | `TUNNOX_LOG_CONSOLE` | 控制台输出 |
| `log.rotation.enabled` | bool | 否 | true | `TUNNOX_LOG_ROTATION_ENABLED` | 启用轮转 |
| `log.rotation.max_size` | int | 否 | 100 | `TUNNOX_LOG_ROTATION_MAX_SIZE` | 单文件最大 MB |
| `log.rotation.max_backups` | int | 否 | 10 | `TUNNOX_LOG_ROTATION_MAX_BACKUPS` | 保留文件数 |
| `log.rotation.max_age` | int | 否 | 30 | `TUNNOX_LOG_ROTATION_MAX_AGE` | 保留天数 |
| `log.rotation.compress` | bool | 否 | false | `TUNNOX_LOG_ROTATION_COMPRESS` | 压缩旧日志 |

**验证规则**:
- `level`: 枚举值 ["debug", "info", "warn", "error"]
- `format`: 枚举值 ["text", "json"]
- `rotation.max_size`: > 0
- `rotation.max_backups`: >= 0
- `rotation.max_age`: >= 0

---

## 9. Health 配置模块 (新增)

### 9.1 健康检查配置

```yaml
health:
  enabled: true
  listen: "0.0.0.0:9090"                # 独立端口避免与业务混合

  endpoints:
    liveness: "/healthz"                # K8s liveness probe
    readiness: "/ready"                 # K8s readiness probe
    startup: "/startup"                 # K8s startup probe

  checks:
    # 存储检查
    storage:
      enabled: true
      timeout: "3s"

    # Redis 检查
    redis:
      enabled: true
      timeout: "3s"

    # 协议监听检查
    protocols:
      enabled: true
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `health.enabled` | bool | 否 | true | `TUNNOX_HEALTH_ENABLED` | 启用健康检查 |
| `health.listen` | string | 否 | "0.0.0.0:9090" | `TUNNOX_HEALTH_LISTEN` | 监听地址 |
| `health.endpoints.liveness` | string | 否 | "/healthz" | `TUNNOX_HEALTH_LIVENESS_PATH` | Liveness 端点 |
| `health.endpoints.readiness` | string | 否 | "/ready" | `TUNNOX_HEALTH_READINESS_PATH` | Readiness 端点 |
| `health.endpoints.startup` | string | 否 | "/startup" | `TUNNOX_HEALTH_STARTUP_PATH` | Startup 端点 |
| `health.checks.storage.enabled` | bool | 否 | true | `TUNNOX_HEALTH_CHECK_STORAGE` | 检查存储 |
| `health.checks.redis.enabled` | bool | 否 | true | `TUNNOX_HEALTH_CHECK_REDIS` | 检查 Redis |
| `health.checks.protocols.enabled` | bool | 否 | true | `TUNNOX_HEALTH_CHECK_PROTOCOLS` | 检查协议 |

---

## 10. Platform 配置模块

### 10.1 云控平台配置

```yaml
platform:
  enabled: false
  url: ""                               # 平台 API 地址
  token: ""                             # API Token
  timeout: "10s"                        # 请求超时
  retry:
    max_retries: 3
    retry_interval: "1s"
```

**配置项详情**:

| 配置项 | 类型 | 必填 | 默认值 | 环境变量 | 说明 |
|--------|------|------|--------|----------|------|
| `platform.enabled` | bool | 否 | false | `TUNNOX_PLATFORM_ENABLED` | 启用云控平台 |
| `platform.url` | string | 条件 | "" | `TUNNOX_PLATFORM_URL` | 平台 API 地址 |
| `platform.token` | secret | 条件 | "" | `TUNNOX_PLATFORM_TOKEN` | API Token |
| `platform.timeout` | duration | 否 | 10s | `TUNNOX_PLATFORM_TIMEOUT` | 请求超时 |

**验证规则**:
- `platform.url`: platform.enabled=true 时必填，有效 URL
- `platform.token`: platform.enabled=true 时推荐配置

---

## 11. 环境变量完整映射表

### 11.1 核心环境变量

| 环境变量 | 配置路径 | 类型 | 说明 |
|----------|----------|------|------|
| `TUNNOX_SERVER_TCP_PORT` | server.protocols.tcp.port | int | TCP 端口 |
| `TUNNOX_SERVER_KCP_PORT` | server.protocols.kcp.port | int | KCP 端口 |
| `TUNNOX_SERVER_QUIC_PORT` | server.protocols.quic.port | int | QUIC 端口 |
| `TUNNOX_MANAGEMENT_LISTEN` | management.listen | string | 管理 API 地址 |
| `TUNNOX_MANAGEMENT_AUTH_TOKEN` | management.auth.token | secret | API Token |
| `TUNNOX_HTTP_LISTEN` | http.listen | string | HTTP 监听地址 |
| `TUNNOX_HTTP_BASE_DOMAINS` | http.modules.domain_proxy.base_domains | []string | 域名列表 |
| `TUNNOX_REDIS_ENABLED` | storage.redis.enabled | bool | 启用 Redis |
| `TUNNOX_REDIS_ADDR` | storage.redis.addr | string | Redis 地址 |
| `TUNNOX_REDIS_PASSWORD` | storage.redis.password | secret | Redis 密码 |
| `TUNNOX_PERSISTENCE_ENABLED` | storage.persistence.enabled | bool | 启用持久化 |
| `TUNNOX_PERSISTENCE_FILE` | storage.persistence.file | string | 持久化文件 |
| `TUNNOX_LOG_LEVEL` | log.level | string | 日志级别 |
| `TUNNOX_LOG_FILE` | log.file | string | 日志文件 |
| `TUNNOX_HEALTH_ENABLED` | health.enabled | bool | 启用健康检查 |
| `TUNNOX_HEALTH_LISTEN` | health.listen | string | 健康检查端口 |
| `TUNNOX_JWT_SECRET_KEY` | security.jwt.secret_key | secret | JWT 密钥 |
| `TUNNOX_PLATFORM_ENABLED` | platform.enabled | bool | 启用云控 |
| `TUNNOX_PLATFORM_URL` | platform.url | string | 平台地址 |
| `TUNNOX_PLATFORM_TOKEN` | platform.token | secret | 平台 Token |

### 11.2 客户端专用环境变量

| 环境变量 | 配置路径 | 类型 | 说明 |
|----------|----------|------|------|
| `TUNNOX_CLIENT_ID` | client.client_id | int64 | 客户端 ID |
| `TUNNOX_CLIENT_TOKEN` | client.auth_token | secret | 认证 Token |
| `TUNNOX_CLIENT_ANONYMOUS` | client.anonymous | bool | 匿名模式 |
| `TUNNOX_CLIENT_DEVICE_ID` | client.device_id | string | 设备 ID |
| `TUNNOX_SERVER_ADDRESS` | client.server.address | string | 服务器地址 |
| `TUNNOX_SERVER_PROTOCOL` | client.server.protocol | string | 协议类型 |

---

## 12. 配置文件示例

### 12.1 最小化服务端配置 (零配置)

```yaml
# config.yaml - 最小化配置，使用默认值
# 无需任何配置即可启动
```

### 12.2 开发环境服务端配置

```yaml
# config.yaml - 开发环境配置
log:
  level: debug
  console: true

server:
  protocols:
    tcp:
      port: 8000
    quic:
      enabled: false

storage:
  persistence:
    enabled: true
    file: "data/dev-tunnox.json"

http:
  modules:
    domain_proxy:
      enabled: true
      base_domains:
        - "localhost.tunnox.dev"
```

### 12.3 生产环境服务端配置

```yaml
# config.yaml - 生产环境配置
log:
  level: info
  file: /var/log/tunnox/server.log
  rotation:
    max_size: 100
    max_backups: 30
    compress: true

server:
  protocols:
    tcp:
      port: 8000
    kcp:
      port: 8000
    quic:
      port: 443
    websocket:
      enabled: true

storage:
  type: redis
  redis:
    enabled: true
    addr: redis-cluster:6379
    password: ${REDIS_PASSWORD}
    pool_size: 20

http:
  listen: "0.0.0.0:80"
  modules:
    domain_proxy:
      enabled: true
      base_domains:
        - "tunnox.example.com"
      ssl:
        enabled: true
        cert_path: /etc/ssl/tunnox/cert.pem
        key_path: /etc/ssl/tunnox/key.pem

security:
  jwt:
    secret_key: ${JWT_SECRET_KEY}
  rate_limit:
    ip:
      enabled: true
      rate: 100
      burst: 200

health:
  enabled: true
  listen: "0.0.0.0:9090"

platform:
  enabled: true
  url: https://platform.tunnox.example.com
  token: ${PLATFORM_TOKEN}
```

### 12.4 .env 文件示例

```bash
# .env - 环境变量配置

# Redis
TUNNOX_REDIS_ENABLED=true
TUNNOX_REDIS_ADDR=redis:6379
TUNNOX_REDIS_PASSWORD=your-redis-password

# HTTP 代理
TUNNOX_HTTP_BASE_DOMAINS=localhost.tunnox.dev,dev.tunnox.local

# 安全
TUNNOX_JWT_SECRET_KEY=your-32-character-secret-key-here

# 日志
TUNNOX_LOG_LEVEL=debug

# 健康检查
TUNNOX_HEALTH_ENABLED=true
TUNNOX_HEALTH_LISTEN=0.0.0.0:9090

# 云控平台
TUNNOX_PLATFORM_ENABLED=true
TUNNOX_PLATFORM_URL=https://platform.tunnox.net
TUNNOX_PLATFORM_TOKEN=your-platform-token
```

---

## 13. 默认值汇总表

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `server.protocols.tcp.enabled` | true | TCP 默认启用 |
| `server.protocols.tcp.port` | 8000 | TCP 默认端口 |
| `server.protocols.websocket.enabled` | true | WebSocket 默认启用 |
| `server.protocols.kcp.enabled` | true | KCP 默认启用 |
| `server.protocols.kcp.port` | 8000 | KCP 默认端口 (UDP) |
| `server.protocols.quic.enabled` | true | QUIC 默认启用 |
| `server.protocols.quic.port` | 8443 | QUIC 默认端口 |
| `server.session.heartbeat_timeout` | 60s | 心跳超时 |
| `server.session.max_connections` | 10000 | 最大连接数 |
| `management.listen` | 0.0.0.0:9000 | 管理 API 地址 |
| `management.auth.type` | bearer | 认证类型 |
| `http.listen` | 0.0.0.0:9000 | HTTP 监听地址 |
| `http.modules.domain_proxy.base_domains` | [] | **需要配置** |
| `http.modules.domain_proxy.default_subdomain_length` | 8 | 子域名长度 |
| `storage.type` | memory | 存储类型 |
| `storage.redis.addr` | localhost:6379 | Redis 地址 |
| `storage.persistence.file` | data/tunnox.json | 持久化文件 |
| `storage.persistence.save_interval` | 30s | 保存间隔 |
| `log.level` | info | 日志级别 |
| `log.rotation.max_size` | 100 | 日志最大 MB |
| `health.enabled` | true | 健康检查默认启用 |
| `health.listen` | 0.0.0.0:9090 | 健康检查端口 |
| `security.jwt.expiration` | 24h | Token 过期时间 |
| `security.rate_limit.ip.rate` | 10 | IP 限流速率 |
| `client.server.address` | https://gw.tunnox.net/_tunnox | 默认服务器 |
| `client.server.protocol` | websocket | 默认协议 |
| `client.anonymous` | true | 默认匿名模式 |

---

**文档结束**
