package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/protocol/udp"
	"tunnox-core/internal/utils"
)

const udpControlMaxPacketSize = 65535

// dialUDPControlConnection 建立到服务器的UDP控制连接
func dialUDPControlConnection(ctx context.Context, address string) (net.Conn, error) {
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

	return newUDPStreamConn(ctx, conn, serverAddr), nil
}

// udpStreamConn 将UDP连接包装成面向字节流的net.Conn
// 通过内部缓冲区保证Read/Write语义符合StreamProcessor的要求
//
// 关键设计：
// - UDP是面向数据包的，每次Read/Write操作对应一个完整的UDP数据包
// - StreamProcessor期望字节流语义，所以需要缓冲区来桥接两者
// - 支持大数据包分片传输，保证可靠性
type udpStreamConn struct {
	conn *net.UDPConn
	addr net.Addr

	// 分片管理器
	fragmentManager *udp.UDPFragmentManager

	// 读取缓冲区
	readBuf []byte // 当前正在读取的完整数据包缓冲区
	readPos int    // 读取位置

	// 写入缓冲区（用于小包累积）
	writeBuf []byte

	mu sync.Mutex
}

func newUDPStreamConn(ctx context.Context, conn *net.UDPConn, addr net.Addr) net.Conn {
	streamConn := &udpStreamConn{
		conn:     conn,
		addr:     addr,
		writeBuf: make([]byte, 0, udpControlMaxPacketSize),
	}

	// 创建分片管理器
	streamConn.fragmentManager = udp.NewUDPFragmentManager(ctx, conn, addr)

	// 启动数据包接收协程
	go streamConn.receiveLoop(ctx)

	return streamConn
}

// receiveLoop 接收数据包循环（处理分片和 ACK）
func (c *udpStreamConn) receiveLoop(ctx context.Context) {
	buffer := make([]byte, udpControlMaxPacketSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 设置读取超时
		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, _, err := c.conn.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 超时是正常的，继续循环
			}
			utils.Debugf("UDP: read error: %v", err)
			return
		}

		if n == 0 {
			continue
		}

		// 复制数据（因为 buffer 会被重用）
		data := make([]byte, n)
		copy(data, buffer[:n])

		// 交给分片管理器处理
		if err := c.fragmentManager.HandlePacket(data); err != nil {
			utils.Debugf("UDP: failed to handle packet: %v", err)
			// 继续处理下一个包
		}
	}
}

func (c *udpStreamConn) Read(p []byte) (int, error) {
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

	// 从分片管理器读取重组后的数据（阻塞等待）
	reassembledData, err := c.fragmentManager.ReadReassembledData(30 * time.Second)
	if err != nil {
		return 0, err
	}

	// 将重组后的数据放入读取缓冲区
	c.mu.Lock()
	c.readBuf = reassembledData
	c.readPos = 0
	c.mu.Unlock()

	// 返回数据
	return c.Read(p)
}

func (c *udpStreamConn) Write(p []byte) (int, error) {
	// 使用分片管理器发送（自动处理分片）
	writeComplete := make(chan error, 1)
	writeErr := make(chan error, 1)

	err := c.fragmentManager.SendFragmented(p,
		func([]byte) {
			// 发送完成回调
			writeComplete <- nil
		},
		func(err error) {
			// 错误回调
			writeErr <- err
		},
	)

	if err != nil {
		return 0, err
	}

	// 等待发送完成或错误
	select {
	case err := <-writeComplete:
		if err != nil {
			return 0, err
		}
		return len(p), nil
	case err := <-writeErr:
		return 0, err
	case <-time.After(30 * time.Second):
		return 0, fmt.Errorf("timeout waiting for fragment send completion")
	}
}

func (c *udpStreamConn) Close() error {
	if c.fragmentManager != nil {
		c.fragmentManager.Close()
	}
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
