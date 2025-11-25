# SOCKS5 Adapter 使用指南

## 概述

SOCKS5 Adapter 实现了完整的 SOCKS5 代理服务器功能，可以作为本地代理接收用户请求，并通过隧道转发到远端执行。

## 应用场景

```
User (浏览器/应用)
  ↓ (设置系统代理: localhost:8888)
Client A (SOCKS5 服务器: 监听 8888)
  ↓ (通过隧道协议)
Server A → Server B
  ↓ (隧道转发)
Client B (实际出口节点)
  ↓ (实际发起连接)
目标网站 (google.com, etc.)
```

**功能说明：**
- User 设置系统代理指向 Client A 的 8888 端口
- Client A 接收 SOCKS5 请求，通过隧道转发给 Client B
- Client B 实际执行网络请求（真正的出口）
- 数据原路返回
- User 的所有流量看起来都是从 Client B 的网络出去的

## 功能特性

### ✅ 完整的 SOCKS5 协议实现

1. **握手阶段**
   - 支持无认证模式 (0x00)
   - 支持用户名/密码认证 (0x02)
   - 自动协商认证方法

2. **命令支持**
   - ✅ CONNECT 命令（TCP 连接）
   - ⚠️ BIND 命令（暂未实现）
   - ⚠️ UDP ASSOCIATE 命令（暂未实现）

3. **地址类型**
   - ✅ IPv4 地址
   - ✅ 域名
   - ✅ IPv6 地址

4. **安全特性**
   - 可选的用户名/密码认证
   - 连接超时控制
   - 详细的错误响应

## 使用方法

### 1. 创建无认证的 SOCKS5 代理

```go
package main

import (
    "context"
    "tunnox-core/internal/protocol/adapter"
)

func main() {
    ctx := context.Background()
    
    // 创建无认证的 SOCKS5 adapter
    socksAdapter := adapter.NewSocksAdapter(ctx, nil, nil)
    
    // 监听本地端口
    if err := socksAdapter.Listen("localhost:8888"); err != nil {
        panic(err)
    }
    
    // 启动接受循环
    for {
        conn, err := socksAdapter.Accept()
        if err != nil {
            continue
        }
        // 连接会被自动处理
    }
}
```

### 2. 创建带认证的 SOCKS5 代理

```go
package main

import (
    "context"
    "tunnox-core/internal/protocol/adapter"
)

func main() {
    ctx := context.Background()
    
    // 配置认证
    config := &adapter.SocksConfig{
        Username: "myuser",
        Password: "mypassword",
    }
    
    // 创建带认证的 SOCKS5 adapter
    socksAdapter := adapter.NewSocksAdapter(ctx, nil, config)
    
    // 监听本地端口
    if err := socksAdapter.Listen("localhost:8888"); err != nil {
        panic(err)
    }
    
    // 启动接受循环
    for {
        conn, err := socksAdapter.Accept()
        if err != nil {
            continue
        }
        // 连接会被自动处理
    }
}
```

### 3. 与 Session 集成（隧道模式）

```go
package main

import (
    "context"
    "tunnox-core/internal/protocol/adapter"
    "tunnox-core/internal/protocol/session"
)

func main() {
    ctx := context.Background()
    
    // 创建 Session（用于隧道转发）
    mySession := session.NewSession(...)
    
    // 创建 SOCKS5 adapter 并关联 Session
    socksAdapter := adapter.NewSocksAdapter(ctx, mySession, nil)
    
    // 监听本地端口
    if err := socksAdapter.Listen("localhost:8888"); err != nil {
        panic(err)
    }
    
    // 当有 SOCKS5 请求时，会自动通过 Session 转发到远端
    for {
        conn, err := socksAdapter.Accept()
        if err != nil {
            continue
        }
    }
}
```

## 客户端配置

### 浏览器配置

**Chrome/Edge:**
```
设置 → 系统 → 代理设置 → 手动代理配置
SOCKS5 代理: localhost
端口: 8888
```

**Firefox:**
```
设置 → 常规 → 网络设置 → 手动代理配置
SOCKS 主机: localhost
端口: 8888
SOCKS v5: 勾选
```

### 命令行工具

**curl:**
```bash
curl --socks5 localhost:8888 https://www.google.com
```

**wget:**
```bash
export socks_proxy=socks5://localhost:8888
wget https://www.google.com
```

### 系统级代理

**macOS:**
```bash
networksetup -setsocksfirewallproxy Wi-Fi localhost 8888
```

**Linux:**
```bash
export ALL_PROXY=socks5://localhost:8888
```

**Windows:**
```
设置 → 网络和 Internet → 代理
使用代理服务器: 开
地址: localhost
端口: 8888
```

## 技术细节

### SOCKS5 协议流程

1. **握手阶段**
   ```
   Client → Server: [VER, NMETHODS, METHODS]
   Server → Client: [VER, METHOD]
   ```

2. **认证阶段**（如果需要）
   ```
   Client → Server: [VER, ULEN, UNAME, PLEN, PASSWD]
   Server → Client: [VER, STATUS]
   ```

3. **请求阶段**
   ```
   Client → Server: [VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT]
   Server → Client: [VER, REP, RSV, ATYP, BND.ADDR, BND.PORT]
   ```

4. **数据转发阶段**
   ```
   双向透明转发数据
   ```

### 配置参数

```go
const (
    socksHandshakeTimeout = 10 * time.Second  // 握手超时
    socksDialTimeout      = 30 * time.Second  // 连接超时
    socksBufferSize       = 32 * 1024         // 缓冲区大小
)
```

### 响应代码

| 代码 | 含义 |
|------|------|
| 0x00 | 成功 |
| 0x01 | 服务器故障 |
| 0x02 | 规则不允许 |
| 0x03 | 网络不可达 |
| 0x04 | 主机不可达 |
| 0x05 | 连接被拒绝 |
| 0x06 | TTL 过期 |
| 0x07 | 不支持的命令 |
| 0x08 | 不支持的地址类型 |

## 性能优化

1. **连接复用**：支持多个并发连接
2. **双向转发**：使用 goroutine 并行传输数据
3. **缓冲区优化**：32KB 缓冲区提高吞吐量
4. **超时控制**：防止资源泄漏

## 安全建议

1. **使用认证**：在公网环境中务必启用用户名/密码认证
2. **限制访问**：只监听本地地址（localhost）
3. **日志审计**：记录所有连接请求
4. **超时配置**：合理设置超时避免资源耗尽

## 限制和注意事项

1. **BIND 和 UDP ASSOCIATE 命令暂未实现**
   - 目前仅支持 CONNECT 命令（足够大多数使用场景）
   
2. **直接连接模式**
   - 如果未设置 Session，将直接连接目标（不通过隧道）
   - 适用于本地测试，但不是预期的生产用途

3. **认证强度**
   - 用户名/密码采用明文传输（SOCKS5 协议限制）
   - 建议配合 TLS/加密隧道使用

## 测试

运行测试：
```bash
go test ./internal/protocol/adapter/socks_adapter_test.go \
        ./internal/protocol/adapter/socks_adapter.go \
        ./internal/protocol/adapter/adapter.go -v
```

## 下一步改进

1. **隧道集成**：完善与 Session 的集成，实现真正的隧道转发
2. **UDP 支持**：实现 UDP ASSOCIATE 命令
3. **BIND 支持**：实现 BIND 命令（如果需要）
4. **流量统计**：添加流量监控和统计
5. **访问控制**：基于规则的访问控制列表
6. **性能监控**：连接数、带宽、延迟等指标

