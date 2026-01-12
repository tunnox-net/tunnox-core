// Package socks5 SOCKS5 代理监听器
// 运行在 ClientA（入口端），接受用户 SOCKS5 连接
package socks5

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// SOCKS5 协议常量
const (
	Version        = 0x05
	AuthNone       = 0x00
	AuthNoMatch    = 0xFF
	CmdConnect     = 0x01
	CmdBind        = 0x02
	CmdUDPAssoc    = 0x03
	AddrIPv4       = 0x01
	AddrDomain     = 0x03
	AddrIPv6       = 0x04
	RepSuccess     = 0x00
	RepFailure     = 0x01
	RepCmdNotSupp  = 0x07
	RepAddrNotSupp = 0x08
)

// ListenerConfig SOCKS5 监听器配置
type ListenerConfig struct {
	ListenAddr     string // 本地监听地址，如 ":11080"
	MappingID      string // 映射ID
	TargetClientID int64  // 出口客户端ID (ClientB)
	SecretKey      string // 映射密钥
}

// TunnelCreator TCP 隧道创建接口
type TunnelCreator interface {
	CreateSOCKS5Tunnel(
		userConn net.Conn,
		mappingID string,
		targetClientID int64,
		targetHost string,
		targetPort int,
		secretKey string,
		onSuccess func(),
	) error
}

// UDPRelayCreator UDP Relay 创建接口
type UDPRelayCreator interface {
	CreateUDPRelay(
		tcpConn net.Conn,
		mappingID string,
		targetClientID int64,
		secretKey string,
	) (bindAddr *net.UDPAddr, err error)
}

// HandshakeResult SOCKS5 握手结果
type HandshakeResult struct {
	Command    byte
	TargetHost string
	TargetPort int
}

// Listener SOCKS5 代理监听器（运行在 ClientA）
type Listener struct {
	*dispose.ServiceBase

	listener        net.Listener
	config          *ListenerConfig
	tunnelCreator   TunnelCreator
	udpRelayCreator UDPRelayCreator
}

// NewListener 创建 SOCKS5 监听器
func NewListener(
	ctx context.Context,
	config *ListenerConfig,
	tunnelCreator TunnelCreator,
) *Listener {
	l := &Listener{
		ServiceBase:   dispose.NewService("SOCKS5Listener", ctx),
		config:        config,
		tunnelCreator: tunnelCreator,
	}

	l.AddCleanHandler(func() error {
		if l.listener != nil {
			return l.listener.Close()
		}
		return nil
	})

	return l
}

// SetUDPRelayCreator 设置 UDP Relay 创建器（可选，启用 UDP ASSOCIATE 支持）
func (l *Listener) SetUDPRelayCreator(creator UDPRelayCreator) {
	l.udpRelayCreator = creator
}

// Start 启动监听
func (l *Listener) Start() error {
	listener, err := net.Listen("tcp", l.config.ListenAddr)
	if err != nil {
		// 检查是否是端口被占用的错误，使用更明确的错误码
		if strings.Contains(err.Error(), "address already in use") ||
			strings.Contains(err.Error(), "bind: address already in use") {
			return coreerrors.Wrapf(err, coreerrors.CodePortConflict,
				"port %s is already in use", l.config.ListenAddr)
		}
		return coreerrors.Wrapf(err, coreerrors.CodeNetworkError,
			"failed to start SOCKS5 listener on %s", l.config.ListenAddr)
	}

	l.listener = listener
	corelog.Infof("SOCKS5Listener: listening on %s for mapping %s",
		l.config.ListenAddr, l.config.MappingID)

	go l.acceptLoop()
	return nil
}

// GetListenAddr 获取监听地址
func (l *Listener) GetListenAddr() string {
	if l.listener != nil {
		return l.listener.Addr().String()
	}
	return l.config.ListenAddr
}

// acceptLoop 接受连接循环
func (l *Listener) acceptLoop() {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			if l.IsClosed() {
				return
			}
			corelog.Warnf("SOCKS5Listener: accept error: %v", err)
			continue
		}

		go l.handleConnection(conn)
	}
}

// handleConnection 处理单个连接
func (l *Listener) handleConnection(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	result, err := l.Handshake(conn)
	if err != nil {
		corelog.Warnf("SOCKS5Listener: handshake failed: %v", err)
		conn.Close()
		return
	}

	conn.SetDeadline(time.Time{})

	switch result.Command {
	case CmdConnect:
		l.handleConnect(conn, result.TargetHost, result.TargetPort)
	case CmdUDPAssoc:
		l.handleUDPAssociate(conn)
	default:
		l.SendError(conn, RepCmdNotSupp)
		conn.Close()
	}
}

