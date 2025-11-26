# Tunnox Core

<div align="center">

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)
![Status](https://img.shields.io/badge/Status-Alpha-orange?style=flat-square)

**企业级内网穿透与端口映射平台**

一个为分布式网络环境设计的高性能隧道解决方案，支持多种传输协议和灵活的部署模式。

[English](README_EN.md) | [架构文档](docs/ARCHITECTURE_DESIGN_V2.2.md) | [API 文档](docs/MANAGEMENT_API.md)

</div>

---

## 项目简介

Tunnox Core 是一个基于 Go 开发的内网穿透平台内核，提供安全、稳定的远程访问能力。项目采用分层架构设计，支持 TCP、WebSocket、UDP、QUIC 等多种传输协议，可灵活适配不同的网络环境和业务场景。

### 核心特性

- **多协议传输**：支持 TCP、WebSocket、UDP、QUIC 四种传输协议
- **端到端加密**：AES-256-GCM 加密，保障数据传输安全
- **数据压缩**：Gzip 压缩，降低带宽消耗
- **流量控制**：令牌桶算法实现精确的带宽限制
- **SOCKS5 代理**：支持 SOCKS5 协议，实现灵活的网络代理
- **分布式架构**：支持集群部署，节点间 gRPC 通信
- **实时配置推送**：通过控制连接实时推送配置变更
- **匿名接入**：支持匿名客户端，降低使用门槛

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

Tunnox 支持四种传输协议，可根据网络环境灵活选择：

| 协议 | 特点 | 适用场景 |
|------|------|----------|
| **TCP** | 稳定可靠，兼容性好 | 传统网络环境，数据库连接 |
| **WebSocket** | HTTP 兼容，防火墙穿透强 | 企业网络，CDN 加速 |
| **UDP** | 低延迟，无连接开销 | 实时应用，游戏服务 |
| **QUIC** | 多路复用，内置加密 | 移动网络，不稳定网络 |

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

### 环境要求

- Go 1.24 或更高版本
- Docker (可选，用于测试环境)

### 编译

```bash
# 克隆仓库
git clone https://github.com/your-org/tunnox-core.git
cd tunnox-core

# 安装依赖
go mod download

# 编译服务端
go build -o bin/tunnox-server ./cmd/server

# 编译客户端
go build -o bin/tunnox-client ./cmd/client
```

### 运行

**1. 启动服务端**

```bash
./bin/tunnox-server -config config.yaml
```

服务端默认监听：
- TCP: 7001
- WebSocket: 7000 (路径: `/_tunnox`)
- QUIC: 7003
- Management API: 9000

**2. 启动客户端**

```bash
# 客户端 A (源端)
./bin/tunnox-client -config client-a.yaml

# 客户端 B (目标端)
./bin/tunnox-client -config client-b.yaml
```

**3. 创建端口映射**

```bash
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "tcp",
    "source_port": 8080,
    "target_host": "localhost",
    "target_port": 3306,
    "enable_compression": true,
    "enable_encryption": true
  }'
```

**4. 访问服务**

```bash
# 通过映射访问目标服务
mysql -h 127.0.0.1 -P 8080 -u user -p
```

### 配置示例

**服务端配置 (server.yaml)**

```yaml
server:
  host: "0.0.0.0"
  port: 7000
  
  protocols:
    tcp:
      enabled: true
      port: 7001
    websocket:
      enabled: true
      port: 7000
    quic:
      enabled: true
      port: 7003

log:
  level: "info"
  format: "text"

cloud:
  type: "built_in"
  built_in:
    jwt_secret_key: "your-secret-key"
```

**客户端配置 (client.yaml)**

```yaml
# 匿名模式
anonymous: true
device_id: "my-device"

# 或注册模式
client_id: 10000001
auth_token: "your-token"

server:
  address: "server.example.com:7001"
  protocol: "tcp"  # tcp/websocket/udp/quic
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
- TCP、WebSocket、UDP、QUIC 四种协议完整实现
- 协议适配器框架和统一接口

**流处理系统** ✅
- 数据包协议和 StreamProcessor
- Gzip 压缩 (Level 1-9)
- AES-256-GCM 加密
- 令牌桶限流

**客户端功能** ✅
- TCP/HTTP/SOCKS5 映射处理器
- 多协议传输支持
- 自动重连和心跳保活

**服务端功能** ✅
- 会话管理和连接路由
- 透明数据转发
- 实时配置推送

**认证系统** ✅
- JWT Token 认证
- 匿名客户端支持
- 客户端认领机制

**Management API** ✅
- RESTful 接口
- 用户、客户端、映射管理
- 统计和监控接口

**集群支持** ✅
- gRPC 节点通信
- Redis/内存消息广播
- 跨节点数据转发

### 开发中功能

**UDP 映射** 🔄
- 服务端 UDP Ingress 已实现
- 客户端 UDP 映射处理器开发中

**配额管理** 🔄
- 基础配额模型已实现
- 配额检查和限制逻辑完善中

**监控系统** 🔄
- 基础指标收集已实现
- Prometheus 集成和可视化开发中

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

### 单节点部署

适合小规模使用或测试环境：

```bash
# 启动服务端
./tunnox-server -config server.yaml

# 启动客户端
./tunnox-client -config client.yaml
```

### 集群部署

适合生产环境和大规模使用：

**基础设施要求**
- Kubernetes 集群
- Redis Cluster (消息广播)
- PostgreSQL/MySQL (可选，持久化存储)

**部署架构**
```
LoadBalancer (80/443)
    ↓
Tunnox Server Pods (多副本)
    ↓
Redis Cluster (会话和消息)
    ↓
Remote Storage (gRPC)
```

详细部署文档参见：[docs/ARCHITECTURE_DESIGN_V2.2.md](docs/ARCHITECTURE_DESIGN_V2.2.md)

### Docker 部署

```bash
# 构建镜像
docker build -t tunnox-server -f Dockerfile.server .
docker build -t tunnox-client -f Dockerfile.client .

# 运行服务端
docker run -d \
  -p 7000:7000 \
  -p 7001:7001 \
  -p 7003:7003 \
  -p 9000:9000 \
  -v ./config.yaml:/app/config.yaml \
  tunnox-server

# 运行客户端
docker run -d \
  -v ./client.yaml:/app/client.yaml \
  tunnox-client
```

---

## 使用示例

### 示例 1：映射 MySQL 数据库

**场景**：访问远程 MySQL 数据库

```bash
# 1. 创建映射
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "tcp",
    "source_port": 13306,
    "target_host": "localhost",
    "target_port": 3306,
    "enable_compression": true,
    "enable_encryption": true
  }'

