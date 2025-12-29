# 配置系统设计方案 - 评审汇总

**日期**: 2025-12-29
**评审文档**: architecture.md, config-schema.md, implementation-plan.md

---

## 评审结论汇总

| 角色 | 结论 | 核心问题 |
|------|------|---------|
| 架构师 | 有问题 | Dispose模式、类型安全、遗漏配置项 |
| 产品经理 | 有问题 | HTTP代理门槛高、端口冲突 |
| 开发工程师 | 有问题 | 环境变量兼容、工期偏乐观 |
| QA工程师 | 有问题 | 测试覆盖不完整、验收标准不可执行 |

---

## 必须修改项 (P0)

### 1. ConfigManager 遵循 Dispose 模式
**来源**: 架构师
```go
type Manager struct {
    *dispose.ServiceBase  // 嵌入 ServiceBase
    // ...
}
```

### 2. Source 接口避免弱类型
**来源**: 架构师
```go
type Source interface {
    LoadInto(cfg *schema.Root) error  // 而非返回 map[string]interface{}
}
```

### 3. HTTP base_domains 提供默认值
**来源**: PM
```yaml
http:
  modules:
    domain_proxy:
      base_domains:
        - "localhost.tunnox.dev"  # 开发默认域名
```

### 4. 环境变量向后兼容
**来源**: 开发
```go
// 同时支持有前缀和无前缀，过渡期 6 个月
func getEnvWithFallback(prefix, key string) string {
    if v := os.Getenv(prefix + "_" + key); v != "" {
        return v
    }
    if v := os.Getenv(key); v != "" {
        utils.Warnf("Environment variable %s is deprecated", key)
        return v
    }
    return ""
}
```

### 5. 测试覆盖率提高到 90%
**来源**: QA

---

## 建议修改项 (P1)

| 序号 | 问题 | 来源 | 建议 |
|------|------|------|------|
| 1 | 端口冲突 (Management API 与 HTTP 都在 9000) | PM | 合并为同一服务 |
| 2 | WebSocket 依赖说明不清 | PM | 配置中添加注释说明 |
| 3 | 协议配置结构选择 | Dev | 暂保持现有 map 结构 |
| 4 | 零值判断策略 | Dev | 使用指针类型 |
| 5 | 依赖验证实现 | Dev | 先用硬编码，不引入表达式引擎 |
| 6 | fsnotify 平台兼容 | Arch | 添加轮询模式备选 |
| 7 | 配置变更审计日志 | Arch | 记录变更详情 |

---

## 遗漏配置项补充

### 会话管理 (来源: 架构师)
- `server.session.connection_timeout` (30s)
- `server.session.buffer_size` (32768)
- `server.session.max_idle_time` (5m)

### 流处理 (来源: 架构师)
- `stream.compression.enabled/level`
- `stream.encryption.enabled/algorithm`
- `stream.rate_limit.enabled/bytes_per_second`

### 集群配置 (来源: 架构师)
- `cluster.enabled`
- `cluster.node_id`
- `cluster.grpc_listen`

### HTTP 超时 (来源: 架构师)
- `http.read_timeout` (30s)
- `http.write_timeout` (30s)
- `http.idle_timeout` (120s)

### 客户端代理 (来源: PM)
- `client.proxy.http_proxy`
- `client.proxy.https_proxy`

---

## 工期调整

| 项目 | 原估算 | 建议调整 |
|------|--------|----------|
| 开发 | 33 人天 | 38 人天 |
| Code Review | - | 5 人天 |
| 测试 | - | 10 人天 |
| Buffer | - | 7 人天 |
| **总计** | **33 人天** | **60 人天** |

---

## 测试场景补充 (P0级别)

1. 配置文件不存在、语法错误、权限不足
2. 多配置源优先级覆盖正确性
3. 嵌套结构合并、数组类型合并
4. 环境变量前缀正确性、类型转换
5. .env 多层级加载顺序、变量插值
6. Secret 脱敏输出、日志泄露防护
7. 健康检查端点响应
8. 现有配置文件兼容性

---

## 实施决策

基于评审意见，决定：

1. **立即修正 P0 问题**：Dispose模式、类型安全、环境变量兼容
2. **添加默认 base_domains**：`localhost.tunnox.dev`
3. **保持现有协议结构**：map[string]ProtocolConfig
4. **提高测试覆盖率**：目标 90%
5. **调整工期**：按 60 人天规划

---

**评审汇总完成**
