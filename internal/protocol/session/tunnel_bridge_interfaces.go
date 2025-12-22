package session

import (
	"io"
	"net"
	"sync"

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
func checkStreamDataForwarder(stream stream.PackageStreamer) StreamDataForwarder {
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
		if w, ok := stream.(writeExact); ok {
			if c, ok := stream.(closer); ok {
				var ra readAvailable
				if streamRA, ok := stream.(readAvailable); ok {
					ra = streamRA
				}
				type getConnID interface {
					GetConnectionID() string
				}
				var gci getConnID
				if streamGCI, ok := stream.(getConnID); ok {
					gci = streamGCI
				}
				return &streamDataForwarderWrapper{
					readExact:     r,
					readAvailable: ra,
					writeExact:    w,
					closer:        c,
					getConnID:     gci,
				}
			}
		}
	}
	return nil
}

// checkStreamDataForwarderFromReaderWriter 检查 reader/writer 是否实现了 StreamDataForwarder 接口
func checkStreamDataForwarderFromReaderWriter(reader io.Reader, writer io.Writer) StreamDataForwarder {
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
		Close() error
	}
	type getConnID interface {
		GetConnectionID() string
	}

	r, hasReadExact := reader.(readExact)
	ra, _ := reader.(readAvailable)
	w, hasWriteExact := writer.(writeExact)

	var c closer
	if readerCloser, ok := reader.(closer); ok {
		c = readerCloser
	} else if writerCloser, ok := writer.(closer); ok {
		c = writerCloser
	}

	var gci getConnID
	if readerGCI, ok := reader.(getConnID); ok {
		gci = readerGCI
	} else if writerGCI, ok := writer.(getConnID); ok {
		gci = writerGCI
	}

	if !hasReadExact || !hasWriteExact {
		return nil
	}

	var closerWrapper interface{ Close() }
	if c != nil {
		closerWrapper = &closerAdapter{c}
	}

	return &streamDataForwarderWrapper{
		readExact:     r,
		readAvailable: ra,
		writeExact:    w,
		closer:        closerWrapper,
		getConnID:     gci,
	}
}

// closerAdapter 将 Close() error 适配为 Close()
type closerAdapter struct {
	closer interface{ Close() error }
}

func (a *closerAdapter) Close() {
	_ = a.closer.Close()
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
	if w.readAvailable != nil {
		return w.readAvailable.ReadAvailable(maxLength)
	}
	// 如果没有 ReadAvailable，回退到 ReadExact
	if maxLength > 256 {
		maxLength = 256
	}
	return w.readExact.ReadExact(maxLength)
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

// createDataForwarder 创建数据转发器（通过接口抽象，不依赖具体协议）
func createDataForwarder(conn net.Conn, stream stream.PackageStreamer) DataForwarder {
	if stream != nil {
		// 首先检查 stream 本身是否实现了 StreamDataForwarder 接口（如 HTTPPoll）
		if streamForwarder := checkStreamDataForwarder(stream); streamForwarder != nil {
			return &streamDataForwarderAdapter{stream: streamForwarder}
		}

		// 然后检查 Stream 的 Reader/Writer
		reader := stream.GetReader()
		writer := stream.GetWriter()

		if reader != nil && writer != nil {
			// 检查 reader 是否实现了 StreamDataForwarder 接口（如 WebSocketServerConn）
			if readerForwarder := checkStreamDataForwarderFromReaderWriter(reader, writer); readerForwarder != nil {
				return &streamDataForwarderAdapter{stream: readerForwarder}
			}

			// 使用 Stream 的 Reader/Writer 创建适配器
			return utils.NewReadWriteCloser(reader, writer, func() error {
				stream.Close()
				return nil
			})
		}
	}

	// 如果 Stream 不可用，回退到 net.Conn（仅用于纯 TCP 协议）
	if conn != nil {
		return conn
	}

	return nil
}
