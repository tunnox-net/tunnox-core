# Tunnox 客户端产品设计

**版本**: v2.0
**更新日期**: 2025-12-26
**负责人**: Product Team

---

## 文档概述

本文档从产品经理视角重新设计 Tunnox 客户端，目标是：
- **新用户友好**：零配置、交互式引导、清晰提示
- **老用户专业**：完整命令行、配置文件、脚本化
- **跨平台统一**：Windows/macOS/Linux 一致体验
- **日志清晰**：分级日志、颜色高亮、易于调试

---

## 目录

- [重要概念：协议分层](#重要概念协议分层)
- [一、产品定位](#一产品定位)
- [二、用户画像与场景](#二用户画像与场景)
- [三、客户端形态](#三客户端形态)
- [四、命令行设计](#四命令行设计)
- [五、交互式向导](#五交互式向导)
- [六、配置文件设计](#六配置文件设计)
- [七、日志系统设计](#七日志系统设计)
- [八、错误处理与提示](#八错误处理与提示)
- [九、安装与分发](#九安装与分发)
- [十、进阶功能](#十进阶功能)

---

## 重要概念：协议分层

**⚠️ 在阅读本文档前，请先理解这个核心概念！**

Tunnox 有**两层协议**，很多新用户容易混淆：

### 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        用户视角                               │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  【本地服务】─────────► 【Tunnox 客户端】                    │
│  localhost:3000         运行在用户本地                       │
│  (HTTP Web 应用)                                             │
│                              │                               │
│                              │ ① 传输协议                    │
│                              │   (TCP/WebSocket/KCP/QUIC)    │
│                              │   【客户端 ↔ 服务器】         │
│                              ▼                               │
│                    【Tunnox 服务器】                          │
│                    (SaaS 或私有部署)                         │
│                              │                               │
│                              │ ② 业务协议                    │
│                              │   (HTTP/TCP/UDP/SOCKS)        │
│                              │   【服务器 ↔ 访问者】         │
│                              ▼                               │
│                    【公网访问者】                             │
│                    https://abc.tunnox.com                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 两层协议说明

| 层级 | 协议 | 作用 | 谁关心 | 对应参数 |
|------|------|------|--------|----------|
| **① 传输层** | TCP / WebSocket / KCP / QUIC | 客户端如何连接到 Tunnox 服务器 | 系统管理员、网络受限环境 | `--transport` |
| **② 业务层** | HTTP / TCP / UDP / SOCKS | 转发什么类型的本地服务 | 普通用户、开发者 | `tunnox http/tcp/udp` |

### 实际例子

```bash
# 命令: tunnox http 3000 --transport quic --server my-server.com

解读:
  tunnox http 3000          ← ② 业务层: 转发本地 3000 端口的 HTTP 服务
         ^^^^                  (用户本地运行着 React/Vue 等 HTTP 服务)

  --transport quic          ← ① 传输层: 客户端用 QUIC 协议连接服务器
              ^^^^             (低延迟、抗丢包，适合移动网络)

  --server my-server.com    ← 服务器地址: 连接到私有部署的服务器
           ^^^^^^^^^^^^^^      (默认是 SaaS: gw.tunnox.net)

工作流程:
  1. 用户本地运行 HTTP 服务在 localhost:3000 (比如 React 开发服务器)
  2. Tunnox 客户端使用 QUIC 协议连接到 my-server.com
  3. 服务器分配公网地址 https://abc123.my-server.com
  4. 访问者访问 https://abc123.my-server.com
  5. 服务器通过 QUIC 隧道转发请求到客户端
  6. 客户端将请求转发到本地 HTTP 服务 localhost:3000
  7. 响应原路返回
```

### 常见问题

**Q: `tunnox http 3000` 中的 `http` 是什么？**
- A: 业务协议，表示要转发本地的 HTTP 服务
- 不是传输协议！传输协议默认自动选择（通常是 WebSocket）

**Q: 我们没有 HTTP 协议的服务器，为什么有 `tunnox http`？**
- A: Tunnox 服务器支持多种**业务协议**转发（HTTP/TCP/UDP/SOCKS）
- Tunnox 服务器与客户端之间的**传输协议**是 TCP/WebSocket/KCP/QUIC
- 这是两个不同的层面！

**Q: 如何使用私有服务器？**
- A: 使用 `--server` 参数指定私服地址
- 例: `tunnox http 3000 --server my-company-server.com:8000`

**Q: 什么时候需要指定传输协议？**
- A: 大多数情况下不需要，自动选择即可
- 特殊场景：
  - 企业防火墙阻止 TCP → 用 `--transport websocket`
  - 移动网络不稳定 → 用 `--transport quic`
  - 追求极致性能 → 用 `--transport kcp`

---

## 一、产品定位

### 1.1 核心目标

Tunnox 客户端是用户与 Tunnox 服务的**第一接触点**，必须做到：

| 目标 | 说明 | 衡量指标 |
|------|------|----------|
| **5 分钟上手** | 新用户从下载到创建第一个隧道 < 5 分钟 | 用户留存率 > 80% |
| **零学习成本** | 不需要阅读文档即可完成基础任务 | 文档访问率 < 30% |
| **专业可靠** | 老用户可以完全脚本化、自动化 | CLI 使用率 > 50% |
| **跨平台一致** | Windows/macOS/Linux 体验一致 | 跨平台问题 < 5% |

### 1.2 设计原则

**渐进式复杂度**
```
新用户路径：下载 → 双击运行 → 向导配置 → 自动连接
老用户路径：下载 → 命令行启动 → 配置文件 → 守护进程
```

**约定优于配置**
- 默认值覆盖 90% 使用场景
- 只在必要时才要求用户输入
- 提供"一键式"快捷命令

**友好的错误提示**
- 不只说"错误"，还要说"怎么修复"
- 提供错误码和文档链接
- 常见错误自动诊断

---

## 二、用户画像与场景

### 2.1 用户画像

**新手小白（40%）**
- 特征：不熟悉命令行，首次使用隧道工具
- 需求：简单、直观、有引导
- 场景：临时分享本地 Web 项目给客户

**个人开发者（35%）**
- 特征：熟悉命令行，有一定技术背景
- 需求：快速、灵活、可配置
- 场景：日常开发调试、远程访问家里的 NAS

**企业用户（20%）**
- 特征：需要稳定、可监控、可批量部署
- 需求：配置文件、日志、守护进程
- 场景：微服务开发、IoT 设备管理

**极客玩家（5%）**
- 特征：追求性能、定制化
- 需求：完整 API、高级参数、性能调优
- 场景：自建服务、性能测试

### 2.2 典型场景

#### 场景 1：新用户首次使用（交互式向导）

```bash
# 用户双击运行客户端
$ ./tunnox

┌─────────────────────────────────────────────────────────────┐
│ 🎉 欢迎使用 Tunnox!                                         │
│                                                              │
│ 这是您第一次运行 Tunnox，让我们快速设置一下。               │
└─────────────────────────────────────────────────────────────┘

? 您想如何使用 Tunnox?
  ▸ 快速体验（匿名模式，无需注册）
    我已有账号（使用 Token 登录）
    查看帮助文档

? 您想分享什么服务?
  ▸ 本地 Web 服务（HTTP/HTTPS）
    SSH 服务器
    数据库（MySQL/PostgreSQL）
    其他 TCP 服务

? 请输入您的本地服务地址:
  ▸ localhost:3000

🔄 正在连接到 Tunnox 服务器...
✅ 连接成功!

🎊 您的隧道已创建!

   访问地址: https://abc123.tunnox.com
   本地地址: http://localhost:3000

   分享上面的访问地址即可让任何人访问您的本地服务。

? 接下来您想做什么?
  ▸ 保持隧道运行（按 Ctrl+C 停止）
    创建另一个隧道
    进入高级模式
    退出
```

#### 场景 2：个人开发者日常使用（快捷命令）

```bash
# 快速创建 HTTP 隧道
$ tunnox http 3000
🔗 Tunnox HTTP Tunnel
   Local:  http://localhost:3000
   Public: https://random123.tunnox.com

   Forwarding HTTP traffic...
   Press Ctrl+C to stop

# 快速创建 TCP 隧道
$ tunnox tcp 22
🔗 Tunnox TCP Tunnel
   Local:  localhost:22
   Public: tcp://tunnox.com:12345

   Forwarding TCP traffic...
   Press Ctrl+C to stop

# 使用配置文件
$ tunnox start -c my-config.yml
✅ Loaded config from my-config.yml
✅ Started 3 tunnels:
   - web-app (HTTP) → https://myapp.tunnox.com
   - ssh (TCP) → tcp://tunnox.com:23456
   - mysql (TCP) → tcp://tunnox.com:23457
```

#### 场景 3：企业用户守护进程部署

```bash
# 使用配置文件 + 守护进程模式
$ tunnox start \
    --config /etc/tunnox/config.yml \
    --daemon \
    --log-file /var/log/tunnox/client.log \
    --pid-file /var/run/tunnox.pid

✅ Tunnox client started in daemon mode
   PID: 12345
   Logs: /var/log/tunnox/client.log

# 查看状态
$ tunnox status
✅ Tunnox is running (PID: 12345)
   Uptime: 2 days 5 hours
   Tunnels: 5 active
   Traffic: 1.2 GB sent, 3.5 GB received

# 停止
$ tunnox stop
✅ Tunnox stopped (PID: 12345)
```

---

## 三、客户端形态

### 3.1 多种运行模式

| 模式 | 适用人群 | 特点 | 使用方式 |
|------|----------|------|----------|
| **交互式向导** | 新手小白 | 图形化引导、零配置 | 双击运行 |
| **快捷命令** | 个人开发者 | 一行命令、快速启动 | `tunnox http 3000` |
| **配置文件** | 企业用户 | 可复用、可版本控制 | `tunnox start -c config.yml` |
| **守护进程** | 服务器部署 | 后台运行、开机自启 | `tunnox start --daemon` |
| **交互式 CLI** | 极客玩家 | 实时控制、动态调整 | `tunnox shell` |

### 3.2 客户端架构

```
┌─────────────────────────────────────────────────────────────┐
│                     tunnox (主程序)                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ 交互式向导    │  │ 快捷命令      │  │ 配置文件      │     │
│  │ (Wizard)     │  │ (Quick)      │  │ (Config)     │     │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │
│         │                 │                 │              │
│         └─────────────────┼─────────────────┘              │
│                           │                                │
│                  ┌────────▼─────────┐                      │
│                  │  核心引擎         │                      │
│                  │  (Core Engine)   │                      │
│                  └────────┬─────────┘                      │
│                           │                                │
│         ┌─────────────────┼─────────────────┐              │
│         │                 │                 │              │
│  ┌──────▼───────┐  ┌──────▼───────┐  ┌──────▼───────┐    │
│  │ 连接管理      │  │ 隧道管理      │  │ 日志系统      │    │
│  └──────────────┘  └──────────────┘  └──────────────┘    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
                  Tunnox Server
```

---

## 四、命令行设计

### 4.1 顶层命令结构

```bash
tunnox [COMMAND] [OPTIONS]
```

#### 4.1.1 核心命令（新设计）

```bash
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 快速启动命令（最常用）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 交互式向导（新用户推荐）
tunnox                              # 启动向导模式

# HTTP 隧道（最常用）
tunnox http <port>                  # 快速创建 HTTP 隧道
tunnox http <port> --subdomain <name>  # 指定子域名
tunnox http <port> --domain <custom>   # 使用自定义域名

# TCP 隧道
tunnox tcp <port>                   # 快速创建 TCP 隧道
tunnox tcp <port> --remote-port <port>  # 指定远程端口

# UDP 隧道
tunnox udp <port>                   # 快速创建 UDP 隧道

# SOCKS 代理
tunnox socks                        # 创建 SOCKS5 代理隧道

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 配置文件模式（企业用户）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox start                        # 使用默认配置启动
tunnox start -c <config>            # 使用指定配置文件
tunnox start --daemon               # 守护进程模式
tunnox stop                         # 停止守护进程
tunnox restart                      # 重启守护进程
tunnox status                       # 查看运行状态

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 交互式 Shell（极客玩家）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox shell                        # 启动交互式 Shell

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 连接码功能（快速临时分享）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox code generate                # 生成连接码（TargetClient）
tunnox code use <code>              # 使用连接码（ListenClient）
tunnox code list                    # 列出我的连接码
tunnox code revoke <code>           # 撤销连接码

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 账户管理
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox login                        # 登录账户
tunnox logout                       # 登出账户
tunnox whoami                       # 查看当前用户
tunnox account                      # 账户信息

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 工具命令
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox config                       # 管理配置
tunnox config init                  # 生成配置文件模板
tunnox config show                  # 显示当前配置
tunnox config edit                  # 编辑配置文件

tunnox logs                         # 查看日志
tunnox logs -f                      # 实时跟踪日志
tunnox logs --tail 100              # 显示最后 100 行

tunnox version                      # 显示版本信息
tunnox update                       # 检查更新
tunnox help                         # 显示帮助
tunnox doctor                       # 诊断问题
```

### 4.2 全局参数

```bash
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 服务器配置（连接到哪个 Tunnox 服务器）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--server <address>      # Tunnox 服务器地址
                        # 默认: gw.tunnox.net (SaaS 官方服务器)
                        # 私服: your-server.com:8000
                        # 示例:
                        #   tunnox http 3000 --server my-server.com:8000

--transport <type>      # 传输协议（客户端到服务器）
                        # 可选: tcp / websocket / kcp / quic / auto
                        # 默认: auto (自动选择最佳协议)
                        # 使用场景:
                        #   tcp        - 直连，速度快，可能被防火墙阻止
                        #   websocket  - 基于 HTTP，穿透防火墙，兼容性好
                        #   kcp        - 基于 UDP，低延迟，抗丢包
                        #   quic       - 现代协议，快速+可靠，推荐
                        # 示例:
                        #   tunnox http 3000 --transport websocket

--region <name>         # 节点区域（仅 SaaS 生效）
                        # 可选: cn-beijing / cn-shanghai / us-west / auto
                        # 默认: auto (自动选择最近节点)
                        # 私服部署忽略此参数

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 认证配置
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--token <token>         # 认证 Token（从 console.tunnox.com 获取）
--auth-file <file>      # Token 文件路径 (默认: ~/.tunnox/auth.yml)
--anonymous             # 匿名模式（无需注册，临时使用）

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 日志配置
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--log-level <level>     # 日志级别: debug / info / warn / error
                        # 默认: info
--log-file <file>       # 日志文件路径
--log-format <format>   # 日志格式: text / json (默认: text)
--no-color              # 禁用颜色输出

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 其他配置
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--config -c <file>      # 配置文件路径
--quiet -q              # 静默模式（仅输出关键信息）
--verbose -v            # 详细模式（输出调试信息）
--yes -y                # 自动确认所有提示
--help -h               # 显示帮助
--version               # 显示版本
```

### 4.3 命令示例与说明

#### 4.3.1 HTTP 隧道（生成连接码）

**核心理念：** 通过连接码机制确保安全，不直接暴露公网地址

```bash
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 基础用法（默认 localhost）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

$ tunnox http 3000

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     ABC123
   目标服务:   http://localhost:3000
   激活期限:   10 分钟
   映射期限:   7 天
   过期时间:   2025-12-26 15:00:00

   💡 将连接码 ABC123 分享给需要访问的人
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 指定目标地址（局域网内其他设备）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

$ tunnox http 192.168.1.50:8080

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     DEF456
   目标服务:   http://192.168.1.50:8080
   激活期限:   10 分钟
   映射期限:   7 天

   💡 这将分享局域网内 192.168.1.50 机器的 Web 服务
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 完整参数（自定义有效期 + 名称）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

$ tunnox http 172.17.0.2:8080 \
    --activation-ttl 30 \
    --mapping-ttl 14 \
    --name "演示环境"

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     GHI789
   名称:       演示环境
   目标服务:   http://172.17.0.2:8080
   激活期限:   30 分钟
   映射期限:   14 天
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

地址格式:
  tunnox http <port>             # 默认 localhost:port
  tunnox http <host>:<port>      # 指定主机地址

选项:
  --activation-ttl <minutes>     # 激活有效期（分钟，默认 10）
  --mapping-ttl <days>           # 映射有效期（天，默认 7）
  --name <name>                  # 连接码名称（方便管理）

  # 传输层配置（高级）
  --server <address>             # 服务器地址（默认: gw.tunnox.net）
  --transport <type>             # 传输协议（默认: auto）
  --region <name>                # 节点区域（默认: auto）

使用场景:
  tunnox http 3000               # 本机开发服务器（React/Vue）
  tunnox http 192.168.1.50:8080  # 局域网 Web 服务
  tunnox http 172.17.0.2:3000    # Docker 容器 API
```

#### 4.3.2 TCP 隧道（生成连接码）

**核心理念：** 通过连接码机制确保安全，不直接暴露公网端口

```bash
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 基础用法（默认 localhost）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

$ tunnox tcp 22

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     ABC123
   目标服务:   tcp://localhost:22
   激活期限:   10 分钟
   映射期限:   7 天
   过期时间:   2025-12-26 15:00:00

   💡 将连接码 ABC123 分享给需要访问的人
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 指定目标地址（局域网内其他设备）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

$ tunnox tcp 10.51.0.7:22

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     DEF456
   目标服务:   tcp://10.51.0.7:22
   激活期限:   10 分钟
   映射期限:   7 天

   💡 这将分享局域网内 10.51.0.7 机器的 SSH 服务
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 完整参数（自定义有效期）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

$ tunnox tcp 192.168.1.100:3306 \
    --activation-ttl 30 \
    --mapping-ttl 14 \
    --name "公司数据库"

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     GHI789
   名称:       公司数据库
   目标服务:   tcp://192.168.1.100:3306
   激活期限:   30 分钟
   映射期限:   14 天
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

地址格式:
  tunnox tcp <port>              # 默认 localhost:port
  tunnox tcp <host>:<port>       # 指定主机地址

选项:
  --activation-ttl <minutes>     # 激活有效期（分钟，默认 10）
  --mapping-ttl <days>           # 映射有效期（天，默认 7）
  --name <name>                  # 连接码名称（方便管理）

使用场景:
  tunnox tcp 22                  # 本机 SSH
  tunnox tcp 10.51.0.7:22       # 局域网机器 SSH
  tunnox tcp 192.168.1.100:3306  # 局域网数据库
  tunnox tcp 172.17.0.2:6379     # Docker 容器 Redis
```

#### 4.3.3 UDP 隧道

```bash
# 基础用法
$ tunnox udp 5060

🔗 Tunnox UDP Tunnel
   Local:     localhost:5060
   Public:    udp://tunnox.com:15060

Options:
  --remote-port <port>    指定远程端口
```

#### 4.3.4 连接码功能（快速临时分享）

**使用场景：** 快速与他人建立临时隧道，无需对方注册账号

**两种角色：**
- **TargetClient（目标端）**：运行服务的一方，生成连接码
- **ListenClient（监听端）**：想访问服务的一方，使用连接码

##### 生成连接码（TargetClient）

```bash
$ tunnox code generate

🔑 Generate Connection Code
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

? 选择协议类型:
  ▸ TCP
    UDP
    SOCKS5

? 目标地址 (格式: host:port):
  ▸ localhost:22

? 激活有效期（分钟，默认 10）:
  ▸ 10

? 映射有效期（天，默认 7）:
  ▸ 7

🔄 正在生成连接码...

✅ 连接码已生成!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   连接码:     ABC123
   目标服务:   tcp://localhost:22
   激活期限:   10 分钟内激活有效
   映射期限:   激活后保持 7 天
   过期时间:   2025-12-26 15:00:00

   💡 将连接码 ABC123 分享给对方
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

##### 使用连接码（ListenClient）

```bash
$ tunnox code use ABC123

🔓 正在激活连接码...
✅ 连接码激活成功!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   目标服务:  tcp://localhost:22 (对方的 SSH)
   公网地址:  tcp://tunnox.com:23456

   连接命令:  ssh user@tunnox.com -p 23456

   📋 状态: 活跃
   ⏱ 剩余时间: 6 天 23 小时 50 分钟

   按 Ctrl+C 停止
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

##### 管理连接码

```bash
# 列出我的连接码
$ tunnox code list

Connection Codes
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CODE        TARGET                  STATUS      ACTIVATED BY   EXPIRES AT
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
ABC123      tcp://localhost:22      activated   client-20001   2025-12-26 15:00
DEF456      tcp://localhost:3306    available   -              2025-12-26 16:30
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Total: 2 codes

# 撤销连接码
$ tunnox code revoke ABC123

✅ 连接码 ABC123 已撤销
   已激活的映射将继续有效，但无法再次使用此连接码
```

##### 连接码 vs 直接映射对比

| 特性 | 直接映射 (`tunnox tcp 22`) | 连接码 (`tunnox code generate`) |
|------|---------------------------|----------------------------------|
| **使用场景** | 长期使用、固定服务 | 临时分享、快速协作 |
| **分享方式** | 分享公网地址（如 tcp://tunnox.com:12345） | 分享 6 位连接码（如 ABC123） |
| **有效期** | 长期有效（直到停止客户端） | 临时有效（激活期 10 分钟，映射期 7 天） |
| **对方要求** | 无（直接访问公网地址） | 需运行 Tunnox 客户端使用连接码 |
| **操作步骤** | 1 步（直接创建） | 2 步（生成码 → 使用码） |
| **适合人群** | 个人使用、团队固定访问 | 临时协作、客户演示、技术支持 |
| **安全性** | 任何人知道地址就能访问 | 只有拿到连接码的人能激活 |

**使用建议：**
```
长期稳定访问 → 使用 tunnox tcp/http/udp（直接映射）
临时快速分享 → 使用 tunnox code generate（连接码）
```

#### 4.3.5 配置文件模式

```bash
# 启动（使用默认配置）
$ tunnox start

✅ Tunnox started with config: ~/.tunnox/config.yml
   Loaded 3 tunnels:

   [1] web-app (HTTP)
       https://myapp.tunnox.com → http://localhost:3000

   [2] ssh (TCP)
       tcp://tunnox.com:12345 → localhost:22

   [3] mysql (TCP)
       tcp://tunnox.com:23456 → localhost:3306

# 指定配置文件
$ tunnox start -c /path/to/config.yml

# 守护进程模式
$ tunnox start --daemon

✅ Tunnox client started in daemon mode
   PID: 12345
   Config: ~/.tunnox/config.yml
   Logs: ~/.tunnox/logs/tunnox.log

   Run 'tunnox status' to check status
   Run 'tunnox logs -f' to view logs
   Run 'tunnox stop' to stop

# 查看状态
$ tunnox status

✅ Tunnox is running
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   PID:       12345
   Uptime:    2 days 5 hours 23 minutes
   Config:    ~/.tunnox/config.yml

   Tunnels:   3 active

   [1] web-app (HTTP) - Active
       https://myapp.tunnox.com → http://localhost:3000
       ↑ 1.2 GB  ↓ 3.5 GB

   [2] ssh (TCP) - Active
       tcp://tunnox.com:12345 → localhost:22
       ↑ 45 MB  ↓ 120 MB

   [3] mysql (TCP) - Active
       tcp://tunnox.com:23456 → localhost:3306
       ↑ 890 MB  ↓ 2.1 GB
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 停止
$ tunnox stop

🛑 Stopping Tunnox (PID: 12345)...
✅ Tunnox stopped

# 重启
$ tunnox restart

🔄 Restarting Tunnox...
✅ Tunnox restarted (PID: 12346)
```

#### 4.3.5 交互式 Shell

```bash
$ tunnox shell

┌─────────────────────────────────────────────────────────────┐
│   _____ _   _ _   _ _   _  _____  __                        │
│  |_   _| | | | \ | | \ | |/ _ \ \/ /                        │
│    | | | | | |  \| |  \| | | | \  /                         │
│    | | | |_| | |\  | |\  | |_| /  \                         │
│    |_|  \___/|_| \_|_| \_|\___/_/\_\                        │
│                                                              │
│  Type 'help' for available commands                         │
└─────────────────────────────────────────────────────────────┘

tunnox> help

Available Commands:
  tunnels                 List all active tunnels
  tunnel create           Create a new tunnel
  tunnel stop <id>        Stop a tunnel
  tunnel restart <id>     Restart a tunnel

  status                  Show connection status
  logs                    View recent logs

  config show             Show current configuration
  config set <key> <val>  Set configuration value

  account                 Show account information
  quota                   Show quota usage

  help                    Show this help
  exit                    Exit shell

tunnox> tunnels

Active Tunnels:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ID   Type   Local              Public
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  1    HTTP   localhost:3000     https://myapp.tunnox.com
  2    TCP    localhost:22       tcp://tunnox.com:12345
  3    TCP    localhost:3306     tcp://tunnox.com:23456
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox> tunnel create http 8080

✅ Tunnel created
   ID:     4
   Type:   HTTP
   Local:  http://localhost:8080
   Public: https://abc789.tunnox.com

tunnox> exit

Goodbye!
```

---

## 五、交互式向导

### 5.1 向导流程设计

#### 首次运行检测

```go
// 检测是否首次运行
if !configExists() {
    runWizard()
} else {
    runNormalMode()
}
```

#### 向导步骤

**步骤 1：欢迎界面**

```
┌─────────────────────────────────────────────────────────────┐
│ 🎉 欢迎使用 Tunnox!                                         │
│                                                              │
│ Tunnox 可以让您安全地将本地服务分享到互联网。               │
│                                                              │
│ 这是您第一次运行，让我们快速设置一下（只需 2 分钟）。       │
└─────────────────────────────────────────────────────────────┘

[继续]  [查看文档]  [跳过向导]
```

**步骤 2：选择使用模式**

```
? 您想如何使用 Tunnox?

  ▸ 快速体验（匿名模式，无需注册）
    推荐新用户，立即创建隧道，每次重启自动生成新凭证

    我已有账号（使用 Token 登录）
    适合已注册用户，保留隧道配置，享受更多功能

    查看帮助文档
```

**步骤 3a：匿名模式 - 选择服务类型**

```
? 您想分享什么服务?

  ▸ Web 应用（HTTP/HTTPS）
    适用于：React/Vue 开发服务器、API 服务、博客等

    SSH 服务器
    适用于：远程登录服务器、树莓派等

    数据库服务
    适用于：MySQL、PostgreSQL、MongoDB 等

    其他 TCP 服务
    适用于：游戏服务器、自定义协议等

    我不确定
    显示所有选项和详细说明
```

**步骤 4a：匿名模式 - 输入本地地址**

```
? 请输入您的本地服务地址:

  示例：localhost:3000  或  127.0.0.1:8080

  ▸ localhost:3000

  💡 提示：
  - 如果您的服务运行在本地，使用 localhost 或 127.0.0.1
  - 端口号是您的服务监听的端口（如 3000、8080）
```

**步骤 5a：匿名模式 - 创建隧道**

```
🔄 正在设置您的隧道...

   [✓] 连接到 Tunnox 服务器
   [✓] 分配公网地址
   [✓] 创建安全隧道

✅ 隧道创建成功!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   本地地址:  http://localhost:3000
   公网地址:  https://abc123.tunnox.com

   📋 分享上面的公网地址即可让任何人访问您的本地服务
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

? 接下来您想做什么?

  ▸ 保持隧道运行（推荐）
    按 Ctrl+C 可随时停止

    在浏览器中打开公网地址
    查看隧道是否正常工作

    创建另一个隧道
    同时分享多个服务

    保存配置文件
    下次直接加载，无需重新配置

    退出
```

**步骤 3b：认证模式 - 输入 Token**

```
? 请输入您的认证 Token:

  您可以在以下位置获取 Token:
  1. 访问 https://console.tunnox.com/tokens
  2. 点击"创建新 Token"
  3. 复制 Token 粘贴到下方

  ▸ eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

  [验证 Token]  [我还没有账号]
```

**步骤 4b：认证模式 - 验证成功**

```
✅ Token 验证成功!

   用户名:  john@example.com
   套餐:    Pro
   到期:    2025-12-31

   可用配额:
   - 隧道数量: 5 / 50
   - 本月流量: 120 GB / 1 TB

? 接下来您想做什么?

  ▸ 创建隧道
    快速创建一个新隧道

    加载配置文件
    使用之前保存的配置

    查看现有隧道
    管理您的所有隧道

    进入交互式 Shell
    高级用户模式
```

### 5.2 向导技术实现

```go
package wizard

import (
    "github.com/manifoldco/promptui"
    "github.com/fatih/color"
)

type Wizard struct {
    config *Config
}

func (w *Wizard) Run() error {
    // 欢迎界面
    w.showWelcome()

    // 选择模式
    mode, err := w.selectMode()
    if err != nil {
        return err
    }

    if mode == "anonymous" {
        return w.runAnonymousFlow()
    } else {
        return w.runAuthenticatedFlow()
    }
}

func (w *Wizard) selectMode() (string, error) {
    prompt := promptui.Select{
        Label: "您想如何使用 Tunnox?",
        Items: []string{
            "快速体验（匿名模式，无需注册）",
            "我已有账号（使用 Token 登录）",
            "查看帮助文档",
        },
        Templates: &promptui.SelectTemplates{
            Label:    "{{ . }}",
            Active:   "▸ {{ . | cyan }}",
            Inactive: "  {{ . }}",
            Selected: "✔ {{ . | green }}",
        },
    }

    idx, _, err := prompt.Run()
    if err != nil {
        return "", err
    }

    switch idx {
    case 0:
        return "anonymous", nil
    case 1:
        return "authenticated", nil
    case 2:
        w.showHelp()
        return w.selectMode() // 递归调用
    }

    return "", fmt.Errorf("invalid selection")
}

func (w *Wizard) runAnonymousFlow() error {
    // 选择服务类型
    serviceType, err := w.selectServiceType()
    if err != nil {
        return err
    }

    // 输入本地地址
    localAddr, err := w.inputLocalAddress(serviceType)
    if err != nil {
        return err
    }

    // 创建隧道
    tunnel, err := w.createTunnel(serviceType, localAddr)
    if err != nil {
        return err
    }

    // 显示结果
    w.showTunnelInfo(tunnel)

    // 下一步操作
    return w.nextSteps(tunnel)
}

// 使用 promptui 库实现美观的交互
```

---

## 六、配置文件设计

### 6.1 配置文件格式（YAML）

**默认路径**
- Linux/macOS: `~/.tunnox/config.yml`
- Windows: `%USERPROFILE%\.tunnox\config.yml`

**配置文件示例**

```yaml
# Tunnox Client Configuration
# 版本: 2.0

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 认证配置
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

auth:
  # 认证方式: token / anonymous
  mode: token

  # 认证 Token（从 console.tunnox.com 获取）
  token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

  # 或者引用外部文件
  # token_file: ~/.tunnox/token.txt

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 服务器配置（客户端连接到哪个 Tunnox 服务器）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

server:
  # 服务器地址
  # - 留空: 使用 SaaS 默认服务器（gw.tunnox.net）
  # - 自定义: 私有部署的服务器地址
  # 示例:
  #   address: ""                          # SaaS 模式（推荐新用户）
  #   address: "tunnox.company.com:8000"   # 企业私有部署
  address: ""

  # 传输协议（客户端到服务器）
  # tcp / websocket / kcp / quic / auto
  # - tcp:       直连，速度快，可能被防火墙阻止
  # - websocket: 基于 HTTP，穿透防火墙，兼容性好
  # - kcp:       基于 UDP，低延迟，抗丢包
  # - quic:      现代协议，快速+可靠，推荐
  # - auto:      自动选择最佳协议（默认）
  transport: auto

  # 节点区域（仅 SaaS 生效，私服忽略此配置）
  # auto / cn-beijing / cn-shanghai / us-west / ...
  region: auto

  # 心跳间隔（秒）
  heartbeat_interval: 30

  # 重连配置
  reconnect:
    enabled: true
    max_retries: 5
    initial_delay: 1s
    max_delay: 60s

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 隧道配置
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnels:
  # HTTP 隧道示例
  - name: web-app
    type: http
    local: localhost:3000
    subdomain: myapp           # 可选：自定义子域名
    # domain: api.example.com  # 可选：自定义域名
    auth: user:password        # 可选：HTTP Basic 认证
    inspect: true              # 可选：启用流量检查

  # TCP 隧道示例（SSH）
  - name: ssh
    type: tcp
    local: localhost:22
    remote_port: 2222          # 可选：指定远程端口（Pro 套餐）

  # TCP 隧道示例（MySQL）
  - name: mysql
    type: tcp
    local: localhost:3306

  # UDP 隧道示例
  - name: voip
    type: udp
    local: localhost:5060

  # SOCKS 代理示例
  - name: proxy
    type: socks
    local: localhost:1080

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 高级配置
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 加密配置
encryption:
  enabled: true
  algorithm: aes-256-gcm

# 压缩配置
compression:
  enabled: true
  level: 6                     # 1-9，越高压缩率越高但 CPU 占用越高

# 日志配置
log:
  # 日志级别: debug / info / warn / error
  level: info

  # 日志格式: text / json
  format: text

  # 日志文件路径（留空则输出到 stderr）
  file: ~/.tunnox/logs/client.log

  # 日志文件最大大小（MB）
  max_size: 100

  # 日志文件保留数量
  max_backups: 10

  # 日志文件保留天数
  max_age: 30

  # 颜色输出
  color: true

# 性能配置
performance:
  # 并发连接数
  max_connections: 100

  # 连接池大小
  connection_pool_size: 10

  # 缓冲区大小（KB）
  buffer_size: 32

# 监控配置
monitoring:
  # Prometheus metrics
  metrics:
    enabled: false
    port: 9090

  # 健康检查
  health_check:
    enabled: true
    port: 8080
```

### 6.2 配置文件管理命令

```bash
# 生成默认配置文件
$ tunnox config init

✅ Config file created: ~/.tunnox/config.yml

   Edit this file to configure your tunnels.
   Run 'tunnox config edit' to open in editor.

# 显示当前配置
$ tunnox config show

Current Configuration:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Config file: ~/.tunnox/config.yml

  Auth:
    Mode:  token
    User:  john@example.com

  Server:
    Address:  auto
    Protocol: auto
    Region:   auto

  Tunnels:  3 configured
    - web-app (HTTP)
    - ssh (TCP)
    - mysql (TCP)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 编辑配置文件（使用默认编辑器）
$ tunnox config edit

Opening ~/.tunnox/config.yml in nano...

# 验证配置文件
$ tunnox config validate

✅ Configuration is valid

Or if there are errors:

❌ Configuration has errors:

  Line 15: Invalid protocol 'http' (must be: tcp/websocket/kcp/quic/auto)
  Line 23: Missing required field 'local' in tunnel 'web-app'

# 设置单个配置项
$ tunnox config set log.level debug

✅ Configuration updated: log.level = debug

# 获取单个配置项
$ tunnox config get log.level

debug
```

### 6.3 配置文件模板

提供多种场景的配置文件模板：

```bash
$ tunnox config init --template <name>

Available templates:
  basic         - 基础配置（1个HTTP隧道）
  developer     - 开发者配置（HTTP + SSH）
  web-dev       - Web开发配置（多个前后端服务）
  database      - 数据库访问配置
  enterprise    - 企业部署配置（完整功能）
  minimal       - 最小化配置（仅必需项）
```

**示例：Web 开发模板**

```yaml
# Tunnox Config Template: Web Development
# 适用场景：前后端分离开发、多服务联调

auth:
  mode: token
  token: "YOUR_TOKEN_HERE"

tunnels:
  # 前端开发服务器
  - name: frontend
    type: http
    local: localhost:3000
    subdomain: myapp-dev

  # 后端 API 服务器
  - name: backend
    type: http
    local: localhost:8080
    subdomain: myapp-api-dev

  # WebSocket 服务
  - name: websocket
    type: http
    local: localhost:8081
    subdomain: myapp-ws-dev

log:
  level: info
  file: ~/.tunnox/logs/dev.log
```

---

## 七、日志系统设计

### 7.1 日志级别

| 级别 | 使用场景 | 颜色 | 示例 |
|------|----------|------|------|
| DEBUG | 调试信息、详细流程 | 灰色 | `[DEBUG] Sending heartbeat packet` |
| INFO | 正常运行信息 | 白色 | `[INFO] Tunnel created: https://abc.tunnox.com` |
| WARN | 警告信息、可恢复错误 | 黄色 | `[WARN] Connection lost, reconnecting...` |
| ERROR | 错误信息、需要关注 | 红色 | `[ERROR] Failed to create tunnel: quota exceeded` |

### 7.2 日志格式

#### 文本格式（默认）

```
2025-12-26 14:30:15.123 [INFO] Tunnox client starting...
2025-12-26 14:30:15.456 [INFO] Connecting to server: auto
2025-12-26 14:30:16.789 [INFO] Connected successfully (protocol: quic, region: cn-beijing)
2025-12-26 14:30:17.012 [INFO] Tunnel created: web-app (HTTP)
2025-12-26 14:30:17.013 [INFO]   Local:  http://localhost:3000
2025-12-26 14:30:17.014 [INFO]   Public: https://myapp.tunnox.com
2025-12-26 14:30:20.123 [INFO] Incoming request: GET / from 123.45.67.89
2025-12-26 14:30:25.456 [WARN] Connection unstable (latency: 250ms)
2025-12-26 14:30:30.789 [ERROR] Failed to forward request: connection timeout
```

#### JSON 格式（适合日志分析）

```json
{"time":"2025-12-26T14:30:15.123Z","level":"info","msg":"Tunnox client starting..."}
{"time":"2025-12-26T14:30:15.456Z","level":"info","msg":"Connecting to server","server":"auto"}
{"time":"2025-12-26T14:30:16.789Z","level":"info","msg":"Connected successfully","protocol":"quic","region":"cn-beijing"}
{"time":"2025-12-26T14:30:17.012Z","level":"info","msg":"Tunnel created","name":"web-app","type":"http","local":"http://localhost:3000","public":"https://myapp.tunnox.com"}
{"time":"2025-12-26T14:30:20.123Z","level":"info","msg":"Incoming request","method":"GET","path":"/","remote_ip":"123.45.67.89"}
{"time":"2025-12-26T14:30:25.456Z","level":"warn","msg":"Connection unstable","latency_ms":250}
{"time":"2025-12-26T14:30:30.789Z","level":"error","msg":"Failed to forward request","error":"connection timeout"}
```

### 7.3 颜色高亮

```go
package log

import (
    "github.com/fatih/color"
)

var (
    debugColor = color.New(color.FgHiBlack)
    infoColor  = color.New(color.FgWhite)
    warnColor  = color.New(color.FgYellow)
    errorColor = color.New(color.FgRed)
)

func Debug(format string, args ...interface{}) {
    if logLevel >= LevelDebug {
        debugColor.Printf("[DEBUG] " + format + "\n", args...)
    }
}

func Info(format string, args ...interface{}) {
    if logLevel >= LevelInfo {
        infoColor.Printf("[INFO] " + format + "\n", args...)
    }
}

func Warn(format string, args ...interface{}) {
    if logLevel >= LevelWarn {
        warnColor.Printf("[WARN] " + format + "\n", args...)
    }
}

func Error(format string, args ...interface{}) {
    if logLevel >= LevelError {
        errorColor.Printf("[ERROR] " + format + "\n", args...)
    }
}
```

### 7.4 结构化日志

```go
// 使用结构化日志记录关键事件
log.WithFields(log.Fields{
    "tunnel_id": tunnel.ID,
    "type": tunnel.Type,
    "local": tunnel.Local,
    "public": tunnel.Public,
}).Info("Tunnel created")

// 记录请求信息
log.WithFields(log.Fields{
    "method": req.Method,
    "path": req.Path,
    "remote_ip": req.RemoteIP,
    "user_agent": req.UserAgent,
    "response_code": resp.StatusCode,
    "response_time_ms": resp.Duration.Milliseconds(),
}).Info("Request processed")

// 记录错误信息
log.WithFields(log.Fields{
    "error": err.Error(),
    "tunnel_id": tunnel.ID,
    "retry_count": retryCount,
}).Error("Failed to forward request")
```

### 7.5 日志查看命令

```bash
# 查看最新日志（默认 50 行）
$ tunnox logs

2025-12-26 14:30:15 [INFO] Tunnox client starting...
2025-12-26 14:30:16 [INFO] Connected successfully
2025-12-26 14:30:17 [INFO] Tunnel created: https://myapp.tunnox.com
...

# 实时跟踪日志
$ tunnox logs -f

2025-12-26 14:30:15 [INFO] Tunnox client starting...
2025-12-26 14:30:16 [INFO] Connected successfully
^C

# 查看最后 100 行
$ tunnox logs --tail 100

# 按级别过滤
$ tunnox logs --level error

2025-12-26 14:25:30 [ERROR] Failed to create tunnel: quota exceeded
2025-12-26 14:28:45 [ERROR] Connection timeout

# 搜索关键词
$ tunnox logs --grep "tunnel"

2025-12-26 14:30:17 [INFO] Tunnel created: https://myapp.tunnox.com
2025-12-26 14:35:22 [INFO] Tunnel stopped: https://myapp.tunnox.com

# 导出日志
$ tunnox logs --export logs-2025-12-26.txt

✅ Logs exported to logs-2025-12-26.txt
```

---

## 八、错误处理与提示

### 8.1 错误分类

| 错误类型 | 处理方式 | 示例 |
|----------|----------|------|
| **用户错误** | 提示如何修复 | 配置文件格式错误、参数缺失 |
| **网络错误** | 自动重试 + 提示 | 连接超时、DNS 解析失败 |
| **服务端错误** | 提示联系支持 | 服务器内部错误 |
| **配额错误** | 提示升级套餐 | 隧道数量超限、流量超限 |

### 8.2 友好的错误提示

#### 错误格式

```
❌ [错误类型] 错误描述

   原因: 详细原因说明

   解决方案:
   1. 第一步操作
   2. 第二步操作

   相关文档: https://docs.tunnox.com/errors/E001
```

#### 示例 1：配置文件错误

```bash
$ tunnox start

❌ [配置错误] 配置文件格式无效

   文件位置: ~/.tunnox/config.yml
   错误行数: 第 15 行

   原因: YAML 语法错误，缩进不正确

   15 |   tunnels:
   16 |  - name: web-app     ← 缩进应为 2 个空格，实际为 1 个
   17 |    type: http

   解决方案:
   1. 使用文本编辑器打开配置文件
   2. 确保缩进使用 2 个空格（不要使用 Tab）
   3. 或运行 'tunnox config validate' 检查配置

   相关文档: https://docs.tunnox.com/config
```

#### 示例 2：网络连接错误

```bash
$ tunnox http 3000

🔗 正在连接到 Tunnox 服务器...

❌ [网络错误] 无法连接到服务器

   服务器地址: https://gw.tunnox.net
   错误信息: connection timeout

   原因: 可能是网络问题或防火墙阻止

   解决方案:
   1. 检查网络连接是否正常
   2. 尝试使用其他协议: tunnox http 3000 --protocol tcp
   3. 检查防火墙设置，确保允许出站连接
   4. 如果在企业网络，联系网络管理员

   自动诊断: 运行 'tunnox doctor' 进行网络诊断
```

#### 示例 3：配额超限错误

```bash
$ tunnox http 3000

❌ [配额错误] 隧道数量已达上限

   当前套餐: Free
   隧道限制: 2
   已创建: 2

   原因: 您的免费套餐最多只能创建 2 个隧道

   解决方案:
   1. 停止现有隧道: tunnox tunnel stop <id>
   2. 或升级到 Basic 套餐（¥29/月，10 个隧道）

   查看现有隧道: tunnox tunnels
   升级套餐: https://console.tunnox.com/pricing
```

#### 示例 4：认证错误

```bash
$ tunnox http 3000

❌ [认证错误] Token 无效或已过期

   原因: 您的认证 Token 无法验证

   解决方案:
   1. 访问 https://console.tunnox.com/tokens
   2. 创建新的 Token
   3. 运行 'tunnox login' 重新登录

   或使用匿名模式: tunnox http 3000 --anonymous
```

### 8.3 诊断工具

```bash
$ tunnox doctor

🔍 Tunnox 诊断工具

正在检查系统配置...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[✓] 配置文件
    位置: ~/.tunnox/config.yml
    格式: 有效

[✓] 认证
    模式: Token
    用户: john@example.com
    状态: 已认证

[✓] 网络连接
    Tunnox 服务器: 可达
    延迟: 45ms
    协议: QUIC

[✓] 本地服务
    localhost:3000: 可访问
    响应时间: 12ms

[⚠] 防火墙
    检测到防火墙，可能影响某些协议
    建议: 允许 UDP 端口 443（QUIC 协议）

[✓] 系统资源
    CPU: 15%
    内存: 120 MB / 8 GB
    磁盘: 500 MB 可用

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

诊断结果: 1 个警告

建议:
  - 配置防火墙允许 UDP 443 端口以使用 QUIC 协议

详细报告已保存到: ~/.tunnox/doctor-report-2025-12-26.txt
```

---

## 九、安装与分发

### 9.1 安装方式

#### Windows

**方式 1：一键安装包（推荐新用户）**
```bash
# 下载 tunnox-installer-windows.exe
# 双击运行，按提示安装
# 安装后自动添加到 PATH
```

**方式 2：Chocolatey**
```bash
choco install tunnox
```

**方式 3：Scoop**
```bash
scoop install tunnox
```

**方式 4：手动安装**
```bash
# 下载 tunnox-windows-amd64.zip
# 解压到 C:\Program Files\Tunnox
# 添加到 PATH 环境变量
```

#### macOS

**方式 1：Homebrew（推荐）**
```bash
brew install tunnox

# 升级
brew upgrade tunnox
```

**方式 2：一键安装脚本**
```bash
curl -fsSL https://get.tunnox.com | sh
```

**方式 3：下载 .app 文件**
```bash
# 下载 Tunnox.app
# 拖到 Applications 文件夹
# 首次运行：右键 → 打开
```

#### Linux

**方式 1：一键安装脚本（推荐）**
```bash
curl -fsSL https://get.tunnox.com | sh

# 或使用 wget
wget -qO- https://get.tunnox.com | sh
```

**方式 2：包管理器**
```bash
# Debian/Ubuntu
sudo apt install tunnox

# Fedora/RHEL
sudo dnf install tunnox

# Arch Linux
yay -S tunnox
```

**方式 3：Docker**
```bash
docker run -it --rm tunnox/client http 3000
```

**方式 4：手动安装**
```bash
# 下载二进制文件
wget https://github.com/tunnox/tunnox/releases/download/v2.0.0/tunnox-linux-amd64

# 添加执行权限
chmod +x tunnox-linux-amd64

# 移动到 PATH
sudo mv tunnox-linux-amd64 /usr/local/bin/tunnox
```

### 9.2 自动更新

```bash
# 检查更新
$ tunnox update check

📦 Tunnox Update Check
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   当前版本: v2.0.0
   最新版本: v2.1.0

   更新内容:
   - 新增: HTTP/3 协议支持
   - 优化: 连接速度提升 30%
   - 修复: Windows 下日志乱码问题

   发布日期: 2025-12-20

   运行 'tunnox update install' 安装更新

# 安装更新
$ tunnox update install

📦 正在下载更新...
[████████████████████] 100% (12.5 MB / 12.5 MB)

✅ 更新下载完成
🔄 正在安装...
✅ 更新安装成功

   旧版本: v2.0.0
   新版本: v2.1.0

   运行 'tunnox version' 确认版本

# 自动更新（后台检查）
$ tunnox start --auto-update

✅ 自动更新已启用
   每天检查一次更新，发现新版本自动下载并提示
```

### 9.3 系统服务（Linux/macOS）

#### systemd 服务（Linux）

```bash
# 安装为系统服务
$ sudo tunnox service install

✅ Tunnox service installed
   Service name: tunnox
   Config: /etc/tunnox/config.yml

   Commands:
   sudo systemctl start tunnox       # 启动
   sudo systemctl stop tunnox        # 停止
   sudo systemctl restart tunnox     # 重启
   sudo systemctl status tunnox      # 状态
   sudo systemctl enable tunnox      # 开机自启

# 卸载服务
$ sudo tunnox service uninstall
```

**服务文件** (`/etc/systemd/system/tunnox.service`)
```ini
[Unit]
Description=Tunnox Client
After=network.target

[Service]
Type=simple
User=tunnox
WorkingDirectory=/var/lib/tunnox
ExecStart=/usr/local/bin/tunnox start --config /etc/tunnox/config.yml --daemon
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

#### launchd 服务（macOS）

```bash
# 安装为用户服务
$ tunnox service install

✅ Tunnox service installed
   Service name: com.tunnox.client
   Config: ~/Library/Application Support/Tunnox/config.yml

   Commands:
   launchctl load ~/Library/LaunchAgents/com.tunnox.client.plist
   launchctl unload ~/Library/LaunchAgents/com.tunnox.client.plist
```

**plist 文件** (`~/Library/LaunchAgents/com.tunnox.client.plist`)
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.tunnox.client</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/tunnox</string>
        <string>start</string>
        <string>--daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

---

## 十、进阶功能

### 10.1 流量检查（Web UI）

```bash
$ tunnox http 3000 --inspect

🔗 Tunnox HTTP Tunnel
   Local:     http://localhost:3000
   Public:    https://abc123.tunnox.com
   Inspector: http://localhost:4040

   💡 打开 Inspector 查看实时流量
```

**Inspector 功能**
- 请求/响应详情
- 时间线视图
- 重放请求
- 流量统计
- 错误日志

### 10.2 自定义域名

```bash
$ tunnox http 3000 --domain api.example.com

🔗 Tunnox HTTP Tunnel
   Local:     http://localhost:3000
   Public:    https://api.example.com

   ⚠️ 请配置 DNS CNAME 记录:

   api.example.com  →  tunnox-proxy.tunnox.com

   DNS 配置后，自动申请 SSL 证书（Let's Encrypt）

   验证状态: 等待 DNS 传播...
```

### 10.3 API 集成

```bash
# 获取 API Token
$ tunnox token create --name "My App"

✅ API Token created
   Name:  My App
   Token: tox_1234567890abcdef

   保存此 Token，不会再次显示

# 使用 API
curl -X POST https://api.tunnox.com/v1/tunnels \
  -H "Authorization: Bearer tox_1234567890abcdef" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "http",
    "local": "localhost:3000",
    "subdomain": "myapp"
  }'
```

### 10.4 Webhook 回调

```yaml
# config.yml
webhooks:
  - url: https://your-server.com/webhook
    events:
      - tunnel.created
      - tunnel.closed
      - connection.opened
      - connection.closed
    secret: your-webhook-secret
```

**Webhook 事件**
```json
{
  "event": "tunnel.created",
  "timestamp": "2025-12-26T14:30:17Z",
  "data": {
    "tunnel_id": "abc123",
    "type": "http",
    "local": "localhost:3000",
    "public": "https://myapp.tunnox.com"
  }
}
```

---

## 附录：对比现有实现

### 现有实现的问题

| 问题 | 现状 | 改进方案 |
|------|------|----------|
| **命令行参数混乱** | `-p`, `-s`, `-id`, `-token`, `-device`, `-anonymous`... | 简化为 `tunnox http 3000` |
| **新用户门槛高** | 需要理解协议、配置文件 | 交互式向导，零配置启动 |
| **错误提示不友好** | 仅显示错误信息 | 提供原因 + 解决方案 + 文档链接 |
| **日志不清晰** | 混杂在一起 | 分级日志、颜色高亮、结构化 |
| **缺少快捷命令** | 只有 `-daemon` 模式和 CLI 模式 | 添加 `http`, `tcp`, `start` 等命令 |
| **配置文件复杂** | 需要理解所有字段 | 提供模板 + 注释 + 验证 |
| **安装不方便** | 需要手动下载、配置 PATH | 一键安装脚本、包管理器 |
| **缺少诊断工具** | 需要手动排查 | `tunnox doctor` 自动诊断 |

### 迁移方案

**向后兼容**
```bash
# 旧命令仍然支持（显示废弃警告）
$ tunnox-client -p quic -s localhost:7003 -anonymous

⚠️ 警告: 此命令格式已废弃，将在 v3.0 移除

   推荐使用新格式:
   tunnox http 3000

   或查看帮助: tunnox help
```

**平滑过渡**
1. v2.0：新旧命令并存，旧命令显示警告
2. v2.5：旧命令标记为废弃
3. v3.0：移除旧命令

---

## 术语表

为帮助用户更好地理解 Tunnox 的各种概念，这里提供一个完整的术语表：

### 核心概念

**业务协议 (Tunnel Type)**
- 指您要转发的**本地服务类型**
- HTTP: Web 应用、API 服务、前端开发服务器
- TCP: SSH、数据库（MySQL/PostgreSQL）、游戏服务器、任意 TCP 服务
- UDP: 语音通话、视频流、游戏、DNS
- SOCKS: 代理服务

**传输协议 (Transport Protocol)**
- 指 Tunnox 客户端如何**连接到服务器**
- TCP: 直连，速度快，可能被防火墙阻止
- WebSocket: 基于 HTTP，兼容性好，穿透防火墙
- KCP: 基于 UDP，低延迟，抗丢包，适合移动网络
- QUIC: 现代协议，快速+可靠，Google 开发，推荐
- Auto: 自动选择最佳协议（默认）

**服务器部署模式**
- SaaS: 使用 Tunnox 官方托管服务器（gw.tunnox.net）
  - 优点：开箱即用、无需维护、全球节点
  - 适合：个人开发者、小团队、快速测试
- 私服: 企业自己部署的 Tunnox 服务器
  - 优点：数据安全、可控性强、自定义配置
  - 适合：企业用户、敏感数据、内网环境

### 命令参数

| 参数 | 层级 | 说明 | 示例 |
|------|------|------|------|
| `tunnox http/tcp/udp` | 业务层 | 指定要转发的服务类型 | `tunnox http 3000` |
| `--server` | 传输层 | 指定服务器地址 | `--server my-server.com:8000` |
| `--transport` | 传输层 | 指定传输协议 | `--transport quic` |
| `--region` | 传输层 | 指定节点区域（仅 SaaS） | `--region cn-beijing` |
| `--subdomain` | 业务层 | 自定义子域名（仅 HTTP） | `--subdomain myapp` |
| `--remote-port` | 业务层 | 指定远程端口（仅 TCP） | `--remote-port 2222` |

### 实际示例解读

```bash
tunnox http 3000 --server company.com:8000 --transport quic --subdomain api
```

**逐层解读:**

```
【业务层】tunnox http 3000
  ↓ 要做什么？
  转发本地 3000 端口的 HTTP 服务（比如 React 开发服务器）

【业务层】--subdomain api
  ↓ HTTP 专属配置
  使用子域名 api（访问地址: https://api.company.com）

【传输层】--server company.com:8000
  ↓ 连接到哪？
  企业私有部署的 Tunnox 服务器（不是 SaaS）

【传输层】--transport quic
  ↓ 怎么连接？
  使用 QUIC 协议连接服务器（快速、可靠）

【结果】
  工作流程:
  1. 用户本地 HTTP 服务: localhost:3000
  2. 客户端用 QUIC 连接到: company.com:8000
  3. 公网访问地址: https://api.company.com
  4. 访问者访问 https://api.company.com → 转发到 localhost:3000
```

### 常见混淆点

**Q: `tunnox http` 和 `--transport websocket` 有什么区别？**
```
tunnox http          → 业务协议：转发 HTTP 服务
--transport websocket → 传输协议：用 WebSocket 连接服务器

完全不同的层面！
```

**Q: 为什么有 `tunnox http` 但 Tunnox 服务器不提供 HTTP 协议？**
```
Tunnox 服务器同时支持两层协议：

业务层: HTTP/TCP/UDP/SOCKS（转发什么服务）
传输层: TCP/WebSocket/KCP/QUIC（客户端怎么连服务器）

`tunnox http` 是业务层，表示转发 HTTP 服务
服务器确实支持转发 HTTP 服务！
```

**Q: 什么时候用 SaaS，什么时候用私服？**
```
使用 SaaS (gw.tunnox.net):
  ✅ 快速测试、个人项目
  ✅ 不想维护服务器
  ✅ 需要全球节点

使用私服 (--server your-server.com):
  ✅ 企业内网环境
  ✅ 敏感数据不能外传
  ✅ 需要自定义配置
  ✅ 高可用、高性能需求
```

---

## 总结

本设计方案的核心思想：

1. **渐进式体验**：从新手向导 → 快捷命令 → 配置文件 → 高级 API
2. **约定优于配置**：默认值覆盖 90% 场景，只在必要时才要求配置
3. **友好的错误提示**：不只说错误，还要说怎么修复
4. **跨平台一致**：Windows/macOS/Linux 统一体验
5. **专业可靠**：满足企业用户的稳定性、可监控性需求

下一步行动：
1. 实现交互式向导（使用 promptui/bubbletea）
2. 重构命令行结构（使用 cobra）
3. 优化日志系统（使用 zap/logrus）
4. 完善配置文件（使用 viper）
5. 编写安装脚本和打包脚本
