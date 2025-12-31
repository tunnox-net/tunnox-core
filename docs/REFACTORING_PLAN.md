# Tunnox-Core 阶段性重构计划

> 本文档由 AI 团队三轮 Code Review 生成
> 生成时间：2025-12-30

---

## 一、Review 概览

| 轮次 | 审查人 | 审查范围 | 发现问题数 |
|------|--------|----------|------------|
| 第一轮 | 通信架构师 | 架构设计、模块划分、依赖关系 | 12 |
| 第二轮 | 开发工程师 | 代码规范、错误处理、类型安全 | 28 |
| 第三轮 | QA 工程师 | 测试覆盖、测试质量、边界条件 | 35 |

---

## 二、问题汇总

### 2.1 严重问题 (Blocker/High) - 共 15 个

| ID | 类别 | 问题 | 位置 | 影响 | 状态 |
|----|------|------|------|------|------|
| H-01 | 架构 | 上帝包：protocol/session 10928 行 | `internal/protocol/session/` | 维护困难 | 待处理 |
| H-02 | 架构 | 上帝包：client 13746 行 | `internal/client/` | 维护困难 | 待处理 |
| H-03 | 架构 | 上帝包：cloud/services 4326 行 | `internal/cloud/services/` | 维护困难 | 已拆分子包 |
| H-04 | 架构 | 上帝包：core/storage 3909 行 | `internal/core/storage/` | 维护困难 | 已拆分子包 |
| ~~H-05~~ | ~~类型安全~~ | ~~Storage 接口使用 interface{}~~ | ~~`internal/core/storage/interface.go`~~ | ~~类型不安全~~ | **[已修复]** 已使用泛型 TypedStorage[T] |
| ~~H-06~~ | ~~类型安全~~ | ~~httpservice 使用 map[string]interface{}~~ | ~~`internal/httpservice/`~~ | ~~类型不安全~~ | **[已修复]** 已使用泛型 APIResponse[T] |
| ~~H-07~~ | ~~类型安全~~ | ~~errors.Details 使用 interface{}~~ | ~~`internal/core/errors/errors.go:77`~~ | ~~类型不安全~~ | **[已修复]** |
| ~~H-08~~ | ~~类型安全~~ | ~~command/utils 使用 interface{}~~ | ~~`internal/command/utils.go:16-17`~~ | ~~类型不安全~~ | **[已修复]** 已使用泛型 TypedCommandUtils |
| ~~H-09~~ | ~~测试~~ | ~~hybrid_storage 测试失败~~ | ~~`internal/core/storage/hybrid_storage_test.go`~~ | ~~CI 中断~~ | **[已修复]** 测试全部通过 |
| ~~H-10~~ | ~~测试~~ | ~~bridge.go (623行) 完全无测试~~ | ~~`internal/protocol/session/tunnel/`~~ | ~~高风险~~ | **[已修复]** 覆盖率 84.9% |
| ~~H-11~~ | ~~测试~~ | ~~domainproxy 模块 (507行) 无测试~~ | ~~`internal/httpservice/modules/domainproxy/`~~ | ~~高风险~~ | **[已修复]** 覆盖率 70.8% |
| H-12 | 测试 | 核心会话管理仅 23.5% 覆盖 | `internal/protocol/session/` | 高风险 | 待提升 |
| H-13 | 测试 | 云服务层仅 16.7% 覆盖 | `internal/cloud/services/` | 中风险 | 待提升 |
| H-14 | 测试 | 客户端核心仅 13.4% 覆盖 | `internal/client/` | 中风险 | 待提升 |
| ~~H-15~~ | ~~架构~~ | ~~QUIC 适配器未正确实现接口~~ | ~~`internal/protocol/adapter/quic_adapter.go`~~ | ~~设计不一致~~ | **[已验证]** 接口已正确实现 |

### 2.2 中等问题 (Medium) - 共 18 个

