# Tunnox 配置系统架构设计

**版本**: 1.0
**日期**: 2025-12-29
**作者**: 通信架构师

---

## 1. 整体架构

### 1.1 架构图

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Application Layer                                    │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐             │
│  │  Server Main    │    │  Client Main    │    │  CLI Commands   │             │
│  └────────┬────────┘    └────────┬────────┘    └────────┬────────┘             │
│           │                      │                      │                        │
│           └──────────────────────┼──────────────────────┘                        │
│                                  ▼                                               │
├─────────────────────────────────────────────────────────────────────────────────┤
│                          ConfigManager (统一入口)                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                        config.Manager                                    │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐       │   │
│  │  │  Load()     │ │  Validate() │ │  Watch()    │ │  Export()   │       │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘       │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                  │                                               │
├──────────────────────────────────┼──────────────────────────────────────────────┤
│                         Configuration Sources                                    │
│                                  ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                        Source Chain (按优先级)                           │   │
│  │                                                                          │   │
│  │  ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌───────┐│   │
│  │  │ CLI     │ >> │ ENV     │ >> │ .env    │ >> │ YAML    │ >> │Default││   │
│  │  │ Flags   │    │ Vars    │    │ Files   │    │ Files   │    │ Values││   │
│  │  └─────────┘    └─────────┘    └─────────┘    └─────────┘    └───────┘│   │
│  │                                                                          │   │
│  │  Priority: 5      Priority: 4   Priority: 3    Priority: 2   Priority: 1 │   │
│  │  (最高)           (高)          (中高)         (中)          (最低)      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                  │                                               │
├──────────────────────────────────┼──────────────────────────────────────────────┤
│                         Configuration Processing                                 │
│                                  ▼                                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐ │
│  │   Merger     │ -> │  Validator   │ -> │  Sanitizer   │ -> │   Binder     │ │
│  │  (配置合并)  │    │  (配置验证)  │    │  (敏感脱敏)  │    │  (结构绑定)  │ │
│  └──────────────┘    └──────────────┘    └──────────────┘    └──────────────┘ │
│                                                                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                         Configuration Structures                                 │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         config.Root                                      │   │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐          │   │
│  │  │ Server  │ │ Client  │ │ Storage │ │Security │ │ HTTP    │ ...      │   │
│  │  │ Config  │ │ Config  │ │ Config  │ │ Config  │ │ Config  │          │   │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 配置加载流程

```
                    ┌─────────────────┐
                    │   Application   │
                    │     Start       │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Parse CLI Args │
                    │ (flag package)  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ Create Manager  │
                    │ config.New()    │
                    └────────┬────────┘
                             │
                             ▼
           ┌─────────────────────────────────────┐
           │        Load Configuration           │
           │                                     │
           │  ┌───────────────────────────────┐ │
           │  │  1. Load Default Values       │ │
           │  │     GetDefaultConfig()        │ │
           │  └───────────────┬───────────────┘ │
           │                  ▼                 │
           │  ┌───────────────────────────────┐ │
           │  │  2. Find & Load YAML Files    │ │
           │  │     config.yaml               │ │
           │  │     config.local.yaml         │ │
           │  └───────────────┬───────────────┘ │
           │                  ▼                 │
           │  ┌───────────────────────────────┐ │
           │  │  3. Load .env Files           │ │
           │  │     .env, .env.local          │ │
           │  └───────────────┬───────────────┘ │
           │                  ▼                 │
           │  ┌───────────────────────────────┐ │
           │  │  4. Apply Environment Vars    │ │
           │  │     TUNNOX_* prefix           │ │
           │  └───────────────┬───────────────┘ │
           │                  ▼                 │
           │  ┌───────────────────────────────┐ │
           │  │  5. Apply CLI Overrides       │ │
           │  │     -config, -port, etc.      │ │
           │  └───────────────────────────────┘ │
           └─────────────────┬───────────────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ Validate Config │
                    │   (required,    │
                    │  constraints)   │
                    └────────┬────────┘
                             │
                     ┌───────┴───────┐
                     │   Valid?      │
                     └───────┬───────┘
                        No   │   Yes
                  ┌──────────┼──────────┐
                  ▼                      ▼
         ┌─────────────────┐    ┌─────────────────┐
         │  Return Error   │    │ Sanitize Secrets│
         │  with Details   │    │  for Logging    │
         └─────────────────┘    └────────┬────────┘
                                         │
                                         ▼
                                ┌─────────────────┐
                                │ Return Config   │
                                │   to App        │
                                └─────────────────┘
```

