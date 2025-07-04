# PackageStream API 文档

## 概述

`PackageStream` 是一个线程安全的数据流包装器，提供了精确读取和写入指定长度字节的功能。它继承自 `utils.Dispose`，支持上下文取消和资源管理。

## 结构体定义

```go
type PackageStream struct {
    reader    io.Reader
    writer    io.Writer
    transLock sync.Mutex
    utils.Dispose
}
```

## 构造函数

### NewPackageStream

```go
func NewPackageStream(reader io.Reader, writer io.Writer, parentCtx context.Context) *PackageStream
```

创建一个新的 `PackageStream` 实例。

**参数：**
- `reader`: 数据读取器
- `writer`: 数据写入器
- `parentCtx`: 父上下文，用于控制生命周期

**返回值：**
- `*PackageStream`: 新创建的 PackageStream 实例

## 方法

### ReadExact

```go
func (ps *PackageStream) ReadExact(length int) ([]byte, error)
```

读取指定长度的字节，直到读完为止。如果读取的字节数不足指定长度，会继续读取直到达到指定长度或遇到错误。

**参数：**
- `length`: 要读取的字节数

**返回值：**
- `[]byte`: 读取到的数据
- `error`: 错误信息

**特性：**
- 线程安全：使用互斥锁保护
- 阻塞读取：直到读取到指定长度或遇到错误
- 上下文感知：支持上下文取消
- 错误处理：正确处理 EOF 和其他错误

**错误情况：**
- `io.EOF`: 流已关闭或数据不足
- `io.ErrClosedPipe`: reader 为 nil
- `context.Canceled`: 上下文被取消

### WriteExact

```go
func (ps *PackageStream) WriteExact(data []byte) error
```

写入指定长度的字节，直到写完为止。如果写入的字节数不足指定长度，会继续写入直到达到指定长度或遇到错误。

**参数：**
- `data`: 要写入的数据

**返回值：**
- `error`: 错误信息

**特性：**
- 线程安全：使用互斥锁保护
- 阻塞写入：直到写入指定长度或遇到错误
- 上下文感知：支持上下文取消
- 错误处理：正确处理写入错误

**错误情况：**
- `io.ErrClosedPipe`: 流已关闭或 writer 为 nil
- `context.Canceled`: 上下文被取消
- 其他写入错误

## 使用示例

### 基本使用

```go
package main

import (
    "bytes"
    "context"
    "fmt"
    "log"
    "tunnox-core/internal/io"
)

func main() {
    // 准备数据
    testData := []byte("Hello, PackageStream!")
    reader := bytes.NewReader(testData)
    var buf bytes.Buffer
    writer := &buf

    // 创建 PackageStream
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    stream := io.NewPackageStream(reader, writer, ctx)
    defer stream.Close()

    // 读取指定长度
    data, err := stream.ReadExact(10)
    if err != nil {
        log.Fatalf("读取失败: %v", err)
    }
    fmt.Printf("读取: %s\n", string(data))

    // 写入数据
    writeData := []byte("写入测试")
    err = stream.WriteExact(writeData)
    if err != nil {
        log.Fatalf("写入失败: %v", err)
    }
    fmt.Printf("写入: %s\n", string(writeData))
}
```

### 大数据处理

```go
func largeDataExample() {
    // 生成大数据
    largeData := make([]byte, 1024*1024) // 1MB
    for i := range largeData {
        largeData[i] = byte(i % 256)
    }

    reader := bytes.NewReader(largeData)
    var buf bytes.Buffer
    writer := &buf

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    stream := io.NewPackageStream(reader, writer, ctx)
    defer stream.Close()

    // 读取大数据
    data, err := stream.ReadExact(512 * 1024) // 读取 512KB
    if err != nil {
        log.Fatalf("读取大数据失败: %v", err)
    }
    fmt.Printf("读取了 %d 字节\n", len(data))

    // 写入大数据
    err = stream.WriteExact(largeData)
    if err != nil {
        log.Fatalf("写入大数据失败: %v", err)
    }
    fmt.Printf("写入了 %d 字节\n", len(largeData))
}
```

### 并发访问

```go
func concurrentExample() {
    testData := []byte("并发测试数据")
    reader := bytes.NewReader(testData)
    var buf bytes.Buffer
    writer := &buf

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    stream := io.NewPackageStream(reader, writer, ctx)
    defer stream.Close()

    // 并发读取
    go func() {
        data, err := stream.ReadExact(5)
        if err != nil {
            log.Printf("并发读取失败: %v", err)
            return
        }
        fmt.Printf("并发读取: %s\n", string(data))
    }()

    // 并发写入
    go func() {
        writeData := []byte("并发写入")
        err := stream.WriteExact(writeData)
        if err != nil {
            log.Printf("并发写入失败: %v", err)
            return
        }
        fmt.Printf("并发写入: %s\n", string(writeData))
    }()

    // 等待执行完成
    time.Sleep(100 * time.Millisecond)
}
```

### 上下文取消

```go
func contextCancellationExample() {
    testData := []byte("上下文取消测试")
    reader := bytes.NewReader(testData)
    var buf bytes.Buffer
    writer := &buf

    ctx, cancel := context.WithCancel(context.Background())

    stream := io.NewPackageStream(reader, writer, ctx)
    defer stream.Close()

    // 启动取消上下文
    go func() {
        time.Sleep(50 * time.Millisecond)
        fmt.Println("取消上下文...")
        cancel()
    }()

    // 尝试读取（会被上下文取消中断）
    data, err := stream.ReadExact(100)
    if err != nil {
        fmt.Printf("读取被取消: %v\n", err)
    }
}
```

## 注意事项

1. **线程安全**: 所有方法都是线程安全的，使用互斥锁保护
2. **资源管理**: 记得调用 `Close()` 方法释放资源
3. **上下文控制**: 支持通过上下文取消操作
4. **错误处理**: 正确处理各种错误情况，包括 EOF 和上下文取消
5. **性能考虑**: 对于大数据量，考虑分块处理以避免内存问题

## 错误处理

### 常见错误

- `io.EOF`: 数据流结束或数据不足
- `io.ErrClosedPipe`: 流已关闭或 reader/writer 为 nil
- `context.Canceled`: 上下文被取消
- 其他 I/O 错误

### 错误处理最佳实践

```go
data, err := stream.ReadExact(length)
switch err {
case nil:
    // 成功读取
    fmt.Printf("读取成功: %d 字节\n", len(data))
case io.EOF:
    // 数据不足或流结束
    fmt.Printf("数据不足，只读取了 %d 字节\n", len(data))
case context.Canceled:
    // 上下文被取消
    fmt.Println("操作被取消")
default:
    // 其他错误
    log.Printf("读取错误: %v", err)
}
```

## 性能特性

- **内存效率**: 只分配必要的缓冲区
- **并发安全**: 使用互斥锁保证线程安全
- **上下文感知**: 支持优雅取消
- **阻塞操作**: 确保完整的数据传输 