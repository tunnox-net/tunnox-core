# UDP 会话空闲超时实现

## 概述

实现了 UDP 可靠传输层的会话空闲超时机制，解决连接空闲 10 分钟后 NAT 映射过期导致的连接卡住问题。

## 核心机制

### 1. 活动时间跟踪

```go
// Session 结构体新增字段
lastActivity time.Time
activityMu   sync.RWMutex

// 更新活动时间
func (s *Session) updateActivity() {
    s.activityMu.Lock()
    s.lastActivity = time.Now()
    s.activityMu.Unlock()
}
```

### 2. 超时配置

```go
const (
    SessionIdleTimeout = 15 * 60 * 1000  // 15 分钟
    KeepAliveInterval  = 30 * 1000       // 30 秒
)
```

### 3. 空闲检测

在 `keepAliveLoop()` 中每 30 秒检查一次：

```go
idleTime := time.Since(s.getLastActivity())
if idleTime > SessionIdleTimeout {
    s.logger.Warnf("Session: idle timeout, closing session %d", s.sessionID)
    go s.Close()
    return
}
```

### 4. 活动更新触发点

- **发送数据**: `sendLoop()` 中调用 `updateActivity()`
- **接收数据**: `handleData()` 中调用 `updateActivity()`

## 工作流程

```
时间线：
T+0s    : 会话建立，lastActivity = now
T+30s   : KeepAlive 检查，空闲 30s < 15min，继续
T+60s   : KeepAlive 检查，空闲 60s < 15min，继续
...
T+5min  : 用户发送数据，lastActivity = now (重置)
T+5.5min: KeepAlive 检查，空闲 30s < 15min，继续
...
T+20.5min: KeepAlive 检查，空闲 15min，触发超时
         - 记录警告日志
         - 调用 Close() 关闭会话
         - 释放资源
```

## 优势

1. **自动清理**: 无需手动管理失效会话
2. **NAT 友好**: 避免使用过期的 NAT 映射
3. **资源高效**: 及时释放空闲会话占用的资源
4. **可配置**: 超时时间可根据需求调整

## 测试

- ✅ 活动时间更新机制
- ✅ 超时检测逻辑
- ✅ 发送数据时更新活动时间
- ✅ 接收数据时更新活动时间

## 相关文件

- `internal/protocol/udp/reliable/session.go` - 活动时间跟踪
- `internal/protocol/udp/reliable/session_send.go` - 空闲检测和发送更新
- `internal/protocol/udp/reliable/session_receive.go` - 接收更新
- `internal/protocol/udp/reliable/protocol.go` - 超时常量
- `internal/protocol/udp/reliable/session_idle_test.go` - 测试用例

## 配置建议

当前配置适用于大多数场景：
- **15 分钟空闲超时**: 足够长以避免误关闭活跃连接
- **30 秒 KeepAlive**: 平衡网络开销和及时检测

如需调整，修改 `protocol.go` 中的常量即可。