# 2. 连接数据库
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### 示例 2：映射 Web 服务

**场景**：临时分享本地 Web 服务

```bash
# 1. 创建映射
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "tcp",
    "source_port": 8080,
    "target_host": "localhost",
    "target_port": 3000
  }'

# 2. 访问服务
curl http://localhost:8080
```

### 示例 3：SOCKS5 代理

**场景**：通过 SOCKS5 访问内网服务

```bash
# 1. 创建 SOCKS5 映射
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "socks5",
    "source_port": 1080
  }'

# 2. 使用 SOCKS5 代理
curl --socks5 localhost:1080 http://internal-service:8080
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
  output: "stdout"     # stdout/file

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
  node_id: "node-001"

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

### v0.1 (当前版本)

- [x] 核心架构设计
- [x] 四种传输协议支持
- [x] 流处理系统
- [x] 基础端口映射
- [x] Management API
- [x] 匿名客户端

### v0.2 (计划中)

- [ ] UDP 端口映射完善
- [ ] 配额检查和限制
- [ ] Prometheus 监控集成
- [ ] 性能优化和压测

### v0.3 (规划中)

- [ ] Web 管理界面
- [ ] 客户端 SDK (Go/Python/Rust)
- [ ] 插件系统
- [ ] 更多协议支持

### v1.0 (长期目标)

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

## 常见问题

**Q: Tunnox 与 frp、ngrok 有什么区别？**

A: Tunnox 在架构上更注重可扩展性和商业化支持，内置完整的云控管理系统、配额管理、多协议支持和集群部署能力。frp 更适合个人使用，ngrok 是闭源商业产品。

**Q: 支持哪些操作系统？**

A: 支持 Linux、macOS、Windows，以及 Docker 容器部署。

**Q: 性能如何？**

A: 在透明转发模式下，单节点可支持 10K+ 并发连接，延迟增加 < 5ms。具体性能取决于硬件配置和网络环境。

**Q: 是否支持 IPv6？**

A: 支持，所有协议适配器均支持 IPv4 和 IPv6。

**Q: 如何保证安全性？**

A: 提供端到端 AES-256-GCM 加密、JWT 认证、细粒度权限控制。建议生产环境启用加密和认证。

**Q: 可以商业使用吗？**

A: 可以，项目采用 MIT 许可证，允许商业使用和二次开发。

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