// handleConnect 处理 TCP CONNECT 请求
func (l *Listener) handleConnect(conn net.Conn, targetHost string, targetPort int) {
	corelog.Debugf("SOCKS5Listener: CONNECT %s:%d from %s", targetHost, targetPort, conn.RemoteAddr())

	if l.tunnelCreator == nil {
		corelog.Errorf("SOCKS5Listener: tunnel creator not set")
		l.SendError(conn, RepFailure)
		conn.Close()
		return
	}

	sendSuccessCallback := func() {
		l.SendSuccess(conn)
	}

	err := l.tunnelCreator.CreateSOCKS5Tunnel(
		conn,
		l.config.MappingID,
		l.config.TargetClientID,
		targetHost,
		targetPort,
		l.config.SecretKey,
		sendSuccessCallback,
	)
	if err != nil {
		corelog.Warnf("SOCKS5Listener: failed to create tunnel: %v", err)
		l.SendError(conn, RepFailure)
		conn.Close()
	}
}

// handleUDPAssociate 处理 UDP ASSOCIATE 请求
func (l *Listener) handleUDPAssociate(conn net.Conn) {
	corelog.Debugf("SOCKS5Listener: UDP ASSOCIATE from %s", conn.RemoteAddr())

	if l.udpRelayCreator == nil {
		corelog.Warnf("SOCKS5Listener: UDP ASSOCIATE not supported (no relay creator)")
		l.SendError(conn, RepCmdNotSupp)
		conn.Close()
		return
	}

	bindAddr, err := l.udpRelayCreator.CreateUDPRelay(
		conn,
		l.config.MappingID,
		l.config.TargetClientID,
		l.config.SecretKey,
	)
	if err != nil {
		corelog.Warnf("SOCKS5Listener: failed to create UDP relay: %v", err)
		l.SendError(conn, RepFailure)
		conn.Close()
		return
	}

	l.SendSuccessWithBind(conn, bindAddr)
	corelog.Infof("SOCKS5Listener: UDP relay bound to %s", bindAddr.String())
}

// Handshake SOCKS5 握手，返回命令类型和目标地址
func (l *Listener) Handshake(conn net.Conn) (*HandshakeResult, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read version")
	}

	if buf[0] != Version {
		return nil, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported SOCKS version: %d", buf[0])
	}

	nmethods := int(buf[1])
	if nmethods == 0 {
		return nil, coreerrors.New(coreerrors.CodeProtocolError, "no authentication methods provided")
	}

	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read methods")
	}

	authMethod := byte(AuthNoMatch)
	for _, m := range methods {
		if m == AuthNone {
			authMethod = AuthNone
			break
		}
	}

	if _, err := conn.Write([]byte{Version, authMethod}); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write auth method")
	}

	if authMethod == AuthNoMatch {
		return nil, coreerrors.New(coreerrors.CodeProtocolError, "no acceptable authentication method")
	}

	buf = make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read request")
	}

	if buf[0] != Version {
		l.SendError(conn, RepFailure)
		return nil, coreerrors.Newf(coreerrors.CodeProtocolError, "invalid version in request: %d", buf[0])
	}

	cmd := buf[1]
	if cmd != CmdConnect && cmd != CmdUDPAssoc {
		l.SendError(conn, RepCmdNotSupp)
		return nil, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported command: %d", cmd)
	}

	addrType := buf[3]
	var targetHost string

	switch addrType {
	case AddrIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read IPv4 address")
		}
		targetHost = net.IP(addr).String()

	case AddrDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read domain length")
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read domain")
		}
		targetHost = string(domain)

	case AddrIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read IPv6 address")
		}
		targetHost = net.IP(addr).String()

	default:
		l.SendError(conn, RepAddrNotSupp)
		return nil, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported address type: %d", addrType)
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read port")
	}
	targetPort := int(binary.BigEndian.Uint16(portBuf))

	return &HandshakeResult{
		Command:    cmd,
		TargetHost: targetHost,
		TargetPort: targetPort,
	}, nil
}

// SendSuccess 发送 SOCKS5 成功响应（绑定地址 0.0.0.0:0）
func (l *Listener) SendSuccess(conn net.Conn) {
	reply := []byte{
		Version, RepSuccess, 0x00, AddrIPv4,
		0, 0, 0, 0,
		0, 0,
	}
	conn.Write(reply)
}

// SendSuccessWithBind 发送带绑定地址的成功响应（用于 UDP ASSOCIATE）
func (l *Listener) SendSuccessWithBind(conn net.Conn, bindAddr *net.UDPAddr) {
	ip := bindAddr.IP.To4()
	if ip == nil {
		ip = net.IPv4zero
	}
	port := uint16(bindAddr.Port)

	reply := []byte{
		Version, RepSuccess, 0x00, AddrIPv4,
		ip[0], ip[1], ip[2], ip[3],
		byte(port >> 8), byte(port & 0xFF),
	}
	conn.Write(reply)
}

// SendError 发送 SOCKS5 错误响应
func (l *Listener) SendError(conn net.Conn, rep byte) {
	reply := []byte{
		Version, rep, 0x00, AddrIPv4,
		0, 0, 0, 0,
		0, 0,
	}
	conn.Write(reply)
}
