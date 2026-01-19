// Package crossnode 提供跨节点通信功能
package crossnode

import (
	"io"
	"net"
	"strings"
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
)

// isConnectionClosedError 检查错误是否表示连接已关闭（正常或异常）
// 这些错误不应该导致连接被标记为 broken，因为是预期的关闭行为
func isConnectionClosedError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	// 检查网络错误
	if netErr, ok := err.(net.Error); ok {
		// 超时不算关闭
		if netErr.Timeout() {
			return false
		}
	}
	// 检查常见的连接关闭错误消息
	errStr := err.Error()
	closedErrors := []string{
		"connection reset by peer",
		"broken pipe",
		"use of closed network connection",
		"connection refused",
		"EOF",
	}
	for _, ce := range closedErrors {
		if strings.Contains(errStr, ce) {
			return true
		}
	}
	return false
}

// TunnelStateTracker 用于跟踪 tunnel 状态（检查 tunnel 是否已关闭）
type TunnelStateTracker interface {
	IsTunnelClosed(tunnelID string) bool
}

// FrameStream 基于帧协议的跨节点数据流
// 封装帧协议细节，提供标准的 io.ReadWriteCloser 接口
// 支持连接复用：tunnel 独占期间使用，结束后归还连接到池
type FrameStream struct {
	conn     *Conn
	tunnelID [16]byte
	tracker  TunnelStateTracker // tunnel 状态跟踪器（可选）
	readMu   sync.Mutex
	writeMu  sync.Mutex
	readEOF  bool
	writeEOF bool
	readBuf  []byte // 当前帧的数据缓冲
	readOff  int    // 缓冲区读取偏移
}

// NewFrameStream 创建基于帧协议的数据流
func NewFrameStream(conn *Conn, tunnelID [16]byte) *FrameStream {
	return &FrameStream{
		conn:     conn,
		tunnelID: tunnelID,
	}
}

// NewFrameStreamWithTracker 创建带状态跟踪的 FrameStream
func NewFrameStreamWithTracker(conn *Conn, tunnelID [16]byte, tracker TunnelStateTracker) *FrameStream {
	return &FrameStream{
		conn:     conn,
		tunnelID: tunnelID,
		tracker:  tracker,
	}
}

// Read 读取数据（实现 io.Reader）
// 自动处理帧协议：
// - FrameTypeData: 返回数据
// - FrameTypeClose: 返回 io.EOF（tunnel 正常结束）
// - 其他错误: 标记连接 broken
func (s *FrameStream) Read(p []byte) (n int, err error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	// 如果已经收到 EOF，直接返回
	if s.readEOF {
		return 0, io.EOF
	}

	// 如果缓冲区还有数据，先返回缓冲区的数据
	if s.readBuf != nil && s.readOff < len(s.readBuf) {
		n = copy(p, s.readBuf[s.readOff:])
		s.readOff += n
		if s.readOff >= len(s.readBuf) {
			s.readBuf = nil
			s.readOff = 0
		}
		return n, nil
	}

	// 读取下一帧（循环直到找到匹配的帧）
	tcpConn := s.conn.GetTCPConn()
	if tcpConn == nil {
		return 0, coreerrors.New(coreerrors.CodeNetworkError, "connection is nil")
	}

	for {
		tunnelID, frameType, data, err := ReadFrame(tcpConn)
		if err != nil {
			// 判断是否应该标记连接为 broken：
			// 1. 如果我们已经发送了 EOF（writeEOF=true），说明我们已经完成发送，
			//    对端关闭连接是预期行为，不标记 broken
			// 2. 如果错误表示连接正常/异常关闭（EOF、connection reset 等），
			//    这是 TCP 层面的正常关闭，不标记 broken
			// 3. 只有真正的网络错误（如超时、协议错误等）才标记 broken
			if !s.writeEOF && !isConnectionClosedError(err) {
				s.conn.MarkBroken()
			}
			// 如果是连接关闭，设置 readEOF 并返回 EOF
			if isConnectionClosedError(err) {
				s.readEOF = true
				return 0, io.EOF
			}
			return 0, err
		}

		// 检查 TunnelID 是否匹配
		if tunnelID != s.tunnelID {
			otherTunnelIDStr := TunnelIDToString(tunnelID)
			if s.tracker != nil && s.tracker.IsTunnelClosed(otherTunnelIDStr) {
				continue // 残留帧，丢弃
			}
			continue // 其他 tunnel 的帧，丢弃
		}

		// TunnelID 匹配，处理帧
		switch frameType {
		case FrameTypeData:
			if len(data) == 0 {
				continue
			}
			s.readBuf = data
			s.readOff = 0
			n = copy(p, s.readBuf)
			s.readOff = n
			if s.readOff >= len(s.readBuf) {
				s.readBuf = nil
				s.readOff = 0
			}
			return n, nil

		case FrameTypeEOF:
			// 半关闭：对端的写入方向结束，但我们仍可以写入
			s.readEOF = true
			return 0, io.EOF

		case FrameTypeClose:
			// 全关闭：整个连接结束
			s.readEOF = true
			return 0, io.EOF

		default:
			continue
		}
	}
}

