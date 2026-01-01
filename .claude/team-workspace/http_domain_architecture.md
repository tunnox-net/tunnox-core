# HTTP 域名映射持久化存储技术方案

## 1. 问题分析

### 1.1 当前实现的问题

当前 `InMemoryDomainRegistry` 存在以下问题：

1. **单点故障**：数据仅存储在单个 pod 内存中，pod 重启后数据丢失
2. **无法集群共享**：多节点部署时，各节点数据不同步
3. **与现有存储体系脱节**：未使用项目的 Repository + HybridStorage 架构
4. **缺乏持久化**：无法满足生产环境的数据可靠性要求

### 1.2 设计目标

1. 复用现有的 `HybridStorage` + `Repository` 分层架构
2. 支持 PostgreSQL 持久化 + Redis 跨节点缓存
3. 保持与现有 command handler 接口的兼容性
4. 实现高性能的域名查找（O(1) 复杂度）

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     Command Handlers                             │
│  (HTTPDomainCreateHandler, HTTPDomainListHandler, etc.)         │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                   DomainRegistry (新接口)                        │
│  - SubdomainChecker (检查子域名可用性)                           │
│  - HTTPDomainCreator (创建域名映射)                              │
│  - HTTPDomainLister  (列出域名映射)                              │
│  - HTTPDomainDeleter (删除域名映射)                              │
│  - DomainLookup      (按域名查找映射)                            │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│              HTTPDomainMappingRepository                         │
│  - 基于 GenericRepositoryImpl[*HTTPDomainMapping]               │
│  - 实现 DomainRegistry 接口                                      │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                     HybridStorage                                │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Local Cache   │  │  Shared Cache   │  │   Persistent    │  │
│  │    (Memory)     │  │    (Redis)      │  │  (PostgreSQL)   │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 数据分类策略

HTTP 域名映射属于 **SharedPersistent** 类型数据：
- 需要跨节点共享（任何节点都可能收到该域名的 HTTP 请求）
- 需要持久化（重启后不丢失）

### 2.3 Redis 缓存键设计

```
# 映射数据（SharedPersistent）
tunnox:http_domain:mapping:{mappingID}  → HTTPDomainMapping JSON

# 域名索引（Shared，仅 Redis，用于 O(1) 快速查找）
tunnox:http_domain:index:{fullDomain}   → mappingID

# 客户端映射列表（SharedPersistent）
tunnox:http_domain:client:{clientID}    → [mappingID1, mappingID2, ...]

# ID 生成器（Shared）
tunnox:http_domain:next_id              → int64
```

## 3. 开发任务拆分

### T001: 创建 HTTPDomainMapping 数据模型
- 文件: `internal/cloud/repos/http_domain_mapping.go`
- 内容: 定义 HTTPDomainMapping 结构体和存储键常量
- 依赖: 无

### T002: 更新 HybridStorage 前缀配置
- 文件: `internal/core/storage/hybrid/config.go`
- 内容: 添加 HTTP 域名相关的 SharedPersistent 和 Shared 前缀
- 依赖: 无

### T003: 实现 HTTPDomainMappingRepo
- 文件: `internal/cloud/repos/http_domain_mapping_repository.go`
- 内容: 实现 DomainRegistry 接口的所有方法
- 依赖: T001, T002

### T004: 重构 HTTPDomainCommandHandlers
- 文件: `internal/app/server/http_domain_command_handlers.go`
- 内容: 使用新的 HTTPDomainMappingRepo 替代 InMemoryDomainRegistry
- 依赖: T003

### T005: 更新 DomainProxy 模块
- 文件: `internal/httpservice/modules/domainproxy/module.go`
- 内容: 使用新的持久化 DomainRegistry 进行域名查找
- 依赖: T003

### T006: 添加单元测试
- 文件: `internal/cloud/repos/http_domain_mapping_repository_test.go`
- 内容: 测试所有 Repository 方法
- 依赖: T003

### T007: 集成测试和 K8s 验证
- 内容: 在云端 K8s 环境验证集群功能
- 依赖: T004, T005
