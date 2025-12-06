# UDP 可靠传输层设计文档 V2

## 1. 设计原则

### 1.1 核心原则
- **传输层不解包**：UDP可靠传输层只负责可靠传输字节流，不解析应用层数据包（TransferPacket）
- **完全兼容现有体系**：与StreamProcessor、SessionManager等现有组件无缝集成
- **可靠高性能重组**：采用高效的分片重组算法，支持大数据包传输
- **传输寻址**：通过ConnectionID实现精确的传输寻址，确保数据能准确回发

### 1.2 架构分层
```
应用层 (TransferPacket)
    ↓ (序列化/反序列化)
StreamProcessor (ReadPacket/WritePacket)
    ↓ (字节流读写)
UDP可靠传输层 (Read/Write, 作为 io.Reader/io.Writer)
    ↓ (分片/重组/ACK/重传)
UDP Socket (DialUDP/ListenUDP)
```

## 2. ConnectionID 设计

### 2.1 ConnectionID 的作用
- **服务端分配**：ConnectionID由服务端在握手时分配，用于标识每个UDP连接
- **传输寻址**：服务端通过ConnectionID查找对应的UDP连接，实现数据回发
- **连接管理**：用于区分不同的UDP连接（控制连接、隧道连接等）

### 2.2 ConnectionID 格式
- **格式**：`IP:Port`（使用 `remoteAddr.String()`）
- **长度**：固定64字节（兼容IPv4/IPv6）
- **编码**：不足64字节用0填充，解码时找到第一个0字节截断

### 2.3 ConnectionID 分配流程
1. **客户端**：初始时ConnectionID为空字符串
2. **服务端**：收到客户端UDP包后，使用 `remoteAddr.String()` 作为ConnectionID
3. **握手响应**：服务端在握手响应中返回ConnectionID
4. **客户端更新**：客户端收到握手响应后，更新UDP连接的ConnectionID

## 3. 分片信息头设计

### 3.1 分片信息头结构（固定大小，等宽）
```
FragmentPacketHeader (96 bytes, 固定大小):
├─ Magic (2 bytes)           - 包类型标识 (0x5546 = "UF")
├─ Version (1 byte)           - 协议版本 (0x01)
├─ Type (1 byte)              - 包类型 (0x02 = Fragment)
├─ Reserved (1 byte)         - 保留字段（对齐）
├─ ConnectionID (64 bytes)   - 连接ID（IP:Port格式，固定长度，兼容IPv4/IPv6）
├─ GroupID (8 bytes)          - 分片组ID（用于重组）
├─ FragmentIndex (2 bytes)   - 当前分片索引（0-based）
├─ TotalFragments (2 bytes)   - 总分片数
├─ OriginalSize (4 bytes)     - 原始数据大小（重组后的总大小）
├─ FragmentSize (2 bytes)      - 当前分片大小（不包括头）
└─ SequenceNum (4 bytes)       - 序列号（用于可靠传输）
```

### 3.2 数据包结构
```
DataPacket (小包，≤1200字节):
├─ Header (80 bytes)
│  ├─ Magic (2) + Version (1) + Type (1) + Reserved (1)
│  ├─ SequenceNum (4)
│  └─ ConnectionID (64) + DataLen (4)
└─ Data (可变长度)

FragmentPacket (大包，>1200字节):
├─ Header (96 bytes) - 固定大小
└─ FragmentData (可变长度，≤1400字节)
```

## 4. 读取流程设计

### 4.1 读取流程（按分片信息头逐步读取）
1. **读取分片信息头**（固定96字节）
   - 使用 `readExact(96)` 读取完整头
   - 解析得到：ConnectionID、FragmentSize、TotalFragments、OriginalSize等

2. **读取分片数据**（根据FragmentSize）
   - 使用 `readExact(FragmentSize)` 读取完整分片
   - 读满FragmentSize字节，说明当前分片读完

3. **重组分片**
   - 使用GroupID作为重组键
   - 收集所有分片后，按FragmentIndex排序
   - 重组为完整数据，提供给StreamProcessor作为字节流

### 4.2 高性能重组方案
- **使用哈希表**：以GroupID为键，快速查找分片组
- **预分配缓冲区**：根据OriginalSize预分配重组缓冲区
- **顺序验证**：检查FragmentIndex是否连续，处理乱序分片
- **超时清理**：对长时间未完成的分片组进行清理

## 5. 写入流程设计

### 5.1 写入流程
1. **接收字节流**：从StreamProcessor接收序列化后的字节流
2. **判断是否需要分片**：
   - 如果数据 ≤ 1200字节：使用DataPacket直接发送
   - 如果数据 > 1200字节：进行分片处理
3. **分片处理**：
   - 生成GroupID（唯一标识分片组）
   - 将数据切分为多个Fragment（每个≤1400字节）
   - 为每个Fragment添加分片信息头（96字节）
4. **发送分片**：
   - 按顺序发送所有分片
   - 等待ACK确认
   - 实现重传机制

## 6. 可靠传输机制

### 6.1 ACK机制
- **单包ACK**：DataPacket使用单包ACK
- **分片组ACK**：FragmentPacket使用分片组ACK（位图）
- **批量ACK**：支持批量确认多个包

### 6.2 重传机制
- **超时重传**：未收到ACK的分片，超时后重传
- **快速重传**：收到重复ACK时，快速重传
- **最大重传次数**：限制重传次数，避免无限重传

### 6.3 序列号管理
- **发送序列号**：每个包分配唯一序列号
- **接收序列号**：维护期望接收的序列号
- **乱序处理**：缓存乱序包，等待缺失包到达

