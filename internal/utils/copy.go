package utils

import (
	"io"
	"net"
	"sync"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/stream/transform"
)

// CloseWriter 支持半关闭（关闭写方向）的接口
type CloseWriter interface {
	CloseWrite() error
}

// readWriteCloser 适配器：将 io.Reader 和 io.Writer 组合成 io.ReadWriteCloser
type readWriteCloser struct {
	io.Reader
	io.Writer
	closeFunc      func() error
	closeWriteFunc func() error // 可选：半关闭函数
}

func (rw *readWriteCloser) Close() error {
	if rw.closeFunc != nil {
		return rw.closeFunc()
	}
	return nil
}

// CloseWrite 关闭写方向（半关闭），用于通知对端 EOF
func (rw *readWriteCloser) CloseWrite() error {
	if rw.closeWriteFunc != nil {
		return rw.closeWriteFunc()
	}
	// 如果没有专门的半关闭函数，尝试调用 Writer 的 CloseWrite
	if cw, ok := rw.Writer.(CloseWriter); ok {
		return cw.CloseWrite()
	}
	// 回退：不做任何操作（让最终的 Close 处理）
	return nil
}

// NewReadWriteCloser 创建 ReadWriteCloser 适配器
// 如果 Reader 或 Writer 为 nil，会返回错误（通过 panic 或返回 nil）
func NewReadWriteCloser(r io.Reader, w io.Writer, closeFunc func() error) io.ReadWriteCloser {
	if r == nil {
		panic("NewReadWriteCloser: Reader cannot be nil")
	}
	if w == nil {
		panic("NewReadWriteCloser: Writer cannot be nil")
	}
	return &readWriteCloser{
		Reader:    r,
		Writer:    w,
		closeFunc: closeFunc,
	}
}

// NewReadWriteCloserWithCloseWrite 创建支持半关闭的 ReadWriteCloser 适配器
func NewReadWriteCloserWithCloseWrite(r io.Reader, w io.Writer, closeFunc func() error, closeWriteFunc func() error) io.ReadWriteCloser {
	if r == nil {
		panic("NewReadWriteCloserWithCloseWrite: Reader cannot be nil")
	}
	if w == nil {
		panic("NewReadWriteCloserWithCloseWrite: Writer cannot be nil")
	}
	return &readWriteCloser{
		Reader:         r,
		Writer:         w,
		closeFunc:      closeFunc,
		closeWriteFunc: closeWriteFunc,
	}
}

// BidirectionalCopyOptions 双向拷贝配置选项
type BidirectionalCopyOptions struct {
	// 流转换器（处理压缩、加密）
	Transformer transform.StreamTransformer

	// 日志前缀（用于区分不同的拷贝场景）
	LogPrefix string

	// 拷贝完成后的回调（可选）
	OnComplete func(sent, received int64, err error)
}

// BidirectionalCopyResult 双向拷贝结果
type BidirectionalCopyResult struct {
	BytesSent     int64 // A→B 发送字节数
	BytesReceived int64 // B→A 接收字节数
	SendError     error // A→B 错误
	ReceiveError  error // B→A 错误
}

// tryCloseWrite 尝试对连接执行半关闭（关闭写方向）
// 支持多种类型：net.TCPConn、CloseWriter 接口、readWriteCloser
func tryCloseWrite(conn io.ReadWriteCloser) {
	// 尝试 net.TCPConn
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.CloseWrite()
		return
	}
	// 尝试自定义的 CloseWriter 接口
	if cw, ok := conn.(CloseWriter); ok {
		cw.CloseWrite()
		return
	}
	// 不支持半关闭，不做操作（最终由 Close 处理）
}

// BidirectionalCopy 通用双向数据拷贝（修复高并发连接关闭问题）
// connA 和 connB 是两个需要双向传输的连接
// options 包含转换器配置和日志前缀
//
// 🔧 修复要点:
// 1. 使用半关闭语义：一个方向结束时使用 CloseWrite() 通知对端 EOF
// 2. 不在单向传输结束时关闭整个连接
// 3. 等待两个方向都完成后再关闭连接
// 4. 解决高并发数据库查询时连接过早关闭导致数据截断的问题
//
// 🚀 性能优化:
// 1. 使用 32KB 缓冲区（性价比最优）
// 2. 移除热路径日志
func BidirectionalCopy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	if options == nil {
		options = &BidirectionalCopyOptions{}
	}
	if options.Transformer == nil {
		options.Transformer = &transform.NoOpTransformer{}
	}

	result := &BidirectionalCopyResult{}
	var wg sync.WaitGroup
	wg.Add(2)

	// A → B：从 A 读取数据写入 B
	go func() {
		defer wg.Done()

		writerB, err := options.Transformer.WrapWriter(connB)
		if err != nil {
			result.SendError = err
			return
		}

		buf := make([]byte, constants.CopyBufferSize)
		var totalWritten int64
		for {
			nr, readErr := connA.Read(buf)
			if nr > 0 {
				nw, writeErr := writerB.Write(buf[:nr])
				if nw > 0 {
					totalWritten += int64(nw)
				}
				if writeErr != nil {
					result.SendError = writeErr
					break
				}
				if nw != nr {
					result.SendError = io.ErrShortWrite
					break
				}
			}
			if readErr != nil {
				result.BytesSent = totalWritten
				if readErr != io.EOF {
					result.SendError = readErr
				}
				break
			}
		}

		// 关闭 writerB（刷新缓冲区）
		writerB.Close()

		// 🔧 关键修复：使用半关闭通知 B 端 EOF，而不是完全关闭
		// 这样 B→A 方向仍可继续接收响应数据
		tryCloseWrite(connB)
	}()

	// B → A：从 B 读取数据写入 A
	go func() {
		defer wg.Done()

		readerB, err := options.Transformer.WrapReader(connB)
		if err != nil {
			result.ReceiveError = err
			return
		}

		buf := make([]byte, constants.CopyBufferSize)
		var totalWritten int64
		for {
			nr, readErr := readerB.Read(buf)
			if nr > 0 {
				nw, writeErr := connA.Write(buf[:nr])
				if nw > 0 {
					totalWritten += int64(nw)
				}
				if writeErr != nil {
					result.ReceiveError = writeErr
					break
				}
				if nw != nr {
					result.ReceiveError = io.ErrShortWrite
					break
				}
			}
			if readErr != nil {
				result.BytesReceived = totalWritten
				if readErr != io.EOF {
					result.ReceiveError = readErr
				}
				break
			}
		}

		// 🔧 关键修复：使用半关闭通知 A 端 EOF
		tryCloseWrite(connA)
	}()

	// 等待两个方向都完成
	wg.Wait()

	// 🔧 在两个方向都完成后，安全地关闭连接
	connA.Close()
	connB.Close()

	// 执行回调
	if options.OnComplete != nil {
		var err error
		if result.SendError != nil {
			err = result.SendError
		} else if result.ReceiveError != nil {
			err = result.ReceiveError
		}
		options.OnComplete(result.BytesSent, result.BytesReceived, err)
	}

	return result
}

// SimpleBidirectionalCopy 简化版本（无转换器）
func SimpleBidirectionalCopy(connA, connB io.ReadWriteCloser, logPrefix string) *BidirectionalCopyResult {
	return BidirectionalCopy(connA, connB, &BidirectionalCopyOptions{
		LogPrefix: logPrefix,
	})
}
