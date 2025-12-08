# HTTP Poll 分片重组分析与设计方案

## 1. 当前实现分析

### 1.1 分片组最大片数限制
- **当前限制**：无明确限制（通过 `MaxFragmentSize` 和 `OriginalSize` 间接限制）
- **问题**：单个分片组可能超过 1000 片
- **需要**：明确限制 `TotalFragments <= 1000`

### 1.2 接收端返回顺序分析

#### 当前实现
- **接收端**：使用 `OrderedFragmentProcessor`，通过 `GetNextCompleteGroup()` 按序列号顺序返回
- **机制**：`nextExpectedSeq` 严格按顺序递增，只有序列号等于 `nextExpectedSeq` 且已完整的分片组才会被返回
- **结论**：**已实现严格按发送顺序返回**

#### 示例场景
```
发送方顺序：1, 2, 3, 4, 5
接收方收齐顺序：3, 2, 4, 5, 1

当前处理流程：
1. 收到分片组 3 → 添加到重组器，等待序列号 1
2. 收到分片组 2 → 添加到重组器，等待序列号 1
3. 收到分片组 4 → 添加到重组器，等待序列号 1
4. 收到分片组 5 → 添加到重组器，等待序列号 1
5. 收到分片组 1 → 添加到重组器，序列号匹配 nextExpectedSeq(1)
   → 返回分片组 1，nextExpectedSeq = 2
6. 分片组 2 已完整，序列号匹配 nextExpectedSeq(2)
   → 返回分片组 2，nextExpectedSeq = 3
7. 分片组 3 已完整，序列号匹配 nextExpectedSeq(3)
   → 返回分片组 3，nextExpectedSeq = 4
... 依此类推

结果：按 1, 2, 3, 4, 5 顺序返回 ✅
```

### 1.3 发送方并行发送分析

#### 当前实现
```go
// server_stream_processor_data.go: WriteExact
sp.writeMu.Lock()
defer sp.writeMu.Unlock()

for i, fragment := range fragments {
    sp.pollDataQueue.Push(fragmentJSON)
}
```

- **结论**：**发送方是串行的**，不是并行发送
- **原因**：`writeMu` 互斥锁确保同一 `WriteExact` 调用的所有分片连续推送
- **影响**：同一分片组的所有分片会连续推送，但不同分片组之间可能交错（如果多个 `WriteExact` 并发调用）

### 1.4 序列号同步问题

#### 当前实现
- **服务器端**：`sequenceNumber` 从 0 开始，每个 `WriteExact` 调用递增
- **客户端**：`nextExpectedSeq` 初始化为 0，如果第一个分片的序列号不是 0，会自动调整
- **问题**：
  1. **断线重连后序列号不同步**：服务器端序列号继续递增，客户端 `nextExpectedSeq` 重置为 0
  2. **序列号跳号处理不完善**：如果序列号 1 丢失，后续序列号（2, 3, 4...）会一直等待

## 2. 设计方案

### 2.1 分片组最大片数限制

**方案**：在 `CalculateFragments` 和 `AddFragment` 中增加验证

```go
const (
    MaxFragmentsPerGroup = 1000  // 单个分片组最大片数
)

// 在 CalculateFragments 中验证
func CalculateFragments(dataSize int) (fragmentSize int, totalFragments int) {
    // ... 现有逻辑 ...
    
    if totalFragments > MaxFragmentsPerGroup {
        // 调整分片大小，确保不超过最大片数
        fragmentSize = (dataSize + MaxFragmentsPerGroup - 1) / MaxFragmentsPerGroup
        totalFragments = MaxFragmentsPerGroup
    }
    
    return fragmentSize, totalFragments
}

// 在 AddFragment 中验证
func (fr *FragmentReassembler) AddFragment(...) {
    if totalFragments > MaxFragmentsPerGroup {
        return nil, coreErrors.Newf(coreErrors.ErrorTypePermanent, 
            "total fragments exceeds limit: %d (max: %d)", totalFragments, MaxFragmentsPerGroup)
    }
    // ... 现有逻辑 ...
}
```

### 2.2 接收端严格按顺序返回（已实现）

**当前实现已满足要求**：
- `GetNextCompleteGroup()` 严格按 `nextExpectedSeq` 顺序返回
- 即使后面的分片组先收齐，也会等待前面的分片组

**优化建议**：
- 增加超时检测：如果某个序列号的分片组长时间不完整，考虑跳过（需要协议层支持）

### 2.3 发送方并行发送优化

**当前问题**：
- `writeMu` 锁粒度太大，多个 `WriteExact` 调用会串行化
- 不同分片组的分片可能交错（虽然同一分片组内是连续的）

**方案**：保持当前设计（串行发送）
- **理由**：
  1. HTTP Poll 协议特性：数据通过 Poll 响应返回，需要按顺序匹配
  2. 并行发送可能导致分片交错，增加重组复杂度
  3. 当前设计已确保同一分片组内分片连续，满足需求

**可选优化**（如果需要提高吞吐量）：
- 使用分片组级别的锁，而不是全局 `writeMu`
- 但需要确保同一分片组的所有分片连续推送

### 2.4 序列号同步与超时处理方案

#### 问题分析
1. **断线重连**：服务器端序列号继续，客户端重置
2. **分片组丢失**：如果某个序列号的分片组丢失（超时），说明连接断过，应该断开连接，而不是一直等待

#### 关键洞察
**用户反馈**：如果某个序列号的分片组丢失，说明连接断过，并且数据也没有重发。如果发送端发送分组3第4片时网络出问题，重试失败后，这个分片就丢失了。即使后续分片（分组3第5片）到达，数据也不完整了。这种情况下应该直接给上层反馈连接断开，而不是一直等待。

