# HTTP 域名映射持久化存储技术方案

**版本**: 1.0
**日期**: 2026-01-01
**作者**: 通信架构师

---

## 1. 问题分析

### 1.1 当前实现的问题

当前 `InMemoryDomainRegistry` 存在以下问题：

1. **单点故障**：数据仅存储在单个 pod 内存中，pod 重启后数据丢失
2. **无法集群共享**：多节点部署时，各节点数据不同步
3. **与现有存储体系脱节**：未使用项目的 Repository + HybridStorage 架构
4. **缺乏持久化**：无法满足生产环境的数据可靠性要求

### 1.2 现有实现分析

当前实现位于 `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/app/server/http_domain_command_handlers.go`：

```go
// InMemoryDomainRegistry 内存中的域名注册表
type InMemoryDomainRegistry struct {
    mu          sync.RWMutex
    mappings    map[string]*HTTPDomainMapping // mappingID -> mapping
    domainIndex map[string]string             // fullDomain -> mappingID
    baseDomains []string                      // 允许的基础域名
    nextID      int64
}
```

问题：
- `mappings` 和 `domainIndex` 都是内存 map
- `nextID` 是进程内的自增 ID，多节点会冲突
- 无 TTL 管理机制

### 1.3 设计目标

1. 复用现有的 `HybridStorage` + `Repository` 分层架构
2. 支持 PostgreSQL 持久化 + Redis 跨节点缓存
3. 保持与现有 command handler 接口的兼容性
4. 实现高性能的域名查找（O(1) 复杂度）

---

## 2. 架构设计

### 2.1 整体架构

```
+---------------------------------------------------------------------+
|                     Command Handlers                                 |
|  (HTTPDomainCreateHandler, HTTPDomainListHandler, etc.)             |
+------------------------------+--------------------------------------+
                               |
                               v
+---------------------------------------------------------------------+
|                   DomainRegistry (新接口)                            |
|  - SubdomainChecker (检查子域名可用性)                               |
|  - HTTPDomainCreator (创建域名映射)                                  |
|  - HTTPDomainLister  (列出域名映射)                                  |
|  - HTTPDomainDeleter (删除域名映射)                                  |
|  - DomainLookup      (按域名查找映射)                                |
+------------------------------+--------------------------------------+
                               |
                               v
+---------------------------------------------------------------------+
|              HTTPDomainMappingRepository                             |
|  - 基于 GenericRepositoryImpl[*HTTPDomainMapping]                   |
|  - 实现 DomainRegistry 接口                                          |
+------------------------------+--------------------------------------+
                               |
                               v
+---------------------------------------------------------------------+
|                     HybridStorage                                    |
|  +---------------+  +---------------+  +---------------+            |
|  |   Local Cache |  |  Shared Cache |  |   Persistent  |            |
|  |    (Memory)   |  |    (Redis)    |  |  (PostgreSQL) |            |
|  +---------------+  +---------------+  +---------------+            |
+---------------------------------------------------------------------+
```

### 2.2 数据分类策略

HTTP 域名映射属于 **SharedPersistent** 类型数据：
- 需要跨节点共享（任何节点都可能收到该域名的 HTTP 请求）
- 需要持久化（重启后不丢失）

这正好符合 `HybridStorage` 的 `DataCategorySharedPersistent` 分类：
- 写入时：同时写入 Redis（共享缓存）+ PostgreSQL（持久化）
- 读取时：Redis -> PostgreSQL -> 回填 Redis

参考现有配置 `/Users/roger.tong/GolandProjects/tunnox/tunnox-core/internal/core/storage/hybrid/config.go`：

```go
SharedPersistentPrefixes: []string{
    "tunnox:client_mappings:", // 客户端映射索引（跨节点共享 + 持久化）
    "tunnox:user_mappings:",   // 用户映射索引（跨节点共享 + 持久化）
    "tunnox:port_mapping:",    // 端口映射配置（跨节点访问 + 持久化）
    "tunnox:mappings:list",    // 映射全局列表（跨节点查询 + 持久化）
},
```

### 2.3 数据流设计

#### 2.3.1 创建域名映射流程

```
Client Request
      |
      v
HTTPDomainCreateHandler.Handle()
      |
      v
HTTPDomainMappingRepository.CreateHTTPDomainMapping()
      |
      +-> 1. 使用 SetNX 原子检查并占用 fullDomain（Redis）
      |
      +-> 2. 生成 mappingID（Redis INCR 或 Snowflake）
      |
      +-> 3. 构建 HTTPDomainMapping 对象
      |
      +-> 4. 保存映射数据
      |   +-> HybridStorage.Set("tunnox:http_domain:mapping:{mappingID}", mapping)
      |       +-> PostgreSQL (持久化)
      |       +-> Redis (共享缓存)
      |
      +-> 5. 域名索引已在步骤1创建
      |
      +-> 6. 添加到客户端映射列表
          +-> HybridStorage.AppendToList("tunnox:http_domain:client:{clientID}", mapping)
```