---

## 2. 目录结构设计

### 2.1 配置模块目录结构

```
tunnox-core/
├── internal/
│   ├── config/                          # 配置统一管理模块 (新建)
│   │   ├── manager.go                   # 配置管理器主入口
│   │   ├── loader.go                    # 配置加载器
│   │   ├── validator.go                 # 配置验证器
│   │   ├── env.go                       # 环境变量处理
│   │   ├── dotenv.go                    # .env 文件支持
│   │   ├── merger.go                    # 配置合并逻辑
│   │   ├── secret.go                    # 敏感信息包装器
│   │   ├── defaults.go                  # 默认值定义
│   │   ├── export.go                    # 配置导出功能
│   │   │
│   │   ├── schema/                      # 配置结构定义
│   │   │   ├── root.go                  # 根配置结构
│   │   │   ├── server.go                # 服务端配置
│   │   │   ├── client.go                # 客户端配置
│   │   │   ├── protocol.go              # 协议配置
│   │   │   ├── storage.go               # 存储配置
│   │   │   ├── security.go              # 安全配置
│   │   │   ├── http.go                  # HTTP 服务配置
│   │   │   ├── log.go                   # 日志配置
│   │   │   └── health.go                # 健康检查配置
│   │   │
│   │   └── testdata/                    # 测试用配置文件
│   │       ├── valid_config.yaml
│   │       ├── invalid_config.yaml
│   │       └── .env.test
│   │
│   ├── app/
│   │   └── server/
│   │       ├── config.go                # 保留，但简化为调用 config 模块
│   │       └── config_env.go            # 废弃，迁移至 config/env.go
│   │
│   └── client/
│       ├── config.go                    # 保留，但简化为调用 config 模块
│       └── config_manager.go            # 简化，复用 config 模块
│
├── config/                              # 配置文件目录 (新建)
│   ├── server.yaml                      # 服务端默认配置
│   ├── server.example.yaml              # 服务端配置示例（带详细注释）
│   ├── client.yaml                      # 客户端默认配置
│   ├── client.example.yaml              # 客户端配置示例
│   └── .env.example                     # 环境变量示例
│
└── cmd/
    ├── server/
    │   ├── main.go                      # 调用 config.Manager
    │   └── config.yaml                  # 保留，但内容简化
    │
    └── client/
        └── main.go                      # 调用 config.Manager
```

### 2.2 配置文件搜索路径

**服务端配置文件搜索顺序**:
1. 命令行指定: `-config /path/to/config.yaml`
2. 当前目录: `./config.yaml`
3. 可执行文件目录: `{exec_dir}/config.yaml`
4. 系统配置目录: `/etc/tunnox/config.yaml` (Linux) / `%PROGRAMDATA%\tunnox\config.yaml` (Windows)

**客户端配置文件搜索顺序**:
1. 命令行指定: `-config /path/to/client-config.yaml`
2. 当前目录: `./client-config.yaml`
3. 可执行文件目录: `{exec_dir}/client-config.yaml`
4. 用户配置目录: `~/.tunnox/client-config.yaml`

**.env 文件搜索顺序**:
1. 配置文件所在目录: `{config_dir}/.env`, `{config_dir}/.env.local`
2. 当前工作目录: `./.env`, `./.env.local`
3. 用户目录: `~/.tunnox/.env`

