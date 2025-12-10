package client

import (
	"context"
	"net"

	"github.com/sirupsen/logrus"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/protocol/udp/reliable"
)

// dialUDP 建立 UDP 控制连接
func dialUDP(ctx context.Context, address string) (net.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeNetwork, "failed to resolve UDP address")
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeNetwork, "failed to dial UDP")
	}

	// 使用全局标准 logger，确保输出到相同的日志文件
	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.DebugLevel)

	// 创建 PacketDispatcher
	dispatcher := reliable.NewPacketDispatcher(conn, logger)
	dispatcher.Start()

	logger.Infof("Client: using UDP transport for control connection")

	// 创建客户端 Transport（会自动进行握手）
	transport, err := reliable.NewClientTransport(conn, udpAddr, dispatcher, logger)
	if err != nil {
		dispatcher.Stop()
		conn.Close()
		return nil, errors.Wrap(err, errors.ErrorTypeNetwork, "failed to create UDP transport")
	}

	logger.Infof("Client: UDP connection established - Local=%s, Remote=%s",
		transport.LocalAddr(), transport.RemoteAddr())

	// Transport 实现了 net.Conn 接口（通过 io.ReadWriteCloser）
	return &udpConn{
		Transport:  transport,
		dispatcher: dispatcher,
	}, nil
}

// udpConn 包装 Transport 和 dispatcher 以实现完整的 net.Conn 接口
type udpConn struct {
	*reliable.Transport
	dispatcher *reliable.PacketDispatcher
}

// Close 关闭连接和 dispatcher
func (c *udpConn) Close() error {
	// 先关闭 transport
	if err := c.Transport.Close(); err != nil {
		return err
	}

	// 再停止 dispatcher
	return c.dispatcher.Stop()
}