| ID | 类别 | 问题 | 位置 | 状态 |
|----|------|------|------|------|
| M-01 | 架构 | 错误处理使用字符串匹配 | `internal/protocol/adapter/adapter.go:261-273` | 待处理 |
| M-02 | 架构 | bridge.go 超过 500 行 | `internal/protocol/session/tunnel/bridge.go` | 已拆分为多文件 |
| M-03 | 架构 | handleConnection 函数过长 (~120行) | `internal/protocol/adapter/adapter.go:162-278` | 待处理 |
| ~~M-04~~ | ~~架构~~ | ~~Legacy 代码未清理~~ | ~~`internal/protocol/session/tunnel/bridge.go`~~ | **[已完成]** 代码已清理 |
| M-05 | 规范 | domainproxy/module.go 超 500 行 | `internal/httpservice/modules/domainproxy/` | 已拆分为多文件 |
| ~~M-06~~ | ~~规范~~ | ~~大量使用 fmt.Errorf 而非 coreerrors~~ | ~~`internal/protocol/session/` 等~~ | **[已完成]** session 目录已迁移 |
| ~~M-07~~ | ~~规范~~ | ~~忽略错误返回值~~ | ~~`internal/client/keepalive_conn.go:19-21`~~ | **[已修复]** 已记录 debug 日志 |
| ~~M-08~~ | ~~规范~~ | ~~忽略错误返回值~~ | ~~`internal/protocol/session/cross_node_session.go:227,230`~~ | **[已修复]** 已记录 debug 日志 |
| M-09 | 规范 | 忽略错误返回值 | `internal/cloud/services/client_service_crud.go:196,199` | 文件已重构 |
| ~~M-10~~ | ~~规范~~ | ~~TCP/UDP target_handler 代码重复~~ | ~~`internal/client/target_handler.go`~~ | **[已完成]** 使用泛型提取公共逻辑 |
| ~~M-11~~ | ~~规范~~ | ~~跨节点转发逻辑重复~~ | ~~`cross_node_session.go` + `cross_node_listener.go`~~ | **[已完成]** 提取 runBidirectionalForward |
| M-12 | 测试 | 协议适配器仅 29.7% 覆盖 | `internal/protocol/adapter/` | 当前 30.4% |
| ~~M-13~~ | ~~测试~~ | ~~流处理仅 37.6% 覆盖~~ | ~~`internal/stream/`~~ | **[已提升]** 当前 69.9% |
| ~~M-14~~ | ~~测试~~ | ~~utils/iocopy 无测试 (456行)~~ | ~~`internal/utils/iocopy/`~~ | **[已修复]** 覆盖率 87.7% |
| ~~M-15~~ | ~~测试~~ | ~~cloud/stats 无测试 (430行)~~ | ~~`internal/cloud/stats/`~~ | **[已完成]** 覆盖率 82.4% |
| ~~M-16~~ | ~~测试~~ | ~~packet/builder, parser, validator 无测试~~ | ~~`internal/packet/`~~ | **[已修复]** builder 85%, parser/validator 100% |
| M-17 | 测试 | 边界条件覆盖不足 | 多处 | 持续改进中 |
| M-18 | 测试 | Mock 使用不完整 | 多处 | 待处理 |

### 2.3 低优先级问题 (Low) - 共 12 个

| ID | 类别 | 问题 | 位置 | 状态 |
|----|------|------|------|------|
| L-01 | 架构 | context.Background() 作为后备 | `internal/command/base_handler.go:190` | 可接受 |
| L-02 | 架构 | 缺少 WebSocket 适配器文件 | `internal/protocol/adapter/` | 待处理 |
| ~~L-03~~ | ~~规范~~ | ~~魔法数字未提取为常量~~ | ~~`internal/protocol/session/manager.go:35-38`~~ | **[已修复]** 已定义常量 |
| ~~L-04~~ | ~~规范~~ | ~~魔法数字未提取为常量~~ | ~~`internal/security/brute_force_protector.go:59`~~ | **[已修复]** 已定义常量 |
| L-05 | 规范 | processor.go 使用 interface{} 注释说明兼容性 | `internal/stream/processor/processor.go:50` | 可接受 |
| L-06 | 测试 | 未使用 t.Parallel() 并行测试 | 全部测试文件 | 低优先级 |
| L-07 | 测试 | 表驱动测试使用较少 (仅5个) | 多处 | 低优先级 |
| L-08 | 测试 | Benchmark 测试覆盖不足 | 多处 | 低优先级 |
| L-09 | 测试 | 测试命名不够清晰 | 部分测试函数 | 低优先级 |
| L-10 | 测试 | 跳过的测试（Redis依赖） | 多处 | 可接受 |
| L-11 | 测试 | 跳过的测试（时间敏感） | `auto_connector_test.go` | 可接受 |
| L-12 | 规范 | CLI 模块使用 fmt.Print (可接受) | `internal/client/cli/` | 可接受 |

