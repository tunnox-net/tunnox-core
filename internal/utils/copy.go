package utils

import (
	"io"
	"sync"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream/transform"
)

// readWriteCloser 适配器：将 io.Reader 和 io.Writer 组合成 io.ReadWriteCloser
type readWriteCloser struct {
	io.Reader
	io.Writer
	closeFunc func() error
}

func (rw *readWriteCloser) Close() error {
	if rw.closeFunc != nil {
		return rw.closeFunc()
	}
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

// BidirectionalCopy 通用双向数据拷贝（极致性能优化版）
// connA 和 connB 是两个需要双向传输的连接
// options 包含转换器配置和日志前缀
//
// 🚀 优化点:
// 1. 使用 32KB 缓冲区（性价比最优：性能与512KB相当，内存占用低16倍）
// 2. 移除所有热路径日志
// 3. 简化错误处理
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

	// A → B（压缩 + 加密）
	go func() {
		defer wg.Done()
		defer connB.Close()

		writerB, err := options.Transformer.WrapWriter(connB)
		if err != nil {
			corelog.Errorf("BidirectionalCopy: failed to wrap writer: %v", err)
			result.SendError = err
			return
		}
		defer writerB.Close()

		// 🚀 性能优化: 使用 32KB 缓冲区
		buf := make([]byte, 32*1024)
		var totalWritten int64
		for {
			nr, err := connA.Read(buf)
			if nr > 0 {
				nw, ew := writerB.Write(buf[:nr])
				if nw > 0 {
					totalWritten += int64(nw)
				}
				if ew != nil {
					result.SendError = ew
					break
				}
				if nw != nr {
					result.SendError = io.ErrShortWrite
					break
				}
			}
			if err != nil {
				result.BytesSent = totalWritten
				if err != io.EOF {
					result.SendError = err
				}
				break
			}
		}
	}()

	// B → A（解密 + 解压）
	go func() {
		defer wg.Done()
		defer connA.Close()

		readerB, err := options.Transformer.WrapReader(connB)
		if err != nil {
			corelog.Errorf("BidirectionalCopy: failed to wrap reader: %v", err)
			result.ReceiveError = err
			return
		}

		// 🚀 性能优化: 使用 32KB 缓冲区
		buf := make([]byte, 32*1024)
		var totalWritten int64
		for {
			nr, err := readerB.Read(buf)
			if nr > 0 {
				nw, ew := connA.Write(buf[:nr])
				if nw > 0 {
					totalWritten += int64(nw)
				}
				if ew != nil {
					result.ReceiveError = ew
					break
				}
				if nw != nr {
					result.ReceiveError = io.ErrShortWrite
					break
				}
			}
			if err != nil {
				result.BytesReceived = totalWritten
				if err != io.EOF {
					result.ReceiveError = err
				}
				break
			}
		}
	}()

	wg.Wait()

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
