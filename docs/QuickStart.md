# Tunnox Core 快速开始指南

本指南将帮助你在 5 分钟内快速上手 Tunnox Core，无需任何外部依赖。

## 核心概念

Tunnox Core 是一个内网穿透工具，由三个组件组成：

- **Server（服务端）**：部署在有公网 IP 的机器上，负责转发数据
- **Target Client（目标端）**：部署在有服务的机器上（如数据库服务器）
- **Listen Client（源端）**：部署在需要访问服务的机器上（如你的笔记本）

**工作原理**：
```
Listen Client (本地) → Server (公网) → Target Client (内网) → 目标服务
```

## 快速开始

### 步骤 1：编译

```bash
git clone https://github.com/your-org/tunnox-core.git
cd tunnox-core

# 编译服务端和客户端
go build -o bin/tunnox-server ./cmd/server
go build -o bin/tunnox-client ./cmd/client
```

### 步骤 2：启动服务端

在有公网 IP 的机器上：

```bash
./bin/tunnox-server
```

服务端会自动启动，默认监听：
- TCP: 8000
- WebSocket: 8443
- KCP: 8000 (基于 UDP)
- QUIC: 443
- HTTP Long Polling: 9000 (Management API)

### 步骤 3：启动目标端客户端

在有服务的机器上（如内网数据库服务器）：

```bash
./bin/tunnox-client -s <服务端IP>:8000 -p tcp -anonymous
```


进入交互式界面后，生成连接码：

```bash
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3306
✅ Connection code generated: mysql-abc-123
```

### 步骤 4：启动源端客户端

在你的本地机器上：

```bash
./bin/tunnox-client -s <服务端IP>:8000 -p tcp -anonymous
```

进入交互式界面后，使用连接码：

```bash
tunnox> use-code mysql-abc-123
Local Listen Address: 127.0.0.1:13306
✅ Mapping created successfully
```

### 步骤 5：访问服务

现在可以通过本地端口访问远程服务：

```bash
mysql -h 127.0.0.1 -P 13306 -u root -p
```

## 常用场景

### 场景 1：远程访问家里的 NAS

1. 在 NAS 上启动目标端客户端
2. 生成连接码（目标地址：localhost:5000）
3. 在外网机器上启动源端客户端
4. 使用连接码创建映射
5. 通过本地端口访问 NAS

### 场景 2：临时分享本地开发的 Web 服务

1. 在开发机器上启动目标端（目标地址：localhost:3000）
2. 生成连接码分享给同事
3. 同事使用连接码创建映射
4. 同事通过本地端口访问你的服务

### 场景 3：通过 SOCKS5 代理访问内网

1. 在内网机器上启动目标端
2. 生成 SOCKS5 连接码（协议选择 SOCKS5）
3. 在外网机器上使用连接码
4. 配置浏览器或应用使用 SOCKS5 代理（127.0.0.1:1080）

## 守护进程模式

如果需要在后台运行客户端（不需要交互式界面）：

```bash
# 启动守护进程模式
./bin/tunnox-client -s <服务端IP>:8000 -p tcp -anonymous -daemon
```

## 配置文件（可选）

如果不想每次都输入命令行参数，可以创建配置文件：

**client-config.yaml**:
```yaml
anonymous: true
device_id: "my-device"

server:
  address: "server-ip:8000"
  protocol: "tcp"

log:
  level: "info"
  output: "file"
  file: "/tmp/tunnox-client.log"
```

使用配置文件启动：
```bash
./bin/tunnox-client -config client-config.yaml
```

## 常见问题

**Q: 需要安装数据库或 Redis 吗？**
A: 不需要，Tunnox Core 默认使用内存存储，零依赖启动。

**Q: 如何选择传输协议？**
A: TCP 最稳定，推荐日常使用；KCP 低延迟，适合实时应用；QUIC 性能更好，适合移动网络；WebSocket 可穿透防火墙；HTTP Long Polling 适合严格防火墙环境。

**Q: 连接码有效期多久？**
A: 默认 24 小时，使用后自动失效。

**Q: 如何查看日志？**
A: 服务端日志：`~/logs/server.log`，客户端日志：`/tmp/tunnox-client.log`

**Q: 如何停止服务？**
A: 在交互式模式下输入 `exit`，或按 Ctrl+C。

## 下一步

- 查看 [README.md](../README.md) 了解更多功能
- 查看 [ARCHITECTURE_DESIGN_V2.2.md](ARCHITECTURE_DESIGN_V2.2.md) 了解架构设计
- 查看 [MANAGEMENT_API.md](MANAGEMENT_API.md) 了解 API 接口

## 技术支持

- GitHub Issues: https://github.com/your-org/tunnox-core/issues
- 文档: https://github.com/your-org/tunnox-core/docs