---

## 三、重构计划

### Phase 1: 紧急修复 (P0)

**目标**: 修复阻塞问题，恢复 CI 稳定性

| 任务 | 优先级 | 工作量 | 负责 |
|------|--------|--------|------|
| 修复 hybrid_storage_test.go 测试失败 | P0 | 2h | Dev |
| 为 bridge.go 添加基础单元测试 | P0 | 4h | QA |
| 为 domainproxy 添加基础单元测试 | P0 | 4h | QA |

### Phase 2: 类型安全重构 (P1)

**目标**: 消除 interface{} 使用，提升类型安全

| 任务 | 优先级 | 工作量 | 负责 |
|------|--------|--------|------|
| 重构 Storage 接口使用泛型 | P1 | 8h | Arch |
| 重构 httpservice 响应类型 | P1 | 4h | Dev |
| 重构 errors.Details 为强类型 | P1 | 2h | Dev |
| 重构 command/utils 参数类型 | P1 | 2h | Dev |
| 重构 core/types/cloud.go 返回类型 | P1 | 2h | Dev |

### Phase 3: 上帝包拆分 (P1)

**目标**: 将大包拆分为职责单一的子包

#### 3.1 protocol/session 拆分 (10928 行 → 6 个子包)

```
internal/protocol/session/
├── connection/      # 连接管理 (~1500行)
│   ├── manager.go
│   ├── lifecycle.go
│   └── state_store.go
├── handler/         # 数据包处理 (~1500行)
│   ├── handshake.go
│   ├── tunnel.go
│   └── command.go
├── tunnel/          # 隧道管理 (已存在，需整理)
│   ├── bridge.go
│   └── migration.go
├── crossnode/       # 跨节点 (已存在)
├── http/            # HTTP 代理 (~500行)
│   └── proxy.go
└── session.go       # 主入口 (~500行)
```

#### 3.2 cloud/services 拆分 (4326 行 → 5 个子包)

```
internal/cloud/services/
├── client/          # 客户端服务 (~1000行)
├── mapping/         # 映射服务 (~800行)
├── auth/            # 认证服务 (~600行)
├── connection/      # 连接码服务 (~600行)
└── registry/        # 服务注册 (~500行)
```

#### 3.3 core/storage 拆分 (3909 行 → 4 个子包)

```
internal/core/storage/
├── memory/          # 内存存储
├── redis/           # Redis 存储
├── remote/          # gRPC 远程存储
├── hybrid/          # 混合存储
└── interface.go     # 接口定义
```

### Phase 4: 错误处理统一 (P2)

**目标**: 将 fmt.Errorf 迁移到 coreerrors

| 模块 | 预估修改点 | 工作量 |
|------|------------|--------|
| internal/protocol/session/ | ~100 处 | 4h |
| internal/client/ | ~150 处 | 6h |
| internal/cloud/services/ | ~50 处 | 2h |
| internal/command/ | ~30 处 | 1h |

### Phase 5: 测试覆盖提升 (P2)

**目标**: 核心模块覆盖率 70%+

| 模块 | 当前覆盖率 | 目标覆盖率 | 工作量 |
|------|------------|------------|--------|
| internal/protocol/session/ | 23.5% | 70% | 16h |
| internal/cloud/services/ | 16.7% | 70% | 12h |
| internal/client/ | 13.4% | 60% | 12h |
| internal/protocol/adapter/ | 29.7% | 70% | 8h |
| internal/stream/ | 37.6% | 80% | 6h |

### Phase 6: 代码清理 (P3)

| 任务 | 工作量 |
|------|--------|
| 删除 bridge.go 中的 Legacy 方法 | 2h |
| 提取魔法数字为常量 | 2h |
| 拆分超长函数 (handleConnection 等) | 4h |
| 处理被忽略的错误返回值 | 2h |
| 消除 target_handler.go 代码重复 | 4h |

---

## 四、实施时间线