---

## 3. 组件职责

### 3.1 ConfigManager

```go
// internal/config/manager.go

// Manager 是配置系统的统一入口
type Manager struct {
    // 配置来源
    loader    *Loader
    validator *Validator

    // 当前配置
    config    *schema.Root
    configMu  sync.RWMutex

    // 变更通知
    onChange  []func(*schema.Root)

    // 选项
    opts      ManagerOptions
}

// ManagerOptions 配置管理器选项
type ManagerOptions struct {
    // 配置文件路径 (可选，为空则自动搜索)
    ConfigFile string

    // 环境变量前缀 (默认 TUNNOX_)
    EnvPrefix string

    // 是否启用 .env 文件
    EnableDotEnv bool

    // 是否启用热重载
    EnableWatch bool

    // 应用类型 (server/client)
    AppType AppType
}

// 核心方法
func New(opts ManagerOptions) (*Manager, error)
func (m *Manager) Load() error
func (m *Manager) Get() *schema.Root
func (m *Manager) GetServer() *schema.ServerConfig
func (m *Manager) GetClient() *schema.ClientConfig
func (m *Manager) Validate() error
func (m *Manager) Export(format string) ([]byte, error)
func (m *Manager) OnChange(fn func(*schema.Root))
func (m *Manager) Close() error
```

### 3.2 Loader

```go
// internal/config/loader.go

// Loader 负责从各种来源加载配置
type Loader struct {
    sources []Source
    merger  *Merger
}

// Source 配置来源接口
type Source interface {
    // Name 返回来源名称 (用于日志和错误信息)
    Name() string

    // Priority 返回优先级 (数字越大优先级越高)
    Priority() int

    // Load 加载配置
    Load() (map[string]interface{}, error)
}

// 内置来源实现
type DefaultSource struct{}     // 默认值来源
type YAMLSource struct{}        // YAML 文件来源
type DotEnvSource struct{}      // .env 文件来源
type EnvSource struct{}         // 环境变量来源
type CLISource struct{}         // 命令行参数来源
```

### 3.3 Validator

```go
// internal/config/validator.go

// Validator 负责配置验证
type Validator struct {
    rules []ValidationRule
}

// ValidationRule 验证规则接口
type ValidationRule interface {
    // Name 规则名称
    Name() string

    // Validate 执行验证
    Validate(cfg *schema.Root) []ValidationError
}

// ValidationError 验证错误
type ValidationError struct {
    Field   string // 字段路径，如 "server.protocols.tcp.port"
    Value   interface{}
    Message string
    Hint    string // 修复建议
}

// 内置验证规则
type RequiredRule struct{}      // 必填字段验证
type RangeRule struct{}         // 范围验证
type PatternRule struct{}       // 格式验证
type DependencyRule struct{}    // 依赖关系验证
type MutualExclusionRule struct{} // 互斥验证
```

### 3.4 Secret

```go
// internal/config/secret.go

// Secret 敏感信息包装器
type Secret struct {
    value string
}

// String 返回脱敏后的字符串 (用于日志输出)
func (s Secret) String() string {
    if len(s.value) == 0 {
        return ""
    }
    if len(s.value) <= 4 {
        return "****"
    }
    return s.value[:2] + "****" + s.value[len(s.value)-2:]
}

// Value 返回原始值 (仅在需要时使用)
func (s Secret) Value() string {
    return s.value
}

// MarshalYAML 序列化时脱敏
func (s Secret) MarshalYAML() (interface{}, error) {
    return s.String(), nil
}

// UnmarshalYAML 反序列化
func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
    s.value = node.Value
    return nil
}
```

---

## 4. 环境变量绑定规则

### 4.1 命名约定

**规则**: `TUNNOX_` + 配置路径（下划线分隔，大写）

