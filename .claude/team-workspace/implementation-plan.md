# Tunnox 配置系统实现计划

**版本**: 1.0
**日期**: 2025-12-29
**作者**: 通信架构师

---

## 1. 项目概述

### 1.1 目标

重构 Tunnox 配置系统，实现：
1. 统一的配置加载入口
2. 多级配置优先级（CLI > ENV > .env > YAML > 默认值）
3. 自动环境变量绑定（TUNNOX_ 前缀）
4. 友好的配置验证和错误提示
5. 敏感信息脱敏处理
6. 配置模板导出功能
7. 健康检查端点配置

### 1.2 范围

| 范围内 | 范围外 |
|--------|--------|
| 配置加载/验证/导出 | 配置中心集成 (Consul/etcd) |
| 环境变量自动绑定 | 配置加密存储 |
| .env 文件支持 | 多租户配置隔离 |
| 敏感信息脱敏 | 配置审计日志 |
| 健康检查配置 | 配置版本管理 |
| 配置热重载（可选） | Web 配置界面 |

### 1.3 预估总工期

**乐观估计**: 4 周
**正常估计**: 5-6 周
**悲观估计**: 8 周

---

## 2. 任务分解

### 2.1 Phase 1: 基础框架 (Week 1-2)

#### Task 1.1: 创建 config 包结构
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0

**描述**: 创建 `internal/config` 包的基础目录结构和核心接口定义。

**产出物**:
```
internal/config/
├── manager.go          # Manager 接口和基础实现
├── loader.go           # Loader 接口
├── source.go           # Source 接口
├── validator.go        # Validator 接口
├── errors.go           # 配置错误类型
└── schema/
    └── types.go        # 基础类型定义 (Secret, Duration 等)
```

**验收标准**:
- [ ] 所有接口定义完整
- [ ] 基础错误类型定义
- [ ] 编译通过
- [ ] 单元测试框架搭建

---

#### Task 1.2: 实现 Secret 类型
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P0
**依赖**: Task 1.1

**描述**: 实现敏感信息包装器 `Secret` 类型，支持 YAML/JSON 序列化和脱敏输出。

**产出物**:
- `internal/config/secret.go`
- `internal/config/secret_test.go`

**代码示例**:
```go
type Secret struct {
    value string
}

func (s Secret) String() string       // 脱敏输出
func (s Secret) Value() string        // 原始值
func (s Secret) MarshalYAML() (interface{}, error)
func (s *Secret) UnmarshalYAML(node *yaml.Node) error
func (s Secret) MarshalJSON() ([]byte, error)
func (s *Secret) UnmarshalJSON(data []byte) error
```

**验收标准**:
- [ ] String() 返回脱敏字符串 (如 "ab****cd")
- [ ] 空值返回空字符串
- [ ] 短值返回 "****"
- [ ] YAML 序列化脱敏
- [ ] JSON 序列化脱敏
- [ ] 单元测试覆盖率 > 90%

---

#### Task 1.3: 定义配置 Schema 结构
**负责人**: 开发工程师
**工期**: 3 天
**优先级**: P0
**依赖**: Task 1.2

**描述**: 定义所有配置结构体，统一 tag 风格（yaml + json），使用 Secret 类型包装敏感字段。

**产出物**:
```
internal/config/schema/
├── root.go             # Root 配置结构
├── server.go           # ServerConfig, ProtocolConfig
├── client.go           # ClientConfig
├── management.go       # ManagementConfig
├── http.go             # HTTPConfig, ModulesConfig
├── storage.go          # StorageConfig, RedisConfig
├── security.go         # SecurityConfig, JWTConfig
├── log.go              # LogConfig, RotationConfig
├── health.go           # HealthConfig (新增)
└── platform.go         # PlatformConfig
```

**验收标准**:
- [ ] 所有配置项覆盖现有功能
- [ ] 统一使用 `yaml:"xxx" json:"xxx"` 双 tag
- [ ] 敏感字段使用 Secret 类型
- [ ] 添加 `validate` tag 用于验证
- [ ] 编译通过

---

#### Task 1.4: 实现默认值管理
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 1.3

**描述**: 实现统一的默认值管理，将分散在各包的默认值集中到 config 包。

**产出物**:
- `internal/config/defaults.go`
- `internal/config/defaults_test.go`

**代码示例**:
```go
// 获取完整默认配置
func GetDefaultConfig() *schema.Root

// 获取模块默认配置
func GetDefaultServerConfig() *schema.ServerConfig
func GetDefaultClientConfig() *schema.ClientConfig
func GetDefaultStorageConfig() *schema.StorageConfig
// ...
```

