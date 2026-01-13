// Package tunnel 提供隧道桥接和路由功能
package tunnel

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/cloud/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils/iocopy"

	"golang.org/x/time/rate"
)

// ============================================================================
// 数据转发接口
// ============================================================================

// DataForwarder 数据转发接口（依赖倒置：不依赖具体协议）
// 抽象了不同协议的数据转发能力
type DataForwarder interface {
	io.ReadWriteCloser
}

// StreamDataForwarder 流数据转发器接口（用于 HTTP 长轮询等协议）
type StreamDataForwarder interface {
	ReadExact(length int) ([]byte, error)
	ReadAvailable(maxLength int) ([]byte, error) // 读取可用数据（不等待完整长度）
	WriteExact(data []byte) error
	Close()
	GetConnectionID() string // 获取连接ID（用于调试）
}

// checkStreamDataForwarder 检查 stream 是否实现了完整的 StreamDataForwarder 接口
func checkStreamDataForwarder(s stream.PackageStreamer) StreamDataForwarder {
	if forwarder, ok := s.(StreamDataForwarder); ok {
		return forwarder
	}
	return nil
}

// ============================================================================
// StreamDataForwarder 适配器
// ============================================================================

// streamDataForwarderAdapter 将 StreamDataForwarder 适配为 DataForwarder
type streamDataForwarderAdapter struct {
	stream StreamDataForwarder
	buf    []byte
	bufMu  sync.Mutex
	closed bool
}

func (a *streamDataForwarderAdapter) Read(p []byte) (int, error) {
	a.bufMu.Lock()
	defer a.bufMu.Unlock()

	if a.closed {
		return 0, io.EOF
	}

	// 如果缓冲区有数据，先返回缓冲区数据
	if len(a.buf) > 0 {
		n := copy(p, a.buf)
		a.buf = a.buf[n:]
		return n, nil
	}

	// 从流读取数据：使用 ReadAvailable 读取可用数据，避免长时间阻塞
	maxLength := len(p)
	if maxLength > 32*1024 {
		maxLength = 32 * 1024
	}
	if maxLength == 0 {
		return 0, nil
	}

	data, err := a.stream.ReadAvailable(maxLength)
	if err != nil {
		if err == io.EOF {
			a.closed = true
		}
		if len(data) > 0 {
			n := copy(p, data)
			return n, nil
		}
		return 0, err
	}

	if len(data) == 0 {
		return 0, nil
	}

	n := copy(p, data)
	if n < len(data) {
		a.buf = append(a.buf, data[n:]...)
	}
	return n, nil
}

func (a *streamDataForwarderAdapter) Write(p []byte) (int, error) {
	if a.closed {
		return 0, io.ErrClosedPipe
	}

	if err := a.stream.WriteExact(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *streamDataForwarderAdapter) Close() error {
	a.bufMu.Lock()
	defer a.bufMu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true
	a.stream.Close()
	return nil
}

// ============================================================================
// 数据转发器工厂
// ============================================================================

// CreateDataForwarder 创建数据转发器
// 优先使用 stream 的底层 Reader/Writer，这是最通用的方式
// 只有实现了完整 StreamDataForwarder 接口的特殊协议才使用适配器
func CreateDataForwarder(conn interface{}, s stream.PackageStreamer) DataForwarder {
	if s != nil {
		// 获取底层 Reader/Writer（对于 TCP，这就是原始的 net.Conn）
		reader := s.GetReader()
		writer := s.GetWriter()

		if reader != nil && writer != nil {
			// 优先使用标准的 io.ReadWriteCloser 方式
			rwc, err := iocopy.NewReadWriteCloser(reader, writer, func() error {
				s.Close()
				return nil
			})
			if err == nil {
				return rwc
			}
			// 如果创建失败，继续尝试其他方式
		}

		// 如果 stream 实现了完整的 StreamDataForwarder 接口（如 HTTP 长轮询）
		if forwarder := checkStreamDataForwarder(s); forwarder != nil {
			return &streamDataForwarderAdapter{stream: forwarder}
		}
	}

	// 直接使用 net.Conn
	if rwc, ok := conn.(io.ReadWriteCloser); ok && rwc != nil {
		return rwc
	}

	return nil
}

// ============================================================================
// 数据拷贝和转发
// ============================================================================

// CopyWithControl 带流量统计和限速的数据拷贝（极致性能优化版）
func (b *Bridge) CopyWithControl(dst io.Writer, src io.Reader, direction string, counter *atomic.Int64) int64 {
	buf := make([]byte, constants.CopyBufferSize)
	var total int64
	var batchCounter int64
	var quotaCheckCounter int64

	checkCounter := 0

	for {
		checkCounter++
		if checkCounter >= constants.ContextCheckInterval {
			checkCounter = 0
			select {
			case <-b.Ctx().Done():
				counter.Add(batchCounter)
				return total
			default:
			}
		}

		nr, err := src.Read(buf)
		if nr > 0 {
			if b.rateLimiter != nil {
				if waitErr := b.rateLimiter.WaitN(b.Ctx(), nr); waitErr != nil {
					break
				}
			}

			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				total += int64(nw)
				batchCounter += int64(nw)

				if b.trafficMeter != nil {
					if direction == "source->target" {
						b.trafficMeter.AddSent(int64(nw))
					} else {
						b.trafficMeter.AddReceived(int64(nw))
					}
				}

				if batchCounter >= constants.BatchUpdateThreshold {
					counter.Add(batchCounter)
					batchCounter = 0
				}

				quotaCheckCounter += int64(nw)
				if b.trafficMeter != nil && quotaCheckCounter >= 1024*1024 {
					quotaCheckCounter = 0
					status := b.trafficMeter.GetQuotaStatus()
					if status.Percentage >= 120 {
						b.Close()
						break
					}
					if status.Throttled && b.rateLimiter == nil {
						b.applyQuotaThrottle(status.CurrentRate)
					}
				}
			}
			if ew != nil {
				break
			}
			if nr != nw {
				break
			}
		}
		if err != nil {
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			break
		}
	}

	if batchCounter > 0 {
		counter.Add(batchCounter)
	}
	return total
}

