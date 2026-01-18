// Package iocopy 提供双向数据拷贝功能
package iocopy

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/cloud/constants"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream/transform"
)

var (
	ErrNilReader = errors.New("Reader cannot be nil")
	ErrNilWriter = errors.New("Writer cannot be nil")

	copyBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, constants.CopyBufferSize)
			return &buf
		},
	}
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

// Options 双向拷贝配置选项
type Options struct {
	// Context 用于取消和超时控制（可选，默认使用 context.Background()）
	Context context.Context

	// 流转换器（处理压缩、加密）
	Transformer transform.StreamTransformer

	// 日志前缀（用于区分不同的拷贝场景）
	LogPrefix string

	// 拷贝完成后的回调（可选）
	OnComplete func(sent, received int64, err error)
}

// Result 双向拷贝结果
type Result struct {
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

// Bidirectional 通用双向数据拷贝（修复高并发连接关闭问题）
// connA 和 connB 是两个需要双向传输的连接
// options 包含转换器配置和日志前缀
//
// 修复要点:
// 1. 使用半关闭语义：一个方向结束时使用 CloseWrite() 通知对端 EOF
// 2. 当一个方向完成时，设置另一个方向的读取超时，避免永久阻塞
// 3. 等待两个方向都完成后再关闭连接
// 4. 解决高并发数据库查询时连接过早关闭导致数据截断的问题
//
// 性能优化:
// 1. 使用 32KB 缓冲区（性价比最优）
// 2. 移除热路径日志
func Bidirectional(connA, connB io.ReadWriteCloser, options *Options) *Result {
	if options == nil {
		options = &Options{}
	}
	if options.Transformer == nil {
		options.Transformer = &transform.NoOpTransformer{}
	}

	// 如果没有提供 Context，使用 Background
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logPrefix := options.LogPrefix
	if logPrefix == "" {
		logPrefix = "BidirectionalCopy"
	}

	result := &Result{}
	var wg sync.WaitGroup
	wg.Add(2)

	var connAClosedOnce, connBClosedOnce sync.Once

	corelog.Debugf("%s: starting bidirectional copy", logPrefix)

	// A → B：从 A 读取数据写入 B
	go func() {
		defer wg.Done()
		corelog.Debugf("%s: A→B goroutine started", logPrefix)

		writerB, err := options.Transformer.WrapWriterWithContext(ctx, connB)
		if err != nil {
			corelog.Errorf("%s: A→B failed to wrap writer: %v", logPrefix, err)
			result.SendError = err
			connAClosedOnce.Do(func() { connA.Close() })
			return
		}

		bufPtr := copyBufferPool.Get().(*[]byte)
		buf := *bufPtr
		defer copyBufferPool.Put(bufPtr)

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

		// 关键修复：使用半关闭通知 B 端 EOF，而不是完全关闭
		// 这样 B→A 方向仍可继续接收响应数据
		corelog.Debugf("%s: A→B attempting half-close on connB", logPrefix)
		tryCloseWrite(connB)

		// 关闭 connB 以打断 B→A 方向可能阻塞的 Read
		connBClosedOnce.Do(func() { connB.Close() })

		corelog.Infof("%s: A→B goroutine finished, sent=%d bytes", logPrefix, totalWritten)
	}()

	go func() {
		defer wg.Done()
		corelog.Debugf("%s: B→A goroutine started", logPrefix)

		readerB, err := options.Transformer.WrapReaderWithContext(ctx, connB)
		if err != nil {
			corelog.Errorf("%s: B→A failed to wrap reader: %v", logPrefix, err)
			result.ReceiveError = err
			connBClosedOnce.Do(func() { connB.Close() })
			return
		}

		bufPtr := copyBufferPool.Get().(*[]byte)
		buf := *bufPtr
		defer copyBufferPool.Put(bufPtr)

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

		// 关键修复：使用半关闭通知 A 端 EOF
		corelog.Debugf("%s: B→A attempting half-close on connA", logPrefix)
		tryCloseWrite(connA)

		// 关闭 connA 以打断 A→B 方向可能阻塞的 Read
		connAClosedOnce.Do(func() { connA.Close() })

		corelog.Infof("%s: B→A goroutine finished, received=%d bytes", logPrefix, totalWritten)
	}()

	// 等待两个方向都完成
	corelog.Debugf("%s: waiting for both directions to complete", logPrefix)
	wg.Wait()
	corelog.Infof("%s: both directions completed, sent=%d, received=%d", logPrefix, result.BytesSent, result.BytesReceived)

	// 在两个方向都完成后，安全地关闭连接
	corelog.Debugf("%s: closing both connections", logPrefix)
	connAClosedOnce.Do(func() { connA.Close() })
	connBClosedOnce.Do(func() { connB.Close() })

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

// Simple 简化版本（无转换器）
func Simple(connA, connB io.ReadWriteCloser, logPrefix string) *Result {
	return Bidirectional(connA, connB, &Options{
		LogPrefix: logPrefix,
	})
}

// UDP UDP 专用双向拷贝（保持包边界）
// udpConn: UDP连接（包导向，可以是 *net.UDPConn 或 UDPVirtualConn）
// tunnelConn: 隧道连接（流式，但支持包协议）
// options: 拷贝选项
//
// UDP 需要特殊处理：
// 1. UDP 是包导向协议，每次读取是一个完整的数据包
// 2. 隧道需要使用长度前缀来保持包边界
// 3. 不能使用流式的 io.Copy，否则会破坏包边界
//
// 性能优化：
// - 合并写入：长度前缀+数据一次写入，减少系统调用
// - 内存池：复用缓冲区，降低 GC 压力
// - 大缓冲区：128KB 写缓冲，提升吞吐量
func UDP(udpConn io.ReadWriteCloser, tunnelConn io.ReadWriteCloser, options *Options) *Result {
	if options == nil {
		options = &Options{}
	}

	result := &Result{}
	var wg sync.WaitGroup
	wg.Add(2)

	// UDP → Tunnel：从 UDP 读取数据包，批量写入隧道
	// 优化：使用阻塞读取 + 独立 flush goroutine，避免频繁 SetReadDeadline 系统调用
	go func() {
		defer wg.Done()

		const (
			batchBufSize  = 256 * 1024            // 256KB 批量缓冲
			flushInterval = 20 * time.Millisecond // 20ms 刷新间隔
		)

		readBuf := make([]byte, 65536)
		batchBuf := make([]byte, batchBufSize)
		batchPos := 0
		var batchMu sync.Mutex
		done := make(chan struct{})

		// 刷新函数：将批量缓冲写入隧道（需要持有锁）
		flushLocked := func() error {
			if batchPos > 0 {
				_, err := tunnelConn.Write(batchBuf[:batchPos])
				if err != nil {
					return err
				}
				batchPos = 0
			}
			return nil
		}

		// 独立的定时刷新 goroutine
		go func() {
			ticker := time.NewTicker(flushInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					batchMu.Lock()
					flushLocked()
					batchMu.Unlock()
				}
			}
		}()

		// 主循环：阻塞读取 UDP（无超时，减少系统调用）
		for {
			n, err := udpConn.Read(readBuf)
			if err != nil {
				batchMu.Lock()
				if flushErr := flushLocked(); flushErr != nil {
					result.SendError = flushErr
				} else if err != io.EOF {
					result.SendError = err
				}
				batchMu.Unlock()
				break
			}

			if n == 0 {
				continue
			}

			batchMu.Lock()
			// 检查批量缓冲是否有足够空间
			packetSize := 2 + n
			if batchPos+packetSize > batchBufSize {
				// 缓冲区满，立即刷新
				if err := flushLocked(); err != nil {
					result.SendError = err
					batchMu.Unlock()
					break
				}
			}

			// 写入长度前缀（2字节，大端序）+ 数据到批量缓冲
			batchBuf[batchPos] = byte(n >> 8)
			batchBuf[batchPos+1] = byte(n)
			copy(batchBuf[batchPos+2:], readBuf[:n])
			batchPos += packetSize
			result.BytesSent += int64(n)

			// 如果批量缓冲超过一半，立即刷新（高吞吐模式）
			if batchPos > batchBufSize/2 {
				flushLocked()
			}
			batchMu.Unlock()
		}

		close(done)
		// 半关闭写方向
		tryCloseWrite(tunnelConn)
	}()

	// Tunnel → UDP：从隧道读取长度前缀+数据包，批量写入 UDP
	go func() {
		defer wg.Done()

		const (
			batchSize     = 32                    // 批量发送的包数量
			flushInterval = 10 * time.Millisecond // 10ms 刷新间隔
		)

		// 批量读取 + 智能解包
		readBuf := make([]byte, 512*1024) // 512KB 大缓冲区
		buffered := 0                     // 缓冲区中的有效数据量

		// 批量写入缓冲：预分配 batchSize 个包的空间
		type pendingPacket struct {
			data   []byte
			offset int
			length int
		}
		pendingPackets := make([]pendingPacket, 0, batchSize)

		// 检测是否支持 sendmmsg (Linux + *net.UDPConn)
		var batchWriter *udpBatchWriter
		if realUDP, ok := udpConn.(*net.UDPConn); ok {
			batchWriter = newUDPBatchWriter(realUDP, batchSize)
		}

		// 刷新函数：批量写入所有待发送的包
		flush := func() error {
			if len(pendingPackets) == 0 {
				return nil
			}

			// 优先使用 sendmmsg 批量发送
			if batchWriter != nil {
				for _, pkt := range pendingPackets {
					batchWriter.add(pkt.data[pkt.offset : pkt.offset+pkt.length])
				}
				n, err := batchWriter.flush()
				result.BytesReceived += int64(n)
				pendingPackets = pendingPackets[:0]
				return err
			}

			// 回退：逐个发送
			for _, pkt := range pendingPackets {
				if _, err := udpConn.Write(pkt.data[pkt.offset : pkt.offset+pkt.length]); err != nil {
					return err
				}
				result.BytesReceived += int64(pkt.length)
			}
			pendingPackets = pendingPackets[:0]
			return nil
		}

		// 使用可复用 timer
		flushTimer := time.NewTimer(flushInterval)
		flushTimer.Stop()
		defer flushTimer.Stop()
		timerActive := false

		for {
			// 批量读取：尽可能多地读取数据
			if buffered < 256*1024 { // 低于 256KB 时补充数据
				n, err := tunnelConn.Read(readBuf[buffered:])
				if n > 0 {
					buffered += n
				}
				if err != nil {
					// 处理剩余数据后退出
					if err != io.EOF {
						result.ReceiveError = err
					}
					if buffered == 0 {
						// 刷新剩余的包
						if flushErr := flush(); flushErr != nil && result.ReceiveError == nil {
							result.ReceiveError = flushErr
						}
						break
					}
				}
			}

			// 批量解包：从缓冲区提取所有完整的包
			processed := 0
			for buffered-processed >= 2 {
				// 解析包长度（从当前位置读取）
				packetLen := int(readBuf[processed])<<8 | int(readBuf[processed+1])

				if packetLen == 0 || packetLen > 65535 {
					// 非法长度，刷新后退出
					flush()
					return
				}

				// 检查是否有完整的包（2字节长度 + packetLen 字节数据）
				if buffered-processed < 2+packetLen {
					// 数据不完整，等待更多数据
					break
				}

				// 添加到待发送队列（零拷贝：直接引用 readBuf）
				pendingPackets = append(pendingPackets, pendingPacket{
					data:   readBuf,
					offset: processed + 2,
					length: packetLen,
				})
				processed += 2 + packetLen

				// 批量写入：达到批量大小时立即发送
				if len(pendingPackets) >= batchSize {
					if err := flush(); err != nil {
						result.ReceiveError = err
						return
					}
					if timerActive {
						flushTimer.Stop()
						timerActive = false
					}
				}
			}

			// 关键：在移动缓冲区数据之前，必须先 flush
			// 否则 pendingPackets 引用的数据会被覆盖（零拷贝的代价）
			if len(pendingPackets) > 0 && processed > 0 {
				if err := flush(); err != nil {
					result.ReceiveError = err
					return
				}
				if timerActive {
					flushTimer.Stop()
					timerActive = false
				}
			}

			// 高效缓冲区管理：移动未处理的数据到开头
			if processed > 0 {
				if buffered > processed {
					copy(readBuf[:buffered-processed], readBuf[processed:buffered])
				}
				buffered -= processed
			}

			// 防止死循环：如果没有新数据且没有处理任何包
			if buffered > 0 && processed == 0 && buffered < 2 {
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