## 7. 服务端寻址机制

### 7.1 ConnectionRegistry 设计
```go
type UDPConnectionRegistry struct {
    mu          sync.RWMutex
    connections map[string]*UDPReliableConnection  // key = ConnectionID
}

// Register 注册UDP连接
func (r *UDPConnectionRegistry) Register(connID string, conn *UDPReliableConnection)

// Get 通过ConnectionID查找UDP连接
func (r *UDPConnectionRegistry) Get(connID string) *UDPReliableConnection
```

### 7.2 服务端数据回发流程
1. **StreamProcessor.WritePacket调用**：
   - StreamProcessor序列化TransferPacket为字节流
   - 调用底层Writer.Write写入字节流

2. **UDP可靠传输层接收**：
   - UDP可靠传输层作为Writer，接收字节流
   - 通过ConnectionID查找对应的UDP连接
   - 将字节流分片并发送到客户端

3. **透传回写**：
   - 服务端透传时，通过ConnectionID查找UDP连接
   - 直接调用UDP连接的Write方法，写入原始数据

### 7.3 与SessionManager集成
- **连接创建**：UDP连接创建时，自动注册到ConnectionRegistry
- **连接查找**：SessionManager通过ConnectionID查找UDP连接
- **数据发送**：通过Connection.Stream.WritePacket发送数据

## 8. 客户端设计

### 8.1 连接建立流程
1. **DialUDP**：客户端使用 `net.DialUDP` 创建已连接的UDP socket
2. **创建UDP可靠连接**：创建UDPReliableConnection，初始ConnectionID为空
3. **发送握手请求**：通过StreamProcessor发送握手请求
4. **接收握手响应**：从握手响应中获取ConnectionID
5. **更新ConnectionID**：更新UDP连接的ConnectionID

### 8.2 数据接收流程
1. **receivePackets启动**：在握手完成后启动receivePackets goroutine
2. **读取UDP包**：使用 `Read` 方法读取UDP包（已连接socket）
3. **解析分片信息头**：先读96字节头，再读分片数据
4. **重组分片**：重组后提供给StreamProcessor

## 9. 接口设计

### 9.1 UDPReliableConnection 接口
```go
type UDPReliableConnection interface {
    io.Reader
    io.Writer
    io.Closer
    
    // GetConnectionID 获取连接ID
    GetConnectionID() string
    
    // SetConnectionID 设置连接ID（服务端分配）
    SetConnectionID(connID string)
    
    // GetRemoteAddr 获取远程地址
    GetRemoteAddr() net.Addr
}
```

### 9.2 UDPConnectionRegistry 接口
```go
type UDPConnectionRegistry interface {
    // Register 注册UDP连接
    Register(connID string, conn UDPReliableConnection)
    
    // Get 通过ConnectionID查找UDP连接
    Get(connID string) (UDPReliableConnection, bool)
    
    // Remove 移除UDP连接
    Remove(connID string)
}
```

## 10. 与现有体系兼容

### 10.1 与StreamProcessor兼容
- **作为io.Reader/io.Writer**：UDP可靠传输层实现io.Reader/io.Writer接口
- **不解包**：不解析TransferPacket，只传输字节流
- **透明传输**：StreamProcessor无需知道底层是UDP

### 10.2 与SessionManager兼容
- **ConnectionID统一**：使用相同的ConnectionID格式
- **连接查找**：通过ConnectionID查找连接
- **数据发送**：通过Connection.Stream.WritePacket发送

### 10.3 与httppoll对齐
- **ConnectionID分配**：服务端在握手时分配ConnectionID
- **连接注册**：使用ConnectionRegistry管理连接
- **数据回发**：通过ConnectionID查找连接并发送数据

## 11. 实现要点

### 11.1 分片重组优化
- **预分配缓冲区**：根据OriginalSize预分配重组缓冲区
- **哈希表查找**：使用GroupID作为键，O(1)查找
- **顺序验证**：检查FragmentIndex连续性
- **超时清理**：清理长时间未完成的分片组

### 11.2 可靠传输优化
- **滑动窗口**：实现发送窗口和接收窗口
- **批量ACK**：减少ACK包数量
- **快速重传**：检测丢包后快速重传
- **拥塞控制**：根据网络状况调整发送速率

### 11.3 性能优化
- **零拷贝**：尽可能减少数据拷贝
- **内存池**：重用缓冲区，减少GC压力
- **并发处理**：使用goroutine处理分片重组和ACK

## 12. 测试要点

### 12.1 功能测试
- **分片重组**：测试大数据包的分片和重组
- **可靠传输**：测试丢包、乱序、重传等场景
- **连接管理**：测试ConnectionID分配和查找

### 12.2 性能测试
- **吞吐量**：测试大数据包传输的吞吐量
- **延迟**：测试分片重组的延迟
- **并发**：测试多连接并发传输

### 12.3 兼容性测试
- **与StreamProcessor集成**：测试与现有StreamProcessor的兼容性
- **与SessionManager集成**：测试与SessionManager的兼容性
- **跨协议测试**：测试UDP与httppoll的兼容性

## 13. 总结

本设计文档提供了一个全新的UDP可靠传输层架构，核心特点：
1. **传输层不解包**：只负责可靠传输字节流
2. **完全兼容现有体系**：与StreamProcessor、SessionManager无缝集成
3. **可靠高性能重组**：采用高效的分片重组算法
4. **精确传输寻址**：通过ConnectionID实现精确的数据回发

该设计确保了UDP可靠传输层能够完全兼容现有体系，同时提供高性能的可靠传输能力。