**验收标准**:
- [ ] 所有默认值集中定义
- [ ] 与现有默认值保持一致
- [ ] HTTP base_domains 添加 "localhost.tunnox.dev" 默认值
- [ ] 健康检查默认启用
- [ ] 单元测试验证默认值

---

### 2.2 Phase 2: 配置加载 (Week 2-3)

#### Task 2.1: 实现 YAML 配置加载器
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 1.4

**描述**: 实现 YAML 配置文件加载，支持多文件合并和路径搜索。

**产出物**:
- `internal/config/loader_yaml.go`
- `internal/config/loader_yaml_test.go`

**功能**:
- 配置文件搜索路径
- 多文件加载合并 (config.yaml + config.local.yaml)
- 路径展开 (`~` 展开为用户目录)
- 错误定位（行号、列号）

**验收标准**:
- [ ] 按优先级搜索配置文件
- [ ] 支持 config.local.yaml 覆盖
- [ ] YAML 解析错误包含位置信息
- [ ] 单元测试覆盖各种场景

---

#### Task 2.2: 实现 .env 文件支持
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P1
**依赖**: Task 2.1

**描述**: 实现 .env 文件加载，支持多层级 .env 文件。

**产出物**:
- `internal/config/dotenv.go`
- `internal/config/dotenv_test.go`

**依赖库**: `github.com/joho/godotenv`

**功能**:
- 加载 .env, .env.local
- 支持 .env.{APP_ENV}
- 变量插值 `${VAR}` 或 `$VAR`

**验收标准**:
- [ ] 按优先级加载 .env 文件
- [ ] 支持变量插值
- [ ] 忽略不存在的文件
- [ ] 单元测试

---

#### Task 2.3: 实现环境变量自动绑定
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 1.3

**描述**: 实现 TUNNOX_ 前缀的环境变量自动绑定到配置结构。

**产出物**:
- `internal/config/env.go`
- `internal/config/env_test.go`

**功能**:
- 自动根据结构体 tag 生成环境变量名
- 类型转换（string, int, bool, duration, []string）
- 嵌套结构支持

**代码示例**:
```go
func BindEnv(cfg interface{}, prefix string) error

// 映射规则:
// server.protocols.tcp.port -> TUNNOX_SERVER_PROTOCOLS_TCP_PORT
// redis.password -> TUNNOX_REDIS_PASSWORD
```

**验收标准**:
- [ ] 支持所有基础类型
- [ ] 支持 []string (逗号分隔)
- [ ] 支持 duration (Go 格式)
- [ ] 嵌套结构正确绑定
- [ ] 单元测试覆盖率 > 90%

---

#### Task 2.4: 实现配置合并器
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 2.1, 2.2, 2.3

**描述**: 实现多来源配置的合并逻辑，按优先级覆盖。

**产出物**:
- `internal/config/merger.go`
- `internal/config/merger_test.go`

**合并优先级** (从低到高):
1. 默认值
2. YAML 配置文件
3. .env 文件
4. 环境变量
5. 命令行参数

**验收标准**:
- [ ] 正确的优先级顺序
- [ ] 嵌套结构正确合并
- [ ] 零值不覆盖已有值
- [ ] 单元测试验证合并逻辑

---

#### Task 2.5: 实现 ConfigManager
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 2.4

**描述**: 实现配置管理器作为统一入口。

**产出物**:
- `internal/config/manager.go` (完善实现)
- `internal/config/manager_test.go`

**接口**:
```go
type Manager struct { ... }

func New(opts ManagerOptions) (*Manager, error)
func (m *Manager) Load() error
func (m *Manager) Get() *schema.Root
func (m *Manager) GetServer() *schema.ServerConfig
func (m *Manager) GetClient() *schema.ClientConfig
func (m *Manager) Reload() error
func (m *Manager) Close() error
```

**验收标准**:
- [ ] 完整的配置加载流程
- [ ] 线程安全的配置访问
- [ ] 支持重新加载
- [ ] 集成测试

---

### 2.3 Phase 3: 配置验证 (Week 3-4)

#### Task 3.1: 实现基础验证框架
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 2.5

**描述**: 实现配置验证框架，支持声明式和编程式验证。

**产出物**:
- `internal/config/validator.go` (完善实现)
- `internal/config/validation_rules.go`
- `internal/config/validator_test.go`

**验证规则类型**:
- 必填验证 (required)
- 范围验证 (min, max, range)
- 格式验证 (pattern, enum)
- 条件验证 (required_if, required_unless)

