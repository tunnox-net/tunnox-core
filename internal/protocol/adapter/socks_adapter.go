package adapter

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/cloud/constants"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/session"
)

const (
	// SOCKS5 版本
	socks5Version = 0x05

	// SOCKS5 认证方法
	socksAuthNone     = 0x00 // 无需认证
	socksAuthPassword = 0x02 // 用户名/密码认证
	socksAuthNoMatch  = 0xFF // 没有可接受的方法

	// SOCKS5 命令
	socksCmdConnect      = 0x01 // CONNECT
	socksCmdBind         = 0x02 // BIND
	socksCmdUDPAssociate = 0x03 // UDP ASSOCIATE

	// SOCKS5 地址类型
	socksAddrTypeIPv4   = 0x01 // IPv4 地址
	socksAddrTypeDomain = 0x03 // 域名
	socksAddrTypeIPv6   = 0x04 // IPv6 地址

	// SOCKS5 响应代码
	socksRepSuccess              = 0x00 // 成功
	socksRepServerFailure        = 0x01 // 服务器故障
	socksRepNotAllowed           = 0x02 // 规则不允许
	socksRepNetworkUnreachable   = 0x03 // 网络不可达
	socksRepHostUnreachable      = 0x04 // 主机不可达
	socksRepConnectionRefused    = 0x05 // 连接被拒绝
	socksRepTTLExpired           = 0x06 // TTL 过期
	socksRepCommandNotSupported  = 0x07 // 不支持的命令
	socksRepAddrTypeNotSupported = 0x08 // 不支持的地址类型

	// 超时配置
	socksHandshakeTimeout = 10 * time.Second
	socksDialTimeout      = 30 * time.Second
)

// SocksAdapter SOCKS5 代理适配器
// 在本地监听 SOCKS5 请求，通过隧道转发到远端执行
type SocksAdapter struct {
	BaseAdapter
	listener    net.Listener
	credentials map[string]string // 用户名 -> 密码
	authEnabled bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// SocksConfig SOCKS5 配置
type SocksConfig struct {
	Username string
	Password string
}

func NewSocksAdapter(parentCtx context.Context, session session.Session, config *SocksConfig) *SocksAdapter {
	ctx, cancel := context.WithCancel(parentCtx)

	adapter := &SocksAdapter{
		credentials: make(map[string]string),
		ctx:         ctx,
		cancel:      cancel,
	}

	// 配置认证
	if config != nil && config.Username != "" && config.Password != "" {
		adapter.authEnabled = true
		adapter.credentials[config.Username] = config.Password
		corelog.Infof("SOCKS5 adapter: authentication enabled")
	} else {
		adapter.authEnabled = false
		corelog.Infof("SOCKS5 adapter: authentication disabled")
	}

	adapter.BaseAdapter = BaseAdapter{}
	adapter.SetName("socks5")
	adapter.SetSession(session)
	adapter.SetCtx(parentCtx, adapter.onClose)
	adapter.SetProtocolAdapter(adapter)

	return adapter
}

// Dial SOCKS5 不需要主动连接（客户端模式），返回错误
func (s *SocksAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	return nil, coreerrors.New(coreerrors.CodeNotImplemented, "SOCKS5 adapter does not support Dial (server mode only)")
}

// Listen 启动 SOCKS5 代理服务器
func (s *SocksAdapter) Listen(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to listen on SOCKS5")
	}

	s.listener = listener
	corelog.Infof("SOCKS5 proxy server listening on %s", addr)

	return nil
}

// Accept 接受 SOCKS5 客户端连接
func (s *SocksAdapter) Accept() (io.ReadWriteCloser, error) {
	if s.listener == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "SOCKS5 listener not initialized")
	}

	// 设置接受超时
	if tcpListener, ok := s.listener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Now().Add(100 * time.Millisecond))
	}

	conn, err := s.listener.Accept()
	if err != nil {
		// 检查是否是超时错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, coreerrors.New(coreerrors.CodeTimeout, "accept timeout")
		}
		return nil, err
	}

	// 在独立的 goroutine 中处理 SOCKS5 握手和请求
	go s.handleSocksConnection(conn)

	// 返回超时错误，让 acceptLoop 继续
	return nil, coreerrors.New(coreerrors.CodeTimeout, "socks connection handled")
}

