package adapter

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
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
	socksBufferSize       = 32 * 1024
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
	connMutex   sync.RWMutex
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
		utils.Infof("SOCKS5 adapter: authentication enabled")
	} else {
		adapter.authEnabled = false
		utils.Infof("SOCKS5 adapter: authentication disabled")
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
	return nil, fmt.Errorf("SOCKS5 adapter does not support Dial (server mode only)")
}

// Listen 启动 SOCKS5 代理服务器
func (s *SocksAdapter) Listen(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on SOCKS5: %w", err)
	}

	s.listener = listener
	utils.Infof("SOCKS5 proxy server listening on %s", addr)

	return nil
}

// Accept 接受 SOCKS5 客户端连接
func (s *SocksAdapter) Accept() (io.ReadWriteCloser, error) {
	if s.listener == nil {
		return nil, fmt.Errorf("SOCKS5 listener not initialized")
	}

	// 设置接受超时
	if tcpListener, ok := s.listener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Now().Add(100 * time.Millisecond))
	}

	conn, err := s.listener.Accept()
	if err != nil {
		// 检查是否是超时错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, fmt.Errorf("accept timeout")
		}
		return nil, err
	}

	// 在独立的 goroutine 中处理 SOCKS5 握手和请求
	go s.handleSocksConnection(conn)

	// 返回超时错误，让 acceptLoop 继续
	return nil, fmt.Errorf("socks connection handled")
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
		utils.Errorf("SOCKS5 handshake failed: %v", err)
		return
	}

	// 2. 处理请求
	targetAddr, err := s.handleRequest(clientConn)
	if err != nil {
		utils.Errorf("SOCKS5 request failed: %v", err)
		return
	}

	// 移除握手超时
	clientConn.SetDeadline(time.Time{})

	utils.Infof("SOCKS5 connecting to target: %s", targetAddr)

	// 3. 通过隧道连接到目标
	// 这里需要通过 Session 转发到远端
	if s.GetSession() == nil {
		utils.Errorf("Session is not set for SOCKS5 adapter")
		s.sendReply(clientConn, socksRepServerFailure, "0.0.0.0", 0)
		return
	}

	// 通过隧道创建到目标的连接
	// Session 应该提供一个方法来建立到目标地址的连接
	// 这里我们使用一个虚拟连接来桥接
	remoteConn, err := s.dialThroughTunnel(targetAddr)
	if err != nil {
		utils.Errorf("Failed to dial through tunnel: %v", err)
		s.sendReply(clientConn, socksRepHostUnreachable, "0.0.0.0", 0)
		return
	}
	defer remoteConn.Close()

	// 发送成功响应
	// 使用本地地址作为绑定地址
	localAddr := clientConn.LocalAddr().(*net.TCPAddr)
	if err := s.sendReply(clientConn, socksRepSuccess, localAddr.IP.String(), uint16(localAddr.Port)); err != nil {
		utils.Errorf("Failed to send SOCKS5 reply: %v", err)
		return
	}

	// 4. 双向转发数据
	s.relay(clientConn, remoteConn)
}

