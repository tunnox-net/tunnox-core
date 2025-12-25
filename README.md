# Tunnox Core

<div align="center">

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)
![Status](https://img.shields.io/badge/Status-Alpha-orange?style=flat-square)

**企业级内网穿透与端口映射平台**

一个为分布式网络环境设计的高性能隧道解决方案，支持多种传输协议和灵活的部署模式。

[English](README_EN.md) | [快速开始](docs/QuickStart.md) | [架构文档](docs/ARCHITECTURE_DESIGN_V2.2.md) | [API 文档](docs/MANAGEMENT_API.md)

</div>

---

## 项目简介

Tunnox Core 是一个基于 Go 开发的内网穿透工具，提供安全、稳定的远程访问能力。项目采用分层架构设计，支持 TCP、WebSocket、UDP、QUIC 等多种传输协议，可灵活适配不同的网络环境和业务场景。

**设计理念**：Tunnox Core 可以作为独立工具直接使用（无需外部存储和管理平台），也可以作为平台内核集成到更大的系统中。

### 核心特性

- **零依赖启动**：无需数据库、Redis 等外部存储，开箱即用
- **多协议传输**：支持 TCP、WebSocket、KCP、QUIC 四种传输协议
- **端到端加密**：AES-256-GCM 加密，保障数据传输安全
- **数据压缩**：Gzip 压缩，降低带宽消耗
- **流量控制**：令牌桶算法实现精确的带宽限制
- **SOCKS5 代理**：支持 SOCKS5 协议，实现灵活的网络代理，支持动态目标地址
- **HTTP 域名代理**：支持通过 HTTP 代理访问目标网络中的 HTTP 服务
- **匿名接入**：支持匿名客户端，无需注册即可使用
- **交互式 CLI**：完善的命令行界面，支持连接码生成、端口映射管理等
- **连接码系统**：一次性连接码，简化隧道建立流程
- **自动连接**：客户端支持多协议自动连接，自动选择最佳可用协议
- **灵活部署**：支持单机部署（内存存储）和集群部署（Redis + gRPC）

### 应用场景

**远程访问**
- 远程访问家庭 NAS、开发机、数据库
- 临时分享本地服务给团队或客户

**IoT 设备管理**
- 工业设备远程监控和控制
- 智能家居设备统一接入

**开发调试**
- 本地服务暴露给外部测试
- Webhook 接收和调试

**企业应用**
- 分支机构内网互联
- 第三方系统安全对接

---

## 技术架构

### 传输协议

Tunnox 支持五种传输协议，可根据网络环境灵活选择：

| 协议 | 特点 | 适用场景 |
|------|------|----------|
| **TCP** | 稳定可靠，兼容性好 | 传统网络环境，数据库连接 |
| **WebSocket** | HTTP 兼容，防火墙穿透强 | 企业网络，CDN 加速 |
| **KCP** | 基于 UDP，低延迟，快速重传 | 实时应用，游戏服务，不稳定网络 |
| **QUIC** | 多路复用，内置加密，0-RTT 连接 | 移动网络，高性能场景 |

### 核心组件

**协议适配层**
- 统一的协议适配器接口，支持多协议透明切换
- 每种协议独立监听端口，互不干扰

**会话管理层**
- 连接生命周期管理，心跳保活
- 支持匿名和注册客户端

**流处理层**
- StreamProcessor 提供数据包的读写和解析
- 支持压缩、加密、限流等流转换

**数据转发层**
- 透明转发模式，服务端不解析业务数据
- 支持跨节点桥接转发

**云控管理层**
- Management API 提供 RESTful 接口
- 实时配置推送，无需客户端重启

### 数据流转

```
客户端A (源端)
    ↓ [压缩+加密]
  服务器
    ↓ [透明转发]
客户端B (目标端)
    ↓ [解压+解密]
  目标服务
```

客户端负责数据的压缩和加密，服务端仅做透明转发，降低服务端计算压力，提高转发效率。

---

## 快速开始

### 最简单的使用方式（无需配置文件）

Tunnox Core 设计为零配置启动，无需数据库、Redis 等外部依赖。

**环境要求**：
- Go 1.24 或更高版本（仅编译时需要）
- 或直接使用编译好的二进制文件

**1. 编译**

```bash
# 克隆仓库
git clone https://github.com/your-org/tunnox-core.git
cd tunnox-core

# 编译服务端和客户端
go build -o bin/tunnox-server ./cmd/server
go build -o bin/tunnox-client ./cmd/client
```

**2. 启动服务端（零配置）**

```bash
# 直接启动，使用默认配置（内存存储，无需外部依赖）
./bin/tunnox-server

# 或指定配置文件
./bin/tunnox-server -config config.yaml
```

服务端默认监听：
- TCP: 8000
- WebSocket: 8443
- KCP: 8000 (基于 UDP)
- QUIC: 443

日志输出到：`~/logs/server.log`

**3. 启动客户端（匿名模式）**

```bash
# 交互式模式（推荐）
./bin/tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous

# 守护进程模式
./bin/tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous -daemon
```

客户端启动后会自动连接到服务器，无需注册账号。

**4. 使用连接码创建隧道（最简单的方式）**

在交互式模式下，使用连接码可以快速建立隧道：

**目标端（有服务的机器）**：
```bash
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3306
✅ Connection code generated: abc-def-123
```

**源端（需要访问服务的机器）**：
```bash
tunnox> use-code abc-def-123
Local Listen Address: 127.0.0.1:13306
✅ Mapping created successfully
```

**5. 访问服务**

```bash
# 现在可以通过本地端口访问远程服务
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### 常用命令

客户端交互式 CLI 支持以下命令：

```bash
tunnox> help                    # 显示帮助信息
tunnox> status                  # 显示连接状态
tunnox> generate-code           # 生成连接码（目标端）
tunnox> use-code <code>         # 使用连接码创建映射（源端）
tunnox> list-codes              # 列出所有连接码
tunnox> list-mappings           # 列出所有端口映射
tunnox> delete-mapping <id>     # 删除映射
tunnox> exit                    # 退出 CLI
```

### 配置说明

**服务端配置是可选的**，不提供配置文件时使用默认值。

**最小化服务端配置 (config.yaml)**

```yaml
server:
  protocols:
    tcp:
      enabled: true
      port: 8000
    websocket:
      enabled: true
      port: 8443
    kcp:
      enabled: true
      port: 8000
    quic:
      enabled: true
      port: 443

log:
  level: "info"
  output: "file"
  file: "~/logs/server.log"

# 使用内置存储，无需外部依赖
cloud:
  type: "built_in"
  built_in:
    jwt_secret_key: "change-this-in-production"

# 使用内存存储，无需 Redis
storage:
  type: "memory"

message_broker:
  type: "memory"
```

**客户端配置 (client-config.yaml)**

```yaml
# 匿名模式（推荐用于快速测试）
anonymous: true
device_id: "my-device"

server:
  address: "127.0.0.1:8000"
  protocol: "tcp"  # tcp/websocket/kcp/quic

log:
  level: "info"
  output: "file"
  file: "/tmp/tunnox-client.log"
```

**命令行参数优先级高于配置文件**，可以直接使用命令行参数覆盖配置：

```bash
# 使用命令行参数，无需配置文件
./bin/tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous -device my-device

# 支持的协议：tcp, websocket, kcp, quic
./bin/tunnox-client -s 127.0.0.1:8000 -p kcp -anonymous
```

---

## 核心功能

### 端口映射

支持多种协议的端口映射：

- **TCP 映射**：数据库、SSH、RDP 等 TCP 服务
- **HTTP 映射**：Web 服务、API 接口
- **SOCKS5 代理**：全局代理，支持任意协议

### 数据处理

**压缩**
- Gzip 压缩，可配置压缩级别 (1-9)
- 自动跳过已压缩数据，避免重复压缩

**加密**
- AES-256-GCM 加密算法
- 每个映射独立密钥，互不影响
- 自动密钥协商和分发

**流量控制**
- 令牌桶算法实现带宽限制
- 支持突发流量处理
- 可按映射单独配置

### 客户端管理

**匿名模式**
- 无需注册，设备 ID 自动分配
- 适合临时使用和快速测试

**注册模式**
- JWT Token 认证
- 支持多客户端管理
- 配额和权限控制

### 集群部署

**节点通信**
- gRPC 连接池，高效的节点间通信
- 支持跨节点数据转发

**消息广播**
- Redis Pub/Sub 或内存模式
- 配置变更实时同步

**存储抽象**
- 内存存储：单节点部署
- Redis 存储：集群缓存
- 混合存储：Redis + 远程 gRPC

---

## 技术亮点

### 1. 多协议统一抽象

通过 `ProtocolAdapter` 接口统一不同传输协议的处理逻辑，新增协议只需实现接口即可无缝集成。

### 2. 流处理架构

`StreamProcessor` 提供统一的数据包读写接口，支持链式组合压缩、加密、限流等转换器，实现灵活的数据处理流水线。

### 3. 透明转发模式

服务端不解析业务数据，仅做透明转发，降低 CPU 开销。压缩和加密在客户端完成，保障端到端安全。

### 4. 持久会话管理

UDP 和 QUIC 等无连接协议通过会话管理实现连接语义，支持 `StreamProcessor` 的数据包协议。

### 5. 资源生命周期管理

基于 `dispose` 模式的层次化资源清理，确保连接、流、会话等资源正确释放，防止内存泄漏。

### 6. 实时配置推送

通过控制连接推送配置变更，客户端无需轮询或重启，配置生效延迟低于 100ms。

---

## 项目结构

```
tunnox-core/
├── cmd/                      # 应用入口
│   ├── server/              # 服务端
│   └── client/              # 客户端
├── internal/                # 内部实现
│   ├── protocol/            # 协议适配层
│   │   ├── adapter/         # TCP/WebSocket/UDP/QUIC 适配器
│   │   └── session/         # 会话管理
│   ├── stream/              # 流处理层
│   │   ├── compression/     # 压缩
│   │   ├── encryption/      # 加密
│   │   └── transform/       # 流转换
│   ├── client/              # 客户端实现
│   │   └── mapping/         # 映射处理器
│   ├── cloud/               # 云控管理
│   │   ├── managers/        # 业务管理器
│   │   ├── repos/           # 数据仓库
│   │   └── services/        # 业务服务
│   ├── bridge/              # 集群通信
│   ├── broker/              # 消息广播
│   ├── api/                 # Management API
│   └── core/                # 核心组件
│       ├── storage/         # 存储抽象
│       ├── dispose/         # 资源管理
│       └── idgen/           # ID 生成
├── docs/                    # 文档
└── test-env/                # 测试环境
```

---

## 开发状态

### 已实现功能

**传输协议** ✅
- TCP、WebSocket、KCP、QUIC 四种协议完整实现
- 协议适配器框架和统一接口
- 客户端多协议自动连接功能

**流处理系统** ✅
- 数据包协议和 StreamProcessor
- Gzip 压缩 (Level 1-9)
- AES-256-GCM 加密
- 令牌桶限流

**客户端功能** ✅
- TCP/HTTP/SOCKS5 映射处理器
- SOCKS5 代理支持（动态目标地址）
- HTTP 域名代理支持
- 多协议传输支持（TCP/WebSocket/KCP/QUIC）
- 多协议自动连接（自动选择最佳可用协议）
- 自动重连和心跳保活
- 交互式 CLI 界面
- 连接码生成和使用
- 端口映射管理（列表、查看、删除）
- 配置热更新（服务端推送配置变更）
- 表格化数据显示

**服务端功能** ✅
- 会话管理和连接路由
- 透明数据转发
- 实时配置推送
- 优雅的启动信息显示
- 日志文件输出（不污染控制台）

**认证系统** ✅
- JWT Token 认证
- 匿名客户端支持
- 客户端认领机制

**Management API** ✅
- RESTful 接口
- 用户、客户端、映射管理
- 统计和监控接口
- 连接码管理接口

**配额管理** ✅
- 用户配额模型（客户端数、连接数、带宽、存储）
- 配额检查和限制
- 连接码和映射数量限制

**监控系统** ✅
- 系统指标收集（CPU、内存、Goroutine）
- 资源监控和统计
- 基础指标接口

**集群支持** ✅
- gRPC 节点通信
- Redis/内存消息广播
- 跨节点数据转发

**开发工具链** ✅
- 版本管理和自动化发布
- GitHub Actions CI/CD
- 统一的版本信息管理

### 开发中功能

**监控系统增强** 🔄
- Prometheus 集成和可视化开发中
- 更丰富的指标导出

**Web 管理界面** 📋
- 规划中，将作为独立项目开发

---

## 性能特性

### 传输性能

基于本地测试环境（Docker Nginx）的性能数据：

| 场景 | 延迟 | 说明 |
|------|------|------|
| TCP 直连 | 2.2ms | 基准性能 |
| TCP + 压缩 | 2.3ms | Gzip Level 6 |
| TCP + 压缩 + 加密 | 2.4ms | AES-256-GCM |
| WebSocket | 2.5ms | 通过 Nginx 代理 |
| QUIC | 2.3ms | 0-RTT 连接 |

### 资源占用

- **内存占用**：单连接 ~100KB
- **CPU 占用**：透明转发模式下 < 5%
- **并发连接**：单节点支持 10K+ 并发

### 优化技术

- **内存池**：复用缓冲区，减少 GC 压力
- **零拷贝**：减少内存分配和数据拷贝
- **流式处理**：边读边写，降低内存占用
- **连接复用**：gRPC 连接池，减少握手开销

---

## 部署方式

### 单机部署（推荐用于个人和小团队）

单机部署无需外部依赖，使用内存存储即可：

```bash
# 1. 启动服务端（零配置）
./tunnox-server

# 2. 启动客户端（匿名模式）
./tunnox-client -s server-ip:8000 -p tcp -anonymous
```

**特点**：
- 无需数据库、Redis 等外部存储
- 配置简单，开箱即用
- 适合个人使用、小团队协作、临时测试

### 集群部署（适合生产环境）

如需高可用和横向扩展，可以部署集群模式：

**基础设施要求**：
- Redis Cluster（用于会话共享和消息广播）
- 负载均衡器（可选，用于多节点负载均衡）

**服务端配置**：

```yaml
# 集群模式配置
storage:
  type: "redis"
  redis:
    address: "redis-cluster:6379"
    password: "your-password"

message_broker:
  type: "redis"
  redis:
    address: "redis-cluster:6379"
    password: "your-password"
  # node_id 会在服务启动时自动分配（node-0001 到 node-1000）
  # 无需手动配置
```

**部署架构**：
```
客户端
  ↓
负载均衡器 (可选)
  ↓
Tunnox Server 节点 1, 2, 3...
  ↓
Redis Cluster (会话和消息)
```

**说明**：
- 节点 ID 由 NodeIDAllocator 在服务启动时自动分配（范围：node-0001 到 node-1000）
- 使用分布式锁机制确保 ID 唯一性
- 节点 crash 后，ID 会在 90 秒后自动释放
- 可通过环境变量 `NODE_ID` 或 `MESSAGE_BROKER_NODE_ID` 手动指定（主要用于测试环境）

### Docker 部署

```bash
# 构建镜像
docker build -t tunnox-server -f Dockerfile .
docker build -t tunnox-client -f Dockerfile.client .

# 运行服务端（单机模式）
docker run -d \
  -p 8000:8000 \
  -p 8443:8443 \
  --name tunnox-server \
  tunnox-server

# 运行客户端
docker run -d \
  -e SERVER_ADDRESS="server-ip:8000" \
  -e PROTOCOL="tcp" \
  -e ANONYMOUS="true" \
  --name tunnox-client \
  tunnox-client
```

---

## 使用示例

### 示例 1：映射 MySQL 数据库

**场景**：在本地访问远程服务器上的 MySQL 数据库

**步骤**：

1. 在远程服务器（有 MySQL 的机器）上启动目标端客户端：
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
```

2. 在目标端生成连接码：
```bash
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3306
✅ Connection code: mysql-abc-123
```

3. 在本地机器上启动源端客户端：
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
```

4. 在源端使用连接码：
```bash
tunnox> use-code mysql-abc-123
Local Listen Address: 127.0.0.1:13306
✅ Mapping created
```

5. 连接数据库：
```bash
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### 示例 2：映射 Web 服务

**场景**：临时分享本地开发的 Web 服务给同事测试

**步骤**：

1. 在开发机器上启动目标端：
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3000
✅ Connection code: web-xyz-456
```

2. 同事在他的机器上启动源端：
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> use-code web-xyz-456
Local Listen Address: 127.0.0.1:8080
✅ Mapping created
```

3. 同事访问服务：
```bash
curl http://localhost:8080
# 或在浏览器打开 http://localhost:8080
```

### 示例 3：SOCKS5 代理

**场景**：通过 SOCKS5 代理访问内网多个服务

**步骤**：

1. 在内网机器上启动目标端：
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> generate-code
Select Protocol: 3 (SOCKS5)
✅ Connection code: socks-def-789
```

2. 在外网机器上启动源端：
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> use-code socks-def-789
Local Listen Address: 127.0.0.1:1080
✅ SOCKS5 proxy created
```

3. 使用 SOCKS5 代理访问内网服务：
```bash
# 通过代理访问内网任意服务
curl --socks5 localhost:1080 http://192.168.1.100:8080
curl --socks5 localhost:1080 http://internal-api.local/api/data

# 配置浏览器使用 SOCKS5 代理：127.0.0.1:1080
```

---

## 配置说明

### 服务端配置

```yaml
server:
  host: "0.0.0.0"
  port: 7000
  
  # 协议配置
  protocols:
    tcp:
      enabled: true
      port: 7001
    websocket:
      enabled: true
      port: 7000
    udp:
      enabled: false
      port: 7002
    quic:
      enabled: true
      port: 7003

# 日志配置
log:
  level: "info"        # debug/info/warn/error
  format: "text"       # text/json
  output: "file"       # 仅支持 file（日志写入文件，不输出到控制台）
  file: "logs/server.log"

# 云控配置
cloud:
  type: "built_in"     # built_in/external
  built_in:
    jwt_secret_key: "your-secret-key"
    jwt_expiration: 3600
    cleanup_interval: 300

# 消息代理
message_broker:
  type: "memory"       # memory/redis
  # node_id 在服务启动时自动分配，无需配置

# Management API
management_api:
  enabled: true
  listen_addr: ":9000"
  auth:
    type: "bearer"
    bearer_token: "your-api-key"
```

### 客户端配置

```yaml
# 匿名模式（推荐用于测试）
anonymous: true
device_id: "my-device-001"

# 注册模式（推荐用于生产）
client_id: 10000001
auth_token: "your-jwt-token"

# 服务器配置
server:
  address: "server.example.com:7001"
  protocol: "tcp"      # tcp/websocket/udp/quic
```

### 映射配置

映射通过 Management API 动态创建，不在配置文件中：

```json
{
  "source_client_id": 10000001,
  "target_client_id": 10000002,
  "protocol": "tcp",
  "source_port": 8080,
  "target_host": "localhost",
  "target_port": 3306,
  "enable_compression": true,
  "compression_level": 6,
  "enable_encryption": true,
  "encryption_method": "aes-256-gcm",
  "bandwidth_limit": 10485760
}
```

---

## Management API

Tunnox 提供完整的 RESTful API 用于管理：

### 端点概览

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/users` | POST | 创建用户 |
| `/api/v1/clients` | GET/POST | 管理客户端 |
| `/api/v1/mappings` | GET/POST/DELETE | 管理端口映射 |
| `/api/v1/stats` | GET | 获取统计信息 |
| `/api/v1/nodes` | GET | 查询节点状态 |

### 认证方式

```bash
# Bearer Token 认证
curl -H "Authorization: Bearer your-api-key" \
  http://localhost:9000/api/v1/stats
```

详细 API 文档：[docs/MANAGEMENT_API.md](docs/MANAGEMENT_API.md)

---

## 测试

### 单元测试

```bash
# 运行所有测试
go test ./... -v

# 运行特定包测试
go test ./internal/stream/... -v

# 测试覆盖率
go test ./... -cover
```

### 集成测试

项目提供了完整的测试环境：

```bash
cd test-env

# 启动测试服务（MySQL、Redis、Nginx 等）
docker-compose up -d

# 运行测试脚本
./test-port-mapping.sh
```

### 性能测试

```bash
# 压力测试
go test -bench=. -benchmem ./internal/stream/...

# 并发测试
go test -race ./...
```

---

## 开发路线图

### v1.0.0 (当前版本)

- [x] 核心架构设计
- [x] 四种传输协议支持
- [x] 流处理系统
- [x] TCP/UDP/HTTP/SOCKS5 端口映射
- [x] Management API
- [x] 匿名客户端
- [x] 交互式 CLI 界面
- [x] 连接码系统
- [x] 服务端启动信息显示
- [x] 版本管理和 CI/CD
- [x] 配额管理系统
- [x] 基础监控和统计

### v1.1.0 (计划中)

- [ ] Prometheus 监控集成和可视化
- [ ] 性能优化和压测
- [ ] 更多监控指标导出

### v1.2.0 (规划中)

- [ ] Web 管理界面
- [ ] 客户端 SDK (Go/Python/Rust)
- [ ] 插件系统
- [ ] 更多协议支持

### v2.0.0 (长期目标)

- [ ] 生产级稳定性
- [ ] 完整的文档和示例
- [ ] 商业化支持
- [ ] 社区生态建设

---

## 贡献指南

我们欢迎各种形式的贡献：

- **代码贡献**：修复 Bug、添加功能、性能优化
- **文档改进**：完善文档、添加示例、翻译
- **问题反馈**：报告 Bug、提出建议
- **测试用例**：添加测试、提高覆盖率

### 贡献流程

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### 代码规范

- 遵循 Go 官方编码规范
- 使用 `gofmt` 格式化代码
- 添加必要的注释和文档
- 确保测试通过

---

## 客户端 CLI 使用

### 主要命令

| 命令 | 说明 | 示例 |
|------|------|------|
| `help` | 显示帮助信息 | `help generate-code` |
| `connect` | 连接到服务器 | `connect` |
| `status` | 显示连接状态 | `status` |
| `generate-code` | 生成连接码（目标端） | `generate-code` |
| `list-codes` | 列出所有连接码 | `list-codes` |
| `use-code <code>` | 使用连接码创建映射（源端） | `use-code abc-def-123` |
| `list-mappings` | 列出所有端口映射 | `list-mappings` |
| `show-mapping <id>` | 显示映射详情 | `show-mapping mapping-001` |
| `delete-mapping <id>` | 删除映射 | `delete-mapping mapping-001` |
| `config` | 配置管理 | `config list` |
| `exit` | 退出 CLI | `exit` |

### 连接码工作流程

1. **目标端生成连接码**：
   ```bash
   tunnox> generate-code
   Select Protocol: TCP/UDP/SOCKS5
   Target Address: 192.168.1.10:8080
   ✅ Connection code generated: abc-def-123
   ```

2. **源端使用连接码**：
   ```bash
   tunnox> use-code abc-def-123
   Local Listen Address: 127.0.0.1:8080
   ✅ Mapping created successfully
   ```

3. **查看映射状态**：
   ```bash
   tunnox> list-mappings
   # 显示所有映射的表格
   ```

---

## 常见问题

**Q: 是否需要数据库或 Redis？**

A: 不需要。Tunnox Core 默认使用内存存储，可以零依赖启动。如果需要集群部署或持久化，可以选择配置 Redis。

**Q: 如何快速测试？**

A: 最简单的方式是在同一台机器上启动服务端和两个客户端：
```bash
# 终端 1：启动服务端
./tunnox-server

# 终端 2：启动目标端客户端
./tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous

# 终端 3：启动源端客户端
./tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous

# 然后使用连接码建立隧道
```

**Q: 匿名模式和注册模式有什么区别？**

A: 匿名模式无需注册，使用设备 ID 即可连接，适合快速测试和个人使用。注册模式需要 JWT Token，支持配额管理和权限控制，适合团队和生产环境。

**Q: 支持哪些协议？**

A: 支持 TCP、WebSocket、KCP、QUIC 四种传输协议。推荐使用 TCP（稳定）、KCP（低延迟）或 QUIC（高性能）。

**Q: 如何选择传输协议？**

A: 
- **TCP**：最稳定，兼容性好，推荐用于数据库连接和日常使用
- **WebSocket**：可穿透 HTTP 代理和防火墙，适合企业网络
- **KCP**：基于 UDP，低延迟，快速重传，适合实时应用和游戏
- **QUIC**：多路复用，0-RTT 连接，适合移动网络和高性能场景

**Q: 性能如何？**

A: 在透明转发模式下，单节点可支持 10K+ 并发连接，延迟增加 < 5ms。具体性能取决于硬件配置和网络环境。

**Q: 是否支持 IPv6？**

A: 支持，所有协议适配器均支持 IPv4 和 IPv6。

**Q: 如何保证安全性？**

A: 提供端到端 AES-256-GCM 加密、JWT 认证、细粒度权限控制。建议生产环境启用加密和认证。

**Q: 可以商业使用吗？**

A: 可以，项目采用 MIT 许可证，允许商业使用和二次开发。

**Q: 日志输出到哪里？**

A: 默认输出到文件，不污染控制台。服务端：`~/logs/server.log`，客户端：`/tmp/tunnox-client.log`。可通过配置文件或命令行参数修改。

**Q: 如何在生产环境部署？**

A: 建议使用守护进程模式运行客户端（`-daemon` 参数），配置系统服务（systemd）实现自动启动和重启。服务端建议使用 Docker 或 Kubernetes 部署。

---

## 许可证

本项目采用 [MIT License](LICENSE) 开源协议。

---

## 联系方式

- **项目主页**：[GitHub Repository](https://github.com/your-org/tunnox-core)
- **问题反馈**：[GitHub Issues](https://github.com/your-org/tunnox-core/issues)
- **技术文档**：[docs/](docs/)

---

<div align="center">

**如果这个项目对你有帮助，欢迎 Star ⭐**

</div>
