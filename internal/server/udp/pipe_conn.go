package udp

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const maxUDPPayload = 65535

// pipeConn 将 UDP 报文转换为流式 net.Conn，供 TunnelBridge 使用
type pipeConn struct {
	listener *net.UDPConn
	remote   *net.UDPAddr

	inbound chan []byte

	readBuf []byte
	readPos int

	writeBuf []byte

	mu      sync.Mutex
	closed  bool
	closeCh chan struct{}
	backlog int
}

func newPipeConn(listener *net.UDPConn, remote *net.UDPAddr, backlog int) *pipeConn {
	if backlog <= 0 {
		backlog = 64
	}
	return &pipeConn{
		listener: listener,
		remote:   remote,
		inbound:  make(chan []byte, backlog),
		closeCh:  make(chan struct{}),
		backlog:  backlog,
	}
}

// Push 将新帧写入读取缓冲
func (c *pipeConn) Push(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return io.ErrClosedPipe
	}
	select {
	case c.inbound <- frame:
		return nil
	default:
		return errors.New("inbound backlog full")
	}
}

func (c *pipeConn) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return 0, io.EOF
		}

		if c.readBuf != nil && c.readPos < len(c.readBuf) {
			n := copy(p, c.readBuf[c.readPos:])
			c.readPos += n
			if c.readPos >= len(c.readBuf) {
				c.readBuf = nil
				c.readPos = 0
			}
			c.mu.Unlock()
			return n, nil
		}
		c.mu.Unlock()

		select {
		case data, ok := <-c.inbound:
			if !ok {
				return 0, io.EOF
			}
			c.mu.Lock()
			c.readBuf = data
			c.readPos = 0
			c.mu.Unlock()
		case <-c.closeCh:
			return 0, io.EOF
		}
	}
}

func (c *pipeConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}

	c.writeBuf = append(c.writeBuf, p...)
	processed := len(p)

	for {
		if len(c.writeBuf) < 4 {
			break
		}
		frameLen := int(binary.BigEndian.Uint32(c.writeBuf[:4]))
		if frameLen <= 0 || frameLen > maxUDPPayload {
			c.closed = true
			close(c.closeCh)
			return 0, errors.New("invalid frame length")
		}
		total := 4 + frameLen
		if len(c.writeBuf) < total {
			break
		}

		payload := c.writeBuf[4:total]
		if _, err := c.listener.WriteToUDP(payload, c.remote); err != nil {
			return 0, err
		}
		c.writeBuf = c.writeBuf[total:]
	}

	return processed, nil
}

func (c *pipeConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.closeCh)
	close(c.inbound)
	c.mu.Unlock()
	return nil
}

func (c *pipeConn) LocalAddr() net.Addr {
	return c.listener.LocalAddr()
}

func (c *pipeConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *pipeConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *pipeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *pipeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *pipeConn) Done() <-chan struct{} {
	return c.closeCh
}
