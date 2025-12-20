// Package client SOCKS5 代理监听器
// 运行在 ClientA（入口端），接受用户 SOCKS5 连接
package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// SOCKS5 协议常量
const (
	socks5Version     = 0x05
	socks5AuthNone    = 0x00
	socks5AuthNoMatch = 0xFF
	socks5CmdConnect  = 0x01
	socks5AddrIPv4    = 0x01
	socks5AddrDomain  = 0x03
	socks5AddrIPv6    = 0x04
	socks5RepSuccess  = 0x00
	socks5RepFailure  = 0x01
)

// SOCKS5ListenerConfig SOCKS5 监听器配置
type SOCKS5ListenerConfig struct {
	ListenAddr     string // 本地监听地址，如 ":11080"
	MappingID      string // 映射ID
	TargetClientID int64  // 出口客户端ID (ClientB)
	SecretKey      string // 映射密钥
}

// SOCKS5TunnelCreator 隧道创建接口
type SOCKS5TunnelCreator interface {
	// CreateSOCKS5Tunnel 创建 SOCKS5 隧道
	CreateSOCKS5Tunnel(
		userConn net.Conn,
		mappingID string,
		targetClientID int64,
		targetHost string,
		targetPort int,
		secretKey string,
	) error
}

// SOCKS5Listener SOCKS5 代理监听器（运行在 ClientA）
type SOCKS5Listener struct {
	*dispose.ServiceBase

	listener      net.Listener
	config        *SOCKS5ListenerConfig
	tunnelCreator SOCKS5TunnelCreator
}

// NewSOCKS5Listener 创建 SOCKS5 监听器
func NewSOCKS5Listener(
	ctx context.Context,
	config *SOCKS5ListenerConfig,
	tunnelCreator SOCKS5TunnelCreator,
) *SOCKS5Listener {
	l := &SOCKS5Listener{
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
func (l *SOCKS5Listener) Start() error {
	listener, err := net.Listen("tcp", l.config.ListenAddr)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to start SOCKS5 listener")
	}

	l.listener = listener
	corelog.Infof("SOCKS5Listener: listening on %s for mapping %s",
		l.config.ListenAddr, l.config.MappingID)

	go l.acceptLoop()
	return nil
}

// GetListenAddr 获取监听地址
func (l *SOCKS5Listener) GetListenAddr() string {
	if l.listener != nil {
		return l.listener.Addr().String()
	}
	return l.config.ListenAddr
}

// acceptLoop 接受连接循环
func (l *SOCKS5Listener) acceptLoop() {
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
func (l *SOCKS5Listener) handleConnection(conn net.Conn) {
	// 设置超时
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// 1. SOCKS5 握手，获取动态目标地址
	targetHost, targetPort, err := l.socks5Handshake(conn)
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
		l.sendSOCKS5Error(conn, socks5RepFailure)
		conn.Close()
		return
	}

	err = l.tunnelCreator.CreateSOCKS5Tunnel(
		conn,
		l.config.MappingID,
		l.config.TargetClientID,
		targetHost,
		targetPort,
		l.config.SecretKey,
	)
	if err != nil {
		corelog.Warnf("SOCKS5Listener: failed to create tunnel: %v", err)
		l.sendSOCKS5Error(conn, socks5RepFailure)
		conn.Close()
	}
}

// socks5Handshake SOCKS5 握手
func (l *SOCKS5Listener) socks5Handshake(conn net.Conn) (string, int, error) {
	// 1. 读取版本和认证方法数量
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", 0, fmt.Errorf("failed to read version: %w", err)
	}

	if buf[0] != socks5Version {
		return "", 0, fmt.Errorf("unsupported SOCKS version: %d", buf[0])
	}

	nmethods := int(buf[1])
	if nmethods == 0 {
		return "", 0, fmt.Errorf("no authentication methods provided")
	}

	// 读取认证方法列表
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", 0, fmt.Errorf("failed to read methods: %w", err)
	}

	// 2. 选择认证方法（当前仅支持无认证）
	authMethod := byte(socks5AuthNoMatch)
	for _, m := range methods {
		if m == socks5AuthNone {
			authMethod = socks5AuthNone
			break
		}
	}

	// 发送认证方法选择
	if _, err := conn.Write([]byte{socks5Version, authMethod}); err != nil {
		return "", 0, fmt.Errorf("failed to write auth method: %w", err)
	}

	if authMethod == socks5AuthNoMatch {
		return "", 0, fmt.Errorf("no acceptable authentication method")
	}

	// 3. 读取 CONNECT 请求
	buf = make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", 0, fmt.Errorf("failed to read request: %w", err)
	}

	if buf[0] != socks5Version {
		l.sendSOCKS5Error(conn, socks5RepFailure)
		return "", 0, fmt.Errorf("invalid version in request: %d", buf[0])
	}

	if buf[1] != socks5CmdConnect {
		l.sendSOCKS5Error(conn, 0x07) // command not supported
		return "", 0, fmt.Errorf("unsupported command: %d", buf[1])
	}

	// 4. 解析目标地址
	addrType := buf[3]
	var targetHost string

	switch addrType {
	case socks5AddrIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, fmt.Errorf("failed to read IPv4 address: %w", err)
		}
		targetHost = net.IP(addr).String()

	case socks5AddrDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", 0, fmt.Errorf("failed to read domain length: %w", err)
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", 0, fmt.Errorf("failed to read domain: %w", err)
		}
		targetHost = string(domain)

	case socks5AddrIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, fmt.Errorf("failed to read IPv6 address: %w", err)
		}
		targetHost = net.IP(addr).String()

	default:
		l.sendSOCKS5Error(conn, 0x08) // address type not supported
		return "", 0, fmt.Errorf("unsupported address type: %d", addrType)
	}

	// 5. 读取端口
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", 0, fmt.Errorf("failed to read port: %w", err)
	}
	targetPort := int(binary.BigEndian.Uint16(portBuf))

	// 6. 发送成功响应
	l.sendSOCKS5Success(conn)

	return targetHost, targetPort, nil
}

// sendSOCKS5Success 发送 SOCKS5 成功响应
func (l *SOCKS5Listener) sendSOCKS5Success(conn net.Conn) {
	reply := []byte{
		socks5Version, socks5RepSuccess, 0x00, socks5AddrIPv4,
		0, 0, 0, 0, // 绑定地址 (0.0.0.0)
		0, 0, // 绑定端口 (0)
	}
	conn.Write(reply)
}

// sendSOCKS5Error 发送 SOCKS5 错误响应
func (l *SOCKS5Listener) sendSOCKS5Error(conn net.Conn, rep byte) {
	reply := []byte{
		socks5Version, rep, 0x00, socks5AddrIPv4,
		0, 0, 0, 0,
		0, 0,
	}
	conn.Write(reply)
}
