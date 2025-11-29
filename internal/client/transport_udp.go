package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/utils"
)

const udpControlMaxPacketSize = 65535

// dialUDPControlConnection 建立到服务器的UDP控制连接
func dialUDPControlConnection(address string) (net.Conn, error) {
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address %s: %w", address, err)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP server %s: %w", address, err)
	}

	if err := conn.SetReadBuffer(udpControlMaxPacketSize); err != nil {
		utils.Warnf("Client: failed to set UDP read buffer: %v", err)
	}
	if err := conn.SetWriteBuffer(udpControlMaxPacketSize); err != nil {
		utils.Warnf("Client: failed to set UDP write buffer: %v", err)
	}

	return newUDPStreamConn(conn), nil
}

// udpStreamConn 将UDP连接包装成面向字节流的net.Conn
// 通过内部缓冲区保证Read/Write语义符合StreamProcessor的要求
//
// 关键设计：
// - UDP是面向数据包的，每次Read/Write操作对应一个完整的UDP数据包
// - StreamProcessor期望字节流语义，所以需要缓冲区来桥接两者
// - 一个StreamProcessor packet可能跨越多个UDP数据包（如果packet > MTU）
type udpStreamConn struct {
	conn *net.UDPConn

	readBuf []byte // 当前正在读取的UDP数据包缓冲区
	readPos int    // 读取位置

	writeBuf []byte // 写入缓冲区，累积数据直到达到合理大小再发送

	mu sync.Mutex
}

func newUDPStreamConn(conn *net.UDPConn) net.Conn {
	return &udpStreamConn{
		conn:     conn,
		writeBuf: make([]byte, 0, udpControlMaxPacketSize),
	}
}

func (c *udpStreamConn) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		// 如果有缓冲数据，先返回缓冲数据
		if c.readBuf != nil && c.readPos < len(c.readBuf) {
			n := copy(p, c.readBuf[c.readPos:])
			c.readPos += n
			if c.readPos >= len(c.readBuf) {
				// 缓冲区读完，清空
				c.readBuf = nil
				c.readPos = 0
			}
			c.mu.Unlock()
			return n, nil
		}
		c.mu.Unlock()

		// 设置合理的读取超时（30秒）
		c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		buf := make([]byte, udpControlMaxPacketSize)
		n, err := c.conn.Read(buf)
		if err != nil {
			return 0, err
		}

		if n == 0 {
			continue // 空数据包，继续读取
		}

		c.mu.Lock()
		c.readBuf = buf[:n]
		c.readPos = 0
		c.mu.Unlock()
	}
}

func (c *udpStreamConn) Write(p []byte) (int, error) {
	// 直接发送，不分片
	// StreamProcessor的每个packet都应该能放入一个UDP数据包
	// 如果packet太大，这里会失败，调用者需要处理
	if len(p) > udpControlMaxPacketSize {
		return 0, fmt.Errorf("data too large for UDP packet: %d > %d", len(p), udpControlMaxPacketSize)
	}

	n, err := c.conn.Write(p)
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (c *udpStreamConn) Close() error {
	return c.conn.Close()
}

func (c *udpStreamConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *udpStreamConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *udpStreamConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *udpStreamConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *udpStreamConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