#### 2.3.2 按域名查找流程（HTTP 代理使用）

```
HTTP Proxy receives request for "myapp.tunnox.net"
      |
      v
HTTPDomainMappingRepository.GetMappingByDomain("myapp.tunnox.net")
      |
      +-> 1. 查询域名索引（Redis）
      |   +-> GET "tunnox:http_domain:index:myapp.tunnox.net"
      |       +-> 返回 mappingID
      |
      +-> 2. 获取映射详情
          +-> HybridStorage.Get("tunnox:http_domain:mapping:{mappingID}")
              +-> Redis (命中) -> 返回
              +-> PostgreSQL (未命中) -> 返回 + 回填 Redis
```

#### 2.3.3 删除域名映射流程

```
Client Request
      |
      v
HTTPDomainDeleteHandler.Handle()
      |
      v
HTTPDomainMappingRepository.DeleteHTTPDomainMapping()
      |
      +-> 1. 获取映射信息（验证存在性和权限）
      |
      +-> 2. 删除域名索引（先删索引，阻止新请求）
      |   +-> HybridStorage.Delete("tunnox:http_domain:index:{fullDomain}")
      |
      +-> 3. 从客户端映射列表移除
      |   +-> HybridStorage.RemoveFromList("tunnox:http_domain:client:{clientID}", mapping)
      |
      +-> 4. 删除映射数据
          +-> HybridStorage.Delete("tunnox:http_domain:mapping:{mappingID}")
              +-> Redis
              +-> PostgreSQL
```

---

## 3. 接口设计

### 3.1 统一的 DomainRegistry 接口

文件位置：`internal/cloud/repos/http_domain_interfaces.go`

```go
package repos

import (
    "time"
    "tunnox-core/internal/command"
)

// HTTPDomainMapping HTTP 域名映射数据模型
type HTTPDomainMapping struct {
    MappingID    string     `json:"mapping_id"`    // 映射ID (hdm_{id})
    ClientID     int64      `json:"client_id"`     // 所属客户端ID
    Subdomain    string     `json:"subdomain"`     // 子域名
    BaseDomain   string     `json:"base_domain"`   // 基础域名
    FullDomain   string     `json:"full_domain"`   // 完整域名 (subdomain.base_domain)
    TargetHost   string     `json:"target_host"`   // 目标主机
    TargetPort   int        `json:"target_port"`   // 目标端口
    Description  string     `json:"description"`   // 描述
    CreatedAt    time.Time  `json:"created_at"`    // 创建时间
    ExpiresAt    *time.Time `json:"expires_at"`    // 过期时间（可选）
    Status       string     `json:"status"`        // 状态：active/expired/deleted
}

// DomainRegistry HTTP 域名注册表接口
// 整合了 SubdomainChecker, HTTPDomainCreator, HTTPDomainLister, HTTPDomainDeleter
type DomainRegistry interface {
    // === SubdomainChecker 接口 ===

    // IsSubdomainAvailable 检查子域名是否可用
    IsSubdomainAvailable(subdomain, baseDomain string) bool

    // IsBaseDomainAllowed 检查基础域名是否允许
    IsBaseDomainAllowed(baseDomain string) bool

    // === HTTPDomainCreator 接口 ===

    // CreateHTTPDomainMapping 创建 HTTP 域名映射
    // 返回: mappingID, fullDomain, expiresAt, error
    CreateHTTPDomainMapping(
        clientID int64,
        targetHost string,
        targetPort int,
        subdomain, baseDomain, description string,
        ttlSeconds int,
    ) (mappingID, fullDomain, expiresAt string, err error)

    // === HTTPDomainLister 接口 ===

    // ListHTTPDomainMappings 列出客户端的所有 HTTP 域名映射
    ListHTTPDomainMappings(clientID int64) ([]command.HTTPDomainMappingInfo, error)

    // === HTTPDomainDeleter 接口 ===

    // DeleteHTTPDomainMapping 删除 HTTP 域名映射
    DeleteHTTPDomainMapping(clientID int64, mappingID string) error

    // === DomainLookup 接口（新增，用于 HTTP 代理） ===

    // GetMappingByDomain 根据完整域名获取映射（用于 HTTP 代理请求路由）
    GetMappingByDomain(fullDomain string) (*HTTPDomainMapping, error)

    // GetMapping 根据映射ID获取映射
    GetMapping(mappingID string) (*HTTPDomainMapping, error)
}
```

