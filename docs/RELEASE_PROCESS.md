# Tunnox Core 版本管理与发布流程

## 📋 概述

本文档描述了 Tunnox Core 的完整版本管理和自动化发布流程。通过 GitHub Actions CI/CD，实现从版本号更新到 GitHub Release 的自动化发布。

## 🎯 设计目标

1. **版本号统一管理**：单一来源（VERSION 文件）
2. **自动化构建**：支持多平台二进制构建
3. **自动化发布**：自动创建 GitHub Release
4. **变更记录**：CHANGELOG.md 自动更新
5. **简单易用**：只需更新版本号和 CHANGELOG

## 📁 文件结构

```
tunnox-core/
├── VERSION                    # 版本号文件（单一来源）
├── CHANGELOG.md              # 变更日志
├── .github/
│   └── workflows/
│       ├── release.yml       # 发布工作流
│       └── build.yml         # 构建工作流
└── internal/
    └── version/
        └── version.go        # 版本信息包
```

## 🔢 版本号规范

采用 [语义化版本](https://semver.org/) (Semantic Versioning)：

- **格式**：`MAJOR.MINOR.PATCH`
- **示例**：`1.0.0`, `1.1.0`, `1.1.1`, `2.0.0`
- **规则**：
  - `MAJOR`：不兼容的 API 变更
  - `MINOR`：向后兼容的功能新增
  - `PATCH`：向后兼容的问题修复

## 📝 发布流程

### 步骤 1：准备发布

1. **更新版本号**
   ```bash
   # 编辑 VERSION 文件
   echo "1.1.0" > VERSION
   ```

2. **更新 CHANGELOG.md**
   ```markdown
   ## [1.1.0] - 2025-01-15
   
   ### Added
   - 新增功能 A
   - 新增功能 B
   
   ### Changed
   - 改进功能 C
   
   ### Fixed
   - 修复问题 D
   ```

3. **提交更改**
   ```bash
   git add VERSION CHANGELOG.md
   git commit -m "chore: bump version to 1.1.0"
   git push origin main
   ```

### 步骤 2：创建发布标签

有两种方式触发发布：

#### 方式 A：手动创建标签（推荐）

```bash
# 创建并推送标签
git tag -a v1.1.0 -m "Release v1.1.0"
git push origin v1.1.0
```

#### 方式 B：通过 GitHub Actions 手动触发

1. 在 GitHub 仓库页面，进入 "Actions" 标签
2. 选择 "Release" 工作流
3. 点击 "Run workflow"
4. 输入版本号（如 `1.1.0`）
5. 点击 "Run workflow"

### 步骤 3：自动化流程

GitHub Actions 会自动执行：

1. ✅ **验证版本号格式**
2. ✅ **验证 VERSION 文件与标签一致**
3. ✅ **构建多平台二进制**
   - Linux (amd64, arm64)
   - macOS (amd64, arm64)
   - Windows (amd64, arm64)
4. ✅ **生成校验和文件**
5. ✅ **创建 GitHub Release**
   - 标题：`v1.1.0`
   - 描述：从 CHANGELOG.md 提取
   - 附件：所有二进制文件和校验和

## 📄 CHANGELOG.md 格式

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 新功能开发中...

## [1.1.0] - 2025-01-15

### Added
- 新增 CLI 表格显示功能
- 新增服务端启动信息横幅
- 新增日志文件输出配置

### Changed
- 优化 CLI 用户体验
- 改进错误提示信息

### Fixed
- 修复表格对齐问题
- 修复日志输出配置

## [1.0.0] - 2025-01-01

### Added
- 初始版本发布
- 支持 TCP、WebSocket、UDP、QUIC 协议
- 支持连接码和端口映射
```

## 🔧 版本信息在代码中的使用

版本信息通过 `internal/version` 包统一管理：

```go
// internal/version/version.go
package version

var (
    Version   = "1.1.0"        // 从 VERSION 文件读取
    BuildTime = ""             // 构建时注入
    GitCommit = ""             // 构建时注入
)
```

在代码中使用：

```go
import "tunnox-core/internal/version"

fmt.Printf("Version: %s\n", version.Version)
```

## 🚀 GitHub Actions 工作流

### release.yml

触发条件：
- 推送标签：`v*`（如 `v1.1.0`）
- 手动触发：通过 GitHub UI

执行步骤：
1. 读取 VERSION 文件
2. 验证版本号格式
3. 构建多平台二进制
4. 生成校验和
5. 创建 GitHub Release

### build.yml

触发条件：
- Push 到 main 分支
- Pull Request

执行步骤：
1. 运行测试
2. 构建二进制（仅当前平台）
3. 上传构建产物（可选）

## 📦 发布产物

每次发布会生成：

```
tunnox-server-v1.1.0-linux-amd64
tunnox-server-v1.1.0-linux-arm64
tunnox-server-v1.1.0-darwin-amd64
tunnox-server-v1.1.0-darwin-arm64
tunnox-server-v1.1.0-windows-amd64.exe
tunnox-server-v1.1.0-windows-arm64.exe
tunnox-client-v1.1.0-linux-amd64
tunnox-client-v1.1.0-linux-arm64
tunnox-client-v1.1.0-darwin-amd64
tunnox-client-v1.1.0-darwin-arm64
tunnox-client-v1.1.0-windows-amd64.exe
tunnox-client-v1.1.0-windows-arm64.exe
checksums.txt
```

## 🔐 权限配置

在 GitHub 仓库设置中配置：

1. **Settings** → **Secrets and variables** → **Actions**
2. 确保以下权限已启用：
   - `Contents: Write`（创建 Release）
   - `Actions: Read`（读取工作流状态）

## 📋 检查清单

发布前检查：

- [ ] VERSION 文件已更新
- [ ] CHANGELOG.md 已更新
- [ ] 所有更改已提交并推送
- [ ] 测试通过
- [ ] 版本号符合语义化版本规范

## 🐛 故障排除

### 问题：版本号验证失败

**原因**：VERSION 文件格式不正确

**解决**：确保 VERSION 文件只包含版本号，如 `1.1.0`（不要包含 `v` 前缀）

### 问题：Release 创建失败

**原因**：权限不足

**解决**：检查 GitHub Actions 权限设置

### 问题：构建失败

**原因**：Go 版本不兼容或依赖问题

**解决**：检查 `go.mod` 和 `go.sum` 是否正确

## 📚 参考资料

- [Semantic Versioning](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/)
- [GitHub Actions](https://docs.github.com/en/actions)