```
配置路径                          环境变量
─────────────────────────────────────────────────────
server.protocols.tcp.port     -> TUNNOX_SERVER_PROTOCOLS_TCP_PORT
server.protocols.tcp.enabled  -> TUNNOX_SERVER_PROTOCOLS_TCP_ENABLED
redis.addr                    -> TUNNOX_REDIS_ADDR
redis.password                -> TUNNOX_REDIS_PASSWORD
log.level                     -> TUNNOX_LOG_LEVEL
http.base_domains             -> TUNNOX_HTTP_BASE_DOMAINS (逗号分隔)
```

### 4.2 类型转换规则

| 配置类型 | 环境变量格式 | 示例 |
|---------|-------------|------|
| string | 原样 | `TUNNOX_LOG_LEVEL=debug` |
| int/int64 | 数字字符串 | `TUNNOX_SERVER_TCP_PORT=8000` |
| bool | true/false/1/0 | `TUNNOX_REDIS_ENABLED=true` |
| duration | Go duration 格式 | `TUNNOX_SESSION_TIMEOUT=30s` |
| []string | 逗号分隔 | `TUNNOX_HTTP_BASE_DOMAINS=a.com,b.com` |

### 4.3 环境变量自动绑定实现

```go
// internal/config/env.go

// BindEnv 自动绑定环境变量到配置结构体
func BindEnv(cfg interface{}, prefix string) error {
    return bindEnvRecursive(reflect.ValueOf(cfg), prefix, "")
}

func bindEnvRecursive(v reflect.Value, prefix, path string) error {
    // 处理指针
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }

    t := v.Type()
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := t.Field(i)

        // 获取字段名
        yamlTag := fieldType.Tag.Get("yaml")
        if yamlTag == "" || yamlTag == "-" {
            continue
        }
        fieldName := strings.Split(yamlTag, ",")[0]

        // 构建环境变量名
        envName := buildEnvName(prefix, path, fieldName)

        // 获取环境变量值
        if envValue := os.Getenv(envName); envValue != "" {
            if err := setFieldValue(field, envValue); err != nil {
                return fmt.Errorf("failed to set %s from %s: %w",
                    buildPath(path, fieldName), envName, err)
            }
        }

        // 递归处理嵌套结构
        if field.Kind() == reflect.Struct {
            if err := bindEnvRecursive(field, prefix, buildPath(path, fieldName)); err != nil {
                return err
            }
        }
    }
    return nil
}
```

---

## 5. .env 文件支持

### 5.1 文件格式

```bash
# .env 文件示例

# 服务端配置
TUNNOX_SERVER_PROTOCOLS_TCP_PORT=8000
TUNNOX_SERVER_PROTOCOLS_QUIC_PORT=8443

# Redis 配置
TUNNOX_REDIS_ENABLED=true
TUNNOX_REDIS_ADDR=localhost:6379
TUNNOX_REDIS_PASSWORD=secret

# HTTP 代理配置
TUNNOX_HTTP_BASE_DOMAINS=localhost.tunnox.dev,tunnox.local

# 日志配置
TUNNOX_LOG_LEVEL=debug

# 健康检查
TUNNOX_HEALTH_ENABLED=true
TUNNOX_HEALTH_PORT=9090
```

### 5.2 .env 文件加载优先级

```
.env              (基础配置，通常提交到版本控制)
.env.local        (本地覆盖，不提交到版本控制)
.env.{APP_ENV}    (环境特定，如 .env.production)
.env.{APP_ENV}.local (环境特定本地覆盖)
```

### 5.3 实现

```go
// internal/config/dotenv.go

// LoadDotEnv 加载 .env 文件
func LoadDotEnv(dirs []string, appEnv string) error {
    // 按优先级从低到高加载
    files := []string{
        ".env",
        ".env.local",
    }

    if appEnv != "" {
        files = append(files, ".env."+appEnv)
        files = append(files, ".env."+appEnv+".local")
    }

    for _, dir := range dirs {
        for _, file := range files {
            path := filepath.Join(dir, file)
            if _, err := os.Stat(path); err == nil {
                if err := godotenv.Load(path); err != nil {
                    utils.Warnf("Failed to load %s: %v", path, err)
                } else {
                    utils.Debugf("Loaded env file: %s", path)
                }
            }
        }
    }
    return nil
}
```