// Write 写入数据（实现 io.Writer）
// 自动封装成 FrameTypeData 帧
func (s *FrameStream) Write(p []byte) (n int, err error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.writeEOF {
		return 0, io.ErrClosedPipe
	}

	if len(p) == 0 {
		return 0, nil
	}

	tcpConn := s.conn.GetTCPConn()
	if tcpConn == nil {
		return 0, coreerrors.New(coreerrors.CodeNetworkError, "connection is nil")
	}

	// 封装成 FrameTypeData 帧并发送
	// 如果数据超过 MaxFrameSize，自动分片发送
	if len(p) > MaxFrameSize {
		written := 0
		for written < len(p) {
			chunkSize := MaxFrameSize
			if written+chunkSize > len(p) {
				chunkSize = len(p) - written
			}
			chunk := p[written : written+chunkSize]
			if err := WriteFrame(tcpConn, s.tunnelID, FrameTypeData, chunk); err != nil {
				s.conn.MarkBroken()
				return written, err
			}
			written += chunkSize
		}
		return written, nil
	}

	// 单帧写入
	if err := WriteFrame(tcpConn, s.tunnelID, FrameTypeData, p); err != nil {
		s.conn.MarkBroken()
		return 0, err
	}

	return len(p), nil
}

// CloseWrite 半关闭数据流的写入方向
// 发送 FrameTypeEOF 帧通知对端"我这边发送完毕，但仍在等待你的数据"
// 用于支持 HTTP 请求-响应模式：客户端发送完请求后等待响应
func (s *FrameStream) CloseWrite() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.writeEOF {
		return nil // 已经关闭过了
	}

	tcpConn := s.conn.GetTCPConn()
	if tcpConn == nil {
		return coreerrors.New(coreerrors.CodeNetworkError, "connection is nil")
	}

	// 发送 FrameTypeEOF 帧（空数据）- 半关闭
	if err := WriteFrame(tcpConn, s.tunnelID, FrameTypeEOF, nil); err != nil {
		s.conn.MarkBroken()
		return err
	}

	s.writeEOF = true
	return nil
}

// Close 关闭数据流（实现 io.Closer）
// 发送 FrameTypeClose 帧通知对端 tunnel 结束
// 注意：这不会关闭底层 TCP 连接，连接将被归还到池
func (s *FrameStream) Close() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.writeEOF {
		return nil // 已经关闭过了
	}

	tcpConn := s.conn.GetTCPConn()
	if tcpConn == nil {
		return coreerrors.New(coreerrors.CodeNetworkError, "connection is nil")
	}

	// 发送 FrameTypeClose 帧（空数据）- 全关闭
	if err := WriteFrame(tcpConn, s.tunnelID, FrameTypeClose, nil); err != nil {
		s.conn.MarkBroken()
		return err
	}

	s.writeEOF = true
	return nil
}

// IsBroken 检查底层连接是否损坏
func (s *FrameStream) IsBroken() bool {
	return s.conn.IsBroken()
}

// GetConn 获取底层连接（用于归还到池）
func (s *FrameStream) GetConn() *Conn {
	return s.conn
}