func (s *SocksAdapter) getConnectionType() string {
	return "SOCKS5"
}

// handleSocksConnection 处理 SOCKS5 连接的完整生命周期
func (s *SocksAdapter) handleSocksConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// 设置握手超时
	clientConn.SetDeadline(time.Now().Add(socksHandshakeTimeout))

	// 1. 握手阶段
	if err := s.handleHandshake(clientConn); err != nil {
		corelog.Errorf("SOCKS5 handshake failed: %v", err)
		return
	}

	// 2. 处理请求
	targetAddr, err := s.handleRequest(clientConn)
	if err != nil {
		corelog.Errorf("SOCKS5 request failed: %v", err)
		return
	}

	// 移除握手超时
	clientConn.SetDeadline(time.Time{})

	corelog.Infof("SOCKS5 connecting to target: %s", targetAddr)

	// 3. 通过隧道连接到目标
	// 这里需要通过 Session 转发到远端
	if s.GetSession() == nil {
		corelog.Errorf("Session is not set for SOCKS5 adapter")
		s.sendReply(clientConn, socksRepServerFailure, "0.0.0.0", 0)
		return
	}

	// 通过隧道创建到目标的连接
	// Session 应该提供一个方法来建立到目标地址的连接
	// 这里我们使用一个虚拟连接来桥接
	remoteConn, err := s.dialThroughTunnel(targetAddr)
	if err != nil {
		corelog.Errorf("Failed to dial through tunnel: %v", err)
		s.sendReply(clientConn, socksRepHostUnreachable, "0.0.0.0", 0)
		return
	}
	defer remoteConn.Close()

	// 发送成功响应
	// 使用本地地址作为绑定地址
	localAddr := clientConn.LocalAddr().(*net.TCPAddr)
	if err := s.sendReply(clientConn, socksRepSuccess, localAddr.IP.String(), uint16(localAddr.Port)); err != nil {
		corelog.Errorf("Failed to send SOCKS5 reply: %v", err)
		return
	}

	// 4. 双向转发数据
	s.relay(clientConn, remoteConn)
}

// dialThroughTunnel 通过隧道连接到目标地址
// 这里需要与 Session 集成，实际建立到远端的连接
func (s *SocksAdapter) dialThroughTunnel(targetAddr string) (net.Conn, error) {
	// 方案1: 直接连接（本地模式）
	// 如果没有配置 Session 或者是本地测试，直接连接
	if s.GetSession() == nil {
		// 直接连接目标（不通过隧道）
		conn, err := net.DialTimeout("tcp", targetAddr, socksDialTimeout)
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeConnectionError, "direct dial failed")
		}
		return conn, nil
	}

	// 方案2: 通过隧道连接（生产模式）
	// 在此处需要通过 Session 建立隧道连接（实现中）
	// 当前先使用直接连接作为备用方案
	conn, err := net.DialTimeout("tcp", targetAddr, socksDialTimeout)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeConnectionError, "tunnel dial failed")
	}
	return conn, nil
}

// relay 在两个连接之间双向转发数据（高性能版本）
func (s *SocksAdapter) relay(client, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端 -> 远程
	go func() {
		defer wg.Done()
		buf := make([]byte, constants.TCPSocketBufferSize)
		io.CopyBuffer(remote, client, buf)
		if tcpConn, ok := remote.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 远程 -> 客户端
	go func() {
		defer wg.Done()
		buf := make([]byte, constants.TCPSocketBufferSize)
		io.CopyBuffer(client, remote, buf)
		if tcpConn, ok := client.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
}

// onClose SOCKS5 特定的资源清理
func (s *SocksAdapter) onClose() error {
	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}

	var err error
	if s.listener != nil {
		err = s.listener.Close()
		s.listener = nil
	}

	baseErr := s.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}
