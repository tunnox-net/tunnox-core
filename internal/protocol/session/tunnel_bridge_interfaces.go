package session

import (
	"io"
	"net"
	"sync"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

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

// checkStreamDataForwarder 检查 stream 是否实现了 StreamDataForwarder 接口
// 使用接口查询，避免跨包类型断言问题
func checkStreamDataForwarder(stream stream.PackageStreamer) StreamDataForwarder {
	// 使用接口查询检查是否有 ReadExact、ReadAvailable 和 WriteExact 方法
	type readExact interface {
		ReadExact(length int) ([]byte, error)
	}
	type readAvailable interface {
		ReadAvailable(maxLength int) ([]byte, error)
	}
	type writeExact interface {
		WriteExact(data []byte) error
	}
	type closer interface {
		Close()
	}

	if r, ok := stream.(readExact); ok {
		corelog.Infof("checkStreamDataForwarder: detected ReadExact method")
		if w, ok := stream.(writeExact); ok {
			corelog.Infof("checkStreamDataForwarder: detected WriteExact method")
			if c, ok := stream.(closer); ok {
				corelog.Infof("checkStreamDataForwarder: detected Close method")
				// 检查是否有 ReadAvailable 方法
				var ra readAvailable
				if streamRA, ok := stream.(readAvailable); ok {
					ra = streamRA
					corelog.Infof("checkStreamDataForwarder: detected ReadAvailable method in stream")
				} else {
					corelog.Warnf("checkStreamDataForwarder: ReadAvailable method not found in stream, will fallback to ReadExact")
				}
				// 检查是否有 GetConnectionID 方法
				type getConnID interface {
					GetConnectionID() string
				}
				var gci getConnID
				if streamGCI, ok := stream.(getConnID); ok {
					gci = streamGCI
					connID := streamGCI.GetConnectionID()
					corelog.Infof("checkStreamDataForwarder: detected GetConnectionID method in stream, connID=%s", connID)
				} else {
					corelog.Warnf("checkStreamDataForwarder: GetConnectionID method not found in stream")
				}
				// 创建一个包装器，实现 StreamDataForwarder 接口
				corelog.Infof("checkStreamDataForwarder: creating streamDataForwarderWrapper, hasReadAvailable=%v, hasGetConnID=%v", ra != nil, gci != nil)
				return &streamDataForwarderWrapper{
					readExact:     r,
					readAvailable: ra,
					writeExact:    w,
					closer:        c,
					getConnID:     gci,
				}
			} else {
				corelog.Warnf("checkStreamDataForwarder: Close method not found")
			}
		} else {
			corelog.Warnf("checkStreamDataForwarder: WriteExact method not found")
		}
	} else {
		corelog.Warnf("checkStreamDataForwarder: ReadExact method not found")
	}
	corelog.Warnf("checkStreamDataForwarder: returning nil (stream does not implement required methods)")
	return nil
}

// streamDataForwarderWrapper 包装实现了 ReadExact/ReadAvailable/WriteExact/Close 的对象
type streamDataForwarderWrapper struct {
	readExact interface {
		ReadExact(length int) ([]byte, error)
	}
	readAvailable interface {
		ReadAvailable(maxLength int) ([]byte, error)
	}
	writeExact interface{ WriteExact(data []byte) error }
	closer     interface{ Close() }
	getConnID  interface{ GetConnectionID() string } // 可选的 GetConnectionID 方法
}

func (w *streamDataForwarderWrapper) ReadExact(length int) ([]byte, error) {
	return w.readExact.ReadExact(length)
}

func (w *streamDataForwarderWrapper) ReadAvailable(maxLength int) ([]byte, error) {
	connID := "unknown"
	if w.getConnID != nil {
		connID = w.getConnID.GetConnectionID()
	}
	if w.readAvailable != nil {
		corelog.Infof("streamDataForwarderWrapper[connID=%s]: ReadAvailable calling underlying ReadAvailable, maxLength=%d", connID, maxLength)
		data, err := w.readAvailable.ReadAvailable(maxLength)
		corelog.Infof("streamDataForwarderWrapper[connID=%s]: ReadAvailable returned, data len=%d, err=%v", connID, len(data), err)
		return data, err
	}
	// 如果没有 ReadAvailable，回退到 ReadExact（但只请求较小的长度）
	corelog.Warnf("streamDataForwarderWrapper[connID=%s]: ReadAvailable not available, falling back to ReadExact, maxLength=%d", connID, maxLength)
	if maxLength > 256 {
		maxLength = 256
	}
	data, err := w.readExact.ReadExact(maxLength)
	corelog.Infof("streamDataForwarderWrapper[connID=%s]: ReadExact (fallback) returned, data len=%d, err=%v", connID, len(data), err)
	return data, err
}

func (w *streamDataForwarderWrapper) WriteExact(data []byte) error {
	return w.writeExact.WriteExact(data)
}

func (w *streamDataForwarderWrapper) Close() {
	w.closer.Close()
}

func (w *streamDataForwarderWrapper) GetConnectionID() string {
	if w.getConnID != nil {
		return w.getConnID.GetConnectionID()
	}
	return "unknown"
}

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
		corelog.Debugf("streamDataForwarderAdapter: Read from buffer, n=%d, remaining=%d", n, len(a.buf))
		return n, nil
	}

	// 从流读取数据：使用 ReadAvailable 读取可用数据，避免长时间阻塞
	// ReadAvailable 会返回可用数据，不等待完整长度，适合 io.Reader 接口
	// 对于大数据包，需要支持更大的 maxLength 以确保数据完整性
	maxLength := len(p)
	if maxLength > 32*1024 {
		maxLength = 32 * 1024 // 限制最大读取长度为 32KB，平衡性能和内存使用
	}
	if maxLength == 0 {
		return 0, nil
	}

	data, err := a.stream.ReadAvailable(maxLength)
	if err != nil {
		if err == io.EOF {
			a.closed = true
		}
		// 如果有部分数据，返回它
		if len(data) > 0 {
			n := copy(p, data)
			return n, nil
		}
		return 0, err
	}

	if len(data) == 0 {
		// 没有数据，返回0但不返回错误（表示暂时没有数据）
		// 注意：io.Reader 的 Read 方法返回 0, nil 表示暂时没有数据，调用者应该重试
		return 0, nil
	}

	// 返回可用数据
	n := copy(p, data)
	if n < len(data) {
		// 如果读取的数据比请求的多，将多余的数据放入缓冲区
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

// createDataForwarder 创建数据转发器（通过接口抽象，不依赖具体协议）
// 优先使用 net.Conn，如果没有则尝试从 Stream 创建适配器
func createDataForwarder(conn net.Conn, stream stream.PackageStreamer) DataForwarder {
	corelog.Infof("createDataForwarder: called, conn=%v, stream=%v, stream type=%T", conn != nil, stream != nil, stream)
	if conn != nil {
		corelog.Infof("createDataForwarder: using net.Conn, remoteAddr=%s", conn.RemoteAddr())
		return conn // net.Conn 实现了 io.ReadWriteCloser
	}
	if stream != nil {
		reader := stream.GetReader()
		writer := stream.GetWriter()
		corelog.Infof("createDataForwarder: stream has GetReader=%v, GetWriter=%v", reader != nil, writer != nil)
		if reader != nil && writer != nil {
			// 使用 Stream 的 Reader/Writer 创建适配器
			corelog.Infof("createDataForwarder: using Stream Reader/Writer adapter")
			return utils.NewReadWriteCloser(reader, writer, func() error {
				stream.Close()
				return nil
			})
		}
		// 如果 GetReader/GetWriter 返回 nil，尝试使用 ReadExact/WriteExact（HTTP 长轮询）
		// 使用接口查询，检查是否有 ReadExact 和 WriteExact 方法
		corelog.Infof("createDataForwarder: checking stream for StreamDataForwarder, stream type=%T", stream)
		if streamForwarder := checkStreamDataForwarder(stream); streamForwarder != nil {
			connID := "unknown"
			if connIDGetter, ok := streamForwarder.(interface{ GetConnectionID() string }); ok {
				connID = connIDGetter.GetConnectionID()
			}
			corelog.Infof("createDataForwarder: StreamDataForwarder detected, creating adapter, connID=%s", connID)
			return &streamDataForwarderAdapter{stream: streamForwarder}
		}
		corelog.Warnf("createDataForwarder: StreamDataForwarder not detected, stream type=%T", stream)
	}
	// 如果都没有，返回 nil（表示该协议不支持桥接）
	corelog.Warnf("createDataForwarder: returning nil (no suitable forwarder found)")
	return nil
}
