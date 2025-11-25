package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"
)

// Socks5MappingHandler SOCKS5 代理映射处理器
type Socks5MappingHandler struct {
	*dispose.ManagerBase

	client   *TunnoxClient
	config   MappingConfig
	listener net.Listener
}

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

// NewSocks5MappingHandler 创建 SOCKS5 映射处理器
func NewSocks5MappingHandler(client *TunnoxClient, config MappingConfig) *Socks5MappingHandler {
	handler := &Socks5MappingHandler{
		ManagerBase: dispose.NewManager(fmt.Sprintf("Socks5Mapping-%s", config.MappingID), client.Ctx()),
		client:      client,
		config:      config,
	}

	// 添加清理处理器
	handler.AddCleanHandler(func() error {
		utils.Infof("Socks5MappingHandler[%s]: cleaning up resources", config.MappingID)
		if handler.listener != nil {
			handler.listener.Close()
		}
		return nil
	})

	return handler
}

// Start 启动 SOCKS5 代理服务器
func (h *Socks5MappingHandler) Start() error {
	addr := fmt.Sprintf(":%d", h.config.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	h.listener = listener
	utils.Infof("Socks5MappingHandler: listening on %s for mapping %s", addr, h.config.MappingID)

	// 启动接受连接的循环
	go h.acceptLoop()

	return nil
}

// acceptLoop 接受 SOCKS5 客户端连接
func (h *Socks5MappingHandler) acceptLoop() {
	for {
		select {
		case <-h.Ctx().Done():
			return
		default:
		}

		// 设置接受超时
		h.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

		clientConn, err := h.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if h.Ctx().Err() != nil {
				return
			}
			utils.Errorf("Socks5MappingHandler: failed to accept connection: %v", err)
			continue
		}

		// 处理 SOCKS5 连接
		go h.handleSOCKS5Connection(clientConn)
	}
}

// handleSOCKS5Connection 处理 SOCKS5 连接
func (h *Socks5MappingHandler) handleSOCKS5Connection(clientConn net.Conn) {
	defer clientConn.Close()

	// 设置握手超时
	clientConn.SetDeadline(time.Now().Add(10 * time.Second))

	// 1. SOCKS5 握手
	if err := h.handleHandshake(clientConn); err != nil {
		utils.Errorf("Socks5MappingHandler: handshake failed: %v", err)
		return
	}

	// 2. 处理请求
	targetAddr, err := h.handleRequest(clientConn)
	if err != nil {
		utils.Errorf("Socks5MappingHandler: request failed: %v", err)
		return
	}

	// 移除握手超时
	clientConn.SetDeadline(time.Time{})

	utils.Infof("Socks5MappingHandler: client requests connection to %s", targetAddr)

	// 3. 通过隧道连接到目标
	tunnelConn, err := h.connectThroughTunnel(targetAddr)
	if err != nil {
		utils.Errorf("Socks5MappingHandler: failed to connect through tunnel: %v", err)
		h.sendReply(clientConn, socksRepServerFailure, "0.0.0.0", 0)
		return
	}
	defer tunnelConn.Close()

	// 4. 发送成功响应
	localAddr := clientConn.LocalAddr().(*net.TCPAddr)
	if err := h.sendReply(clientConn, socksRepSuccess, localAddr.IP.String(), uint16(localAddr.Port)); err != nil {
		utils.Errorf("Socks5MappingHandler: failed to send reply: %v", err)
		return
	}

	// 5. 双向转发数据
	utils.BidirectionalCopy(clientConn, tunnelConn, &utils.BidirectionalCopyOptions{
		LogPrefix: fmt.Sprintf("SOCKS5[%s]", targetAddr),
	})
}

// handleHandshake 处理 SOCKS5 握手
func (h *Socks5MappingHandler) handleHandshake(conn net.Conn) error {
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

// handleRequest 处理 SOCKS5 请求
func (h *Socks5MappingHandler) handleRequest(conn net.Conn) (string, error) {
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
		h.sendReply(conn, socksRepCommandNotSupported, "0.0.0.0", 0)
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
		h.sendReply(conn, socksRepAddrTypeNotSupported, "0.0.0.0", 0)
		return "", fmt.Errorf("unsupported address type: %d", addrType)
	}

	// 读取端口
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", fmt.Errorf("read port failed: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	return fmt.Sprintf("%s:%d", targetAddr, port), nil
}

