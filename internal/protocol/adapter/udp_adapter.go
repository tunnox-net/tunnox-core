package adapter

import (
	"context"
	"io"
	"net"

	"tunnox-core/internal/core/dispose"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/protocol/udp/reliable"

	"github.com/sirupsen/logrus"
)

// UdpAdapter UDP协议适配器
// 使用新的可靠 UDP 传输协议
type UdpAdapter struct {
	BaseAdapter
	listener   *net.UDPConn
	dispatcher *reliable.PacketDispatcher
	logger     *logrus.Logger
}

// NewUdpAdapter 创建 UDP 适配器
func NewUdpAdapter(parentCtx context.Context, session session.Session) *UdpAdapter {
	// 使用全局标准 logger，确保输出到相同的日志文件
	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.DebugLevel)

	u := &UdpAdapter{
		BaseAdapter: BaseAdapter{
			ResourceBase: dispose.NewResourceBase("UdpAdapter"),
		},
		logger: logger,
	}
	u.Initialize(parentCtx)
	u.AddCleanHandler(u.onClose)
	u.SetName("udp")
	u.SetSession(session)
	u.SetProtocolAdapter(u) // 设置协议适配器引用

	return u
}

// Dial 建立 UDP 连接 (客户端)
func (u *UdpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to resolve UDP address")
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to dial UDP")
	}

	u.logger.Infof("UdpAdapter: client dialing %s", addr)

	// IMPORTANT: Each dial creates its own dispatcher and connection
	// This ensures that each tunnel has its own independent UDP connection
	// and packet dispatcher, avoiding conflicts between multiple tunnels
	dispatcher := reliable.NewPacketDispatcher(conn, u.logger)
	dispatcher.Start()

	// Create client transport (this will initiate handshake)
	transport, err := reliable.NewClientTransport(conn, udpAddr, dispatcher, u.logger)
	if err != nil {
		dispatcher.Stop()
		return nil, coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to create client transport")
	}

	u.logger.Infof("UdpAdapter: client connected to %s - Local=%s, Remote=%s",
		addr, transport.LocalAddr(), transport.RemoteAddr())

	return transport, nil
}

// Listen 启动 UDP 监听 (服务端)
func (u *UdpAdapter) Listen(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to resolve UDP address")
	}

	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to listen UDP")
	}

	u.listener = listener
	u.logger.Infof("UdpAdapter: listening on %s", addr)

	// Create and start packet dispatcher
	u.dispatcher = reliable.NewPacketDispatcher(listener, u.logger)
	u.dispatcher.Start()
	u.logger.Info("UdpAdapter: PacketDispatcher started")

	// Start accept loop manually here since BaseAdapter.ListenFrom is not being called
	go u.startAcceptLoop()

	return nil
}

// startAcceptLoop manually starts the accept loop for UDP adapter
func (u *UdpAdapter) startAcceptLoop() {
	for !u.IsClosed() {
		conn, err := u.Accept()

		if err != nil {
			if u.IsClosed() {
				return
			}
			continue
		}

		if u.IsClosed() {
			u.logger.Warnf("UDP connection closed")
			return
		}

		// Lifecycle: Managed by adapter context
		// Cleanup: Triggered by adapter.Close() via context cancellation
		// Shutdown: Waits for connection handling to complete
		go func(conn io.ReadWriteCloser, adapter *UdpAdapter) {
			defer func() {
				if r := recover(); r != nil {
					adapter.logger.Errorf("UDP handleConnection panic: %v", r)
				}
			}()
			
			adapter.BaseAdapter.handleConnection(adapter, conn)
		}(conn, u)
	}
}

// Accept 接受 UDP 连接 (服务端)
// 等待 dispatcher 接收到新的连接握手
func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
	if u.dispatcher == nil {
		return nil, coreErrors.New(coreErrors.ErrorTypePermanent, "UDP dispatcher not initialized")
	}

	// Wait for new session from dispatcher
	session, err := u.dispatcher.Accept()
	if err != nil {
		return nil, coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to accept connection")
	}

	// Wrap session in transport
	transport := reliable.NewServerTransport(session, u.logger)

	u.logger.Infof("UdpAdapter: accepted connection from %s, session=%d",
		transport.RemoteAddr(), session.GetSessionID())

	return transport, nil
}

// getConnectionType 返回连接类型
func (u *UdpAdapter) getConnectionType() string {
	return "UDP"
}

// onClose UDP 特定的资源清理
func (u *UdpAdapter) onClose() error {
	u.logger.Info("UdpAdapter: closing...")

	var err error

	// Stop dispatcher first (this will close all sessions)
	if u.dispatcher != nil {
		if dispErr := u.dispatcher.Stop(); dispErr != nil {
			u.logger.Errorf("UdpAdapter: failed to stop dispatcher: %v", dispErr)
			err = dispErr
		}
		u.dispatcher = nil
	}

	// Close listener
	if u.listener != nil {
		if closeErr := u.listener.Close(); closeErr != nil {
			u.logger.Errorf("UdpAdapter: failed to close listener: %v", closeErr)
			if err == nil {
				err = closeErr
			}
		}
		u.listener = nil
	}

	// Call base cleanup
	baseErr := u.BaseAdapter.onClose()
	if err == nil {
		err = baseErr
	}

	if err != nil {
		return err
	}

	u.logger.Info("UdpAdapter: closed successfully")
	return nil
}

// GetStats returns dispatcher statistics (for debugging/monitoring)
func (u *UdpAdapter) GetStats() (packetsReceived, packetsDropped, bytesReceived uint64) {
	if u.dispatcher != nil {
		return u.dispatcher.GetStats()
	}
	return 0, 0, 0
}
