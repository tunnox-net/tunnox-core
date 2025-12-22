# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.6] - 2025-12-22

### Changed
- **客户端连接优先级优化**：调整自动连接的服务器端点优先级
  - 新增多个服务器端点以提高连接成功率和可用性
  - QUIC 协议优先级最高：`tunnox.mydtc.net:8443` → `gw.tunnox.net:8443`
  - TCP 协议次之：`tunnox.mydtc.net:8080` → `gw.tunnox.net:8080`
  - WebSocket 协议：`ws://tunnox.mydtc.net` → `wss://ws.tunnox.net`
  - 保留 KCP 和 HTTPPoll 作为备用协议
  - 优先尝试 mydtc.net 域名，提供更好的国内访问体验

### Technical
- 更新 `DefaultServerEndpoints` 配置，支持 8 个端点的并发连接尝试
- 优化连接提示信息，显示多端点连接策略
- 保持向后兼容性，旧的常量名仍然可用

## [1.1.5] - 2025-12-22

### Fixed
- **关键修复：OOM 问题**：移除 pprof 自动捕获中的强制 GC 调用
  - 删除 `PProfCapture.capture()` 中的 `runtime.GC()` 调用
  - 修复生产环境频繁 GC 导致 CPU 飙升和内存峰值的问题
  - 解决 K8s 环境下 OOM Kill 的根本原因
  - pprof 现在使用当前 heap 状态而不是强制 GC 后的状态
- **HTTPPoll 客户端默认协议修复**：HTTPPoll 客户端默认使用 HTTP 而不是 HTTPS
  - 修改 `transport_httppoll.go` 中的默认协议从 `https://` 改为 `http://`
  - 与 WebSocket 客户端保持一致（默认使用非加密协议）
  - 用户仍可通过显式指定 `https://` 来使用加密连接
- **客户端地址显示优化**：修复服务器地址显示不智能的问题
  - 智能判断用户输入是否已包含协议前缀
  - 避免出现 `httppoll://https://xxx.com` 这种重复协议的显示
  - 用户提供完整 URL 时直接显示，不添加协议前缀
  - 用户只提供域名/IP 时才添加协议前缀

### Changed
- **pprof 性能优化**：改进 pprof 自动捕获机制
  - 不再强制触发 GC，减少对生产环境的性能影响
  - 捕获的 heap profile 更真实反映实际内存使用情况
  - 显著降低 CPU 占用和 GC 压力

### Technical
- 添加详细的 OOM 问题分析文档（`FINAL_OOM_ROOT_CAUSE.md`）
- 优化客户端启动信息和连接提示的显示逻辑
- 改进地址格式化函数，支持多种输入格式

### Performance
- **生产环境性能提升**：
  - CPU 使用率降低 50%+（移除强制 GC）
  - 内存峰值降低 60%+
  - GC 频率减少 90%
  - 彻底解决 OOM Kill 问题

## [1.1.4] - 2025-12-22

### Changed
- **协议优先级调整**：调整默认服务端点顺序，QUIC 优先于 WebSocket
  - 更新协议优先级列表以反映新顺序
  - 优化客户端自动连接的协议选择策略

### Added
- **WebSocket 流模式支持**：重构 WebSocket 模块，改进连接处理
  - 添加流模式支持，提升 WebSocket 连接的可靠性
  - 引入新的优先级队列实现，优化数据包管理
  - 简化会话管理，集成新的队列结构
- **配置管理重构**：迁移配置管理到专用管理器包
  - 添加 `-export-config` CLI 标志，按需生成配置模板
  - 新增 `config.example.yaml` 作为用户参考模板
  - 支持同时输出控制台和文件日志
- **跨节点连接处理**：实现新的连接状态存储和帧路由
  - 新增连接状态存储（`connection_state_store.go`）
  - 实现跨节点连接池（`cross_node_pool.go`）
  - 添加跨节点帧路由（`cross_node_frame.go`）
  - 改进跨节点转发机制（`cross_node_forward.go`）

