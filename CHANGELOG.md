# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