### 3.2 扩展 interfaces.go

文件位置：`internal/cloud/repos/interfaces.go`

```go
// IHTTPDomainMappingRepository HTTP 域名映射数据访问接口
type IHTTPDomainMappingRepository interface {
    // 基础 CRUD
    SaveMapping(mapping *HTTPDomainMapping) error
    CreateMapping(mapping *HTTPDomainMapping) error
    UpdateMapping(mapping *HTTPDomainMapping) error
    GetMapping(mappingID string) (*HTTPDomainMapping, error)
    DeleteMapping(mappingID string) error

    // 域名索引操作
    GetMappingByDomain(fullDomain string) (*HTTPDomainMapping, error)
    SetDomainIndex(fullDomain, mappingID string, ttl time.Duration) error
    DeleteDomainIndex(fullDomain string) error
    DomainIndexExists(fullDomain string) (bool, error)

    // 客户端关联查询
    GetClientMappings(clientID int64) ([]*HTTPDomainMapping, error)
    AddMappingToClient(clientID int64, mapping *HTTPDomainMapping) error
    RemoveMappingFromClient(clientID int64, mappingID string) error

    // 过期清理
    CleanupExpiredMappings() (int, error)
}
```

---

## 4. 数据模型

### 4.1 PostgreSQL 表结构

通过 tunnox-storage gRPC 服务访问，使用 KV 存储模型。

**表结构设计**（tunnox-storage 侧参考）：

```sql
-- http_domain_mappings 表
CREATE TABLE http_domain_mappings (
    mapping_id   VARCHAR(64) PRIMARY KEY,     -- hdm_{snowflake_id}
    client_id    BIGINT NOT NULL,             -- 所属客户端ID
    subdomain    VARCHAR(64) NOT NULL,        -- 子域名
    base_domain  VARCHAR(128) NOT NULL,       -- 基础域名
    full_domain  VARCHAR(256) NOT NULL UNIQUE,-- 完整域名（唯一索引）
    target_host  VARCHAR(256) NOT NULL,       -- 目标主机
    target_port  INT NOT NULL,                -- 目标端口
    description  TEXT,                        -- 描述
    status       VARCHAR(16) DEFAULT 'active',-- 状态
    created_at   TIMESTAMP DEFAULT NOW(),     -- 创建时间
    expires_at   TIMESTAMP,                   -- 过期时间
    updated_at   TIMESTAMP DEFAULT NOW()      -- 更新时间
);

-- 索引
CREATE INDEX idx_hdm_client_id ON http_domain_mappings(client_id);
CREATE INDEX idx_hdm_full_domain ON http_domain_mappings(full_domain);
CREATE INDEX idx_hdm_status ON http_domain_mappings(status);
CREATE INDEX idx_hdm_expires_at ON http_domain_mappings(expires_at)
    WHERE expires_at IS NOT NULL;
```

**注意**：由于 tunnox-core 通过 gRPC 访问 tunnox-storage，数据实际以 JSON 格式存储在 KV 模型中。

### 4.2 Redis 缓存键设计

```
# 映射数据（SharedPersistent，同时写入 Redis 和 PostgreSQL）
键:   tunnox:http_domain:mapping:{mappingID}
值:   {HTTPDomainMapping JSON}
TTL:  24h（SharedCacheTTL）

# 域名索引（Shared，仅 Redis，用于 O(1) 快速查找）
键:   tunnox:http_domain:index:{fullDomain}
值:   mappingID
TTL:  与映射 ExpiresAt 对齐，或默认 7d

# 客户端映射列表（SharedPersistent）
键:   tunnox:http_domain:client:{clientID}
值:   [mappingID1, mappingID2, ...]（JSON 数组或 Redis List）
TTL:  24h

# ID 生成器（Shared）
键:   tunnox:http_domain:next_id
值:   int64（自增计数器）
TTL:  永久
```

### 4.3 HybridStorage 前缀配置

需要更新 `internal/core/storage/hybrid/config.go` 的 `DefaultConfig()`：

```go
// SharedPersistentPrefixes 添加：
"tunnox:http_domain:mapping:", // HTTP 域名映射数据
"tunnox:http_domain:client:",  // 客户端映射列表

// SharedPrefixes 添加：
"tunnox:http_domain:index:",   // 域名索引（仅缓存，快速查找）
"tunnox:http_domain:next_id",  // ID 生成器
```

