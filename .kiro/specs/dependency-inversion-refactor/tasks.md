# Implementation Plan

## Phase 1: P0 - 常量迁移和依赖倒置修复

- [x] 1. 创建 Kernel Contract 常量包
  - 创建 `internal/constants/ttl.go` 定义所有 TTL 常量
  - 将 `internal/cloud/constants` 中的跨层常量移动到 `internal/constants`
  - 确保常量定义清晰且有文档注释
  - _Requirements: 2.1, 2.2, 2.3, 7.1, 7.2, 7.3_

- [ ]* 1.1 编写属性测试：存储键常量源验证
  - **Property 4: Storage key constants sourcing**
  - **Validates: Requirements 2.1, 7.1**

- [ ]* 1.2 编写属性测试：TTL 常量源验证
  - **Property 5: TTL constants sourcing**
  - **Validates: Requirements 2.2**

- [x] 2. 更新 storage 层移除 cloud 依赖
  - 修改 `internal/core/storage/memory.go` 使用 `internal/constants.DefaultDataTTL`
  - 修改 `internal/core/storage/redis_storage.go` 使用 `internal/constants.DefaultDataTTL`
  - 删除所有对 `internal/cloud/constants` 的导入
  - _Requirements: 1.1, 3.1, 3.2_

- [ ]* 2.1 编写属性测试：Kernel 层导入隔离
  - **Property 1: Kernel layer import isolation**
  - **Validates: Requirements 1.1**

- [ ]* 2.2 编写属性测试：存储测试隔离
  - **Property 7: Storage test isolation**
  - **Validates: Requirements 3.3**

- [x] 3. 更新所有常量引用点
  - 使用 `grep` 或 IDE 查找所有 `internal/cloud/constants` 的引用
  - 逐个更新为 `internal/constants` 的对应常量
  - 确保编译通过
  - _Requirements: 2.1, 2.2, 2.3_

- [ ]* 3.1 编写属性测试：依赖图无环性
  - **Property 2: Dependency graph acyclicity**
  - **Validates: Requirements 1.2**

- [ ]* 3.2 编写属性测试：层间无循环依赖
  - **Property 3: No circular dependencies between layers**
  - **Validates: Requirements 1.4**

- [x] 4. 清理 cloud/constants 包
  - 移除已迁移到 `internal/constants` 的常量
  - 保留仅 cloud 层使用的常量
  - 更新包文档说明其用途范围
  - _Requirements: 7.4_

- [ ]* 4.1 编写属性测试：Cloud 常量隔离
  - **Property 16: Cloud constants isolation**
  - **Validates: Requirements 7.4**

- [x] 5. Checkpoint - 验证常量迁移完成
  - 确保所有测试通过，询问用户是否有问题

## Phase 2: P0 - SDK 入口点实现

- [ ] 6. 设计并实现 ServerConfig 结构
  - 创建 `internal/app/server/sdk_config.go`
  - 定义简化的 `ServerConfig` 结构体
  - 实现配置验证函数 `validateServerConfig()`
  - 添加清晰的错误消息
  - _Requirements: 4.1, 4.6_

- [ ]* 6.1 编写属性测试：配置验证错误清晰度
  - **Property 9: Configuration validation error clarity**
  - **Validates: Requirements 4.6**

- [ ] 7. 实现 server.Run() 函数
  - 创建 `internal/app/server/sdk.go`
  - 实现 `Run(ctx context.Context, config *ServerConfig) error`
  - 处理初始化、启动、等待 context、优雅关闭
  - 确保所有资源正确清理
  - _Requirements: 4.1, 4.3_

- [ ]* 7.1 编写属性测试：SDK 生命周期管理
  - **Property 8: SDK lifecycle management**
  - **Validates: Requirements 4.3**

- [ ]* 7.2 编写集成测试：server.Run() 生命周期
  - 测试启动、运行、优雅关闭流程
  - 验证 context 取消触发关闭
  - _Requirements: 4.1, 4.3_

- [ ] 8. 设计并实现 ClientConfig 结构
  - 创建 `internal/app/client/sdk_config.go`
  - 定义简化的 `ClientConfig` 结构体
  - 实现配置验证函数 `validateClientConfig()`
  - _Requirements: 4.2, 4.6_

- [ ] 9. 实现 client.Run() 函数
  - 创建 `internal/app/client/sdk.go`
  - 实现 `Run(ctx context.Context, config *ClientConfig) error`
  - 处理初始化、连接、等待 context、优雅关闭
  - _Requirements: 4.2, 4.3_

- [ ]* 9.1 编写集成测试：client.Run() 生命周期
  - 测试启动、连接、优雅关闭流程
  - _Requirements: 4.2, 4.3_

- [ ] 10. 重构 cmd/server/main.go
  - 简化为仅包含参数解析和 `server.Run()` 调用
  - 移除所有初始化逻辑到 SDK 层
  - 保持命令行接口不变
  - _Requirements: 4.4_

- [ ] 11. 重构 cmd/client/main.go
  - 简化为仅包含参数解析和 `client.Run()` 调用
  - 移除所有初始化逻辑到 SDK 层
  - 保持命令行接口不变
  - _Requirements: 4.5_

- [ ] 12. Checkpoint - 验证 SDK 入口点工作正常
  - 确保所有测试通过，询问用户是否有问题

## Phase 3: P1 - 拆解高复杂度文件（第一批）

- [ ] 13. 拆分 internal/app/server/config.go
- [ ] 13.1 创建 config_defaults.go
  - 提取所有默认值和常量定义
  - 实现 `NewDefaultConfig()` 函数
  - _Requirements: 5.1_

