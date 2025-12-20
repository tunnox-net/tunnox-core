# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