---

## 5. 关键实现点

### 5.1 缓存一致性保证

#### 写入一致性

采用 **Write-Through** 策略：
1. 先写 PostgreSQL（持久化优先）
2. 再写 Redis（缓存更新）
3. 任一步骤失败则整体失败并回滚

```go
func (r *HTTPDomainMappingRepo) CreateHTTPDomainMapping(
    clientID int64,
    targetHost string,
    targetPort int,
    subdomain, baseDomain, description string,
    ttlSeconds int,
) (mappingID, fullDomain, expiresAt string, err error) {
    fullDomain = subdomain + "." + baseDomain

    // 1. 生成 mappingID
    mappingID, err = r.generateMappingID()
    if err != nil {
        return "", "", "", coreerrors.Wrap(err, coreerrors.CodeStorageError,
            "failed to generate mapping ID")
    }

    // 2. 原子性获取域名（使用 SetNX）
    indexTTL := time.Duration(ttlSeconds) * time.Second
    if ttlSeconds <= 0 {
        indexTTL = 7 * 24 * time.Hour // 默认 7 天
    }

    acquired, err := r.tryAcquireDomain(fullDomain, mappingID, indexTTL)
    if err != nil {
        return "", "", "", coreerrors.Wrap(err, coreerrors.CodeStorageError,
            "failed to acquire domain")
    }
    if !acquired {
        return "", "", "", coreerrors.Newf(coreerrors.CodeAlreadyExists,
            "domain already in use: %s", fullDomain)
    }

    // 3. 构建映射对象
    var expiresAtTime *time.Time
    if ttlSeconds > 0 {
        t := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
        expiresAtTime = &t
        expiresAt = t.Format(time.RFC3339)
    }

    mapping := &HTTPDomainMapping{
        MappingID:   mappingID,
        ClientID:    clientID,
        Subdomain:   subdomain,
        BaseDomain:  baseDomain,
        FullDomain:  fullDomain,
        TargetHost:  targetHost,
        TargetPort:  targetPort,
        Description: description,
        CreatedAt:   time.Now(),
        ExpiresAt:   expiresAtTime,
        Status:      "active",
    }

    // 4. 保存映射数据（HybridStorage 自动处理 PostgreSQL + Redis）
    if err := r.SaveMapping(mapping); err != nil {
        // 回滚：删除域名索引
        _ = r.DeleteDomainIndex(fullDomain)
        return "", "", "", err
    }

    // 5. 添加到客户端映射列表
    if err := r.AddMappingToClient(clientID, mapping); err != nil {
        // 回滚
        _ = r.DeleteDomainIndex(fullDomain)
        _ = r.DeleteMapping(mappingID)
        return "", "", "", err
    }

    corelog.Infof("HTTPDomainMappingRepo: created mapping %s: %s -> %s:%d",
        mappingID, fullDomain, targetHost, targetPort)

    return mappingID, fullDomain, expiresAt, nil
}
```

#### 读取一致性

采用 **Cache-Aside** 策略（HybridStorage 已实现）：
1. 读取时先查 Redis
2. 缓存未命中则查 PostgreSQL
3. 查到后回填 Redis

#### 删除一致性

采用 **逆序删除** 策略：
1. 先删除域名索引（防止新请求命中）
2. 从客户端列表移除
3. 最后删除映射数据

### 5.2 并发安全

#### 域名占用检查

使用 Redis 的 `SetNX` 原子操作：

```go
func (r *HTTPDomainMappingRepo) tryAcquireDomain(
    fullDomain, mappingID string,
    ttl time.Duration,
) (bool, error) {
    indexKey := KeyPrefixHTTPDomainIndex + fullDomain

    // 使用 CAS 存储的 SetNX 方法
    if casStore, ok := r.storage.(storage.CASStore); ok {
        return casStore.SetNX(indexKey, mappingID, ttl)
    }

    // 回退：先检查后设置（非原子，仅用于测试）
    exists, err := r.storage.Exists(indexKey)
    if err != nil {
        return false, err
    }
    if exists {
        return false, nil
    }

    return true, r.storage.Set(indexKey, mappingID, ttl)
}
```

#### ID 生成

使用 Redis 的 `INCR` 原子操作生成全局唯一 ID：

```go
func (r *HTTPDomainMappingRepo) generateMappingID() (string, error) {
    // 使用 Counter 存储生成全局唯一 ID
    if counterStore, ok := r.storage.(storage.CounterStore); ok {
        id, err := counterStore.Incr(KeyHTTPDomainNextID)
        if err != nil {
            return "", err
        }
        return fmt.Sprintf("hdm_%d", id), nil
    }

    // 回退：使用时间戳 + 随机数
    return fmt.Sprintf("hdm_%d", time.Now().UnixNano()), nil
}
```

