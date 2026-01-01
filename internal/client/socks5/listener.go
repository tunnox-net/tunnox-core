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
	Version     = 0x05
	AuthNone    = 0x00
	AuthNoMatch = 0xFF
	CmdConnect  = 0x01
	AddrIPv4    = 0x01
	AddrDomain  = 0x03
	AddrIPv6    = 0x04
	RepSuccess  = 0x00
	RepFailure  = 0x01
)

// ListenerConfig SOCKS5 监听器配置
type ListenerConfig struct {
	ListenAddr     string // 本地监听地址，如 ":11080"
	MappingID      string // 映射ID
	TargetClientID int64  // 出口客户端ID (ClientB)
	SecretKey      string // 映射密钥
}

// TunnelCreator 隧道创建接口
type TunnelCreator interface {
	// CreateSOCKS5Tunnel 创建 SOCKS5 隧道
	// onSuccess 回调在隧道建立成功后、数据转发开始前调用
	// 用于发送 SOCKS5 成功响应
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

// Listener SOCKS5 代理监听器（运行在 ClientA）
type Listener struct {
	*dispose.ServiceBase

	listener      net.Listener
	config        *ListenerConfig
	tunnelCreator TunnelCreator
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
	// 设置超时
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// 1. SOCKS5 握手，获取动态目标地址
	targetHost, targetPort, err := l.Handshake(conn)
	if err != nil {
		corelog.Warnf("SOCKS5Listener: handshake failed: %v", err)
		conn.Close()
		return
	}

	// 清除超时
	conn.SetDeadline(time.Time{})

	corelog.Debugf("SOCKS5Listener: CONNECT %s:%d from %s",
		targetHost, targetPort, conn.RemoteAddr())

	// 2. 请求创建隧道到 ClientB
	if l.tunnelCreator == nil {
		corelog.Errorf("SOCKS5Listener: tunnel creator not set")
		l.SendError(conn, RepFailure)
		conn.Close()
		return
	}

	// 发送成功响应的回调函数
	// 只有在隧道建立成功后才调用，解决响应时机问题
	sendSuccessCallback := func() {
		l.SendSuccess(conn)
	}

	err = l.tunnelCreator.CreateSOCKS5Tunnel(
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

// Handshake SOCKS5 握手
func (l *Listener) Handshake(conn net.Conn) (string, int, error) {
	// 1. 读取版本和认证方法数量
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read version")
	}

	if buf[0] != Version {
		return "", 0, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported SOCKS version: %d", buf[0])
	}

	nmethods := int(buf[1])
	if nmethods == 0 {
		return "", 0, coreerrors.New(coreerrors.CodeProtocolError, "no authentication methods provided")
	}

	// 读取认证方法列表
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read methods")
	}

	// 2. 选择认证方法（当前仅支持无认证）
	authMethod := byte(AuthNoMatch)
	for _, m := range methods {
		if m == AuthNone {
			authMethod = AuthNone
			break
		}
	}

	// 发送认证方法选择
	if _, err := conn.Write([]byte{Version, authMethod}); err != nil {
		return "", 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write auth method")
	}

	if authMethod == AuthNoMatch {
		return "", 0, coreerrors.New(coreerrors.CodeProtocolError, "no acceptable authentication method")
	}

	// 3. 读取 CONNECT 请求
	buf = make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read request")
	}

	if buf[0] != Version {
		l.SendError(conn, RepFailure)
		return "", 0, coreerrors.Newf(coreerrors.CodeProtocolError, "invalid version in request: %d", buf[0])
	}

	if buf[1] != CmdConnect {
		l.SendError(conn, 0x07) // command not supported
		return "", 0, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported command: %d", buf[1])
	}

	// 4. 解析目标地址
	addrType := buf[3]
	var targetHost string

	switch addrType {
	case AddrIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read IPv4 address")
		}
		targetHost = net.IP(addr).String()

	case AddrDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read domain length")
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read domain")
		}
		targetHost = string(domain)

	case AddrIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read IPv6 address")
		}
		targetHost = net.IP(addr).String()

	default:
		l.SendError(conn, 0x08) // address type not supported
		return "", 0, coreerrors.Newf(coreerrors.CodeProtocolError, "unsupported address type: %d", addrType)
	}

	// 5. 读取端口
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", 0, coreerrors.Wrap(err, coreerrors.CodeProtocolError, "failed to read port")
	}
	targetPort := int(binary.BigEndian.Uint16(portBuf))

	// 注意：不在这里发送成功响应
	// 成功响应应该在隧道建立成功后发送，由 handleConnection 负责

	return targetHost, targetPort, nil
}

// SendSuccess 发送 SOCKS5 成功响应
func (l *Listener) SendSuccess(conn net.Conn) {
	reply := []byte{
		Version, RepSuccess, 0x00, AddrIPv4,
		0, 0, 0, 0, // 绑定地址 (0.0.0.0)
		0, 0, // 绑定端口 (0)
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
