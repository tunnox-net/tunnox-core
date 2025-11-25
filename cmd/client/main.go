package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"

	"gopkg.in/yaml.v3"
)

// ClientConfig 客户端配置
type ClientConfig struct {
	// 注册客户端认证
	ClientID  int64  `yaml:"client_id"`
	AuthToken string `yaml:"auth_token"`

	// 匿名客户端认证
	Anonymous bool   `yaml:"anonymous"`
	DeviceID  string `yaml:"device_id"`

	Server struct {
		Address  string `yaml:"address"`  // 服务器地址，例如 "localhost:7000"
		Protocol string `yaml:"protocol"` // tcp/websocket/quic
	} `yaml:"server"`
	// 注意：映射配置由服务器通过指令连接动态推送，不在配置文件中
}

// MappingConfig 映射配置
type MappingConfig struct {
	MappingID  string `yaml:"mapping_id"`
	SecretKey  string `yaml:"secret_key"`
	LocalPort  int    `yaml:"local_port"`
	TargetHost string `yaml:"target_host"`
	TargetPort int    `yaml:"target_port"`

	// ✅ 压缩、加密配置（从服务器推送）
	EnableCompression bool   `json:"enable_compression"`
	CompressionLevel  int    `json:"compression_level"`
	EnableEncryption  bool   `json:"enable_encryption"`
	EncryptionMethod  string `json:"encryption_method"`
	EncryptionKey     string `json:"encryption_key"`
}

// TunnoxClient 客户端
type TunnoxClient struct {
	config *ClientConfig
	ctx    context.Context
	cancel context.CancelFunc

	// 指令连接
	controlConn   net.Conn
	controlStream stream.PackageStreamer

	// 映射管理
	mappingHandlers map[string]*MappingHandler
	mu              sync.RWMutex
}

// NewTunnoxClient 创建客户端
func NewTunnoxClient(config *ClientConfig) *TunnoxClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &TunnoxClient{
		config:          config,
		ctx:             ctx,
		cancel:          cancel,
		mappingHandlers: make(map[string]*MappingHandler),
	}
}

// Connect 连接到服务器并建立指令连接
func (c *TunnoxClient) Connect() error {
	utils.Infof("Client: connecting to server %s", c.config.Server.Address)

	// 1. 建立 TCP 连接
	conn, err := net.DialTimeout("tcp", c.config.Server.Address, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to dial server: %w", err)
	}

	c.controlConn = conn

	// 2. 创建 Stream
	streamFactory := stream.NewDefaultStreamFactory(c.ctx)
	c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)

	// 3. 发送握手请求
	if err := c.sendHandshake(); err != nil {
		c.controlStream.Close()
		conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// 4. 启动读取循环（接收服务器命令）
	go c.readLoop()

	// 5. 启动心跳循环
	go c.heartbeatLoop()

	utils.Infof("Client: control connection established successfully")
	return nil
}