---

## 6. 配置验证机制

### 6.1 验证规则类型

```go
// 声明式验证 tag
type ProtocolConfig struct {
    Enabled bool   `yaml:"enabled"`
    Port    int    `yaml:"port" validate:"required,min=1,max=65535"`
    Host    string `yaml:"host" validate:"required,ip|hostname"`
}

type RedisConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Addr     string `yaml:"addr" validate:"required_if=Enabled true,hostname_port"`
    Password Secret `yaml:"password"`
    DB       int    `yaml:"db" validate:"min=0,max=15"`
}
```

### 6.2 依赖验证规则

```go
// 配置依赖关系验证
var configDependencies = []Dependency{
    {
        Condition: "redis.enabled == true",
        Required:  []string{"redis.addr"},
        Message:   "当 Redis 启用时，redis.addr 必须配置",
    },
    {
        Condition: "http.modules.domain_proxy.enabled == true",
        Required:  []string{"http.modules.domain_proxy.base_domains"},
        Message:   "当域名代理启用时，base_domains 必须配置",
    },
    {
        Condition: "server.protocols.websocket.enabled == true",
        Required:  []string{"http.enabled"},
        Message:   "WebSocket 协议依赖 HTTP 服务，请启用 http.enabled",
    },
}
```

### 6.3 友好错误提示

```go
// 验证错误输出示例
type ValidationResult struct {
    Valid  bool
    Errors []ValidationError
}

func (r *ValidationResult) FormatError() string {
    var sb strings.Builder
    sb.WriteString("Configuration validation failed:\n\n")

    for i, err := range r.Errors {
        sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Field))
        sb.WriteString(fmt.Sprintf("     Current value: %v\n", err.Value))
        sb.WriteString(fmt.Sprintf("     Error: %s\n", err.Message))
        if err.Hint != "" {
            sb.WriteString(fmt.Sprintf("     Hint: %s\n", err.Hint))
        }
        sb.WriteString("\n")
    }

    return sb.String()
}

// 输出示例:
// Configuration validation failed:
//
//   1. server.protocols.tcp.port
//      Current value: 80
//      Error: port 80 requires root privileges
//      Hint: Use a port >= 1024 or run as root
//
//   2. http.modules.domain_proxy.base_domains
//      Current value: []
//      Error: base_domains is required when domain_proxy is enabled
//      Hint: Add at least one domain, e.g., "localhost.tunnox.dev"
```

---

## 7. 配置热重载

### 7.1 支持热重载的配置项

| 配置项 | 热重载 | 说明 |
|--------|--------|------|
| log.level | 是 | 日志级别可动态调整 |
| rate_limit.* | 是 | 限流配置可动态调整 |
| server.protocols.*.enabled | 否 | 需要重启监听 |
| server.protocols.*.port | 否 | 需要重启监听 |
| redis.* | 否 | 需要重建连接 |
| storage.* | 否 | 需要重建存储 |

### 7.2 热重载实现

```go
// internal/config/watch.go

func (m *Manager) Watch() error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }

    // 监听配置文件
    for _, path := range m.configPaths {
        watcher.Add(path)
    }

    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Write == fsnotify.Write {
                    m.handleConfigChange(event.Name)
                }
            case err := <-watcher.Errors:
                utils.Errorf("Config watcher error: %v", err)
            }
        }
    }()

    return nil
}

func (m *Manager) handleConfigChange(path string) {
    utils.Infof("Configuration file changed: %s", path)

    // 加载新配置
    newConfig, err := m.loadConfig()
    if err != nil {
        utils.Errorf("Failed to reload config: %v", err)
        return
    }

    // 验证新配置
    if err := m.validateConfig(newConfig); err != nil {
        utils.Errorf("Invalid new config: %v", err)
        return
    }

    // 检查哪些配置发生变化
    changes := m.diffConfig(m.config, newConfig)

    // 检查是否有不支持热重载的配置变化
    if hasNonReloadableChanges(changes) {
        utils.Warnf("Config changes require restart: %v", changes)
        return
    }

    // 应用新配置
    m.configMu.Lock()
    m.config = newConfig
    m.configMu.Unlock()

    // 通知订阅者
    for _, fn := range m.onChange {
        fn(newConfig)
    }

    utils.Infof("Configuration reloaded successfully")
}
```