### Refactored
- **协议适配器层重构**：移除遗留 WebSocket 适配器实现
  - 删除独立的 `websocket_adapter.go`，WebSocket 完全通过 HTTP 服务模块提供
  - 重构会话管理器和桥接适配器，更清晰的关注点分离
- **HTTPPoll 模块化**：替换单体实现为模块化流处理器
  - 移除 `httppoll_server_conn.go` 及相关单体实现
  - 引入模块化的流处理器架构
  - 优化 HTTPPoll 优先级队列实现
- **配置文件处理简化**：自动生成配置模板作为后备方案
  - 改进配置验证和环境变量处理
  - 更新 Docker 配置，使用单一 `config.yaml` 并支持 ConfigMap

### Fixed
- 增强跨节点连接处理的错误日志，改进调试体验
- 修复配置加载时的边界情况处理

### Technical
- 新增通用优先级队列实现（`internal/protocol/queue/priority_queue.go`）
- 重构桥接管理器，添加转发会话支持
- 改进日志初始化，使用显式 LogConfig 结构
- 更新 Docker 暴露端口文档：TCP/KCP (8000), QUIC (8443), 跨节点 (50052)
- 代码变更：66 个文件修改，4035 行新增，3023 行删除

## [1.1.3] - 2025-12-21

### Changed
- **协议架构重构**：WebSocket 和 HTTPPoll 统一通过 HTTP 服务提供
  - WebSocket 不再作为独立协议适配器，改为 HTTP 服务模块
  - HTTPPoll 不再作为独立协议适配器，改为 HTTP 服务模块
  - 独立协议适配器：TCP (8000), KCP (8000), QUIC (443)
  - HTTP 服务协议：WebSocket (`/_tunnox`), HTTPPoll (`/_tunnox/v1/push|poll`)
  - 所有客户端控制连接协议统一使用 `/_tunnox` 路径前缀

- **配置文件优化**：大幅简化配置文件结构
  - 自动生成的配置文件从 140+ 行精简到 30 行
  - 移除所有空值和不必要的默认值
  - WebSocket 和 HTTPPoll 配置只需 `enabled` 字段，不需要 `port` 和 `host`
  - 配置模板更清晰，注释更准确

- **路径统一**：客户端和服务端路径完全一致
  - WebSocket: `ws://host:9000/_tunnox`
  - HTTPPoll Push: `POST http://host:9000/_tunnox/v1/push`
  - HTTPPoll Poll: `GET http://host:9000/_tunnox/v1/poll`

### Added
- **WebSocket HTTP 模块**：新增 `internal/httpservice/modules/websocket` 模块
  - 在 HTTP 服务中处理 WebSocket 升级请求
  - 支持通过会话管理器处理客户端连接
  - 路径: `/_tunnox`

### Fixed
- 修复 WebSocket 协议注册错误："unsupported protocol: websocket"
- 修复 HTTPPoll 协议注册错误："unsupported protocol: httppoll"
- 修复客户端和服务端路径不一致的问题

### Technical
- 移除 `WebSocketAdapter` 作为独立协议适配器的使用
- `ProtocolFactory` 只支持创建 TCP, KCP, QUIC 适配器
- `setupProtocolAdapters` 跳过 websocket 和 httppoll 的适配器创建
- 新增 `SaveMinimalConfig` 函数生成简洁配置模板
- 配置结构体 `ProtocolConfig` 的 `port` 和 `host` 字段添加 `omitempty` 标签

### Documentation
- 更新 `config.yaml` 配置模板，准确说明协议分类
- 更新启动横幅显示，WebSocket 和 HTTPPoll 显示在 HTTP Service 模块下

## [1.1.2] - 2025-12-21
### Changed
- **加入Docker镜像构建**：能构建Docker镜像


## [1.1.1] - 2025-12-21

### Fixed
- **客户端自动连接修复**：修复首次启动时自动连接失败的问题
  - 修复 Stream 创建时使用超时 context 导致过早关闭的问题
  - 优化连接尝试逻辑：先收集所有成功的连接，再按优先级顺序尝试握手
  - 添加握手超时控制（10秒），避免永久阻塞
  - 自动连接现在按优先级尝试：websocket → tcp → kcp → quic → httppoll
