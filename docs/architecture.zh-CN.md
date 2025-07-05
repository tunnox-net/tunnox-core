# 多协议适配器架构

## 设计理念

- **协议无关**：所有协议适配器（TCP、WebSocket、UDP、QUIC）都实现统一的 Adapter 接口。
- **统一业务入口**：所有连接最终都交由 ConnectionSession.AcceptConnection(reader, writer) 处理，业务逻辑与协议解耦。
- **易于扩展**：新增协议只需实现 Adapter 接口并注册。
- **线程安全**：所有流和连接均为并发安全设计。

## 架构图

```mermaid
graph TD
    subgraph 适配器层
        TCP["TCP Adapter"]
        WS["WebSocket Adapter"]
        UDP["UDP Adapter"]
        QUIC["QUIC Adapter"]
    end
    subgraph 业务层
        CS["ConnectionSession (统一业务处理)"]
    end
    TCP --> CS
    WS --> CS
    UDP --> CS
    QUIC --> CS
```

## Adapter 接口

```go
type Adapter interface {
    ConnectTo(serverAddr string) error
    ListenFrom(serverAddr string) error
    Start(ctx context.Context) error
    Stop() error
    Name() string
    GetReader() io.Reader
    GetWriter() io.Writer
    Close()
}
```

## ConnectionSession 统一业务入口

```go
func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) {
    // 业务逻辑在这里实现，与协议无关
}
```

## 扩展新协议

1. 实现 Adapter 接口
2. 注册到协议管理器
3. 业务逻辑无需修改

## 典型调用流程

1. 适配器监听/连接
2. 新连接到来时，调用 session.AcceptConnection(reader, writer)
3. 业务逻辑处理
4. 连接关闭自动清理

## 适用场景
- 多协议中转/代理/隧道服务
- 需要统一业务逻辑、支持多种网络协议的场景
- 易于扩展和维护的大型分布式系统 