### 5.3 过期清理机制

#### 主动清理（定时任务）

在 Server 启动时启动清理协程：

```go
func (r *HTTPDomainMappingRepo) StartCleanupTask(
    ctx context.Context,
    interval time.Duration,
) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                count, err := r.CleanupExpiredMappings()
                if err != nil {
                    corelog.Errorf("HTTPDomainMappingRepo: cleanup failed: %v", err)
                } else if count > 0 {
                    corelog.Infof("HTTPDomainMappingRepo: cleaned up %d expired mappings",
                        count)
                }
            }
        }
    }()
}
```

#### 被动清理（访问时检查）

```go
func (r *HTTPDomainMappingRepo) GetMapping(mappingID string) (*HTTPDomainMapping, error) {
    mapping, err := r.Get(mappingID, KeyPrefixHTTPDomainMapping)
    if err != nil {
        return nil, err
    }

    // 检查是否过期
    if mapping.ExpiresAt != nil && time.Now().After(*mapping.ExpiresAt) {
        // 异步删除过期映射
        go func() {
            _ = r.DeleteMapping(mappingID)
            _ = r.DeleteDomainIndex(mapping.FullDomain)
        }()
        return nil, coreerrors.New(coreerrors.CodeNotFound, "mapping expired")
    }

    return mapping, nil
}
```

### 5.4 错误处理

```go
// 错误类型定义
var (
    ErrDomainAlreadyExists = coreerrors.New(coreerrors.CodeAlreadyExists,
        "domain already exists")
    ErrDomainNotAllowed    = coreerrors.New(coreerrors.CodePermissionDenied,
        "base domain not allowed")
    ErrMappingNotFound     = coreerrors.New(coreerrors.CodeNotFound,
        "mapping not found")
    ErrMappingExpired      = coreerrors.New(coreerrors.CodeNotFound,
        "mapping expired")
    ErrPermissionDenied    = coreerrors.New(coreerrors.CodePermissionDenied,
        "permission denied")
)
```

---

## 6. Repository 实现

### 6.1 存储键前缀

文件位置：`internal/constants/storage_keys.go`（新增）

```go
// HTTP 域名映射相关键前缀
const (
    // KeyPrefixHTTPDomainMapping HTTP 域名映射数据
    // 格式：tunnox:http_domain:mapping:{mapping_id}
    // 存储：SharedPersistent（Redis + PostgreSQL）
    KeyPrefixHTTPDomainMapping = "tunnox:http_domain:mapping:"

    // KeyPrefixHTTPDomainIndex HTTP 域名索引
    // 格式：tunnox:http_domain:index:{full_domain}
    // 存储：Shared（仅 Redis）
    KeyPrefixHTTPDomainIndex = "tunnox:http_domain:index:"

    // KeyPrefixHTTPDomainClient 客户端的 HTTP 域名映射列表
    // 格式：tunnox:http_domain:client:{client_id}
    // 存储：SharedPersistent（Redis + PostgreSQL）
    KeyPrefixHTTPDomainClient = "tunnox:http_domain:client:"

    // KeyHTTPDomainNextID HTTP 域名映射 ID 生成器
    // 格式：tunnox:http_domain:next_id
    // 存储：Shared（仅 Redis）
    KeyHTTPDomainNextID = "tunnox:http_domain:next_id"
)
```

### 6.2 HTTPDomainMappingRepo 实现

文件位置：`internal/cloud/repos/http_domain_mapping_repository.go`

