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

// checkStreamDataForwarder 检查 stream 是否实现了完整的 StreamDataForwarder 接口
// 注意：必须同时实现 ReadExact、ReadAvailable、WriteExact 和 Close
func checkStreamDataForwarder(s stream.PackageStreamer) StreamDataForwarder {
	// 直接检查是否实现了完整接口
	if forwarder, ok := s.(StreamDataForwarder); ok {
		return forwarder
	}
	return nil
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

// createDataForwarder 创建数据转发器
// 优先使用 stream 的底层 Reader/Writer，这是最通用的方式
// 只有实现了完整 StreamDataForwarder 接口的特殊协议才使用适配器
func createDataForwarder(conn net.Conn, s stream.PackageStreamer) DataForwarder {
	if s != nil {
		// 获取底层 Reader/Writer（对于 TCP，这就是原始的 net.Conn）
		reader := s.GetReader()
		writer := s.GetWriter()

		if reader != nil && writer != nil {
			// 优先使用标准的 io.ReadWriteCloser 方式
			rwc, err := utils.NewReadWriteCloser(reader, writer, func() error {
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
	if conn != nil {
		return conn
	}

	return nil
}
