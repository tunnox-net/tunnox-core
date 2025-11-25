# Tunnox Client CLI

Tunnox 客户端命令行工具，支持**双连接模型**（指令连接 + 映射连接）。

## 📦 功能特性

✅ **指令连接**（Control Connection）
- JWT 认证
- 长连接保持
- 接收服务器命令和配置推送

✅ **映射连接**（Tunnel Connection）
- 基于 `secret_key` 独立认证
- 按需建立，纯数据透传
- 支持并发多条映射连接

✅ **本地端口映射**
- 自动监听本地端口
- 透明转发到远程服务
- 支持 TCP 协议（MySQL、PostgreSQL、Redis、HTTP 等）

## 🚀 快速开始

### 1. 准备配置文件

```bash
cd cmd/client
cp client-config.example.yaml client-config.yaml
```

### 2. 修改配置

编辑 `client-config.yaml`：

```yaml
client_id: 600000001                          # 你的客户端 ID
auth_token: "eyJhbGciOiJIUzI1NiIsInR5cCI..."  # JWT 认证令牌

server:
  address: "localhost:7000"                    # 服务器地址
  protocol: "tcp"
```

**注意**：映射配置由服务器通过指令连接自动推送，无需在配置文件中配置！

### 3. 启动客户端

```bash
# 使用默认配置文件 client-config.yaml
go run main.go

# 或指定配置文件
go run main.go -config my-config.yaml

# 构建后运行
go build -o tunnox-client main.go
./tunnox-client
```

### 4. 接收映射配置

客户端启动后，会通过指令连接从服务器接收映射配置。服务器会推送：
- 映射 ID (`mapping_id`)
- 映射秘钥 (`secret_key`)
- 本地监听端口 (`local_port`)
- 目标主机和端口 (`target_host`, `target_port`)

客户端收到配置后，会自动在本地端口监听。

### 5. 使用映射

映射建立后，直接连接本地端口即可访问远程服务。例如，如果服务器推送了 MySQL 映射到本地 3306 端口：

```bash
# 直接连接本地端口即可访问远程 MySQL
mysql -h 127.0.0.1 -P 3306 -u root -p
```

## 📖 配置说明

### 客户端标识

| 字段 | 说明 | 示例 |
|------|------|------|
| `client_id` | 客户端 ID（8位数字） | `600000001` |
| `auth_token` | JWT 认证令牌 | `"eyJ..."` |

**获取方式**：
- 从服务器管理平台注册客户端获取
- 或使用 Management API 创建客户端

### 服务器配置

| 字段 | 说明 | 示例 |
|------|------|------|
| `server.address` | 服务器地址和端口 | `"localhost:7000"` |
| `server.protocol` | 连接协议 | `"tcp"` / `"websocket"` / `"quic"` |

### 映射配置（由服务器推送）

映射配置由云控平台统一管理，通过指令连接动态推送到客户端：

| 字段 | 说明 | 示例 |
|------|------|------|
| `mapping_id` | 映射 ID（由服务器生成） | `"pm-mysql-001"` |
| `secret_key` | 映射秘钥（由服务器生成） | `"sk_mapping_abc123..."` |
| `local_port` | 本地监听端口（由服务器指定） | `3306` |
| `target_host` | 目标主机 | `"localhost"` |
| `target_port` | 目标端口 | `3306` |

**工作流程**：
1. 在云控平台创建映射（指定源客户端、目标客户端、端口等）
2. 服务器通过指令连接推送映射配置到源客户端
3. 客户端自动在本地端口监听
4. 用户连接本地端口时，客户端建立映射连接并透传数据

## 🔐 双连接模型

### 指令连接（Control Connection）

- **用途**：命令传输、配置推送、心跳保活
- **认证**：JWT Token
- **生命周期**：长连接（客户端在线期间一直保持）
- **数量**：每个客户端 **1 条**

### 映射连接（Tunnel Connection）

- **用途**：纯数据透传
- **认证**：每条连接独立验证 `secret_key`
- **生命周期**：按需建立（用户连接时），用户断开时关闭
- **数量**：可以有**多条并发**连接（每个用户请求对应 1 条）

## 📊 工作流程

```
用户应用 → 本地端口 (3306) → ClientA → ServerA → ServerB → ClientB → MySQL
                 ↑                   ↑                    ↑
            本地监听          指令连接(1条)        映射连接(N条)
                            JWT 认证           secret_key 认证
```

### 详细流程

1. **客户端启动**
   - 连接到服务器（TCP）
   - 发送握手请求（携带 JWT Token）
   - 建立指令连接（长连接）
   - 等待服务器推送映射配置

2. **接收映射配置**
   - 服务器通过指令连接推送映射配置
   - 客户端解析配置（mapping_id、secret_key、local_port 等）
   - 在本地端口监听（如 3306）
   - 等待用户连接

3. **用户连接**
   - 用户应用连接到本地端口（如 `mysql -h 127.0.0.1 -P 3306`）
   - 客户端建立新的 TCP 连接到服务器（映射连接）
   - 发送 `TunnelOpen` 包（携带 `mapping_id` + `secret_key`）
   - 服务器验证秘钥，建立隧道
   - 开始透传数据

4. **数据传输**
   - 用户应用 ↔ 本地端口 ↔ 客户端 ↔ 服务器 ↔ 目标客户端 ↔ 目标服务
   - 全程透传，无协议解析

5. **连接关闭**
   - 用户断开连接
   - 客户端发送 `TunnelClose` 包
   - 映射连接关闭

6. **配置更新**
   - 云控平台修改映射配置
   - 服务器推送新配置到客户端
   - 客户端动态更新本地监听

## 🛠️ 命令行参数

```bash
./tunnox-client -config <配置文件路径>
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-config` | 配置文件路径 | `client-config.yaml` |

## 📝 日志

客户端日志输出到 `client.log` 文件，日志级别为 `debug`。

可以通过查看日志了解连接状态：

```bash
tail -f client.log
```

## ❓ 常见问题

### 1. 连接失败

**问题**：`Failed to connect: dial tcp: connection refused`

**解决**：
- 检查服务器地址和端口是否正确
- 确认服务器已启动
- 检查防火墙设置

### 2. 认证失败

**问题**：`Handshake failed: invalid token`

**解决**：
- 检查 `auth_token` 是否正确
- 确认 JWT Token 未过期
- 检查 `client_id` 是否匹配

### 3. 未收到映射配置

**问题**：客户端启动后一直等待，没有开始监听本地端口

**解决**：
- 检查云控平台是否已创建映射并分配给当前客户端
- 确认映射状态为 `active`
- 查看客户端日志，确认指令连接已建立
- 在云控平台手动触发配置推送

### 4. 映射连接失败

**问题**：`Tunnel open failed: invalid secret key`

**解决**：
- 这通常是服务器推送的配置有问题
- 检查云控平台的映射配置是否正确
- 尝试重启客户端，重新接收配置
- 联系管理员检查服务器端配置

### 5. 本地端口被占用

**问题**：`Failed to listen on :3306: address already in use`

**解决**：
- 检查本地端口是否已被其他程序占用
- 停止占用端口的程序
- 或在云控平台修改映射配置，使用其他可用端口

## 🔧 开发调试

### 编译

```bash
go build -o tunnox-client main.go
```

### 交叉编译

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o tunnox-client-linux-amd64 main.go

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o tunnox-client-windows-amd64.exe main.go

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o tunnox-client-darwin-arm64 main.go
```

## 📄 许可证

与 Tunnox Core 项目保持一致。

