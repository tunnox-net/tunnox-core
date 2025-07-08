# StreamFactory 架构改进说明

## 📋 概述

本次改进完善了 StreamFactory 的架构分层，实现了真正的工厂模式，并建立了清晰的分层架构。

## 🎯 主要改进

### 1. 重新实现 StreamFactory

#### 问题
- StreamFactory 接口已定义但实现被删除
- 代码中直接调用具体的构造函数
- 缺乏统一的流组件创建和管理机制

#### 解决方案
- ✅ 实现了 `DefaultStreamFactory` 和 `ConfigurableStreamFactory`
- ✅ 提供了统一的流组件创建接口
- ✅ 支持配置化的流组件创建

```go
// 默认流工厂
factory := stream.NewDefaultStreamFactory(ctx)

// 可配置流工厂
config := stream.StreamFactoryConfig{
    DefaultCompression: true,
    DefaultRateLimit:   1024,
    BufferSize:         4096,
    EnableMemoryPool:   true,
}
factory := stream.NewConfigurableStreamFactory(ctx, config)
```

### 2. 创建 StreamManager

#### 功能特性
- ✅ 统一管理所有流组件的生命周期
- ✅ 提供流的创建、获取、移除、列表等功能
- ✅ 支持并发安全的流管理
- ✅ 流指标统计和监控

```go
// 创建流管理器
manager := stream.NewStreamManager(factory, ctx)

// 创建流
stream, err := manager.CreateStream("connection-1", reader, writer)

// 获取流
retrievedStream, exists := manager.GetStream("connection-1")

// 移除流
err = manager.RemoveStream("connection-1")

// 获取指标
metrics := manager.GetMetrics()
```

### 3. 完善架构分层

#### 分层结构
```
应用层 (Application Layer)
    ↓
协议层 (Protocol Layer)
    ↓
会话层 (Session Layer)
    ↓
流管理层 (Stream Management Layer)
    ↓
工厂层 (Factory Layer)
    ↓
实现层 (Implementation Layer)
```

#### 设计原则
- **依赖倒置**：高层模块不依赖低层模块，都依赖抽象
- **单一职责**：每层只负责自己的核心功能
- **开闭原则**：对扩展开放，对修改关闭
- **接口隔离**：通过接口进行解耦，降低耦合度

### 4. 配置化支持

#### 预定义配置模板
```go
// 支持的配置模板
"default"           // 默认配置
"high_performance"  // 高性能配置
"bandwidth_saving"  // 带宽节省配置
"low_latency"       // 低延迟配置
```

#### 使用示例
```go
// 从配置模板创建工厂
factory, err := stream.CreateFactoryFromProfile(ctx, "high_performance")

// 从配置模板创建管理器
manager, err := stream.CreateManagerFromProfile(ctx, "bandwidth_saving")
```

## 🏗️ 架构图

### 可视化架构分层图

项目包含两种架构图：

1. **整体架构图**：展示整个系统的组件关系
2. **流处理架构分层图**：详细展示流处理的分层架构

#### 生成图片版本

```bash
# 安装 mermaid-cli
npm install -g @mermaid-js/mermaid-cli

# 生成PNG图片
./scripts/generate-architecture-diagram.sh
```

生成的图片将保存在 `docs/images/architecture-layers.png`

## 📊 测试验证

### 测试覆盖
- ✅ StreamFactory 基础功能测试
- ✅ StreamManager 操作测试
- ✅ 流配置模板测试
- ✅ 并发操作测试
- ✅ 项目编译验证

### 运行测试
```bash
# 运行所有流工厂相关测试
go test ./tests -v -run TestStreamFactory
go test ./tests -v -run TestStreamManager
go test ./tests -v -run TestStreamProfiles
```

## 🔄 代码变更

### 新增文件
- `internal/stream/factory.go` - 流工厂实现
- `internal/stream/manager.go` - 流管理器
- `internal/stream/config.go` - 流配置模板
- `tests/stream_factory_test.go` - 流工厂测试
- `docs/architecture-layers.mmd` - 架构分层图
- `scripts/generate-architecture-diagram.sh` - 图片生成脚本

### 修改文件
- `internal/protocol/session.go` - 集成 StreamManager
- `cmd/server/main.go` - 使用新的架构
- `README.md` - 更新架构说明和文档

## 🎯 主要优势

### 1. 解耦性
- 各层通过接口交互，降低耦合度
- 协议层不再直接依赖具体的流实现

### 2. 可扩展性
- 易于添加新的流类型和配置
- 支持自定义流工厂实现

### 3. 可测试性
- 每层都可以独立测试
- 支持 Mock 和依赖注入

### 4. 可配置性
- 支持多种预定义配置模板
- 运行时配置调整

### 5. 资源管理
- 统一的流生命周期管理
- 自动资源清理和监控

### 6. 并发安全
- 支持并发操作
- 线程安全的流管理

## 🚀 使用示例

### 基本使用
```go
// 1. 创建流工厂
factory := stream.NewDefaultStreamFactory(ctx)

// 2. 创建流管理器
manager := stream.NewStreamManager(factory, ctx)

// 3. 创建流
stream, err := manager.CreateStream("conn-1", reader, writer)
if err != nil {
    log.Fatal(err)
}

// 4. 使用流
written, err := stream.WritePacket(packet, false, 0)

// 5. 清理资源
manager.RemoveStream("conn-1")
```

### 配置化使用
```go
// 1. 使用高性能配置
manager, err := stream.CreateManagerFromProfile(ctx, "high_performance")
if err != nil {
    log.Fatal(err)
}

// 2. 创建流
stream, err := manager.CreateStream("conn-1", reader, writer)

// 3. 获取指标
metrics := manager.GetMetrics()
fmt.Printf("活跃流数量: %d\n", metrics.ActiveStreams)
```

## 📈 性能影响

### 正面影响
- ✅ 更好的资源管理和清理
- ✅ 统一的配置管理
- ✅ 更好的并发控制
- ✅ 可监控的流状态

### 轻微开销
- 🔄 工厂模式的轻微性能开销（可忽略）
- 🔄 流管理器的内存开销（每个流约 100-200 字节）

## 🔮 未来扩展

### 计划中的功能
- [ ] 流组件的热插拔支持
- [ ] 更丰富的配置模板
- [ ] 流性能基准测试
- [ ] 分布式流管理
- [ ] 流组件的插件化支持

### 扩展点
- 自定义流工厂实现
- 新的流组件类型
- 自定义配置模板
- 流监控和告警

## 📝 总结

本次 StreamFactory 架构改进实现了：

1. **真正的工厂模式**：统一管理流组件创建
2. **清晰的分层架构**：遵循 SOLID 原则
3. **配置化支持**：预定义模板和自定义配置
4. **资源管理**：统一的流生命周期管理
5. **可扩展性**：为未来功能扩展奠定基础

这个改进为项目提供了更加健壮、可维护和可扩展的架构基础。 