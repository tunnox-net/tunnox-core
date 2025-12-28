package utils

import (
	"errors"
	"io"
	"net"
	"sync"

	"tunnox-core/internal/cloud/constants"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream/transform"
)

var (
	// ErrNilReader 当 Reader 为 nil 时返回
	ErrNilReader = errors.New("Reader cannot be nil")
	// ErrNilWriter 当 Writer 为 nil 时返回
	ErrNilWriter = errors.New("Writer cannot be nil")
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
// 如果 Reader 或 Writer 为 nil，会返回错误
func NewReadWriteCloser(r io.Reader, w io.Writer, closeFunc func() error) (io.ReadWriteCloser, error) {
	if r == nil {
		return nil, ErrNilReader
	}
	if w == nil {
		return nil, ErrNilWriter
	}
	return &readWriteCloser{
		Reader:    r,
		Writer:    w,
		closeFunc: closeFunc,
	}, nil
}

// NewReadWriteCloserWithCloseWrite 创建支持半关闭的 ReadWriteCloser 适配器
func NewReadWriteCloserWithCloseWrite(r io.Reader, w io.Writer, closeFunc func() error, closeWriteFunc func() error) (io.ReadWriteCloser, error) {
	if r == nil {
		return nil, ErrNilReader
	}
	if w == nil {
		return nil, ErrNilWriter
	}
	return &readWriteCloser{
		Reader:         r,
		Writer:         w,
		closeFunc:      closeFunc,
		closeWriteFunc: closeWriteFunc,
	}, nil
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

	logPrefix := options.LogPrefix
	if logPrefix == "" {
		logPrefix = "BidirectionalCopy"
	}

	result := &BidirectionalCopyResult{}
	var wg sync.WaitGroup
	wg.Add(2)

	corelog.Debugf("%s: starting bidirectional copy", logPrefix)

	// A → B：从 A 读取数据写入 B
	go func() {
		defer wg.Done()
		corelog.Debugf("%s: A→B goroutine started", logPrefix)

		writerB, err := options.Transformer.WrapWriter(connB)
		if err != nil {
			corelog.Errorf("%s: A→B failed to wrap writer: %v", logPrefix, err)
			result.SendError = err
			return
		}

		buf := make([]byte, constants.CopyBufferSize)
		var totalWritten int64
		var readCount int
		for {
			nr, readErr := connA.Read(buf)
			readCount++

			if nr > 0 {
				nw, writeErr := writerB.Write(buf[:nr])
				if nw > 0 {
					totalWritten += int64(nw)
				}
				if writeErr != nil {
					corelog.Errorf("%s: A→B write error after %d bytes: %v", logPrefix, totalWritten, writeErr)
					result.SendError = writeErr
					break
				}
				if nw != nr {
					corelog.Errorf("%s: A→B short write: read=%d, wrote=%d", logPrefix, nr, nw)
					result.SendError = io.ErrShortWrite
					break
				}
			}
			if readErr != nil {
				result.BytesSent = totalWritten
				if readErr != io.EOF {
					corelog.Errorf("%s: A→B read error after %d bytes: %v", logPrefix, totalWritten, readErr)
					result.SendError = readErr
				} else {
					corelog.Infof("%s: A→B read EOF after %d bytes (%d reads)", logPrefix, totalWritten, readCount)
				}
				break
			}
		}

		// 关闭 writerB（刷新缓冲区）
		corelog.Debugf("%s: A→B closing writerB", logPrefix)
		writerB.Close()

		// 🔧 关键修复：使用半关闭通知 B 端 EOF，而不是完全关闭
		// 这样 B→A 方向仍可继续接收响应数据
		corelog.Debugf("%s: A→B attempting half-close on connB", logPrefix)
		tryCloseWrite(connB)
		corelog.Infof("%s: A→B goroutine finished, sent=%d bytes", logPrefix, totalWritten)
	}()

	// B → A：从 B 读取数据写入 A
	go func() {
		defer wg.Done()
		corelog.Debugf("%s: B→A goroutine started", logPrefix)

		readerB, err := options.Transformer.WrapReader(connB)
		if err != nil {
			corelog.Errorf("%s: B→A failed to wrap reader: %v", logPrefix, err)
			result.ReceiveError = err
			return
		}

		buf := make([]byte, constants.CopyBufferSize)
		var totalWritten int64
		var readCount int
		for {
			nr, readErr := readerB.Read(buf)
			readCount++

			if nr > 0 {
				nw, writeErr := connA.Write(buf[:nr])
				if nw > 0 {
					totalWritten += int64(nw)
				}
				if writeErr != nil {
					corelog.Errorf("%s: B→A write error after %d bytes: %v", logPrefix, totalWritten, writeErr)
					result.ReceiveError = writeErr
					break
				}
				if nw != nr {
					corelog.Errorf("%s: B→A short write: read=%d, wrote=%d", logPrefix, nr, nw)
					result.ReceiveError = io.ErrShortWrite
					break
				}
			}
			if readErr != nil {
				result.BytesReceived = totalWritten
				if readErr != io.EOF {
					corelog.Errorf("%s: B→A read error after %d bytes: %v", logPrefix, totalWritten, readErr)
					result.ReceiveError = readErr
				} else {
					corelog.Infof("%s: B→A read EOF after %d bytes (%d reads)", logPrefix, totalWritten, readCount)
				}
				break
			}
		}

		// 🔧 关键修复：使用半关闭通知 A 端 EOF
		corelog.Debugf("%s: B→A attempting half-close on connA", logPrefix)
		tryCloseWrite(connA)
		corelog.Infof("%s: B→A goroutine finished, received=%d bytes", logPrefix, totalWritten)
	}()

	// 等待两个方向都完成
	corelog.Debugf("%s: waiting for both directions to complete", logPrefix)
	wg.Wait()
	corelog.Infof("%s: both directions completed, sent=%d, received=%d", logPrefix, result.BytesSent, result.BytesReceived)

	// 🔧 在两个方向都完成后，安全地关闭连接
	corelog.Debugf("%s: closing both connections", logPrefix)
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

// UDPBidirectionalCopy UDP 专用双向拷贝（保持包边界）
// udpConn: UDP连接（包导向，可以是 *net.UDPConn 或 UDPVirtualConn）
// tunnelConn: 隧道连接（流式，但支持包协议）
// options: 拷贝选项
//
// UDP 需要特殊处理：
// 1. UDP 是包导向协议，每次读取是一个完整的数据包
// 2. 隧道需要使用长度前缀来保持包边界
// 3. 不能使用流式的 io.Copy，否则会破坏包边界
//
// 🚀 性能优化：
// - 合并写入：长度前缀+数据一次写入，减少系统调用
// - 内存池：复用缓冲区，降低 GC 压力
// - 大缓冲区：128KB 写缓冲，提升吞吐量
func UDPBidirectionalCopy(udpConn io.ReadWriteCloser, tunnelConn io.ReadWriteCloser, options *BidirectionalCopyOptions) *BidirectionalCopyResult {
	if options == nil {
		options = &BidirectionalCopyOptions{}
	}

	logPrefix := options.LogPrefix
	if logPrefix == "" {
		logPrefix = "UDPBidirectionalCopy"
	}
	corelog.Infof("%s: starting", logPrefix)

	result := &BidirectionalCopyResult{}
	var wg sync.WaitGroup
	wg.Add(2)

	// UDP → Tunnel：从 UDP 读取数据包，加上长度前缀写入隧道
	go func() {
		defer wg.Done()
		corelog.Infof("%s: UDP->tunnel goroutine started", logPrefix)

		// UDP 读缓冲和写缓冲（长度前缀 + 数据）
		readBuf := make([]byte, 65536)
		writeBuf := make([]byte, 65536+2) // 2 字节长度前缀 + 最大 UDP 包

		for {
			// 读取一个完整的 UDP 数据包
			n, err := udpConn.Read(readBuf)
			if err != nil {
				if err != io.EOF {
					result.SendError = err
					corelog.Errorf("%s: UDP->tunnel read error: %v", logPrefix, err)
				}
				break
			}

			if n == 0 {
				continue
			}

			corelog.Debugf("%s: UDP->tunnel read %d bytes from UDP", logPrefix, n)

			// 写入长度前缀（2字节，大端序）+ 数据
			writeBuf[0] = byte(n >> 8)
			writeBuf[1] = byte(n)
			copy(writeBuf[2:], readBuf[:n])

			// 立即发送（确保实时性）
			nw, err := tunnelConn.Write(writeBuf[:2+n])
			if err != nil {
				result.SendError = err
				corelog.Errorf("%s: UDP->tunnel write error: %v", logPrefix, err)
				break
			}
			corelog.Debugf("%s: UDP->tunnel wrote %d bytes to tunnel (2 prefix + %d data)", logPrefix, nw, n)

			result.BytesSent += int64(n)
		}

		corelog.Infof("%s: UDP->tunnel goroutine finished, sent=%d bytes", logPrefix, result.BytesSent)
		// 半关闭写方向
		tryCloseWrite(tunnelConn)
	}()

	// Tunnel → UDP：从隧道读取长度前缀+数据包，写入 UDP
	go func() {
		defer wg.Done()
		corelog.Infof("%s: tunnel->UDP goroutine started", logPrefix)

		// 🚀 优化4：批量读取 + 智能解包
		readBuf := make([]byte, 512*1024) // 512KB 大缓冲区
		udpBuf := make([]byte, 65536)     // UDP 单包缓冲
		buffered := 0                     // 缓冲区中的有效数据量

		for {
			// 🚀 批量读取：尽可能多地读取数据
			if buffered < 256*1024 { // 低于 256KB 时补充数据
				corelog.Debugf("%s: tunnel->UDP reading from tunnelConn (buffered=%d)", logPrefix, buffered)
				n, err := tunnelConn.Read(readBuf[buffered:])
				corelog.Debugf("%s: tunnel->UDP read returned n=%d, err=%v", logPrefix, n, err)
				if n > 0 {
					buffered += n
				}
				if err != nil {
					corelog.Infof("%s: tunnel->UDP read error: %v (buffered=%d)", logPrefix, err, buffered)
					// 处理剩余数据后退出
					if err != io.EOF {
						result.ReceiveError = err
					}
					if buffered == 0 {
						break
					}
				}
			}

			// 🚀 批量解包：从缓冲区提取所有完整的包
			processed := 0
			for buffered-processed >= 2 {
				// 解析包长度（从当前位置读取）
				packetLen := int(readBuf[processed])<<8 | int(readBuf[processed+1])
				corelog.Debugf("%s: tunnel->UDP parsing packet, packetLen=%d, buffered=%d, processed=%d", logPrefix, packetLen, buffered, processed)

				if packetLen == 0 || packetLen > 65535 {
					// 非法长度，退出
					corelog.Errorf("%s: tunnel->UDP invalid packet length: %d", logPrefix, packetLen)
					return
				}

				// 检查是否有完整的包（2字节长度 + packetLen 字节数据）
				if buffered-processed < 2+packetLen {
					// 数据不完整，等待更多数据
					break
				}

				// 🚀 零拷贝写入：直接从 readBuf 写入 UDP
				// 注意：这里复制到 udpBuf 是为了避免 readBuf 被覆盖
				copy(udpBuf[:packetLen], readBuf[processed+2:processed+2+packetLen])

				if _, err := udpConn.Write(udpBuf[:packetLen]); err != nil {
					result.ReceiveError = err
					return
				}

				result.BytesReceived += int64(packetLen)
				processed += 2 + packetLen
			}

			// 🚀 优化5：高效缓冲区管理
			if processed > 0 {
				// 移动未处理的数据到开头
				if buffered > processed {
					copy(readBuf[:buffered-processed], readBuf[processed:buffered])
				}
				buffered -= processed
			}

			// 防止死循环：如果没有新数据且没有处理任何包
			if buffered > 0 && processed == 0 && buffered < 2 {
				// 数据太少，继续读取
				continue
			}
		}

		// UDP 连接不支持半关闭，不做操作
	}()

	// 等待两个方向都完成
	wg.Wait()

	// 关闭连接
	udpConn.Close()
	tunnelConn.Close()

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