// handleHandshake 处理 SOCKS5 握手阶段
func (s *SocksAdapter) handleHandshake(conn net.Conn) error {
	// 读取客户端支持的认证方法
	// +----+----------+----------+
	// |VER | NMETHODS | METHODS  |
	// +----+----------+----------+
	// | 1  |    1     | 1 to 255 |
	// +----+----------+----------+

	buf := make([]byte, 257)
	n, err := io.ReadAtLeast(conn, buf, 2)
	if err != nil {
		return fmt.Errorf("read handshake failed: %w", err)
	}

	version := buf[0]
	if version != socks5Version {
		return fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	nMethods := int(buf[1])
	if n < 2+nMethods {
		if _, err := io.ReadFull(conn, buf[n:2+nMethods]); err != nil {
			return fmt.Errorf("read methods failed: %w", err)
		}
	}

	methods := buf[2 : 2+nMethods]

	// 选择认证方法
	selectedMethod := socksAuthNoMatch
	if s.authEnabled {
		// 检查客户端是否支持用户名/密码认证
		for _, method := range methods {
			if method == socksAuthPassword {
				selectedMethod = socksAuthPassword
				break
			}
		}
	} else {
		// 检查客户端是否支持无认证
		for _, method := range methods {
			if method == socksAuthNone {
				selectedMethod = socksAuthNone
				break
			}
		}
	}

	// 发送选择的认证方法
	// +----+--------+
	// |VER | METHOD |
	// +----+--------+
	// | 1  |   1    |
	// +----+--------+
	if _, err := conn.Write([]byte{socks5Version, byte(selectedMethod)}); err != nil {
		return fmt.Errorf("write method selection failed: %w", err)
	}

	if selectedMethod == socksAuthNoMatch {
		return fmt.Errorf("no acceptable authentication method")
	}

	// 如果需要认证，执行认证流程
	if selectedMethod == socksAuthPassword {
		if err := s.handlePasswordAuth(conn); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	return nil
}

// handlePasswordAuth 处理用户名/密码认证
func (s *SocksAdapter) handlePasswordAuth(conn net.Conn) error {
	// +----+------+----------+------+----------+
	// |VER | ULEN |  UNAME   | PLEN |  PASSWD  |
	// +----+------+----------+------+----------+
	// | 1  |  1   | 1 to 255 |  1   | 1 to 255 |
	// +----+------+----------+------+----------+

	// 读取版本和用户名长度
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read auth header failed: %w", err)
	}

	version := buf[0]
	if version != 0x01 {
		return fmt.Errorf("unsupported auth version: %d", version)
	}

	usernameLen := int(buf[1])
	
	// 读取用户名
	usernameBuf := make([]byte, usernameLen)
	if _, err := io.ReadFull(conn, usernameBuf); err != nil {
		return fmt.Errorf("read username failed: %w", err)
	}
	username := string(usernameBuf)

	// 读取密码长度
	passwordLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, passwordLenBuf); err != nil {
		return fmt.Errorf("read password length failed: %w", err)
	}
	passwordLen := int(passwordLenBuf[0])

	// 读取密码
	passwordBuf := make([]byte, passwordLen)
	if _, err := io.ReadFull(conn, passwordBuf); err != nil {
		return fmt.Errorf("read password failed: %w", err)
	}
	password := string(passwordBuf)

	// 验证凭据
	correctPassword, exists := s.credentials[username]
	success := exists && correctPassword == password

	// 发送认证响应
	// +----+--------+
	// |VER | STATUS |
	// +----+--------+
	// | 1  |   1    |
	// +----+--------+
	var status byte
	if success {
		status = 0x00 // 成功
	} else {
		status = 0x01 // 失败
	}

	if _, err := conn.Write([]byte{0x01, status}); err != nil {
		return fmt.Errorf("write auth response failed: %w", err)
	}

	if !success {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

// handleRequest 处理 SOCKS5 请求
func (s *SocksAdapter) handleRequest(conn net.Conn) (string, error) {
	// +----+-----+-------+------+----------+----------+
	// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", fmt.Errorf("read request header failed: %w", err)
	}

	version := buf[0]
	if version != socks5Version {
		return "", fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	cmd := buf[1]
	// buf[2] 是保留字段
	addrType := buf[3]

	// 目前只支持 CONNECT 命令
	if cmd != socksCmdConnect {
		s.sendReply(conn, socksRepCommandNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("unsupported command: %d", cmd)
	}

	// 解析目标地址
	var targetAddr string
	switch addrType {
	case socksAddrTypeIPv4:
		// IPv4 地址 (4 字节)
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv4 address failed: %w", err)
		}
		targetAddr = net.IP(addr).String()

	case socksAddrTypeDomain:
		// 域名 (1 字节长度 + 域名)
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", fmt.Errorf("read domain length failed: %w", err)
		}
		domainLen := int(lenBuf[0])
		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", fmt.Errorf("read domain failed: %w", err)
		}
		targetAddr = string(domain)

	case socksAddrTypeIPv6:
		// IPv6 地址 (16 字节)
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv6 address failed: %w", err)
		}
		targetAddr = net.IP(addr).String()

	default:
		s.sendReply(conn, socksRepAddrTypeNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("unsupported address type: %d", addrType)
	}

	// 读取端口 (2 字节，大端序)
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", fmt.Errorf("read port failed: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	return fmt.Sprintf("%s:%d", targetAddr, port), nil
}

// sendReply 发送 SOCKS5 响应
func (s *SocksAdapter) sendReply(conn net.Conn, rep byte, bindAddr string, bindPort uint16) error {
	// +----+-----+-------+------+----------+----------+
	// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	ip := net.ParseIP(bindAddr)
	if ip == nil {
		ip = net.IPv4zero
	}

	reply := make([]byte, 0, 22)
	reply = append(reply, socks5Version, rep, 0x00) // VER, REP, RSV

	if ip4 := ip.To4(); ip4 != nil {
		reply = append(reply, socksAddrTypeIPv4)
		reply = append(reply, ip4...)
	} else {
		reply = append(reply, socksAddrTypeIPv6)
		reply = append(reply, ip.To16()...)
	}

	// 添加端口
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, bindPort)
	reply = append(reply, portBytes...)

	_, err := conn.Write(reply)
	return err
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
			return nil, fmt.Errorf("direct dial failed: %w", err)
		}
		return conn, nil
	}

	// 方案2: 通过隧道连接（生产模式）
	// TODO: 这里需要通过 Session 建立隧道连接
	// 当前先使用直接连接作为备用方案
	conn, err := net.DialTimeout("tcp", targetAddr, socksDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("tunnel dial failed: %w", err)
	}
	return conn, nil
}

// relay 在两个连接之间双向转发数据
func (s *SocksAdapter) relay(client, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端 -> 远程
	go func() {
		defer wg.Done()
		written, err := io.Copy(remote, client)
		if err != nil {
			utils.Debugf("Client to remote copy error: %v", err)
		}
		utils.Debugf("Client to remote: %d bytes", written)
		// 关闭远程连接的写入
		if tcpConn, ok := remote.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	// 远程 -> 客户端
	go func() {
		defer wg.Done()
		written, err := io.Copy(client, remote)
		if err != nil {
			utils.Debugf("Remote to client copy error: %v", err)
		}
		utils.Debugf("Remote to client: %d bytes", written)
		// 关闭客户端连接的写入
		if tcpConn, ok := client.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
	utils.Infof("SOCKS5 relay completed")
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