**验收标准**:
- [ ] 支持 struct tag 声明式验证
- [ ] 支持自定义验证规则
- [ ] 验证错误包含字段路径
- [ ] 单元测试

---

#### Task 3.2: 实现依赖关系验证
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P1
**依赖**: Task 3.1

**描述**: 实现配置项之间的依赖关系验证。

**产出物**:
- `internal/config/validation_dependencies.go`
- `internal/config/validation_dependencies_test.go`

**依赖规则示例**:
```go
var configDependencies = []Dependency{
    {
        Condition: "storage.redis.enabled == true",
        Required:  []string{"storage.redis.addr"},
        Message:   "当 Redis 启用时，redis.addr 必须配置",
    },
    {
        Condition: "http.modules.domain_proxy.enabled == true",
        Required:  []string{"http.modules.domain_proxy.base_domains"},
        Message:   "当域名代理启用时，base_domains 必须配置",
    },
}
```

**验收标准**:
- [ ] 支持条件表达式
- [ ] 正确检测依赖缺失
- [ ] 清晰的错误消息
- [ ] 单元测试

---

#### Task 3.3: 实现友好错误提示
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P1
**依赖**: Task 3.1

**描述**: 实现用户友好的验证错误输出格式。

**产出物**:
- `internal/config/validation_error.go`
- `internal/config/validation_error_test.go`

**错误输出格式**:
```
Configuration validation failed:

  1. server.protocols.tcp.port
     Current value: 80
     Error: port 80 requires root privileges
     Hint: Use a port >= 1024 or run as root

  2. http.modules.domain_proxy.base_domains
     Current value: []
     Error: base_domains is required when domain_proxy is enabled
     Hint: Add at least one domain, e.g., "localhost.tunnox.dev"

  3. storage.redis.addr
     Current value: ""
     Error: redis.addr is required when redis.enabled is true
     Hint: Set redis.addr, e.g., "localhost:6379"
```

**验收标准**:
- [ ] 清晰的错误格式
- [ ] 包含修复建议
- [ ] 支持多个错误同时显示
- [ ] 终端着色输出（可选）

---

### 2.4 Phase 4: 配置导出 (Week 4)

#### Task 4.1: 实现 YAML 导出
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P1
**依赖**: Task 2.5

**描述**: 实现配置导出为 YAML 格式。

**产出物**:
- `internal/config/export.go`
- `internal/config/export_yaml.go`
- `internal/config/export_test.go`

**功能**:
- 导出当前配置
- 导出默认配置模板
- 添加注释说明（可选）
- 敏感信息脱敏

**验收标准**:
- [ ] 正确的 YAML 格式
- [ ] 敏感信息脱敏
- [ ] 可选添加注释
- [ ] 单元测试

---

#### Task 4.2: 实现环境变量导出
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P2
**依赖**: Task 4.1

**描述**: 实现配置导出为 .env 格式。

**产出物**:
- `internal/config/export_env.go`
- `internal/config/export_env_test.go`

**输出示例**:
```bash
# Server Configuration
TUNNOX_SERVER_TCP_PORT=8000
TUNNOX_SERVER_TCP_ENABLED=true

# Redis Configuration
TUNNOX_REDIS_ENABLED=true
TUNNOX_REDIS_ADDR=localhost:6379
TUNNOX_REDIS_PASSWORD=****
```

**验收标准**:
- [ ] 正确的环境变量格式
- [ ] 敏感信息脱敏
- [ ] 分组注释
- [ ] 单元测试

---

#### Task 4.3: 实现配置命令行工具
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P2
**依赖**: Task 4.1, 4.2

**描述**: 实现配置相关的 CLI 命令。

**产出物**:
- 扩展 `cmd/server/main.go` 或新建 `cmd/tunnox-config/main.go`

**命令**:
```bash
# 验证配置
tunnox config validate -c config.yaml

# 导出配置模板
tunnox config export --format yaml --include-comments > config.example.yaml
tunnox config export --format env > .env.example

# 显示当前配置
tunnox config show --sanitize

# 显示配置搜索路径
tunnox config paths
```

**验收标准**:
- [ ] 所有命令可执行
- [ ] 帮助信息完整
- [ ] 退出码正确

---

### 2.5 Phase 5: 集成与迁移 (Week 4-5)

#### Task 5.1: 迁移服务端配置
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 2.5, 3.3

**描述**: 将服务端从现有配置系统迁移到新配置系统。

**修改文件**:
- `cmd/server/main.go`
- `internal/app/server/config.go`
- `internal/app/server/config_env.go` (废弃)
- `internal/app/server/server.go`

