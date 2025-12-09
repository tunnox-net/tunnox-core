# MySQL 查询卡住问题分析

## 问题现象

1. 第一次执行 `SELECT * FROM log.log_db_record LIMIT 7000` 成功
2. 后续查询会卡住（有时第2次，有时第3-4次）
3. 间隔时间长点，卡住机率小一些

## 根本原因

### 协议不匹配导致的数据传输问题

从配置文件分析：
- **Target Client** (MySQL 端): 使用 **TCP 协议** 连接到服务器 (`127.0.0.1:8000`)
- **Listen Client** (应用端): 使用 **HTTP Poll 协议** 连接到服务器 (`http://127.0.0.1:9000`)

### 数据流分析

```
MySQL (3306)
    ↓ (TCP connection)
Target Client (TCP protocol, port 8000)
    ↓ (writes data to tunnel)
Server (receives TCP data)
    ↓ (needs to forward to Listen Client)
Server HTTP Poll Queue (pollDataQueue + pollDataChan)
    ↓ (waits for poll requests)
Listen Client (HTTP Poll protocol, port 9000)
    ↓ (long polling with 30s timeout)
Application (port 7788)
```

## 核心问题

### 1. Poll 请求超时和数据积压

从日志可见：
```
time="2025-12-09T18:13:48+08:00" level=error msg="HTTP long polling: [HANDLE_POLL] HandlePollRequest returned error: context deadline exceeded, connID=conn_cdcedf73"
```

**Listen Client 不断出现:**
- `timeout waiting for response` (35秒)
- `context deadline exceeded` (30秒)
- 超时后重新连接和重启 poll loops

### 2. 队列容量限制

服务端 `ServerStreamProcessor` 配置：
```go
pollDataQueue:  session.NewPriorityQueue(3),  // 优先级队列
pollDataChan:   make(chan []byte, 100),       // 缓冲 100
```

当 MySQL 查询返回大量数据（7000行 ~17MB）时：
1. Target Client (TCP) 快速将数据写入服务器
2. 服务器将数据分片后推送到 `pollDataQueue`
3. Listen Client 通过 poll 请求拉取数据，但速度可能跟不上

### 3. Fragment 数据没有被正确处理

从 Target Client 日志：
- **没有任何 fragment 相关日志**，说明 TCP 连接没有使用分片发送
- 数据是通过普通的 TCP stream 发送的

从 Listen Client 日志：
- 不断的 poll 超时和重连
- **没有收到 fragment 数据**

## 数据传输路径问题

### TCP → HTTP Poll 转换的缺失

当 Target Client 使用 TCP 发送数据时：
1. 数据通过 TCP `WriteExact` 发送 → 直接写入 TCP stream
2. 服务器从 TCP stream 读取数据
3. **问题**：服务器需要将 TCP stream 的数据转换为 HTTP Poll 的分片格式
4. **关键**：这个转换过程可能存在问题或数据积压

### HTTP Poll WriteExact 的分片逻辑

```go
// internal/protocol/httppoll/stream_processor.go:399
func (sp *StreamProcessor) WriteExact(data []byte) error {
    // 对大数据包进行分片处理
    fragments, err := SplitDataIntoFragments(data, sequenceNumber)

    // 使用互斥锁确保fragments按sequenceNumber顺序发送
    sp.sendDataMu.Lock()
    defer sp.sendDataMu.Unlock()

    // 发送每个分片
    for i, fragment := range fragments {
        // ... 通过 HTTP POST 发送每个分片
    }
}
```

但是 **Target Client 使用 TCP 协议**，不会调用这个分片逻辑！

## 可能的问题点

### 1. 跨协议数据转发没有正确实现

- TCP 协议的 `WriteExact` 直接写入 TCP stream
- HTTP Poll 协议的 `ReadExact` 期望接收分片格式的数据
- **中间的转换层缺失或有Bug**

### 2. pollDataQueue 积压

每次查询后，可能有数据残留在队列中：
- Poll 请求超时（30秒）后，正在传输的数据会怎样？
- 是否有清理机制？
- 下次查询时，旧数据是否还在队列中？

### 3. Poll Rate Limiter 限制

服务端设置：
```go
pollRateLimiter: NewPollRateLimiter(5),  // 每个连接最多5个并发poll
```

虽然 Listen Client 启动了 3 个 poll loops，但如果服务端限制到了 5个并发，可能不够快速消费数据。

## 解决方案建议

### 方案 1：统一协议（推荐）

**将 Target Client 也改为 HTTP Poll 协议：**

修改 `/Users/roger.tong/GolandProjects/tunnox-core/client-config.yaml`:
```yaml
server:
    address: http://127.0.0.1:9000
    protocol: httppoll  # 改为 httppoll
```

**优点**：
- 两端都使用分片逻辑，数据格式统一
- 避免跨协议转换的复杂性
- Poll rate limiter 和分片重组逻辑都能正常工作

### 方案 2：增加 TCP → HTTP Poll 的数据转换层

在服务器端，当从 TCP 连接接收到数据需要转发给 HTTP Poll 连接时：
1. 检测目标连接的协议类型
2. 如果是 HTTP Poll，则将数据进行分片处理
3. 将分片推送到 `pollDataQueue`

### 方案 3：增大 pollDataQueue 和 pollDataChan 容量

修改 `internal/protocol/httppoll/server_stream_processor.go:90-91`:
```go
pollDataQueue: session.NewPriorityQueue(100),  // 增大到100
pollDataChan:  make(chan []byte, 1000),        // 增大到1000
```

### 方案 4：添加队列清理机制

当 poll 请求超时或连接重置时，清理 `pollDataQueue` 中的旧数据。

## 验证步骤

1. **确认协议配置**：
   ```bash
   # 检查两个 client 的协议配置
   grep "protocol:" /Users/roger.tong/GolandProjects/tunnox-core/client-config.yaml
   grep "protocol:" /Users/roger.tong/GolandProjects/docs/client-config.yaml
   ```

2. **尝试方案 1**：
   - 修改 Target Client 为 HTTP Poll 协议
   - 重启测试
   - 观察是否还会卡住

3. **监控队列状态**：
   - 添加日志输出 `pollDataQueue` 的长度
   - 监控 `pollDataChan` 的使用情况

4. **分析数据流**：
   - 在服务端添加日志，跟踪数据从 TCP 到 HTTP Poll 的转换过程
   - 确认分片是否被正确创建和发送

## 总结

**核心问题**：Target Client 使用 TCP 协议，Listen Client 使用 HTTP Poll 协议，两者之间的数据转换存在问题。

**最快解决方案**：将两个 client 都配置为相同的协议（建议都用 HTTP Poll），避免跨协议数据转换的复杂性。

**长期方案**：完善服务器端的跨协议数据转发机制，确保数据能够正确地从任何协议转换到任何协议。