```go
package repos

import (
    "encoding/json"
    "fmt"
    "time"

    "tunnox-core/internal/command"
    coreerrors "tunnox-core/internal/core/errors"
    corelog "tunnox-core/internal/core/log"
    "tunnox-core/internal/core/storage"
)

// 编译时接口断言
var _ DomainRegistry = (*HTTPDomainMappingRepo)(nil)
var _ IHTTPDomainMappingRepository = (*HTTPDomainMappingRepo)(nil)

// HTTPDomainMappingRepo HTTP 域名映射 Repository
type HTTPDomainMappingRepo struct {
    *GenericRepositoryImpl[*HTTPDomainMapping]
    baseDomains []string // 允许的基础域名列表
}

// NewHTTPDomainMappingRepo 创建 HTTP 域名映射 Repository
func NewHTTPDomainMappingRepo(repo *Repository, baseDomains []string) *HTTPDomainMappingRepo {
    if len(baseDomains) == 0 {
        baseDomains = []string{"tunnox.net"} // 默认域名
    }

    genericRepo := NewGenericRepository[*HTTPDomainMapping](repo,
        func(m *HTTPDomainMapping) (string, error) {
            return m.MappingID, nil
        })

    return &HTTPDomainMappingRepo{
        GenericRepositoryImpl: genericRepo,
        baseDomains:           baseDomains,
    }
}

// === DomainRegistry 接口实现 ===

// IsSubdomainAvailable 检查子域名是否可用
func (r *HTTPDomainMappingRepo) IsSubdomainAvailable(subdomain, baseDomain string) bool {
    fullDomain := subdomain + "." + baseDomain
    exists, err := r.DomainIndexExists(fullDomain)
    if err != nil {
        corelog.Warnf("HTTPDomainMappingRepo: IsSubdomainAvailable error: %v", err)
        return false // 出错时保守返回不可用
    }
    return !exists
}

// IsBaseDomainAllowed 检查基础域名是否允许
func (r *HTTPDomainMappingRepo) IsBaseDomainAllowed(baseDomain string) bool {
    for _, d := range r.baseDomains {
        if d == baseDomain {
            return true
        }
    }
    return len(r.baseDomains) == 0 // 空列表允许所有
}

// ListHTTPDomainMappings 列出客户端的 HTTP 域名映射
func (r *HTTPDomainMappingRepo) ListHTTPDomainMappings(
    clientID int64,
) ([]command.HTTPDomainMappingInfo, error) {
    mappings, err := r.GetClientMappings(clientID)
    if err != nil {
        return nil, err
    }

    result := make([]command.HTTPDomainMappingInfo, 0, len(mappings))
    for _, m := range mappings {
        // 过滤已过期的映射
        if m.ExpiresAt != nil && time.Now().After(*m.ExpiresAt) {
            continue
        }

        expiresAt := ""
        if m.ExpiresAt != nil {
            expiresAt = m.ExpiresAt.Format(time.RFC3339)
        }

        result = append(result, command.HTTPDomainMappingInfo{
            MappingID:  m.MappingID,
            FullDomain: m.FullDomain,
            TargetURL:  fmt.Sprintf("http://%s:%d", m.TargetHost, m.TargetPort),
            Status:     m.Status,
            CreatedAt:  m.CreatedAt.Format(time.RFC3339),
            ExpiresAt:  expiresAt,
        })
    }

    return result, nil
}

// DeleteHTTPDomainMapping 删除 HTTP 域名映射
func (r *HTTPDomainMappingRepo) DeleteHTTPDomainMapping(
    clientID int64,
    mappingID string,
) error {
    // 1. 获取映射信息
    mapping, err := r.GetMapping(mappingID)
    if err != nil {
        return err
    }

    // 2. 验证权限
    if mapping.ClientID != clientID {
        return coreerrors.New(coreerrors.CodePermissionDenied,
            "permission denied: mapping belongs to another client")
    }

    // 3. 删除域名索引（先删索引，阻止新请求）
    if err := r.DeleteDomainIndex(mapping.FullDomain); err != nil {
        corelog.Warnf("HTTPDomainMappingRepo: failed to delete domain index: %v", err)
    }

    // 4. 从客户端列表移除
    if err := r.RemoveMappingFromClient(clientID, mappingID); err != nil {
        corelog.Warnf("HTTPDomainMappingRepo: failed to remove from client list: %v", err)
    }

    // 5. 删除映射数据
    if err := r.DeleteMapping(mappingID); err != nil {
        return err
    }

    corelog.Infof("HTTPDomainMappingRepo: deleted mapping %s", mappingID)
    return nil
}

// GetMappingByDomain 根据域名获取映射（用于 HTTP 代理）
func (r *HTTPDomainMappingRepo) GetMappingByDomain(
    fullDomain string,
) (*HTTPDomainMapping, error) {
    // 1. 从索引获取 mappingID
    indexKey := KeyPrefixHTTPDomainIndex + fullDomain
    value, err := r.storage.Get(indexKey)
    if err != nil {
        return nil, coreerrors.Newf(coreerrors.CodeNotFound,
            "domain not found: %s", fullDomain)
    }

    mappingID, ok := value.(string)
    if !ok {
        return nil, coreerrors.New(coreerrors.CodeStorageError,
            "invalid index value type")
    }

    // 2. 获取映射详情
    return r.GetMapping(mappingID)
}

// GetMapping 根据映射ID获取映射
func (r *HTTPDomainMappingRepo) GetMapping(mappingID string) (*HTTPDomainMapping, error) {
    mapping, err := r.Get(mappingID, KeyPrefixHTTPDomainMapping)
    if err != nil {
        return nil, err
    }

    // 检查是否过期
    if mapping.ExpiresAt != nil && time.Now().After(*mapping.ExpiresAt) {
        // 异步清理过期映射
        go func() {
            _ = r.DeleteMapping(mappingID)
            _ = r.DeleteDomainIndex(mapping.FullDomain)
        }()
        return nil, coreerrors.New(coreerrors.CodeNotFound, "mapping expired")
    }

    return mapping, nil
}

// === 内部方法 ===

func (r *HTTPDomainMappingRepo) SaveMapping(mapping *HTTPDomainMapping) error {
    return r.Save(mapping, KeyPrefixHTTPDomainMapping, 0)
}

func (r *HTTPDomainMappingRepo) DeleteMapping(mappingID string) error {
    return r.Delete(mappingID, KeyPrefixHTTPDomainMapping)
}

func (r *HTTPDomainMappingRepo) DomainIndexExists(fullDomain string) (bool, error) {
    indexKey := KeyPrefixHTTPDomainIndex + fullDomain
    return r.storage.Exists(indexKey)
}

func (r *HTTPDomainMappingRepo) SetDomainIndex(
    fullDomain, mappingID string,
    ttl time.Duration,
) error {
    indexKey := KeyPrefixHTTPDomainIndex + fullDomain
    return r.storage.Set(indexKey, mappingID, ttl)
}

func (r *HTTPDomainMappingRepo) DeleteDomainIndex(fullDomain string) error {
    indexKey := KeyPrefixHTTPDomainIndex + fullDomain
    return r.storage.Delete(indexKey)
}

func (r *HTTPDomainMappingRepo) tryAcquireDomain(
    fullDomain, mappingID string,
    ttl time.Duration,
) (bool, error) {
    indexKey := KeyPrefixHTTPDomainIndex + fullDomain

    if casStore, ok := r.storage.(storage.CASStore); ok {
        return casStore.SetNX(indexKey, mappingID, ttl)
    }

    // 回退：先检查后设置
    exists, err := r.storage.Exists(indexKey)
    if err != nil {
        return false, err
    }
    if exists {
        return false, nil
    }

    return true, r.storage.Set(indexKey, mappingID, ttl)
}

func (r *HTTPDomainMappingRepo) generateMappingID() (string, error) {
    if counterStore, ok := r.storage.(storage.CounterStore); ok {
        id, err := counterStore.Incr(KeyHTTPDomainNextID)
        if err != nil {
            return "", err
        }
        return fmt.Sprintf("hdm_%d", id), nil
    }

    return fmt.Sprintf("hdm_%d", time.Now().UnixNano()), nil
}

func (r *HTTPDomainMappingRepo) GetClientMappings(
    clientID int64,
) ([]*HTTPDomainMapping, error) {
    listKey := fmt.Sprintf("%s%d", KeyPrefixHTTPDomainClient, clientID)
    return r.List(listKey)
}

func (r *HTTPDomainMappingRepo) AddMappingToClient(
    clientID int64,
    mapping *HTTPDomainMapping,
) error {
    listKey := fmt.Sprintf("%s%d", KeyPrefixHTTPDomainClient, clientID)
    return r.AddToList(mapping, listKey)
}

func (r *HTTPDomainMappingRepo) RemoveMappingFromClient(
    clientID int64,
    mappingID string,
) error {
    mappings, err := r.GetClientMappings(clientID)
    if err != nil {
        return err
    }

    var filtered []*HTTPDomainMapping
    for _, m := range mappings {
        if m.MappingID != mappingID {
            filtered = append(filtered, m)
        }
    }

    listKey := fmt.Sprintf("%s%d", KeyPrefixHTTPDomainClient, clientID)
    _ = r.storage.Delete(listKey)

    for _, m := range filtered {
        if err := r.AddMappingToClient(clientID, m); err != nil {
            return err
        }
    }

    return nil
}

// CleanupExpiredMappings 清理过期映射
func (r *HTTPDomainMappingRepo) CleanupExpiredMappings() (int, error) {
    // 此方法需要遍历所有映射，在大规模场景下应该由 PostgreSQL 定时任务完成
    // TODO: 实现基于持久化存储的批量过期清理
    return 0, nil
}
```