// applyQuotaThrottle 应用配额限速
func (b *Bridge) applyQuotaThrottle(rateLimit int64) {
	if rateLimit <= 0 {
		rateLimit = 100 * 1024
	}
	b.rateLimiter = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit*2))
}

// dynamicSourceWriter 动态获取 sourceForwarder 的 Writer 包装器
// 用于在 target->source 方向时，每次写入都使用最新的 sourceForwarder
type dynamicSourceWriter struct {
	bridge *Bridge
}

func (w *dynamicSourceWriter) Write(p []byte) (n int, err error) {
	w.bridge.sourceConnMu.RLock()
	sourceForwarder := w.bridge.sourceForwarder
	w.bridge.sourceConnMu.RUnlock()

	if sourceForwarder == nil {
		return 0, io.ErrClosedPipe
	}
	return sourceForwarder.Write(p)
}

// ============================================================================
// Start 方法
// ============================================================================

// Start 启动桥接（高性能版本）
func (b *Bridge) Start() error {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	select {
	case <-b.ready:
		// 目标连接已建立
	case <-timer.C:
		return coreerrors.New(coreerrors.CodeTimeout, "timeout waiting for target connection")
	case <-b.Ctx().Done():
		return coreerrors.New(coreerrors.CodeCancelled, "bridge cancelled before target connection")
	}

	// 跨节点场景：数据转发由 CrossNodeListener 负责，这里只管理生命周期
	// periodicTrafficReport 已在 NewBridge 中通过 StartPeriodicTrafficReport 启动
	if b.GetCrossNodeConnection() != nil {
		// 等待跨节点转发完成（由 CrossNodeListener.runBridgeForward 处理）
		<-b.Ctx().Done()
		return nil
	}

	// 检查数据转发器是否可用
	if b.sourceForwarder == nil {
		b.sourceForwarder = CreateDataForwarder(b.sourceConn, b.sourceStream)
	}
	if b.targetForwarder == nil {
		b.targetForwarder = CreateDataForwarder(b.targetConn, b.targetStream)
	}

	// 如果源端或目标端没有数据转发器，只管理连接生命周期
	if b.sourceForwarder == nil || b.targetForwarder == nil {
		<-b.Ctx().Done()
		return nil
	}

	// 使用 WaitGroup 等待任一方向的数据转发完成
	// 当任一方向完成时，关闭 bridge 并等待另一方向也完成
	var wg sync.WaitGroup
	var closeOnce sync.Once
	closeBridge := func() {
		closeOnce.Do(func() {
			b.Close()
		})
	}

	wg.Add(2)

	// 启动双向数据转发
	// 源端 -> 目标端
	go func() {
		defer wg.Done()
		defer closeBridge()

		for {
			b.sourceConnMu.RLock()
			sourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if sourceForwarder == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			b.CopyWithControl(b.targetForwarder, sourceForwarder, "source->target", &b.bytesSent)

			// 检查连接是否更新
			b.sourceConnMu.RLock()
			newSourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if newSourceForwarder == nil || newSourceForwarder == sourceForwarder {
				break
			}
		}
	}()

	// 目标端 -> 源端
	go func() {
		defer wg.Done()
		defer closeBridge()

		dynamicWriter := &dynamicSourceWriter{bridge: b}
		b.CopyWithControl(dynamicWriter, b.targetForwarder, "target->source", &b.bytesReceived)
	}()

	// 等待两个方向的数据转发都完成
	// 当任一方向完成时，closeBridge() 会被调用，关闭连接
	// 这会导致另一方向的 Read/Write 返回错误，从而也完成
	wg.Wait()

	return nil
}