// sendHandshake 发送握手请求
func (c *TunnoxClient) sendHandshake() error {
	var req *packet.HandshakeRequest

	if c.config.Anonymous {
		// 匿名模式
		utils.Infof("Client: using anonymous authentication")
		req = &packet.HandshakeRequest{
			ClientID: 0, // 匿名客户端，ClientID 为 0
			Token:    fmt.Sprintf("anonymous:%s", c.config.DeviceID),
			Version:  "2.0",
			Protocol: c.config.Server.Protocol,
		}
	} else {
		// 注册客户端模式
		utils.Infof("Client: using registered authentication, client_id=%d", c.config.ClientID)
		req = &packet.HandshakeRequest{
			ClientID: c.config.ClientID,
			Token:    c.config.AuthToken,
			Version:  "2.0",
			Protocol: c.config.Server.Protocol,
		}
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake request: %w", err)
	}

	handshakePkt := &packet.TransferPacket{
		PacketType: packet.Handshake,
		Payload:    reqData,
	}

	// 发送握手包
	if _, err := c.controlStream.WritePacket(handshakePkt, false, 0); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	utils.Infof("Client: handshake request sent, waiting for response...")

	// 等待握手响应
	respPkt, _, err := c.controlStream.ReadPacket()
	if err != nil {
		return fmt.Errorf("failed to read handshake response: %w", err)
	}

	if respPkt.PacketType != packet.HandshakeResp {
		return fmt.Errorf("unexpected response type: %v", respPkt.PacketType)
	}

	var resp packet.HandshakeResponse
	if err := json.Unmarshal(respPkt.Payload, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal handshake response: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("handshake failed: %s", resp.Error)
	}

	// 匿名模式下，服务器会在响应中返回分配的 ClientID
	if c.config.Anonymous && resp.Message != "" {
		// 尝试从 Message 中解析 ClientID
		// 格式：如 "Anonymous client authenticated, client_id=200000001"
		var assignedClientID int64
		if _, err := fmt.Sscanf(resp.Message, "Anonymous client authenticated, client_id=%d", &assignedClientID); err == nil {
			c.config.ClientID = assignedClientID
			utils.Infof("Client: anonymous authentication successful, assigned client_id=%d", assignedClientID)
		} else {
			utils.Infof("Client: handshake successful (anonymous mode)")
		}
	} else {
		utils.Infof("Client: handshake successful, authenticated as client_id=%d", c.config.ClientID)
	}

	return nil
}

// readLoop 读取循环（接收服务器命令）
func (c *TunnoxClient) readLoop() {
	utils.Infof("Client: read loop started")

	for {
		select {
		case <-c.ctx.Done():
			utils.Infof("Client: read loop stopped")
			return
		default:
			// 读取服务器发来的数据包
			pkt, _, err := c.controlStream.ReadPacket()
			if err != nil {
				if err != io.EOF {
					utils.Errorf("Client: failed to read packet: %v", err)
				}
				return
			}

			// 处理不同类型的数据包
			switch pkt.PacketType & 0x3F {
			case packet.Heartbeat:
				// 心跳响应
				c.handleHeartbeat(pkt)
			case packet.JsonCommand:
				// 命令处理
				c.handleCommand(pkt)
			default:
				utils.Warnf("Client: unknown packet type: %v", pkt.PacketType)
			}
		}
	}
}

// heartbeatLoop 心跳循环（定时发送心跳）
func (c *TunnoxClient) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒发送一次心跳
	defer ticker.Stop()

	utils.Infof("Client: heartbeat loop started")

	for {
		select {
		case <-c.ctx.Done():
			utils.Infof("Client: heartbeat loop stopped")
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				utils.Errorf("Client: failed to send heartbeat: %v", err)
				// 心跳发送失败，可能连接已断开
				// TODO: 触发重连机制
			} else {
				utils.Debugf("Client: heartbeat sent successfully")
			}
		}
	}
}

// sendHeartbeat 发送心跳包
func (c *TunnoxClient) sendHeartbeat() error {
	heartbeatPkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
		Payload:    []byte{},
	}

	if _, err := c.controlStream.WritePacket(heartbeatPkt, false, 0); err != nil {
		return fmt.Errorf("failed to write heartbeat packet: %w", err)
	}

	return nil
}

// handleHeartbeat 处理心跳响应
func (c *TunnoxClient) handleHeartbeat(pkt *packet.TransferPacket) {
	utils.Debugf("Client: heartbeat response received")
}

