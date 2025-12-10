# UDP 缓冲区优化

## 问题背景

在高负载或网络质量差的情况下，出现两个高频错误：

1. **"failed to write to pipe: io: read/write on closed pipe"**
   - 会话关闭后仍有数据试图写入 pipe
   - 发生在接收数据包时会话已在关闭过程中

2. **"recv buffer full, dropping packet"**
   - 接收缓冲区已满，无法接收更多乱序包
   - 导致丢包和重传，影响性能

## 优化方案

### 1. 增加缓冲区大小

参考 httppoll 的配置（MaxFragmentGroups = 5000），将缓冲区从 5000 增加到 10000：

```go
const (
    MaxSendBufSize = 10000  // 10000 个包 * 1444 字节 ≈ 14.4MB
    MaxRecvBufSize = 10000  // 支持更多乱序包
)
```

**优势**：
- 支持更高吞吐量传输
- 应对高延迟网络（更多在途数据）
- 容忍更多网络抖动和乱序包

### 2. 智能清理陈旧包

添加 `CleanupStaleRecvPackets()` 方法，只清理明显过期的包：

```go
// 清理序列号间隔超过 1000 的旧包
// 这些包说明中间有大量包丢失，不太可能再收到了
cleaned := s.bufferManager.CleanupStaleRecvPackets(expectedSeq, 1000)
```

**清理策略**：
- **不清理**：正常的乱序包（序列号接近期望值）
- **清理**：序列号远小于期望值的包（间隔 > 1000）
- **原因**：间隔太大说明中间包已丢失很久，不太可能收到

**示例**：
```
期望序列号: 10000
缓冲区中的包:
- seq=9950: 保留（间隔 50，可能很快收到中间的包）
- seq=9500: 保留（间隔 500，仍在合理范围）
- seq=8900: 清理（间隔 1100，中间包已丢失太久）
- seq=8000: 清理（间隔 2000，明显过期）
```

### 3. 优化关闭时的 Pipe 写入

在 `deliverData()` 中检查会话状态：

```go
func (s *Session) deliverData(data []byte) {
    // 检查会话是否正在关闭
    if s.getState() == StateClosed {
        return
    }
    
    _, err := s.pipeWriter.Write(data)
    if err != nil {
        // 忽略 closed pipe 错误（关闭时正常）
        if err.Error() != "io: read/write on closed pipe" {
            s.logger.Errorf("Session: failed to write to pipe: %v", err)
        }
    }
}
```

### 4. 改进日志策略

只在真正需要时记录警告：

```go
// 缓冲区满时，先尝试清理陈旧包
cleaned := s.bufferManager.CleanupStaleRecvPackets(expectedSeq, 1000)
if cleaned > 0 {
    s.logger.Infof("Session: cleaned %d stale recv packets", cleaned)
}

// 清理后仍然满才记录警告
if recvBufSize >= MaxRecvBufSize {
    s.logger.Warnf("Session: recv buffer full (%d/%d), dropping packet",
        recvBufSize, MaxRecvBufSize)
}
```

## 对比分析

### 优化前
- 缓冲区大小：5000 包（≈ 7.2MB）
- 缓冲区满时：直接丢包
- 日志：高频警告（即使是正常流控）
- Pipe 错误：总是记录错误日志

### 优化后
- 缓冲区大小：10000 包（≈ 14.4MB）
- 缓冲区满时：先清理陈旧包，再丢包
- 日志：只在真正异常时记录
- Pipe 错误：忽略关闭时的正常错误

## 性能影响

### 内存使用
- 增加：每个会话最多增加 7.2MB（5000 包 * 1444 字节）
- 实际：大多数情况下不会用满缓冲区
- 权衡：内存换性能和稳定性

### 吞吐量
- 提升：支持更高的带宽延迟积（BDP）
- 示例：100Mbps * 100ms RTT = 1.25MB，现在可以支持更高

### 丢包率
- 降低：更大的缓冲区容忍更多乱序
- 智能清理：只清理真正过期的包

## 测试结果

所有测试通过：
- ✅ TestBufferManager_SendBuffer
- ✅ TestBufferManager_RecvBuffer
- ✅ TestBufferManager_BufferFull
- ✅ TestBufferManager_Cleanup
- ✅ 所有集成测试

## 相关文件

- `internal/protocol/udp/reliable/buffer_manager.go` - 缓冲区管理
- `internal/protocol/udp/reliable/session_receive.go` - 接收逻辑
- `docs/UDP_SESSION_IDLE_TIMEOUT_ISSUE.md` - 空闲超时问题

## 监控建议

建议监控以下指标：
1. 缓冲区使用率（sendBuf/recvBuf size）
2. 陈旧包清理频率
3. 实际丢包率
4. 内存使用情况

如果发现缓冲区经常满，可能需要：
- 进一步增加缓冲区大小
- 优化网络质量
- 调整拥塞控制参数