**迁移策略**:
1. 保留现有 Config 结构体作为别名
2. 修改配置加载调用新 Manager
3. 验证所有功能正常
4. 移除旧代码

**验收标准**:
- [ ] 服务端正常启动
- [ ] 环境变量覆盖正常
- [ ] 现有配置文件兼容
- [ ] 单元测试通过
- [ ] 集成测试通过

---

#### Task 5.2: 迁移客户端配置
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P0
**依赖**: Task 5.1

**描述**: 将客户端从现有配置系统迁移到新配置系统。

**修改文件**:
- `cmd/client/main.go`
- `internal/client/config.go`
- `internal/client/config_manager.go`
- `internal/client/cli/*.go`

**验收标准**:
- [ ] 客户端正常启动
- [ ] 快捷命令正常工作
- [ ] 配置文件搜索路径兼容
- [ ] 单元测试通过

---

#### Task 5.3: 添加健康检查配置
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P1
**依赖**: Task 5.1

**描述**: 实现健康检查功能的配置和端点。

**新增文件**:
- `internal/health/handler.go`
- `internal/health/checker.go`

**修改文件**:
- `internal/app/server/server.go` (启动健康检查)

**功能**:
- `/healthz` - Liveness 探针
- `/ready` - Readiness 探针
- 检查 Redis 连接
- 检查协议监听状态

**验收标准**:
- [ ] 健康端点可访问
- [ ] 正确返回健康状态
- [ ] K8s 探针兼容
- [ ] 单元测试

---

#### Task 5.4: 更新文档和示例
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P1
**依赖**: Task 5.1, 5.2

**描述**: 更新配置相关文档和示例文件。

**产出物**:
- `config/server.example.yaml` (完整注释)
- `config/client.example.yaml` (完整注释)
- `config/.env.example`
- 更新 `CLAUDE.md` 配置相关部分

**验收标准**:
- [ ] 示例文件包含所有配置项
- [ ] 注释清晰完整
- [ ] 文档与代码一致

---

### 2.6 Phase 6: 高级功能 (Week 5-6, 可选)

#### Task 6.1: 实现配置热重载
**负责人**: 开发工程师
**工期**: 2 天
**优先级**: P2
**依赖**: Task 5.1

**描述**: 实现配置文件变更的热重载功能。

**产出物**:
- `internal/config/watcher.go`
- `internal/config/watcher_test.go`

**依赖库**: `github.com/fsnotify/fsnotify`

**功能**:
- 监听配置文件变更
- 验证新配置
- 应用可热重载的配置项
- 通知订阅者

**热重载支持**:
| 配置项 | 支持热重载 |
|--------|-----------|
| log.level | 是 |
| rate_limit.* | 是 |
| server.protocols.* | 否 |
| redis.* | 否 |

**验收标准**:
- [ ] 文件变更触发重载
- [ ] 无效配置不应用
- [ ] 不支持热重载的配置给出警告
- [ ] 单元测试

---

#### Task 6.2: 实现配置 Diff
**负责人**: 开发工程师
**工期**: 1 天
**优先级**: P3
**依赖**: Task 4.1

**描述**: 实现配置差异对比功能。

**产出物**:
- `internal/config/diff.go`
- `internal/config/diff_test.go`

**命令**:
```bash
tunnox config diff config.yaml config.new.yaml
```

**输出示例**:
```
Configuration diff:

  server.protocols.tcp.port:
    - 8000
    + 8080

  log.level:
    - info
    + debug

  + http.modules.domain_proxy.enabled: true
```

**验收标准**:
- [ ] 正确识别差异
- [ ] 友好的输出格式
- [ ] 单元测试

---

## 3. 依赖关系图

```
                    Task 1.1 (config 包结构)
                           │
                           ▼
                    Task 1.2 (Secret 类型)
                           │
                           ▼
                    Task 1.3 (Schema 定义)
                           │
                           ▼
                    Task 1.4 (默认值管理)
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        Task 2.1      Task 2.2     Task 2.3
       (YAML 加载)   (.env 支持)  (ENV 绑定)
              │            │            │
              └────────────┼────────────┘
                           ▼
                    Task 2.4 (配置合并)
                           │
                           ▼
                    Task 2.5 (ConfigManager)
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        Task 3.1      Task 4.1     Task 5.1
       (验证框架)    (YAML 导出)  (服务端迁移)
              │            │            │
              ▼            ▼            ▼
        Task 3.2      Task 4.2     Task 5.2
       (依赖验证)    (ENV 导出)   (客户端迁移)
              │            │            │
              ▼            ▼            ▼
        Task 3.3      Task 4.3     Task 5.3
       (错误提示)    (CLI 工具)   (健康检查)
                                       │
                                       ▼
                                 Task 5.4
                                (文档更新)
                                       │
                           ┌───────────┴───────────┐
                           ▼                       ▼
                     Task 6.1                Task 6.2
                    (热重载)                 (配置 Diff)
```

