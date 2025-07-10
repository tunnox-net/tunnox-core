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
```

### 配置化使用
```go
// 1. 创建配置
config := stream.StreamFactoryConfig{
    DefaultCompression: true,
    DefaultRateLimit:   1024,
    BufferSize:         4096,
    EnableMemoryPool:   true,
}

// 2. 创建可配置工厂
factory := stream.NewConfigurableStreamFactory(ctx, config)

// 3. 创建管理器
manager := stream.NewStreamManager(factory, ctx)

// 4. 使用流
stream, err := manager.CreateStream("conn-1", reader, writer)
```

### 预定义配置使用
```go
// 1. 从预定义配置创建工厂
factory, err := stream.CreateFactoryFromProfile(ctx, "high_performance")
if err != nil {
    log.Fatal(err)
}

// 2. 创建管理器
manager := stream.NewStreamManager(factory, ctx)

// 3. 使用流
stream, err := manager.CreateStream("conn-1", reader, writer)
```

## 📈 性能优化

### 内存池优化
- 减少内存分配开销
- 降低 GC 压力
- 提升数据传输效率

### 零拷贝优化
- 缓冲区复用
- 减少内存拷贝
- 提升传输性能

### 流式处理优化
- 支持压缩和限速
- 优化网络带宽使用
- 灵活的数据包处理

## 🔧 配置说明

### 流工厂配置
```go
type StreamFactoryConfig struct {
    DefaultCompression bool   // 默认启用压缩
    DefaultRateLimit   int64  // 默认限速值
    BufferSize         int    // 缓冲区大小
    EnableMemoryPool   bool   // 启用内存池
}
```

### 预定义配置模板
- **default**：默认配置，平衡性能和资源使用
- **high_performance**：高性能配置，优先考虑性能
- **bandwidth_saving**：带宽节省配置，优先考虑带宽优化
- **low_latency**：低延迟配置，优先考虑延迟优化

## 🛠️ 开发指南

### 添加新的流类型
1. 实现 `Stream` 接口
2. 在工厂中添加创建方法
3. 添加相应的测试用例

### 添加新的配置模板
1. 在 `config.go` 中定义配置
2. 在 `CreateFactoryFromProfile` 中添加支持
3. 更新文档和测试

### 扩展流管理器
1. 在 `StreamManager` 中添加新方法
2. 确保线程安全
3. 添加相应的测试用例

## 📋 总结

本次改进实现了：

**架构优化**
- 清晰的分层架构设计
- 真正的工厂模式实现
- 统一的流管理机制

**功能完善**
- 配置化的流组件创建
- 预定义配置模板支持
- 完整的生命周期管理

**性能提升**
- 内存池和零拷贝优化
- 流式处理支持
- 并发安全设计

**开发体验**
- 简洁的 API 设计
- 完善的测试覆盖
- 详细的文档说明

这些改进为 Tunnox Core 提供了更加稳定、高效、可扩展的流处理架构基础。 