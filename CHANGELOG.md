# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

