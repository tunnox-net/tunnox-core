# /review - 代码审查

按 Tunnox Core 编码规范进行代码审查。

## 审查范围

可指定文件或目录：`/review internal/protocol/`
不指定则审查最近修改的文件

## 审查规则

### Dispose 体系 (核心)

- [ ] 组件嵌入正确的 dispose 基类 (ManagerBase/ServiceBase)
- [ ] Context 从 parent.Ctx() 派生，禁止 context.Background()
- [ ] 子资源在 onClose 中正确清理
- [ ] goroutine 监听 ctx.Done() 退出

### 分层架构

- [ ] Repository 层只做数据访问
- [ ] Service 层包含业务逻辑
- [ ] Manager 层协调多个 Service
- [ ] 无跨层直接调用

### 类型安全

- [ ] 禁止 `interface{}`、`any`、`map[string]interface{}`
- [ ] 使用强类型结构体或泛型
- [ ] 使用 coreerrors 包的类型化错误

### 命令框架

- [ ] 使用泛型 BaseCommandHandler
- [ ] 请求/响应类型明确

### 代码质量

- [ ] 文件不超过 500 行
- [ ] 函数不超过 100 行
- [ ] 命名规范 (snake_case 文件，PascalCase 类型)
- [ ] 错误正确处理，不忽略

### 并发安全

- [ ] map 并发访问有锁保护
- [ ] goroutine 有退出机制
- [ ] 资源正确释放

### 性能

- [ ] 缓冲区使用 sync.Pool
- [ ] 避免不必要的内存分配
- [ ] 合理的超时设置

## 输出格式

```
## 审查结果

### 严重问题 (必须修复)
- file:line - 问题描述

### 警告 (建议修复)
- file:line - 问题描述

### 建议 (可选优化)
- file:line - 优化建议
```

## 检查命令

```bash
# 编译检查
go build ./...

# Vet 检查
go vet ./...

# 竞态检测
go test -race ./...

# 测试覆盖
go test ./... -cover
```
