# Tunnox 配置系统产品分析报告

**分析时间**: 2025-12-29
**分析角色**: 产品经理
**分析范围**: tunnox-core 配置系统

---

## 一、配置系统现状总览

### 1.1 配置文件结构

| 组件 | 配置文件位置 | 配置代码位置 |
|------|-------------|-------------|
| 服务端 | `cmd/server/config.yaml` | `internal/app/server/config.go` |
| 客户端 | `client-config.yaml` (多路径搜索) | `internal/client/config.go` |
| HTTP 服务 | 内嵌于服务端配置 | `internal/httpservice/config.go` |
| Session | 代码配置 | `internal/protocol/session/config.go` |

### 1.2 环境变量覆盖支持

服务端支持的环境变量（定义于 `config_env.go`）：

| 环境变量 | 用途 |
|---------|------|
| `REDIS_ENABLED/ADDR/PASSWORD/DB` | Redis 配置 |
| `PERSISTENCE_ENABLED/FILE/AUTO_SAVE/SAVE_INTERVAL` | 持久化配置 |
| `STORAGE_ENABLED/URL/TOKEN/TIMEOUT` | 远程存储配置 |
| `PLATFORM_ENABLED/URL/TOKEN/TIMEOUT` | 云控平台配置 |
| `LOG_LEVEL/LOG_FILE` | 日志配置 |
| `SERVER_TCP_PORT/SERVER_KCP_PORT/SERVER_QUIC_PORT` | 协议端口配置 |
| `MANAGEMENT_LISTEN/AUTH_TYPE/AUTH_TOKEN/PPROF_ENABLED` | 管理 API 配置 |

---

## 二、配置缺失分析

### 2.1 HTTP 代理功能配置缺失 (严重)

**问题**: `tunnox http <port>` 命令需要 HTTP 域名代理功能，但配置中缺少关键配置项。

**当前状态**:
```go
// internal/httpservice/config.go
type DomainProxyModuleConfig struct {
    BaseDomains []string `yaml:"base_domains"` // 基础域名配置
    // ...
}

// 默认配置中 BaseDomains 为空！
DomainProxy: DomainProxyModuleConfig{
    Enabled:       false,  // 默认禁用
    BaseDomains:   nil,    // 没有默认域名
}
```

**缺失配置项**:

| 配置项 | 说明 | 用户影响 |
|--------|------|---------|
| `base_domains` | HTTP 代理的基础域名 | 用户无法使用 `tunnox http` 功能 |
| `default_subdomain_prefix` | 默认子域名前缀 | 没有自动生成子域名的规则 |
| `ssl_cert_path/ssl_key_path` | HTTPS 证书路径 | 无法提供 HTTPS 访问 |
| `auto_ssl` | 是否自动申请 Let's Encrypt | 无法自动化 HTTPS |

**产品设计文档期望** (CLIENT_PRODUCT_DESIGN.md):
```bash
$ tunnox http 3000
# 期望输出：
#   公网地址: https://abc123.tunnox.com
```

**当前实际**: 功能不完整，没有默认域名配置。

### 2.2 协议端口配置不一致

**问题**: 文档、配置文件、代码中的默认端口不一致。

| 协议 | CLAUDE.md 文档 | 配置代码默认值 | config.yaml | config.example.yaml |
|------|---------------|---------------|-------------|---------------------|
| TCP | 8000 | 8000 | 8080 | 8000 |
| WebSocket | 8443 | (依附 HTTP) | enabled: true | 8443 |
| KCP | 8000 (UDP) | 8000 | 8000 | 8000 |
| QUIC | 443 | 8443 | 8443 | 443 |

**用户困惑**: 不同地方看到的端口不同，不知道该用哪个。

### 2.3 WebSocket 协议配置缺失

**问题**: WebSocket 作为独立协议适配器，但在配置中没有独立端口。

**当前配置**:
```yaml
websocket:
  enabled: true
  # 没有 port 配置！
```

**代码行为**: WebSocket 实际通过 HTTP 服务 (端口 9000) 的 `/_tunnox` 路径提供。

**用户困惑**: 配置文件暗示 WebSocket 是独立协议，但实际依附于 HTTP 服务。

### 2.4 客户端配置搜索路径不明确

**问题**: 客户端配置文件搜索路径在多处定义，用户不知道配置文件应该放在哪里。

**搜索路径** (config_manager.go):
1. 可执行文件所在目录/client-config.yaml
2. 当前工作目录/client-config.yaml
3. ~/.tunnox/client-config.yaml

