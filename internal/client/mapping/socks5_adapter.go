package mapping

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"tunnox-core/internal/config"
	"tunnox-core/internal/utils"
)

// SOCKS5常量定义
const (
	socks5Version = 0x05

	socksAuthNone    = 0x00
	socksAuthNoMatch = 0xFF

	socksCmdConnect = 0x01

	socksAddrTypeIPv4   = 0x01
	socksAddrTypeDomain = 0x03
	socksAddrTypeIPv6   = 0x04

	socksRepSuccess              = 0x00
	socksRepServerFailure        = 0x01
	socksRepCommandNotSupported  = 0x07
	socksRepAddrTypeNotSupported = 0x08
)

// SOCKS5MappingAdapter SOCKS5映射适配器
type SOCKS5MappingAdapter struct {
	listener    net.Listener
	credentials map[string]string // 用户名/密码认证（未来扩展）
}

// NewSOCKS5MappingAdapter 创建SOCKS5映射适配器
func NewSOCKS5MappingAdapter(credentials map[string]string) *SOCKS5MappingAdapter {
	return &SOCKS5MappingAdapter{
		credentials: credentials,
	}
}

// StartListener 启动SOCKS5监听
func (a *SOCKS5MappingAdapter) StartListener(config config.MappingConfig) error {
	addr := fmt.Sprintf(":%d", config.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	a.listener = listener
	utils.Debugf("SOCKS5MappingAdapter: listening on %s", addr)
	return nil
}

// Accept 接受SOCKS5连接
func (a *SOCKS5MappingAdapter) Accept() (io.ReadWriteCloser, error) {
	if a.listener == nil {
		return nil, fmt.Errorf("SOCKS5 listener not initialized")
	}

	// 设置接受超时，允许优雅关闭
	if tcpListener, ok := a.listener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
	}

	conn, err := a.listener.Accept()
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// PrepareConnection SOCKS5握手处理
// 处理SOCKS5握手和CONNECT请求，解析目标地址
// 注意：由于SOCKS5目标地址是动态的，实际目标连接由后续逻辑处理
func (a *SOCKS5MappingAdapter) PrepareConnection(conn io.ReadWriteCloser) error {
	netConn, ok := conn.(net.Conn)
	if !ok {
		return fmt.Errorf("SOCKS5 requires net.Conn")
	}

	// 设置握手超时
	netConn.SetDeadline(time.Now().Add(10 * time.Second))
	defer netConn.SetDeadline(time.Time{}) // 移除超时

	// 1. SOCKS5 握手
	if err := a.handleHandshake(netConn); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	// 2. 处理请求（解析目标地址）
	targetAddr, err := a.handleRequest(netConn)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	utils.Infof("SOCKS5MappingAdapter: client requests connection to %s", targetAddr)

	// 注意：这里我们不发送响应，因为需要先建立隧道
	// 实际的响应会在隧道建立后发送（通过包装的连接）

	return nil
}

// handleHandshake 处理SOCKS5握手
func (a *SOCKS5MappingAdapter) handleHandshake(conn net.Conn) error {
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

	// 选择无认证方法
	if _, err := conn.Write([]byte{socks5Version, socksAuthNone}); err != nil {
		return fmt.Errorf("write method selection failed: %w", err)
	}

	return nil
}

// handleRequest 处理SOCKS5请求
func (a *SOCKS5MappingAdapter) handleRequest(conn net.Conn) (string, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", fmt.Errorf("read request header failed: %w", err)
	}

	version := buf[0]
	if version != socks5Version {
		return "", fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	cmd := buf[1]
	addrType := buf[3]

	// 只支持 CONNECT 命令
	if cmd != socksCmdConnect {
		a.sendReply(conn, socksRepCommandNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("unsupported command: %d", cmd)
	}

	// 解析目标地址
	var targetAddr string
	switch addrType {
	case socksAddrTypeIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv4 address failed: %w", err)
		}
		targetAddr = net.IP(addr).String()

	case socksAddrTypeDomain:
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
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv6 address failed: %w", err)
		}
		targetAddr = net.IP(addr).String()

	default:
		a.sendReply(conn, socksRepAddrTypeNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("unsupported address type: %d", addrType)
	}

	// 读取端口
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", fmt.Errorf("read port failed: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	fullAddr := fmt.Sprintf("%s:%d", targetAddr, port)

	// 发送成功响应
	localAddr := conn.LocalAddr().(*net.TCPAddr)
	if err := a.sendReply(conn, socksRepSuccess, localAddr.IP.String(), uint16(localAddr.Port)); err != nil {
		return "", fmt.Errorf("send reply failed: %w", err)
	}

	return fullAddr, nil
}

// sendReply 发送SOCKS5响应
func (a *SOCKS5MappingAdapter) sendReply(conn net.Conn, rep byte, bindAddr string, bindPort uint16) error {
	reply := make([]byte, 4)
	reply[0] = socks5Version
	reply[1] = rep
	reply[2] = 0x00 // Reserved

	// 解析绑定地址
	ip := net.ParseIP(bindAddr)
	if ip4 := ip.To4(); ip4 != nil {
		reply[3] = socksAddrTypeIPv4
		reply = append(reply, ip4...)
	} else if ip6 := ip.To16(); ip6 != nil {
		reply[3] = socksAddrTypeIPv6
		reply = append(reply, ip6...)
	} else {
		reply[3] = socksAddrTypeIPv4
		reply = append(reply, 0, 0, 0, 0)
	}

	// 添加端口
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, bindPort)
	reply = append(reply, portBytes...)

	_, err := conn.Write(reply)
	return err
}

// GetProtocol 获取协议名称
func (a *SOCKS5MappingAdapter) GetProtocol() string {
	return "socks5"
}

// Close 关闭资源
func (a *SOCKS5MappingAdapter) Close() error {
	if a.listener != nil {
		return a.listener.Close()
	}
	return nil
}

