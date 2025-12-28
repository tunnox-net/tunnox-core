# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 构建和运行

```bash
# 构建服务端和客户端
go build -o bin/server ./cmd/server
go build -o bin/client ./cmd/client

# 运行服务端（零配置模式）
./bin/server

# 运行客户端（匿名模式）
./bin/client -s 127.0.0.1:8000 -p tcp -anonymous

# 指定配置文件运行
./bin/server -config config.yaml
./bin/client -config client-config.yaml
```

## 测试

```bash
# 运行所有测试
go test ./... -v

# 运行特定包测试
go test ./internal/stream/... -v

# 运行单个测试
go test -run TestNewDispose ./internal/core/dispose/...

# 测试覆盖率
go test ./... -cover

# 竞态检测
go test -race ./...

# 性能测试
go test -bench=. -benchmem ./internal/stream/...

# 端到端测试（启动服务端+两个客户端）
./start_test.sh
```

## 架构概览

Tunnox Core 是一个企业级内网穿透平台，支持 TCP/WebSocket/KCP/QUIC 四种传输协议。

### 分层架构

```
internal/
├── protocol/           # 协议适配层
│   ├── adapter/       # TCP/WebSocket/UDP/QUIC 协议适配器
│   └── session/       # 会话管理、连接生命周期
├── stream/            # 流处理层（压缩/加密/限流）
├── client/            # 客户端实现（映射处理器、CLI）
├── cloud/             # 云控管理层
│   ├── repos/         # Repository 层 - 纯数据访问
│   ├── services/      # Service 层 - 业务逻辑
│   └── managers/      # Manager 层 - 跨领域协调
├── command/           # 命令框架（泛型处理器）
├── broker/            # 消息广播（Redis/内存）
├── core/              # 核心组件
│   ├── storage/       # 存储抽象
│   ├── dispose/       # 资源生命周期管理
│   └── errors/        # 类型化错误
└── httpservice/       # HTTP 域名代理
```

### Dispose 模式（核心）

所有组件必须嵌入 dispose 基类实现生命周期管理：

```go
// Manager 级组件
type MyManager struct {
    *dispose.ManagerBase
}

func NewMyManager(parentCtx context.Context) *MyManager {
    return &MyManager{ManagerBase: dispose.NewManager("MyManager", parentCtx)}
}

// Service 级组件
type MyService struct {
    *dispose.ServiceBase
}

func NewMyService(parentCtx context.Context) *MyService {
    return &MyService{ServiceBase: dispose.NewService("MyService", parentCtx)}
}
```

**Context 规则**：必须从 `parent.Ctx()` 派生子 context，禁止使用 `context.Background()`。

### 命令框架

使用泛型基础处理器：

```go
type MyHandler struct {
    command.BaseCommandHandler[MyRequest, MyResponse]
}
```

## 类型安全

禁止使用 `interface{}`、`any`、`map[string]interface{}`，改用强类型结构体或泛型。

### 错误处理

```go
import coreerrors "tunnox-core/internal/core/errors"

err := coreerrors.New(coreerrors.ErrorTypeStorage, "connection failed")
err := coreerrors.Wrap(originalErr, coreerrors.ErrorTypeNetwork, "dial failed")
```

## 日志

```go
import "tunnox-core/internal/utils"

utils.Debugf("Processing: %s", id)
utils.Infof("Connected: %d", clientID)
utils.Warnf("Retry: %d", attempt)
utils.Errorf("Failed: %v", err)
```

## 代码约束

- 单个文件 < 500 行，单个函数 < 100 行
- 禁止 `panic()`、忽略错误 `_, _ := ...`、魔法数字
- 文件名：小写下划线（如 `session_manager.go`）
- 类型/函数：PascalCase（导出）、camelCase（私有）

## 协议端口

| 协议 | 端口 | 使用场景 |
|------|------|----------|
| TCP | 8000 | 稳定可靠，传统网络 |
| WebSocket | 8443 | 防火墙穿透 |
| KCP | 8000 (UDP) | 低延迟，实时应用 |
| QUIC | 443 | 多路复用，移动网络 |

## 核心概念

- **连接码**：一次性代码，简化目标端和源端客户端之间的隧道建立
- **匿名模式**：无需注册，使用设备 ID 快速接入
- **透明转发**：服务端不解析业务数据，压缩/加密在客户端完成
