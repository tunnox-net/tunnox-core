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

// BidirectionalCopy 通用双向数据拷贝
// connA 和 connB 是两个需要双向传输的连接
// options 包含转换器配置和日志前缀
//
// 数据流向：
//
//	A → B: 从 connA 读取 → 应用转换器（压缩、加密） → 写入 connB
//	B → A: 从 connB 读取 → 应用转换器（解密、解压） → 写入 connA
//
// 返回拷贝结果，包含发送/接收字节数和错误信息
func BidirectionalCopy(connA, connB io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	// 默认选项
	if options == nil {
		options = &BidirectionalCopyOptions{}
	}
	if options.LogPrefix == "" {
		options.LogPrefix = "BidirectionalCopy"
	}
	if options.Transformer == nil {
		options.Transformer = &transform.NoOpTransformer{}
	}

	corelog.Infof("%s: BidirectionalCopy called, connA=%v, connB=%v", options.LogPrefix, connA != nil, connB != nil)

	result := &BidirectionalCopyResult{}
	var wg sync.WaitGroup
	wg.Add(2)

	// A → B（压缩 + 加密）
	go func() {
		defer wg.Done()
		defer connB.Close() // 关闭写端

		corelog.Infof("%s: A→B started", options.LogPrefix)
		// 包装 Writer：压缩 → 加密
		writerB, err := options.Transformer.WrapWriter(connB)
		if err != nil {
			corelog.Errorf("%s: failed to wrap writer: %v", options.LogPrefix, err)
			result.SendError = err
			return
		}
		defer writerB.Close() // 确保 flush 缓冲

		// 使用带缓冲的拷贝，以便跟踪数据流
		buf := make([]byte, 32*1024)
		var totalWritten int64
		for {
			corelog.Infof("%s: A→B calling connA.Read(buf), buf size=%d", options.LogPrefix, len(buf))
			nr, err := connA.Read(buf)
			corelog.Infof("%s: A→B connA.Read returned, n=%d, err=%v", options.LogPrefix, nr, err)
			if nr > 0 {
				// 循环写入，确保所有数据都被写入
				written := 0
				for written < nr {
					nw, ew := writerB.Write(buf[written:nr])
					if nw > 0 {
						written += nw
						totalWritten += int64(nw)
						corelog.Infof("%s: A→B wrote %d bytes to tunnel (total: %d, remaining: %d)", options.LogPrefix, nw, totalWritten, nr-written)
					}
					if ew != nil {
						corelog.Errorf("%s: A→B write error: %v", options.LogPrefix, ew)
						result.SendError = ew
						break
					}
					if nw == 0 {
						corelog.Errorf("%s: A→B write returned 0 bytes, possible blocking", options.LogPrefix)
						break
					}
				}
				if written != nr {
					corelog.Errorf("%s: A→B incomplete write: %d != %d", options.LogPrefix, written, nr)
					result.SendError = io.ErrShortWrite
					break
				}
			}
			if err != nil {
				if err == io.EOF {
					corelog.Infof("%s: A→B completed, sent %d bytes (EOF)", options.LogPrefix, totalWritten)
				} else {
					corelog.Debugf("%s: A→B error: %v (total: %d bytes)", options.LogPrefix, err, totalWritten)
				}
				result.BytesSent = totalWritten
				result.SendError = err
				break
			}
		}
	}()

	// B → A（解密 + 解压）
	go func() {
		defer wg.Done()
		defer connA.Close() // 关闭写端

		corelog.Infof("%s: B→A started", options.LogPrefix)
		// 包装 Reader：解密 → 解压
		readerB, err := options.Transformer.WrapReader(connB)
		if err != nil {
			corelog.Errorf("%s: failed to wrap reader: %v", options.LogPrefix, err)
			result.ReceiveError = err
			return
		}

		// 使用带缓冲的拷贝，以便跟踪数据流
		buf := make([]byte, 32*1024)
		var totalWritten int64
		for {
			corelog.Infof("%s: B→A calling readerB.Read(buf), buf size=%d", options.LogPrefix, len(buf))
			nr, err := readerB.Read(buf)
			corelog.Infof("%s: B→A readerB.Read returned, n=%d, err=%v", options.LogPrefix, nr, err)
			if nr > 0 {
				corelog.Infof("%s: B→A read %d bytes from tunnel", options.LogPrefix, nr)
				// 循环写入，确保所有数据都被写入
				written := 0
				for written < nr {
					nw, ew := connA.Write(buf[written:nr])
					if nw > 0 {
						written += nw
						totalWritten += int64(nw)
						corelog.Infof("%s: B→A wrote %d bytes to local connection (total: %d, remaining: %d)", options.LogPrefix, nw, totalWritten, nr-written)
					}
					if ew != nil {
						corelog.Errorf("%s: B→A write error: %v", options.LogPrefix, ew)
						result.ReceiveError = ew
						break
					}
					if nw == 0 {
						corelog.Errorf("%s: B→A write returned 0 bytes, possible blocking", options.LogPrefix)
						break
					}
				}
				if written != nr {
					corelog.Errorf("%s: B→A incomplete write: %d != %d", options.LogPrefix, written, nr)
					result.ReceiveError = io.ErrShortWrite
					break
				}
			}
			if err != nil {
				if err == io.EOF {
					corelog.Infof("%s: B→A completed, received %d bytes (EOF)", options.LogPrefix, totalWritten)
				} else {
					corelog.Debugf("%s: B→A error: %v (total: %d bytes)", options.LogPrefix, err, totalWritten)
				}
				result.BytesReceived = totalWritten
				result.ReceiveError = err
				break
			}
		}
	}()

	wg.Wait()
	corelog.Debugf("%s: completed (sent: %d, received: %d)",
		options.LogPrefix, result.BytesSent, result.BytesReceived)

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