```
Week 1: Phase 1 (紧急修复)
  - Day 1-2: 修复测试失败
  - Day 3-5: 添加核心模块基础测试

Week 2-3: Phase 2 (类型安全)
  - Storage 接口泛型重构
  - httpservice 响应类型重构

Week 4-6: Phase 3 (上帝包拆分)
  - protocol/session 拆分
  - cloud/services 拆分
  - core/storage 拆分

Week 7-8: Phase 4+5 (错误处理 + 测试)
  - fmt.Errorf → coreerrors 迁移
  - 测试覆盖率提升

Week 9: Phase 6 (代码清理)
  - Legacy 代码删除
  - 代码规范整理
```

---

## 五、重构原则

### 5.1 不破坏现有功能
- 每次重构后运行完整测试
- 小步提交，便于回滚
- 保持 API 兼容性

### 5.2 渐进式重构
- 先添加测试，再重构
- 先重构基础层，再重构业务层
- 避免同时修改多个模块

### 5.3 代码规范
- 单个文件 < 500 行
- 单个函数 < 100 行
- 单个包 < 2000 行
- 禁止使用 interface{}、any
- 使用 coreerrors 包处理错误

---

## 六、风险评估

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| 重构引入新 bug | 中 | 高 | 先补充测试，小步重构 |
| 重构周期过长 | 中 | 中 | 按优先级分阶段实施 |
| 团队资源不足 | 低 | 中 | 可调整优先级，延后 P3 任务 |
| API 不兼容 | 低 | 高 | 保持接口稳定，内部重构 |

---

## 七、验收标准

### Phase 1 完成标准
- [x] 所有测试通过 ✅ **2025-12-30 完成**
- [x] bridge.go 测试覆盖率 > 50% ✅ **实际达到 84.9%**
- [x] domainproxy 测试覆盖率 > 50% ✅ **实际达到 70.8%**

### Phase 2 完成标准
- [x] Storage 接口无 interface{} ✅ **已使用泛型 TypedStorage[T]**
- [x] httpservice 无 map[string]interface{} ✅ **已使用泛型 APIResponse[T]**
- [x] 编译无警告 ✅ **2025-12-30 验证通过**

### Phase 3 完成标准
- [ ] protocol/session 包 < 2000 行
- [ ] cloud/services 包 < 2000 行
- [ ] core/storage 包 < 2000 行

### Phase 4+5 完成标准
- [ ] 核心模块无 fmt.Errorf
- [ ] 核心模块测试覆盖率 > 60%

### Phase 6 完成标准
- [ ] 无 Legacy 后缀的方法
- [ ] 无超过 500 行的文件
- [ ] 无超过 100 行的函数

---

## 八、附录

### A. 代码行数统计

| 包 | 当前行数 | 目标行数 |
|----|----------|----------|
| internal/client/ | 13746 | < 8000 (已有子包) |
| internal/protocol/ | 12787 | < 8000 |
| internal/cloud/ | 11732 | < 8000 |
| internal/core/ | 7183 | < 5000 |
| internal/protocol/session/ | 10928 | < 2000 |
| internal/cloud/services/ | 4326 | < 2000 |
| internal/core/storage/ | 3909 | < 2000 |

### B. 测试覆盖率目标

| 模块 | 当前 | 目标 |
|------|------|------|
| internal/protocol/session/ | 23.5% | 70% |
| internal/cloud/services/ | 16.7% | 70% |
| internal/client/ | 13.4% | 60% |
| internal/cloud/managers/ | 13.4% | 60% |
| 总体 | ~30% | 50%+ |

### C. interface{} 使用位置清单

1. `internal/core/storage/interface.go` - 所有存储方法
2. ~~`internal/core/errors/errors.go:77` - Details 字段~~ **[已修复]** 已重构为强类型 `map[string]DetailValue`，`DetailValue` 支持字符串和整数两种类型
3. ~~`internal/core/types/cloud.go:40,46` - HandleAuth/HandleTunnelOpen 返回值~~ **[已修复]** 接口已使用强类型返回值 (`*packet.HandshakeResponse`, `error`)
4. ~~`internal/command/utils.go:16-17` - requestData/responseData~~ **[已修复]** 已废弃 `CommandUtils`，新代码使用泛型 `TypedCommandUtils[TReq, TResp]`
5. `internal/broker/factory.go:26` - NATS 字段
6. `internal/httpservice/module.go:79,82` - GetNetConn/GetStream
7. `internal/httpservice/server.go:246,255` - JSON 响应
8. `internal/httpservice/middleware.go:15,128` - Data 字段和 respondJSON

---

*文档结束*
