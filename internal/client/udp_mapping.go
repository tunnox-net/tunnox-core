package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"
)

// UdpMappingHandler UDP 端口映射处理器
type UdpMappingHandler struct {
	*dispose.ManagerBase

	client   *TunnoxClient
	config   MappingConfig
	conn     *net.UDPConn
	sessions map[string]*udpSession
	sessLock sync.RWMutex
}

// udpSession UDP 会话
type udpSession struct {
	userAddr   *net.UDPAddr
	tunnelConn net.Conn
	lastActive time.Time
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex

	// 缓存的转换器和读写器
	transformer transform.StreamTransformer
	reader      io.Reader
	writer      io.Writer
}

const (
	udpSessionTimeout  = 30 * time.Second
	udpCleanupInterval = 10 * time.Second
	udpMaxPacketSize   = 65535
)

// NewUdpMappingHandler 创建 UDP 映射处理器
func NewUdpMappingHandler(client *TunnoxClient, config MappingConfig) *UdpMappingHandler {
	handler := &UdpMappingHandler{
		ManagerBase: dispose.NewManager(fmt.Sprintf("UdpMapping-%s", config.MappingID), client.Ctx()),
		client:      client,
		config:      config,
		sessions:    make(map[string]*udpSession),
	}

	// 添加清理处理器
	handler.AddCleanHandler(func() error {
		utils.Infof("UdpMappingHandler[%s]: cleaning up resources", config.MappingID)

		// 关闭所有会话
		handler.sessLock.Lock()
		for _, session := range handler.sessions {
			session.cancel()
			if session.tunnelConn != nil {
				session.tunnelConn.Close()
			}
		}
		handler.sessions = make(map[string]*udpSession)
		handler.sessLock.Unlock()

		// 关闭 UDP 连接
		if handler.conn != nil {
			handler.conn.Close()
		}

		return nil
	})

	return handler
}

// Start 启动 UDP 映射监听
func (h *UdpMappingHandler) Start() error {
	addr := fmt.Sprintf(":%d", h.config.LocalPort)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	h.conn = conn
	utils.Infof("UdpMappingHandler: listening on %s for mapping %s", addr, h.config.MappingID)

	// 启动接收循环
	go h.receiveLoop()

	// 启动会话清理循环
	go h.cleanupLoop()

	return nil
}

// receiveLoop 接收用户发来的 UDP 数据包
func (h *UdpMappingHandler) receiveLoop() {
	buffer := make([]byte, udpMaxPacketSize)

	for {
		select {
		case <-h.Ctx().Done():
			return
		default:
		}

		h.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, userAddr, err := h.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if h.Ctx().Err() != nil {
				return
			}
			utils.Errorf("UdpMappingHandler: failed to read packet: %v", err)
			continue
		}

		if n == 0 {
			continue
		}

		// 复制数据
		data := make([]byte, n)
		copy(data, buffer[:n])

		// 处理数据包
		go h.handlePacket(userAddr, data)
	}
}

// handlePacket 处理单个 UDP 数据包
func (h *UdpMappingHandler) handlePacket(userAddr *net.UDPAddr, data []byte) {
	addrKey := userAddr.String()

	// 获取或创建会话
	h.sessLock.RLock()
	session := h.sessions[addrKey]
	h.sessLock.RUnlock()

	if session == nil {
		session = h.createSession(userAddr)
		if session == nil {
			return
		}
	}

	// 更新会话活跃时间
	session.updateActivity()

	// 通过隧道发送数据
	if err := h.sendToTunnel(session, data); err != nil {
		utils.Errorf("UdpMappingHandler: failed to send data: %v", err)
		h.removeSession(addrKey)
	}
}

// createSession 创建新的 UDP 会话
func (h *UdpMappingHandler) createSession(userAddr *net.UDPAddr) *udpSession {
	utils.Infof("UdpMappingHandler: creating session for %s", userAddr)

	// 生成 TunnelID
	tunnelID := fmt.Sprintf("udp-tunnel-%d-%s", time.Now().UnixNano(), userAddr.String())

	// 建立隧道连接
	tunnelConn, tunnelStream, err := h.client.DialTunnel(tunnelID, h.config.MappingID, h.config.SecretKey)
	if err != nil {
		utils.Errorf("UdpMappingHandler: failed to dial tunnel: %v", err)
		return nil
	}

	utils.Infof("UdpMappingHandler: tunnel %s established for %s", tunnelID, userAddr)

	// 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 创建转换器和读写器
	transformer, err := h.createTransformer()
	if err != nil {
		utils.Errorf("UdpMappingHandler: failed to create transformer: %v", err)
		tunnelConn.Close()
		return nil
	}

	var reader io.Reader = tunnelConn
	var writer io.Writer = tunnelConn
	if transformer != nil {
		reader, err = transformer.WrapReader(tunnelConn)
		if err != nil {
			utils.Errorf("UdpMappingHandler: failed to wrap reader: %v", err)
			tunnelConn.Close()
			return nil
		}
		writer, err = transformer.WrapWriter(tunnelConn)
		if err != nil {
			utils.Errorf("UdpMappingHandler: failed to wrap writer: %v", err)
			tunnelConn.Close()
			return nil
		}
	}

	// 创建会话
	sessionCtx, sessionCancel := context.WithCancel(h.Ctx())
	session := &udpSession{
		userAddr:    userAddr,
		tunnelConn:  tunnelConn,
		lastActive:  time.Now(),
		ctx:         sessionCtx,
		cancel:      sessionCancel,
		transformer: transformer,
		reader:      reader,
		writer:      writer,
	}

	// 注册会话
	addrKey := userAddr.String()
	h.sessLock.Lock()
	h.sessions[addrKey] = session
	h.sessLock.Unlock()

	// 启动接收隧道数据的循环
	go h.receiveTunnelData(session, addrKey)

	return session
}