// sendReply 发送 SOCKS5 响应
func (h *Socks5MappingHandler) sendReply(conn net.Conn, rep byte, bindAddr string, bindPort uint16) error {
	ip := net.ParseIP(bindAddr)
	if ip == nil {
		ip = net.IPv4zero
	}

	reply := make([]byte, 0, 22)
	reply = append(reply, socks5Version, rep, 0x00)

	if ip4 := ip.To4(); ip4 != nil {
		reply = append(reply, socksAddrTypeIPv4)
		reply = append(reply, ip4...)
	} else {
		reply = append(reply, socksAddrTypeIPv6)
		reply = append(reply, ip.To16()...)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, bindPort)
	reply = append(reply, portBytes...)

	_, err := conn.Write(reply)
	return err
}

// connectThroughTunnel 通过隧道连接到目标
func (h *Socks5MappingHandler) connectThroughTunnel(targetAddr string) (net.Conn, error) {
	// 生成 TunnelID
	tunnelID := fmt.Sprintf("socks5-tunnel-%d-%s", time.Now().UnixNano(), targetAddr)

	// 建立隧道连接
	tunnelConn, tunnelStream, err := h.client.DialTunnel(tunnelID, h.config.MappingID, h.config.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to dial tunnel: %w", err)
	}

	utils.Infof("Socks5MappingHandler: tunnel %s established for %s", tunnelID, targetAddr)

	// 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 应用转换器
	transformer, err := h.createTransformer()
	if err != nil {
		tunnelConn.Close()
		return nil, fmt.Errorf("failed to create transformer: %w", err)
	}

	if transformer != nil {
		// 创建并缓存 reader/writer
		reader, err := transformer.WrapReader(tunnelConn)
		if err != nil {
			tunnelConn.Close()
			return nil, fmt.Errorf("failed to wrap reader: %w", err)
		}
		writer, err := transformer.WrapWriter(tunnelConn)
		if err != nil {
			tunnelConn.Close()
			return nil, fmt.Errorf("failed to wrap writer: %w", err)
		}

		return &transformedConn{
			Conn:        tunnelConn,
			transformer: transformer,
			reader:      reader,
			writer:      writer,
		}, nil
	}

	return tunnelConn, nil
}

// transformedConn 包装的连接（应用转换器）
type transformedConn struct {
	net.Conn
	transformer transform.StreamTransformer
	reader      io.Reader
	writer      io.Writer
}

func (tc *transformedConn) Read(p []byte) (n int, err error) {
	if tc.reader != nil {
		return tc.reader.Read(p)
	}
	return tc.Conn.Read(p)
}

func (tc *transformedConn) Write(p []byte) (n int, err error) {
	if tc.writer != nil {
		return tc.writer.Write(p)
	}
	return tc.Conn.Write(p)
}

// createTransformer 创建流转换器
func (h *Socks5MappingHandler) createTransformer() (transform.StreamTransformer, error) {
	config := &transform.TransformConfig{
		EnableCompression: h.config.EnableCompression,
		CompressionLevel:  h.config.CompressionLevel,
		EnableEncryption:  h.config.EnableEncryption,
		EncryptionMethod:  h.config.EncryptionMethod,
		EncryptionKey:     h.config.EncryptionKey,
	}
	return transform.NewTransformer(config)
}

// Stop 停止映射处理器
func (h *Socks5MappingHandler) Stop() {
	utils.Infof("Socks5MappingHandler: stopping mapping %s", h.config.MappingID)
	h.Close()
}

// GetConfig 返回映射配置
func (h *Socks5MappingHandler) GetConfig() MappingConfig {
	return h.config
}

// GetContext 返回上下文
func (h *Socks5MappingHandler) GetContext() context.Context {
	return h.Ctx()
}

// GetMappingID 获取映射ID
func (h *Socks5MappingHandler) GetMappingID() string {
	return h.config.MappingID
}

// GetProtocol 获取协议
func (h *Socks5MappingHandler) GetProtocol() string {
	return "socks5"
}