**文档未说明**: 没有文档告诉用户这三个位置的优先级和推荐使用场景。

---

## 三、用户场景配置分析

### 3.1 快速启动场景（零配置）

**目标用户**: 新手小白、临时体验

**期望体验**:
```bash
# 零配置启动服务端
./server

# 零配置启动客户端
./client -anonymous
```

**当前状态**: 基本可用

| 功能 | 状态 | 问题 |
|------|------|------|
| 服务端零配置 | 可用 | 默认配置合理 |
| 客户端匿名模式 | 可用 | - |
| 自动连接 | 部分可用 | 默认服务器是 SaaS，私服需配置 |
| HTTP 隧道 | 不可用 | 缺少 base_domains 配置 |

**配置缺失**:
- 服务端没有内置的 HTTP base domain 默认值
- 没有本地开发用的默认域名（如 `localhost.tunnox.dev`）

### 3.2 开发调试场景（开发者友好）

**目标用户**: 个人开发者

**期望体验**:
```bash
# 快捷命令
tunnox http 3000     # HTTP 隧道
tunnox tcp 22        # TCP 隧道

# 使用配置文件
tunnox start -c my-config.yaml
```

**当前状态**: 部分可用

| 功能 | 状态 | 问题 |
|------|------|------|
| 快捷命令 | 已实现 | quickcmd.go 中实现完整 |
| 配置文件 | 可用 | 配置项说明不够 |
| 日志输出 | 可用 | CLI 模式只写文件，调试不便 |
| 热重载 | 不支持 | 修改配置需重启 |

**配置痛点**:
1. `tunnox http` 命令生成连接码，但没有显示公网访问地址（设计如此，但与竞品差异大）
2. 日志默认写文件，CLI 模式下看不到实时日志
3. 没有 `--dry-run` 选项预览配置效果

### 3.3 生产部署场景（运维友好）

**目标用户**: 企业用户、DevOps

**期望体验**:
```bash
# 守护进程模式
./server -config /etc/tunnox/config.yaml

# Systemd 集成
systemctl start tunnox
```

**当前状态**: 基本可用

| 功能 | 状态 | 问题 |
|------|------|------|
| 配置文件 | 可用 | - |
| 守护进程 | 部分可用 | 服务端没有 --daemon 参数 |
| PID 文件 | 不支持 | 无法用 systemd 管理 |
| 配置校验 | 基本可用 | 只有启动时校验 |
| 配置热重载 | 不支持 | 需要重启 |

**缺失配置项**:
- `pid_file`: PID 文件路径
- `graceful_shutdown_timeout`: 优雅关闭超时
- `health_check.enabled/port`: 健康检查端点
- `metrics.enabled/port`: Prometheus 指标端点

### 3.4 容器化部署场景（K8s/Docker）

**目标用户**: 云原生团队

**期望体验**:
```yaml
# docker-compose.yml
environment:
  - TUNNOX_SERVER_TCP_PORT=8000
  - TUNNOX_REDIS_ADDR=redis:6379
```

**当前状态**: 部分支持

| 功能 | 状态 | 问题 |
|------|------|------|
| 环境变量配置 | 部分支持 | 覆盖范围有限 |
| 无状态运行 | 支持 | 可用 Redis 存储 |
| 健康检查 | 不支持 | K8s liveness/readiness probe 无法配置 |
| 资源限制 | 不支持 | 没有 Go runtime 参数配置 |

**缺失环境变量**:
| 环境变量 | 用途 |
|---------|------|
| `TUNNOX_HTTP_BASE_DOMAINS` | HTTP 代理基础域名 |
| `TUNNOX_HTTP_LISTEN` | HTTP 服务监听地址 |
| `TUNNOX_HEALTH_CHECK_PORT` | 健康检查端口 |
| `TUNNOX_GOMAXPROCS` | Go 并发限制 |
| `TUNNOX_WEBSOCKET_PATH` | WebSocket 路径 |

---

## 四、配置项间依赖关系

### 4.1 隐式依赖（用户容易踩坑）

```
启用 HTTP 代理功能：
├── http_service.modules.domain_proxy.enabled = true
├── http_service.modules.domain_proxy.base_domains 必须非空
└── 需要 DNS 配置将 *.base_domain 指向服务器

启用 WebSocket 协议：
├── server.protocols.websocket.enabled = true
├── 隐式依赖: http_service 也必须启用
└── 隐式依赖: http_service.modules.websocket.enabled = true

启用集群模式：
├── redis.enabled = true
├── redis.addr 必须配置
├── 隐式行为: 自动禁用本地持久化
└── 隐式行为: 启用跨节点广播
```