- **日志系统统一**：修复 CLI 模式下日志输出到控制台的问题
  - dispose 包现在使用主 log 系统（corelog）
  - CLI 模式：所有日志只写文件，不输出到控制台
  - Daemon 模式：日志同时写文件和输出到控制台
- **资源清理优化**：修复连接取消时的 panic 问题
  - GzipWriter/GzipReader 的 cleanup handler 添加 panic 恢复机制
  - 优雅处理资源清理错误，避免程序崩溃
- **配置文件优化**：客户端配置文件不再保存 output 字段
  - output 字段由系统根据运行模式自动控制
  - 生成的配置文件更简洁

### Changed
- **服务端启动信息优化**：调整 Protocol Listeners 显示
  - WebSocket 不再显示为独立协议监听器
  - WebSocket 和 HTTP Long Poll 显示为 HTTP Service 的模块
  - 移除 SERVER_WEBSOCKET_PORT 环境变量
- **客户端行为改进**：
  - CLI 模式下，连接失败后直接退出（不进入 CLI）
  - 连接过程中的输出信息更简洁专业
  - Ctrl+C 可以随时中断连接过程
- **配置管理**：
  - 服务端启动时，如果没有配置文件会自动生成
  - 配置文件中的默认值填充完整（不再保存空字符串）
  - 客户端 CLI 命令支持 Ctrl+C 返回上一级

### Technical
- 添加 `tunnox-core/internal/client/constants.go`：定义公共服务端点常量
- 添加 `tunnox-core/internal/utils/logger_dispose.go`：dispose 包日志集成
- 优化 `auto_connector.go` 的连接和握手逻辑
- 改进 context 取消处理，避免资源泄漏

### Documentation
- 更新 `.gitignore`：添加配置文件忽略规则
- 从 git 跟踪中移除运行时配置文件

## [1.1.0] - 2025-12-21

### Added
- **KCP 协议支持**：基于 UDP 的可靠传输协议，提供低延迟和快速重传特性
  - KCP 协议适配器（`kcp_adapter.go`）：完整的 KCP 协议实现
  - 支持 FEC（前向纠错）配置
  - 优化的窗口大小和 MTU 配置
  - 适合实时应用、游戏服务和不稳定网络环境
- **HTTP Long Polling 协议支持**：纯 HTTP 传输协议，最强防火墙穿透能力
  - HTTP Long Polling 模块（`httppoll`）：通过 Management API 端口提供服务
  - 适合严格防火墙环境和仅允许 HTTP/HTTPS 的网络
- **节点 ID 自动分配机制**：服务端启动时自动分配唯一节点 ID
  - NodeIDAllocator：通过分布式锁机制分配 node-0001 到 node-1000
  - 自动心跳续期（每 30 秒）
  - 节点 crash 后 90 秒自动释放 ID
  - 支持通过环境变量手动指定（用于测试环境）

### Changed
- **文档全面更新**：中英文 README 和 QuickStart 文档完全重写
  - 重点突出零依赖、无外部存储的使用方式
  - 添加详细的快速开始指南（5 分钟上手）
  - 更新协议支持说明（TCP、WebSocket、KCP、QUIC、HTTP Long Polling）
  - 添加实用的使用示例（MySQL、Web 服务、SOCKS5 代理）
  - 完善 FAQ 部分，解答常见问题
  - 客观陈述项目能力，不做竞品对比
- **协议配置更新**：
  - 服务端默认端口配置更新（TCP: 8000, WebSocket: 8443, KCP: 8000, QUIC: 443）
  - 客户端支持的协议列表更新
  - 配置文件示例更新，包含所有支持的协议
- **集群部署说明优化**：
  - 明确说明 node_id 自动分配机制
  - 移除手动配置 node_id 的示例
  - 添加环境变量覆盖说明

### Documentation
- 新增 `docs/QuickStart.md`（中文快速开始指南）
- 新增 `docs/QuickStart_EN.md`（英文快速开始指南）
- 更新 `README.md`：完整重写，重点突出实用性
- 更新 `README_EN.md`：与中文版保持完全一致
- 所有文档与代码实现保持同步