---

## 7. 迁移方案

### 7.1 迁移步骤

**阶段 1：添加新实现（不破坏现有功能）**

1. 创建 `HTTPDomainMapping` 数据模型（可移至 `internal/cloud/models/http_domain.go`）
2. 创建 `HTTPDomainMappingRepo` 实现
3. 更新 `HybridStorage` 前缀配置
4. 添加 constants 定义

**阶段 2：修改 HTTPDomainCommandHandlers**

修改构造函数，注入新的 Repository：

```go
// 修改前
func NewHTTPDomainCommandHandlers(
    sessionMgr *session.SessionManager,
    baseDomains []string,
) *HTTPDomainCommandHandlers {
    return &HTTPDomainCommandHandlers{
        sessionMgr:     sessionMgr,
        baseDomains:    baseDomains,
        domainRegistry: NewInMemoryDomainRegistry(), // 旧实现
    }
}

// 修改后
func NewHTTPDomainCommandHandlers(
    sessionMgr *session.SessionManager,
    baseDomains []string,
    storage storage.Storage,
) *HTTPDomainCommandHandlers {
    repo := repos.NewRepository(storage)
    domainRegistry := repos.NewHTTPDomainMappingRepo(repo, baseDomains)

    return &HTTPDomainCommandHandlers{
        sessionMgr:     sessionMgr,
        baseDomains:    baseDomains,
        domainRegistry: domainRegistry, // 新实现
    }
}
```