**问题**: 这些依赖关系没有在配置文件或错误提示中说明。

### 4.2 互斥配置

```
存储模式互斥：
├── 纯内存模式: redis.enabled=false, persistence.enabled=false
├── 本地持久化: redis.enabled=false, persistence.enabled=true
├── Redis 集群: redis.enabled=true (persistence 自动忽略)
└── 混合模式: 不存在，用户可能误配置
```

---

## 五、配置痛点总结

### 5.1 高优先级问题

| 问题 | 影响 | 建议 |
|------|------|------|
| HTTP base_domains 无默认值 | `tunnox http` 功能不可用 | 添加默认本地域名 |
| 端口配置不一致 | 用户困惑 | 统一文档和默认值 |
| WebSocket 依赖不明确 | 配置错误 | 明确配置关系 |
| 环境变量覆盖不全 | K8s 部署困难 | 完善环境变量支持 |

### 5.2 中优先级问题

| 问题 | 影响 | 建议 |
|------|------|------|
| 无配置热重载 | 运维不便 | 支持 SIGHUP 热重载 |
| 无健康检查端点 | K8s 集成困难 | 添加 /healthz 端点 |
| 无 PID 文件 | Systemd 管理困难 | 添加 --pid-file 参数 |
| 配置搜索路径不明确 | 用户困惑 | 添加 --show-config-path 命令 |

### 5.3 低优先级问题

| 问题 | 影响 | 建议 |
|------|------|------|
| 无 dry-run 模式 | 调试不便 | 添加 --dry-run 参数 |
| 无配置导出 | 备份不便 | 添加 config export 命令 |
| 无配置差异对比 | 升级困难 | 添加 config diff 命令 |

---

## 六、配置改进建议

### 6.1 短期改进（1-2 周）

1. **统一端口配置文档**
   - 更新 CLAUDE.md 与代码默认值一致
   - 在配置模板中添加注释说明

2. **完善 HTTP 代理配置**
   ```yaml
   http_service:
     modules:
       domain_proxy:
         enabled: true
         base_domains:
           - "localhost.tunnox.dev"  # 本地开发用
         default_subdomain_length: 8
   ```

3. **添加配置校验命令**
   ```bash
   tunnox config validate -c config.yaml
   ```

### 6.2 中期改进（1 个月）

1. **完善环境变量支持**
   ```bash
   TUNNOX_HTTP_BASE_DOMAINS=a.com,b.com
   TUNNOX_HTTP_LISTEN=0.0.0.0:9000
   TUNNOX_HEALTH_CHECK_ENABLED=true
   ```

2. **添加健康检查端点**
   ```yaml
   health:
     enabled: true
     listen: "0.0.0.0:9090"
     endpoints:
       liveness: /healthz
       readiness: /ready
   ```

3. **支持配置热重载**
   - SIGHUP 信号触发
   - 仅重载可热更新的配置项

### 6.3 长期改进（3 个月）

1. **配置中心集成**
   - 支持从 Consul/etcd 读取配置
   - 支持配置变更推送

2. **配置版本管理**
   - 配置 schema 版本号
   - 自动迁移旧配置

3. **交互式配置向导**
   ```bash
   tunnox config wizard  # 引导式配置生成
   ```

---

## 七、附录

### 7.1 关键配置文件路径

| 文件 | 路径 |
|------|------|
| 服务端主配置 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/cmd/server/config.yaml` |
| 服务端配置代码 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/config.go` |
| 服务端环境变量覆盖 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/config_env.go` |
| 客户端配置代码 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/config.go` |
| 客户端配置管理 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/config_manager.go` |
| HTTP 服务配置 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/httpservice/config.go` |
| Session 配置 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/protocol/session/config.go` |
| 快捷命令实现 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/client/cli/quickcmd.go` |
| 产品设计文档 | `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/next-docs/CLIENT_PRODUCT_DESIGN.md` |

### 7.2 相关命令

```bash
# 查看服务端默认配置
go run ./cmd/server -help

# 生成客户端配置模板
tunnox config init

# 查看当前客户端配置
tunnox config show
```

---

*报告完成 - 产品经理视角*