// handleCommand 处理命令（服务器推送的配置更新等）
func (c *TunnoxClient) handleCommand(pkt *packet.TransferPacket) {
	if pkt.CommandPacket == nil {
		utils.Warnf("Client: command packet is nil")
		return
	}

	cmdType := pkt.CommandPacket.CommandType
	utils.Infof("Client: received command, type=%v", cmdType)

	switch cmdType {
	case packet.ConfigGet:
		// 配置查询响应
		c.handleConfigUpdate(pkt.CommandPacket.CommandBody)

	case packet.TunnelOpenRequestCmd:
		// ✅ 隧道打开请求（作为目标客户端）
		c.handleTunnelOpenRequest(pkt.CommandPacket.CommandBody)

	default:
		utils.Debugf("Client: unhandled command type: %v", cmdType)
	}
}

// handleTunnelOpenRequest 处理隧道打开请求（作为目标客户端）
func (c *TunnoxClient) handleTunnelOpenRequest(cmdBody string) {
	utils.Infof("Client: received tunnel open request")

	// ✅ 解析请求（包含压缩、加密配置）
	var req struct {
		TunnelID   string `json:"tunnel_id"`
		MappingID  string `json:"mapping_id"`
		SecretKey  string `json:"secret_key"`
		TargetHost string `json:"target_host"`
		TargetPort int    `json:"target_port"`

		// ✅ 压缩、加密配置
		EnableCompression bool   `json:"enable_compression"`
		CompressionLevel  int    `json:"compression_level"`
		EnableEncryption  bool   `json:"enable_encryption"`
		EncryptionMethod  string `json:"encryption_method"`
		EncryptionKey     string `json:"encryption_key"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &req); err != nil {
		utils.Errorf("Client: failed to parse tunnel open request: %v", err)
		return
	}

	utils.Infof("Client: processing tunnel open request, tunnel_id=%s, target=%s:%d, compression=%v, encryption=%v",
		req.TunnelID, req.TargetHost, req.TargetPort, req.EnableCompression, req.EnableEncryption)

	// ✅ 构造转换配置
	transformConfig := &transform.TransformConfig{
		EnableCompression: req.EnableCompression,
		CompressionLevel:  req.CompressionLevel,
		EnableEncryption:  req.EnableEncryption,
		EncryptionMethod:  req.EncryptionMethod,
		EncryptionKey:     req.EncryptionKey,
	}

	// 异步处理（不阻塞指令连接）
	go c.handleTunnelOpenRequestAsync(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
}

// handleTunnelOpenRequestAsync 异步处理隧道打开请求
func (c *TunnoxClient) handleTunnelOpenRequestAsync(tunnelID, mappingID, secretKey, targetHost string, targetPort int, transformConfig *transform.TransformConfig) {
	// 1. ✅ 连接到目标服务
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		utils.Errorf("Client: failed to connect to target %s: %v", targetAddr, err)
		return
	}

	utils.Infof("Client: connected to target %s for tunnel %s", targetAddr, tunnelID)

	// 2. ✅ 创建映射连接到服务器
	serverConn, err := net.DialTimeout("tcp", c.config.Server.Address, 10*time.Second)
	if err != nil {
		utils.Errorf("Client: failed to connect to server: %v", err)
		targetConn.Close()
		return
	}

	// 创建 StreamProcessor
	streamFactory := stream.NewDefaultStreamFactory(c.ctx)
	tunnelStream := streamFactory.CreateStreamProcessor(serverConn, serverConn)
	if err != nil {
		utils.Errorf("Client: failed to create tunnel stream: %v", err)
		serverConn.Close()
		targetConn.Close()
		return
	}

	// 3. ✅ 发送 TunnelOpen（作为目标端）
	tunnelOpenReq := &packet.TunnelOpenRequest{
		MappingID: mappingID,
		TunnelID:  tunnelID,
		SecretKey: secretKey,
	}

	tunnelOpenData, _ := json.Marshal(tunnelOpenReq)
	tunnelOpenPkt := &packet.TransferPacket{
		PacketType: packet.TunnelOpen,
		Payload:    tunnelOpenData,
	}

	if _, err := tunnelStream.WritePacket(tunnelOpenPkt, false, 0); err != nil {
		utils.Errorf("Client: failed to send tunnel open: %v", err)
		serverConn.Close()
		targetConn.Close()
		return
	}

	utils.Infof("Client: tunnel open sent for tunnel %s", tunnelID)

	// 4. ✅ 等待 TunnelOpenAck
	ackPkt, _, err := tunnelStream.ReadPacket()
	if err != nil {
		utils.Errorf("Client: failed to read tunnel open ack: %v", err)
		serverConn.Close()
		targetConn.Close()
		return
	}

	if ackPkt.PacketType != packet.TunnelOpenAck {
		utils.Errorf("Client: unexpected packet type: %v", ackPkt.PacketType)
		serverConn.Close()
		targetConn.Close()
		return
	}

	var ack packet.TunnelOpenAckResponse
	if err := json.Unmarshal(ackPkt.Payload, &ack); err != nil {
		utils.Errorf("Client: failed to parse tunnel open ack: %v", err)
		serverConn.Close()
		targetConn.Close()
		return
	}

	if !ack.Success {
		utils.Errorf("Client: tunnel open failed: tunnel_id=%s", tunnelID)
		serverConn.Close()
		targetConn.Close()
		return
	}

	utils.Infof("Client: tunnel open ack received, tunnel ready, tunnel_id=%s", tunnelID)

	// 5. ✅ 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 6. ✅ 启动纯透传（直接 io.Copy，应用压缩、加密）
	c.bidirectionalCopyAsTarget(serverConn, targetConn, tunnelID, transformConfig)
}

// bidirectionalCopyAsTarget ✅ 双向纯透传（作为目标客户端）
// ServerConn ↔ TargetConn，✅ 应用压缩、加密
func (c *TunnoxClient) bidirectionalCopyAsTarget(serverConn, targetConn net.Conn, tunnelID string, transformConfig *transform.TransformConfig) {
	// 创建转换器
	transformer, err := transform.NewTransformer(transformConfig)
	if err != nil {
		utils.Errorf("Client: failed to create transformer for tunnel %s: %v", tunnelID, err)
		serverConn.Close()
		targetConn.Close()
		return
	}

	// ✅ 使用通用双向拷贝函数（注意：目标端的连接方向是反向的）
	utils.BidirectionalCopy(targetConn, serverConn, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("Client[target][tunnel:%s]", tunnelID),
	})
}

// handleConfigUpdate 处理配置更新
func (c *TunnoxClient) handleConfigUpdate(configBody string) {
	utils.Infof("Client: received config update")

	// 解析配置
	var configUpdate struct {
		Mappings []MappingConfig `json:"mappings"`
	}

	if err := json.Unmarshal([]byte(configBody), &configUpdate); err != nil {
		utils.Errorf("Client: failed to parse config update: %v", err)
		return
	}

	// 动态更新映射
	for _, mappingConfig := range configUpdate.Mappings {
		c.addOrUpdateMapping(mappingConfig)
	}

	utils.Infof("Client: config updated, total mappings=%d", len(c.mappingHandlers))
}

// addOrUpdateMapping 添加或更新映射
func (c *TunnoxClient) addOrUpdateMapping(config MappingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在
	if handler, exists := c.mappingHandlers[config.MappingID]; exists {
		// 如果配置相同，不做处理
		if handler.config.LocalPort == config.LocalPort &&
			handler.config.SecretKey == config.SecretKey {
			utils.Debugf("Client: mapping %s already exists with same config", config.MappingID)
			return
		}

		// 配置不同，先停止旧的
		utils.Infof("Client: updating mapping %s", config.MappingID)
		handler.Stop()
		delete(c.mappingHandlers, config.MappingID)
	}

	// 创建新的映射处理器
	handler := NewMappingHandler(c, config)
	if err := handler.Start(); err != nil {
		utils.Errorf("Client: failed to start mapping %s: %v", config.MappingID, err)
		return
	}

	c.mappingHandlers[config.MappingID] = handler
	utils.Infof("Client: mapping %s started on port %d", config.MappingID, config.LocalPort)
}

// removeMapping 移除映射
func (c *TunnoxClient) removeMapping(mappingID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	handler, exists := c.mappingHandlers[mappingID]
	if !exists {
		return
	}

	handler.Stop()
	delete(c.mappingHandlers, mappingID)
	utils.Infof("Client: mapping %s stopped", mappingID)
}

// Stop 停止客户端
func (c *TunnoxClient) Stop() {
	utils.Infof("Client: stopping...")

	// 停止所有映射
	c.mu.Lock()
	for _, handler := range c.mappingHandlers {
		handler.Stop()
	}
	c.mu.Unlock()

	// 取消上下文
	c.cancel()

	// 关闭指令连接
	if c.controlStream != nil {
		c.controlStream.Close()
	}
	if c.controlConn != nil {
		c.controlConn.Close()
	}

	utils.Infof("Client: stopped successfully")
}

// =============================================================================
// MappingHandler 映射处理器
// =============================================================================

// MappingHandler 处理单个映射
type MappingHandler struct {
	client   *TunnoxClient
	config   MappingConfig
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewMappingHandler 创建映射处理器
func NewMappingHandler(client *TunnoxClient, config MappingConfig) *MappingHandler {
	ctx, cancel := context.WithCancel(client.ctx)
	return &MappingHandler{
		client: client,
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动映射监听
func (h *MappingHandler) Start() error {
	// 创建本地监听
	addr := fmt.Sprintf(":%d", h.config.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	h.listener = listener
	utils.Infof("MappingHandler: listening on %s for mapping %s", addr, h.config.MappingID)

	// 启动接受连接的循环
	go h.acceptLoop()

	return nil
}

// acceptLoop 接受用户连接
func (h *MappingHandler) acceptLoop() {
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			// 设置接受超时
			h.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

			userConn, err := h.listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if h.ctx.Err() != nil {
					return // 正常关闭
				}
				utils.Errorf("MappingHandler: failed to accept connection: %v", err)
				continue
			}

			// 处理用户连接
			go h.handleUserConnection(userConn)
		}
	}
}

// handleUserConnection 处理用户连接（建立映射连接）
func (h *MappingHandler) handleUserConnection(userConn net.Conn) {
	defer userConn.Close()

	utils.Infof("MappingHandler: user connected from %s, establishing tunnel", userConn.RemoteAddr())

	// 1. 建立新的 TCP 连接到服务器（映射连接）
	tunnelConn, err := net.DialTimeout("tcp", h.client.config.Server.Address, 10*time.Second)
	if err != nil {
		utils.Errorf("MappingHandler: failed to dial server for tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	// 2. 创建 Stream（仅用于前置包）
	streamFactory := stream.NewDefaultStreamFactory(h.ctx)
	tunnelStream := streamFactory.CreateStreamProcessor(tunnelConn, tunnelConn)

	// 3. 生成 TunnelID
	tunnelID := fmt.Sprintf("tunnel-%d-%d", time.Now().UnixNano(), h.config.LocalPort)

	// 4. ✅ 发送前置包（TunnelOpen，唯一一次）
	if err := h.sendTunnelOpen(tunnelStream, tunnelID); err != nil {
		utils.Errorf("MappingHandler: failed to send tunnel open: %v", err)
		return
	}

	// 5. ✅ 等待前置响应（TunnelOpenAck，唯一一次）
	if err := h.waitTunnelOpenAck(tunnelStream); err != nil {
		utils.Errorf("MappingHandler: tunnel open failed: %v", err)
		return
	}

	utils.Infof("MappingHandler: tunnel %s established successfully", tunnelID)

	// 6. ✅ 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 7. ✅ 开始纯透传（直接 io.Copy，不再组包）
	h.bidirectionalCopy(userConn, tunnelConn, tunnelID)
}

// sendTunnelOpen 发送 TunnelOpen 包
func (h *MappingHandler) sendTunnelOpen(tunnelStream stream.PackageStreamer, tunnelID string) error {
	req := &packet.TunnelOpenRequest{
		MappingID: h.config.MappingID,
		TunnelID:  tunnelID,
		SecretKey: h.config.SecretKey,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel open request: %w", err)
	}

	openPkt := &packet.TransferPacket{
		PacketType: packet.TunnelOpen,
		TunnelID:   tunnelID,
		Payload:    reqData,
	}

	if _, err := tunnelStream.WritePacket(openPkt, false, 0); err != nil {
		return fmt.Errorf("failed to send tunnel open: %w", err)
	}

	return nil
}

// waitTunnelOpenAck 等待 TunnelOpenAck
func (h *MappingHandler) waitTunnelOpenAck(tunnelStream stream.PackageStreamer) error {
	// 设置超时
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()

	// 读取响应
	respCh := make(chan *packet.TransferPacket, 1)
	errCh := make(chan error, 1)

	go func() {
		pkt, _, err := tunnelStream.ReadPacket()
		if err != nil {
			errCh <- err
			return
		}
		respCh <- pkt
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for tunnel open ack")
	case err := <-errCh:
		return fmt.Errorf("failed to read tunnel open ack: %w", err)
	case pkt := <-respCh:
		if pkt.PacketType != packet.TunnelOpenAck {
			return fmt.Errorf("unexpected response type: %v", pkt.PacketType)
		}

		var ack packet.TunnelOpenAckResponse
		if err := json.Unmarshal(pkt.Payload, &ack); err != nil {
			return fmt.Errorf("failed to unmarshal tunnel open ack: %w", err)
		}

		if !ack.Success {
			return fmt.Errorf("tunnel open failed: %s", ack.Error)
		}

		return nil
	}
}

// bidirectionalCopy ✅ 双向纯透传（裸连接模式，不再组包）
// ✅ 应用压缩、加密：顺序为 压缩 → 加密 → 网络传输 → 解密 → 解压
func (h *MappingHandler) bidirectionalCopy(userConn, tunnelConn net.Conn, tunnelID string) {
	// 根据映射配置创建转换器
	transformer, err := h.createTransformer()
	if err != nil {
		utils.Errorf("MappingHandler: failed to create transformer: %v", err)
		userConn.Close()
		tunnelConn.Close()
		return
	}

	// ✅ 使用通用双向拷贝函数
	utils.BidirectionalCopy(userConn, tunnelConn, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("MappingHandler[source][tunnel:%s]", tunnelID),
	})
}

// createTransformer 创建流转换器（根据映射配置）
func (h *MappingHandler) createTransformer() (transform.StreamTransformer, error) {
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
func (h *MappingHandler) Stop() {
	h.cancel()
	if h.listener != nil {
		h.listener.Close()
	}
}

// =============================================================================
// main 函数
// =============================================================================

func main() {
	// 命令行参数
	configFile := flag.String("config", "client-config.yaml", "配置文件路径")
	flag.Parse()

	// 初始化日志
	utils.InitLogger(&utils.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "file",
		File:   "client.log",
	})

	// 加载配置
	config, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	utils.Infof("Client: loaded config, client_id=%d, server=%s", config.ClientID, config.Server.Address)

	// 创建客户端
	client := NewTunnoxClient(config)

	// 连接到服务器
	if err := client.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}

	utils.Infof("Client: connected successfully, waiting for config from server...")
	fmt.Println("Client started successfully.")
	fmt.Println("Waiting for mapping configuration from server...")
	fmt.Println("Press Ctrl+C to stop.")

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping client...")
	client.Stop()
	fmt.Println("Client stopped.")
}

// loadConfig 加载配置文件
func loadConfig(filename string) (*ClientConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ClientConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}