// sendToTunnel 通过隧道发送数据
func (h *UdpMappingHandler) sendToTunnel(session *udpSession, data []byte) error {
	session.mu.RLock()
	defer session.mu.RUnlock()

	if session.tunnelConn == nil {
		return fmt.Errorf("tunnel connection closed")
	}

	// 使用会话缓存的 writer
	writer := session.writer
	if writer == nil {
		writer = session.tunnelConn
	}

	// UDP 数据包格式：[2字节长度][数据]
	length := uint16(len(data))
	lengthBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBuf, length)

	if _, err := writer.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// receiveTunnelData 接收隧道返回的数据
func (h *UdpMappingHandler) receiveTunnelData(session *udpSession, addrKey string) {
	defer h.removeSession(addrKey)

	// 使用会话缓存的 reader
	reader := session.reader
	if reader == nil {
		reader = session.tunnelConn
	}

	for {
		select {
		case <-session.ctx.Done():
			return
		default:
		}

		// 读取长度
		lengthBuf := make([]byte, 2)
		if _, err := io.ReadFull(reader, lengthBuf); err != nil {
			if err != io.EOF {
				utils.Debugf("UdpMappingHandler: read error: %v", err)
			}
			return
		}

		length := binary.BigEndian.Uint16(lengthBuf)
		if length == 0 || length > udpMaxPacketSize {
			utils.Errorf("UdpMappingHandler: invalid packet length: %d", length)
			return
		}

		// 读取数据
		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			utils.Errorf("UdpMappingHandler: failed to read data: %v", err)
			return
		}

		// 发送回用户
		if _, err := h.conn.WriteToUDP(data, session.userAddr); err != nil {
			utils.Errorf("UdpMappingHandler: failed to write to user: %v", err)
			return
		}

		session.updateActivity()
	}
}

// createTransformer 创建流转换器
func (h *UdpMappingHandler) createTransformer() (transform.StreamTransformer, error) {
	config := &transform.TransformConfig{
		EnableCompression: h.config.EnableCompression,
		CompressionLevel:  h.config.CompressionLevel,
		EnableEncryption:  h.config.EnableEncryption,
		EncryptionMethod:  h.config.EncryptionMethod,
		EncryptionKey:     h.config.EncryptionKey,
	}
	return transform.NewTransformer(config)
}

// updateActivity 更新会话活跃时间
func (s *udpSession) updateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive = time.Now()
}

// isExpired 检查会话是否过期
func (s *udpSession) isExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastActive) > udpSessionTimeout
}

// cleanupLoop 清理过期会话
func (h *UdpMappingHandler) cleanupLoop() {
	ticker := time.NewTicker(udpCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.Ctx().Done():
			return
		case <-ticker.C:
			h.sessLock.Lock()
			for addr, session := range h.sessions {
				if session.isExpired() {
					utils.Infof("UdpMappingHandler: cleaning up session for %s", addr)
					session.cancel()
					session.tunnelConn.Close()
					delete(h.sessions, addr)
				}
			}
			h.sessLock.Unlock()
		}
	}
}

// removeSession 移除会话
func (h *UdpMappingHandler) removeSession(addrKey string) {
	h.sessLock.Lock()
	defer h.sessLock.Unlock()

	if session, exists := h.sessions[addrKey]; exists {
		session.cancel()
		session.tunnelConn.Close()
		delete(h.sessions, addrKey)
		utils.Infof("UdpMappingHandler: session removed for %s", addrKey)
	}
}

// Stop 停止映射处理器
func (h *UdpMappingHandler) Stop() {
	utils.Infof("UdpMappingHandler: stopping mapping %s", h.config.MappingID)
	h.Close()
}

// GetConfig 返回映射配置
func (h *UdpMappingHandler) GetConfig() MappingConfig {
	return h.config
}

// GetContext 返回上下文
func (h *UdpMappingHandler) GetContext() context.Context {
	return h.Ctx()
}

// GetMappingID 获取映射ID
func (h *UdpMappingHandler) GetMappingID() string {
	return h.config.MappingID
}

// GetProtocol 获取协议
func (h *UdpMappingHandler) GetProtocol() string {
	return "udp"
}