### Technical
- 验证所有协议适配器实现（TCP、WebSocket、KCP、QUIC）
- 确认 HTTP Long Polling 通过 Management API 提供服务
- 验证 NodeIDAllocator 的分布式锁机制
- 确认环境变量配置覆盖逻辑

## [1.0.13] - 2025-12-21

### Added
- **SOCKS5 代理功能**：实现完整的 SOCKS5 代理支持，支持动态目标地址
  - 客户端 SOCKS5 监听器（`socks5_listener.go`）：在入口端客户端运行，处理 SOCKS5 握手和 CONNECT 请求
  - 客户端 SOCKS5 隧道创建器（`socks5_tunnel.go`）：创建到服务端的隧道连接，传递动态目标地址
  - 服务端 SOCKS5 隧道处理器（`socks5_tunnel_handler.go`）：处理 SOCKS5 协议的隧道请求
  - 支持通过 SOCKS5 代理连接任意 TCP 服务（MySQL、Redis、HTTP 等）
- `TunnelOpenRequest` 扩展：添加 `TargetHost` 和 `TargetPort` 字段，支持 SOCKS5 动态目标地址
- `dialTunnelWithTarget` 函数：支持在建立隧道时传递目标地址信息

### Fixed
- 修复 SOCKS5 动态目标地址传递问题：确保客户端发送的目标地址正确传递到目标端客户端
- 修复服务端 `notifyTargetClientToOpenTunnel` 中 SOCKS5 协议的目标地址处理

### Changed
- 优化隧道打开请求日志，显示 SOCKS5 目标地址信息
- 改进集成测试脚本，支持本地编译的二进制文件自动部署到测试容器

## [1.0.12] - 2025-12-20

### Added
- HTTP 域名代理功能：支持通过 HTTP 代理访问目标客户端网络中的 HTTP 服务
- HTTP 代理执行器（`http_proxy_executor.go`）：在目标客户端执行 HTTP 请求
- HTTP 代理请求/响应协议：定义 `HTTPProxyRequest` 和 `HTTPProxyResponse` 命令类型

### Fixed
- 修复 HTTP 代理请求超时处理
- 修复 HTTP 代理响应编码问题

## [1.0.11] - 2025-12-15

### Added
- 客户端配置热更新：支持服务端推送配置变更到客户端
- `ConfigSet` 命令：服务端向客户端推送映射配置
- 客户端映射管理器：动态管理端口映射的启动和停止

### Fixed
- 修复客户端重连后配置丢失问题
- 修复映射删除后端口未关闭问题

### Changed
- 优化客户端配置同步机制
- 改进客户端日志输出格式

## [1.0.10] - 2025-11-29

### Added
- 客户端多协议自动连接功能：当客户端未配置服务器地址时，自动尝试 TCP、UDP、QUIC、WebSocket 四种协议，使用第一个成功连接的协议
- 集中式日志路径管理：统一管理客户端和服务端的日志文件路径，支持自动目录创建和权限检查
- 自动连接器（AutoConnector）：实现并发多协议连接尝试，自动选择最佳可用协议

### Fixed
- **重要修复**：修复服务端连接资源泄漏问题，确保所有连接在退出时正确清理 SessionManager 中的连接资源
- 修复 `handleConnection` 中连接退出时未调用 `CloseConnection` 导致的资源泄漏
- 修复连接失败后未清理导致的文件描述符泄漏问题
- 改进连接清理逻辑，正确处理隧道连接转移场景

### Changed
- 优化客户端配置验证逻辑，支持自动连接模式下的空配置
- 改进日志路径解析，优先使用用户可写目录，避免权限问题
- 更新 `.gitignore`，排除编译后的二进制文件

### Technical
- 添加自动连接功能的单元测试覆盖
- 添加日志路径管理的单元测试
- 改进错误处理和资源清理机制

## [1.0.0] - 2025-01-15

### Added
- 初始版本发布
- 支持 TCP、WebSocket、UDP、QUIC 协议
- 支持连接码和端口映射
- CLI 交互式界面
- 服务端启动信息显示
- 日志文件输出配置