- [ ] 13.2 创建 config_validation.go
  - 提取所有验证逻辑
  - 实现 `Validate()` 方法
  - 确保错误消息清晰
  - _Requirements: 5.1_

- [ ] 13.3 创建 config_loader.go
  - 提取配置加载和合并逻辑
  - 实现 `LoadFromFile()` 和 `LoadFromEnv()` 函数
  - _Requirements: 5.1_

- [ ] 13.4 精简 config.go
  - 仅保留结构体定义和基本构造函数
  - 确保文件 < 200 行
  - _Requirements: 5.1, 5.5_

- [ ]* 13.5 编写静态分析测试：验证 config 文件拆分
  - 检查文件存在性和职责分离
  - _Requirements: 5.1_

- [ ] 14. 拆分 internal/protocol/session/packet_handler_tunnel.go
- [ ] 14.1 创建 packet_handler_tunnel_data.go
  - 提取数据包处理逻辑
  - 实现数据传输相关的处理函数
  - _Requirements: 5.2_

- [ ] 14.2 创建 packet_handler_tunnel_control.go
  - 提取控制包处理逻辑
  - 实现连接控制相关的处理函数
  - _Requirements: 5.2_

- [ ] 14.3 创建 packet_handler_tunnel_error.go
  - 提取错误处理逻辑
  - 实现错误恢复机制
  - _Requirements: 5.2_

- [ ] 14.4 精简 packet_handler_tunnel.go
  - 仅保留接口定义和路由逻辑
  - 确保文件 < 200 行
  - _Requirements: 5.2, 5.5_

- [ ]* 14.5 编写静态分析测试：验证 packet_handler 文件拆分
  - 检查文件存在性和职责分离
  - _Requirements: 5.2_

- [ ] 15. Checkpoint - 验证第一批文件拆分完成
  - 确保所有测试通过，询问用户是否有问题

## Phase 4: P1 - 拆解高复杂度文件（第二批）

- [ ] 16. 拆分 internal/stream/stream_processor.go
- [ ] 16.1 创建 stream_processor_init.go
  - 提取初始化和配置逻辑
  - 实现 `NewStreamProcessor()` 和配置方法
  - _Requirements: 5.3_

- [ ] 16.2 创建 stream_processor_transform.go
  - 提取数据转换逻辑
  - 实现压缩、加密、转换函数
  - _Requirements: 5.3_

- [ ] 16.3 创建 stream_processor_lifecycle.go
  - 提取生命周期管理逻辑
  - 实现启动、停止、清理函数
  - _Requirements: 5.3_

- [ ] 16.4 精简 stream_processor.go
  - 仅保留接口和核心结构体
  - 确保文件 < 200 行
  - _Requirements: 5.3, 5.5_

- [ ]* 16.5 编写静态分析测试：验证 stream_processor 文件拆分
  - 检查文件存在性和职责分离
  - _Requirements: 5.3_

- [ ] 17. 拆分 internal/cloud/services/service_registry.go
- [ ] 17.1 创建 service_registry_ops.go
  - 提取注册、查找、注销操作
  - 实现服务管理的核心操作
  - _Requirements: 5.4_

- [ ] 17.2 创建 service_registry_lifecycle.go
  - 提取服务生命周期管理
  - 实现启动、停止、健康检查
  - _Requirements: 5.4_

- [ ] 17.3 创建 service_registry_di.go
  - 提取依赖注入逻辑
  - 实现依赖解析和注入
  - _Requirements: 5.4_

- [ ] 17.4 精简 service_registry.go
  - 仅保留接口和核心结构体
  - 确保文件 < 200 行
  - _Requirements: 5.4, 5.5_

- [ ]* 17.5 编写静态分析测试：验证 service_registry 文件拆分
  - 检查文件存在性和职责分离
  - _Requirements: 5.4_

- [ ]* 18. 编写属性测试：文件大小约束
  - **Property 10: File size constraint**
  - **Validates: Requirements 5.5**

- [ ] 19. Checkpoint - 验证第二批文件拆分完成
  - 确保所有测试通过，询问用户是否有问题

## Phase 5: P1 - Goroutine 生命周期文档化

- [ ] 20. 识别所有长期运行的 goroutine
  - 使用 `grep -r "go func"` 和 `grep -r "go [a-z]"` 查找
  - 创建 goroutine 清单列表
  - 分类：网络处理、定时任务、消息处理等
  - _Requirements: 6.3_

- [ ] 21. 为 goroutine 添加生命周期注释
  - 为每个 goroutine 添加标准格式注释
  - 注释格式：`// Lifecycle: ... // Cleanup: ... // Shutdown: ...`
  - 确保注释在 goroutine 启动前 3 行内
  - _Requirements: 6.1, 6.2_

- [ ]* 21.1 编写属性测试：Goroutine 生命周期文档
  - **Property 11: Goroutine lifecycle documentation**
  - **Validates: Requirements 6.1**

- [ ]* 21.2 编写属性测试：Goroutine 清理文档
  - **Property 12: Goroutine cleanup documentation**
  - **Validates: Requirements 6.2**

- [ ] 22. 创建 Goroutine 生命周期图
  - 创建 `docs/architecture/goroutine-lifecycle.md`
  - 使用 Mermaid 绘制生命周期图
  - 列出所有长期 goroutine 及其控制 context
  - _Requirements: 6.3_

- [ ]* 22.1 编写集成测试：Goroutine 终止超时
  - **Property 13: Goroutine termination timeout**
  - **Validates: Requirements 6.5**

- [ ] 23. Checkpoint - 验证 Goroutine 文档化完成
  - 确保所有测试通过，询问用户是否有问题

## 实施完成

所有 Phase 1-5 的任务完成后，重构工作即告完成。你可以根据需要选择性地添加文档和进一步的验证工作。