---

## 4. 风险评估

### 4.1 技术风险

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| 环境变量自动绑定复杂度高 | 中 | 中 | 充分测试，渐进实现 |
| 配置迁移导致回归 | 中 | 高 | 完整的集成测试，灰度发布 |
| 热重载导致竞态条件 | 低 | 高 | 使用读写锁，限制可热重载配置项 |
| 验证规则漏网 | 中 | 中 | 参考现有验证，逐步补充 |

### 4.2 进度风险

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| 需求变更 | 低 | 中 | 明确范围，变更走流程 |
| 测试时间不足 | 中 | 中 | 预留 buffer，自动化测试 |
| 依赖任务阻塞 | 中 | 高 | 及时沟通，并行开发非依赖任务 |

---

## 5. 测试策略

### 5.1 单元测试

**覆盖范围**:
- 每个公开函数都需要单元测试
- 目标覆盖率 > 80%

**测试文件命名**: `*_test.go`

### 5.2 集成测试

**测试场景**:
1. 完整配置加载流程
2. 环境变量覆盖
3. .env 文件加载
4. 配置验证
5. 配置导出

**测试数据**: `internal/config/testdata/`

### 5.3 端到端测试

**测试场景**:
1. 服务端零配置启动
2. 服务端完整配置启动
3. 客户端连接测试
4. 健康检查端点测试

**测试命令**:
```bash
# 运行所有测试
go test ./internal/config/... -v

# 运行覆盖率测试
go test ./internal/config/... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out

# 运行端到端测试
./start_test.sh
```

---

## 6. 里程碑

| 里程碑 | 日期 | 交付物 |
|--------|------|--------|
| M1: 基础框架完成 | Week 2 末 | config 包基础结构，Schema 定义 |
| M2: 配置加载完成 | Week 3 末 | 完整的配置加载流程 |
| M3: 验证与导出完成 | Week 4 中 | 验证框架，导出功能 |
| M4: 迁移完成 | Week 5 末 | 服务端/客户端迁移完成 |
| M5: 高级功能完成 | Week 6 末 | 热重载（可选） |

---

## 7. 资源需求

### 7.1 人员

| 角色 | 人数 | 职责 |
|------|------|------|
| 开发工程师 | 1-2 | 编码实现 |
| QA 工程师 | 1 | 测试验证 |
| 架构师 | 1 | 设计评审、问题答疑 |

### 7.2 依赖库

| 库 | 用途 | 许可证 |
|---|------|--------|
| `gopkg.in/yaml.v3` | YAML 解析 | MIT |
| `github.com/joho/godotenv` | .env 文件 | MIT |
| `github.com/fsnotify/fsnotify` | 文件监听 (可选) | BSD-3 |

---

## 8. 验收标准

### 8.1 功能验收

- [ ] 零配置启动服务端正常
- [ ] 完整配置启动服务端正常
- [ ] 环境变量覆盖配置正常
- [ ] .env 文件加载正常
- [ ] 配置验证错误提示清晰
- [ ] 敏感信息日志不泄露
- [ ] 健康检查端点可用
- [ ] 配置导出正确

### 8.2 非功能验收

- [ ] 配置加载时间 < 100ms
- [ ] 单元测试覆盖率 > 80%
- [ ] 无内存泄漏
- [ ] 线程安全

### 8.3 兼容性验收

- [ ] 现有配置文件无需修改
- [ ] 现有环境变量（无前缀）仍支持（过渡期）
- [ ] 客户端配置搜索路径不变

---

## 9. 工作量估算汇总

| Phase | 任务数 | 预估工时 | 备注 |
|-------|--------|----------|------|
| Phase 1: 基础框架 | 4 | 8 人天 | 核心基础 |
| Phase 2: 配置加载 | 5 | 9 人天 | 核心功能 |
| Phase 3: 配置验证 | 3 | 4 人天 | 用户体验 |
| Phase 4: 配置导出 | 3 | 3 人天 | 运维支持 |
| Phase 5: 集成迁移 | 4 | 6 人天 | 上线关键 |
| Phase 6: 高级功能 | 2 | 3 人天 | 可选 |

**总计**: 21 个任务，约 33 人天（不含 Phase 6 约 30 人天）

---

**文档结束**