**阶段 3：清理**

1. 删除 `InMemoryDomainRegistry` 类型
2. 删除相关的内存实现代码

### 7.2 向后兼容

#### 接口兼容

新的 `HTTPDomainMappingRepo` 实现了所有现有接口：
- `command.SubdomainChecker`
- `command.HTTPDomainCreator`
- `command.HTTPDomainLister`
- `command.HTTPDomainDeleter`

可直接替换，无需修改 command handler 代码。

#### 数据迁移

由于 `InMemoryDomainRegistry` 不持久化，重启后数据自然丢失。新实现启动后，用户需要重新创建域名映射。

如果需要热迁移（保留运行时数据），可以：
1. 在 `InMemoryDomainRegistry` 中导出所有映射
2. 调用 `HTTPDomainMappingRepo.CreateHTTPDomainMapping` 批量导入

---

## 8. 配置变更

### 8.1 HybridStorage 配置更新

文件：`internal/core/storage/hybrid/config.go`

```go
func DefaultConfig() *Config {
    return &Config{
        // ... 现有配置 ...

        SharedPersistentPrefixes: []string{
            // ... 现有前缀 ...
            "tunnox:http_domain:mapping:", // HTTP 域名映射数据
            "tunnox:http_domain:client:",  // 客户端映射列表
        },
        SharedPrefixes: []string{
            // ... 现有前缀 ...
            "tunnox:http_domain:index:",   // 域名索引（仅缓存）
            "tunnox:http_domain:next_id",  // ID 生成器
        },
    }
}
```

---

## 9. 测试要点

### 9.1 单元测试

- `TestIsSubdomainAvailable`: 测试子域名可用性检查
- `TestCreateHTTPDomainMapping`: 测试创建流程
- `TestCreateHTTPDomainMapping_Duplicate`: 测试重复域名创建
- `TestListHTTPDomainMappings`: 测试列表查询
- `TestDeleteHTTPDomainMapping`: 测试删除流程
- `TestDeleteHTTPDomainMapping_PermissionDenied`: 测试权限检查
- `TestGetMappingByDomain`: 测试按域名查找
- `TestGetMappingByDomain_Expired`: 测试过期处理
- `TestConcurrentCreate`: 测试并发创建同一域名

### 9.2 集成测试

- 测试 Redis 故障时的行为
- 测试 PostgreSQL 故障时的行为
- 测试多节点环境下的数据一致性
- 测试大量并发创建的性能

---

## 10. 实现工作量估计

| 任务 | 工时 |
|------|------|
| Repository 实现 | 2 天 |
| HybridStorage 配置更新 | 0.5 天 |
| HTTPDomainCommandHandlers 重构 | 1 天 |
| 单元测试 | 1.5 天 |
| 集成测试 | 1 天 |
| **总计** | **6 天** |

---

## 11. 总结

本技术方案通过复用现有的 `HybridStorage` + `Repository` 架构，实现了 HTTP 域名映射的持久化存储，具有以下优点：

1. **架构一致性**：遵循项目既有的分层架构和设计模式
2. **高可用性**：Redis 缓存 + PostgreSQL 持久化，支持多节点部署
3. **高性能**：域名查找 O(1) 复杂度，采用热点缓存模式
4. **兼容性好**：完全兼容现有 command handler 接口
5. **可维护性强**：代码结构清晰，职责分离

---

**文档结束**