#### 方案：超时检测 + 连接断开（推荐）

**设计**：
1. **序列号重置**：每次连接建立时，服务器端和客户端都重置序列号为 0
2. **超时检测**：在 `GetNextCompleteGroup()` 中检测 `nextExpectedSeq` 对应的分片组是否超时
3. **连接断开**：如果超时，返回连接断开错误，向上层报告

**实现**：
```go
// 在 GetNextCompleteGroup 中检测超时
func (fr *FragmentReassembler) GetNextCompleteGroup() (*FragmentGroup, bool, error) {
    fr.mu.Lock()
    defer fr.mu.Unlock()

    // 只检查期望的下一个序列号（确保严格按顺序）
    group, exists := fr.sequenceGroups[fr.nextExpectedSeq]
    if !exists {
        // 期望的序列号还不存在，等待
        return nil, false, nil
    }

    // 检查是否超时
    if time.Since(group.CreatedTime) > FragmentGroupTimeout {
        // 分片组超时，说明连接断过，数据丢失
        utils.Errorf("FragmentReassembler: fragment group timeout, sequenceNumber=%d, groupID=%s, age=%v. Connection may be broken.", 
            fr.nextExpectedSeq, group.GroupID, time.Since(group.CreatedTime))
        
        // 清理超时的分片组
        delete(fr.groups, group.GroupID)
        delete(fr.sequenceGroups, fr.nextExpectedSeq)
        
        // 返回连接断开错误
        return nil, false, coreErrors.Newf(coreErrors.ErrorTypePermanent, 
            "fragment group timeout: sequenceNumber=%d, connection broken", fr.nextExpectedSeq)
    }

    // 检查是否已完整
    group.mu.Lock()
    isComplete := group.ReceivedCount == group.TotalFragments && !group.reassembled
    group.mu.Unlock()

    if isComplete {
        // 找到期望的下一个已完整的分片组，更新期望序列号
        oldSeq := fr.nextExpectedSeq
        fr.nextExpectedSeq++
        return group, true, nil
    }

    // 期望的序列号存在但还不完整，等待
    return nil, false, nil
}
```

**调用链处理**：
```go
// OrderedFragmentProcessor.GetNextReassembledData()
func (p *OrderedFragmentProcessor) GetNextReassembledData() ([]byte, bool, error) {
    nextGroup, found, err := p.reassembler.GetNextCompleteGroup()
    if err != nil {
        // 如果是连接断开错误，直接返回
        return nil, false, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "connection broken")
    }
    // ... 现有逻辑 ...
}

// transport_httppoll_poll.go: pollLoop()
for {
    reassembledData, found, err := c.fragmentProcessor.GetNextReassembledData()
    if err != nil {
        // 连接断开错误，关闭连接
        utils.Errorf("HTTP long polling: connection broken: %v, mappingID=%s", err, c.mappingID)
        c.Close() // 关闭连接，向上层报告
        return
    }
    // ... 现有逻辑 ...
}
```

**优点**：
1. **及时检测连接断开**：超时立即报告，不无限等待
2. **避免数据不完整**：如果分片组丢失，后续数据也不完整，直接断开
3. **简单可靠**：无需复杂的跳号检测和恢复机制

**缺点**：
- 如果网络只是暂时延迟（但最终会到达），可能会误判为连接断开
- **缓解**：`FragmentGroupTimeout = 30秒` 已经足够长，可以容忍大部分网络延迟

### 2.5 推荐方案

**综合推荐**：**超时检测 + 连接断开**

**理由**：
1. **符合用户需求**：分片组丢失时直接断开连接，不无限等待
2. **简单可靠**：超时检测逻辑清晰，易于实现和维护
3. **避免数据不完整**：如果分片组丢失，后续数据也不完整，断开连接是正确的选择

**实现要点**：
1. 连接建立时，服务器端和客户端都重置序列号
2. 保持当前的严格按顺序返回机制
3. 增加分片组最大片数限制（1000）

## 3. 实现计划

### 3.1 立即实现
1. ✅ 增加 `MaxFragmentsPerGroup = 1000` 常量
2. ✅ 在 `CalculateFragments` 中验证和调整
3. ✅ 在 `AddFragment` 中验证

### 3.2 超时检测与连接断开
1. 在 `GetNextCompleteGroup()` 中检测 `nextExpectedSeq` 对应的分片组是否超时
2. 如果超时，返回连接断开错误
3. 在 `GetNextReassembledData()` 中处理错误，向上层报告
4. 在 `pollLoop()` 中，如果收到连接断开错误，关闭连接

### 3.3 序列号重置
1. 服务器端：连接建立时重置 `sequenceNumber = 0`
2. 客户端：连接建立时重置 `nextExpectedSeq = 0` 和重组器状态

### 3.4 可选优化
1. 发送方并行发送优化（如果需要提高吞吐量）

## 4. 总结

### 当前状态
- ✅ **接收端已实现严格按顺序返回**
- ✅ **发送方串行发送，同一分片组内分片连续**
- ⚠️ **序列号同步问题**：断线重连后不同步
- ⚠️ **分片组最大片数**：无明确限制
- ⚠️ **超时处理**：分片组超时后不会断开连接，导致无限等待

### 推荐改进
1. **增加分片组最大片数限制**（1000片）
2. **实现超时检测与连接断开**（分片组超时立即断开连接）
3. **实现序列号重置机制**（连接建立时重置）
4. **保持当前严格按顺序返回机制**（已满足需求）