---

## 8. 配置导出功能

### 8.1 导出格式

```go
// internal/config/export.go

// ExportFormat 导出格式
type ExportFormat string

const (
    FormatYAML     ExportFormat = "yaml"
    FormatJSON     ExportFormat = "json"
    FormatEnv      ExportFormat = "env"
    FormatMarkdown ExportFormat = "markdown"
)

// Export 导出配置
func (m *Manager) Export(format ExportFormat, opts ExportOptions) ([]byte, error) {
    switch format {
    case FormatYAML:
        return m.exportYAML(opts)
    case FormatJSON:
        return m.exportJSON(opts)
    case FormatEnv:
        return m.exportEnv(opts)
    case FormatMarkdown:
        return m.exportMarkdown(opts)
    default:
        return nil, fmt.Errorf("unsupported format: %s", format)
    }
}

type ExportOptions struct {
    // 是否包含默认值
    IncludeDefaults bool

    // 是否包含注释
    IncludeComments bool

    // 是否脱敏敏感信息
    SanitizeSecrets bool

    // 仅导出指定模块
    Modules []string
}
```

### 8.2 导出示例命令

```bash
# 导出完整配置模板
tunnox config export --format yaml --include-comments > config.example.yaml

# 导出当前运行配置
tunnox config export --format yaml --sanitize-secrets

# 导出环境变量格式
tunnox config export --format env > .env.example

# 导出配置文档
tunnox config export --format markdown > CONFIG.md
```

---

## 9. 迁移计划

### 9.1 Phase 1: 基础框架 (Week 1-2)

1. 创建 `internal/config` 包基础结构
2. 实现 `schema` 子包的配置结构定义
3. 实现 `Secret` 类型
4. 实现基础 `Manager` 和 `Loader`

### 9.2 Phase 2: 环境变量支持 (Week 2-3)

1. 实现自动环境变量绑定
2. 添加 .env 文件支持
3. 迁移现有 `config_env.go` 逻辑

### 9.3 Phase 3: 验证与导出 (Week 3-4)

1. 实现 `Validator` 验证框架
2. 实现友好错误提示
3. 实现配置导出功能

### 9.4 Phase 4: 集成与迁移 (Week 4-5)

1. 修改服务端使用新配置系统
2. 修改客户端使用新配置系统
3. 更新文档和示例

### 9.5 Phase 5: 高级功能 (Week 5-6)

1. 实现配置热重载
2. 添加健康检查配置
3. 完善测试覆盖

---

## 10. 设计原则

### 10.1 遵循的原则

1. **单一入口**: 所有配置通过 `config.Manager` 管理
2. **类型安全**: 所有配置使用强类型结构体
3. **敏感信息保护**: 使用 `Secret` 类型包装敏感字段
4. **友好错误**: 提供清晰的验证错误和修复建议
5. **向后兼容**: 保留现有配置文件格式
6. **渐进增强**: 可选启用新功能（.env、热重载）

### 10.2 不做的事情

1. **不引入 Viper**: 避免引入重型依赖，自行实现轻量级方案
2. **不强制 .env**: 作为可选增强，不强制使用
3. **不破坏现有配置**: 保持现有 YAML 结构兼容
4. **不过度抽象**: 保持代码简单直接

---

**文档结束**
