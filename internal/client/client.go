package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"
)

// TunnoxClient 隧道客户端
type TunnoxClient struct {
	*dispose.ManagerBase

	config *ClientConfig

	// 指令连接
	controlConn   net.Conn
	controlStream stream.PackageStreamer

	// 映射管理
	mappingHandlers map[string]MappingHandlerInterface
	mu              sync.RWMutex
}

// NewClient 创建客户端
func NewClient(ctx context.Context, config *ClientConfig) *TunnoxClient {
	client := &TunnoxClient{
		ManagerBase:     dispose.NewManager("TunnoxClient", ctx),
		config:          config,
		mappingHandlers: make(map[string]MappingHandlerInterface),
	}

	// 添加清理处理器
	client.AddCleanHandler(func() error {
		utils.Infof("Client: cleaning up client resources")
		
		// 关闭所有映射处理器
		client.mu.RLock()
		handlers := make([]MappingHandlerInterface, 0, len(client.mappingHandlers))
		for _, handler := range client.mappingHandlers {
			handlers = append(handlers, handler)
		}
		client.mu.RUnlock()

		for _, handler := range handlers {
			handler.Stop()
		}

		// 关闭控制连接
		if client.controlConn != nil {
			client.controlConn.Close()
		}

		return nil
	})

	return client
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
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
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
		req = &packet.HandshakeRequest{
			ClientID: 0,
			Token:    fmt.Sprintf("anonymous:%s", c.config.DeviceID),
			Version:  "2.0",
			Protocol: c.config.Server.Protocol,
		}
	} else {
		req = &packet.HandshakeRequest{
			ClientID: c.config.ClientID,
			Token:    c.config.AuthToken,
			Version:  "2.0",
			Protocol: c.config.Server.Protocol,
		}
	}

	reqData, _ := json.Marshal(req)
	handshakePkt := &packet.TransferPacket{
		PacketType: packet.Handshake,
		Payload:    reqData,
	}

	if _, err := c.controlStream.WritePacket(handshakePkt, false, 0); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

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

	// 匿名模式下，服务器会返回分配的 ClientID
	if c.config.Anonymous && resp.Message != "" {
		var assignedClientID int64
		if _, err := fmt.Sscanf(resp.Message, "Anonymous client authenticated, client_id=%d", &assignedClientID); err == nil {
			c.config.ClientID = assignedClientID
			utils.Infof("Client: assigned client_id=%d", assignedClientID)
		}
	}

	return nil
}

// readLoop 读取循环（接收服务器命令）
func (c *TunnoxClient) readLoop() {
	for {
		select {
		case <-c.Ctx().Done():
			return
		default:
		}

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
			utils.Debugf("Client: heartbeat response received")
		case packet.JsonCommand:
			// 命令处理
			c.handleCommand(pkt)
		}
	}
}

// handleCommand 处理命令
func (c *TunnoxClient) handleCommand(pkt *packet.TransferPacket) {
	if pkt.CommandPacket == nil {
		return
	}

	cmdType := pkt.CommandPacket.CommandType
	utils.Infof("Client: received command, type=%v", cmdType)

	switch cmdType {
	case packet.ConfigGet:
		// 配置查询响应
		c.handleConfigUpdate(pkt.CommandPacket.CommandBody)

	case packet.TunnelOpenRequestCmd:
		// 隧道打开请求（作为目标客户端）
		// 根据协议类型分发处理
		c.handleTunnelOpenRequest(pkt.CommandPacket.CommandBody)
	}
}

// heartbeatLoop 心跳循环
func (c *TunnoxClient) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				utils.Errorf("Client: failed to send heartbeat: %v", err)
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
	_, err := c.controlStream.WritePacket(heartbeatPkt, false, 0)
	return err
}

// handleConfigUpdate 处理配置更新
func (c *TunnoxClient) handleConfigUpdate(configBody string) {
	var configUpdate struct {
		Mappings []MappingConfig `json:"mappings"`
	}

	if err := json.Unmarshal([]byte(configBody), &configUpdate); err != nil {
		utils.Errorf("Client: failed to parse config update: %v", err)
		return
	}

	for _, mappingConfig := range configUpdate.Mappings {
		c.addOrUpdateMapping(mappingConfig)
	}

	utils.Infof("Client: config updated, total mappings=%d", len(c.mappingHandlers))
}

// addOrUpdateMapping 添加或更新映射
func (c *TunnoxClient) addOrUpdateMapping(config MappingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在，存在则先停止
	if handler, exists := c.mappingHandlers[config.MappingID]; exists {
		utils.Infof("Client: updating mapping %s", config.MappingID)
		handler.Stop()
		delete(c.mappingHandlers, config.MappingID)
	}

	// 根据协议类型创建不同的映射处理器
	protocol := config.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	var handler MappingHandlerInterface
	var err error

	switch protocol {
	case "tcp":
		handler = NewTcpMappingHandler(c, config)
	case "udp":
		handler = NewUdpMappingHandler(c, config)
	case "socks5":
		handler = NewSocks5MappingHandler(c, config)
	default:
		utils.Errorf("Client: unsupported protocol: %s", protocol)
		return
	}

	if err = handler.Start(); err != nil {
		utils.Errorf("Client: failed to start %s mapping %s: %v", protocol, config.MappingID, err)
		return
	}

	c.mappingHandlers[config.MappingID] = handler
	utils.Infof("Client: %s mapping %s started on port %d", protocol, config.MappingID, config.LocalPort)
}

// RemoveMapping 移除映射
func (c *TunnoxClient) RemoveMapping(mappingID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if handler, exists := c.mappingHandlers[mappingID]; exists {
		handler.Stop()
		delete(c.mappingHandlers, mappingID)
		utils.Infof("Client: mapping %s stopped", mappingID)
	}
}

// Stop 停止客户端
func (c *TunnoxClient) Stop() {
	utils.Infof("Client: stopping...")
	c.Close()
}

// GetContext 获取上下文（供映射处理器使用）
func (c *TunnoxClient) GetContext() context.Context {
	return c.Ctx()
}

// GetConfig 获取配置（供映射处理器使用）
func (c *TunnoxClient) GetConfig() *ClientConfig {
	return c.config
}

// dialTunnel 建立隧道连接（通用方法）
func (c *TunnoxClient) dialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error) {
	// 建立到服务器的连接
	conn, err := net.DialTimeout("tcp", c.config.Server.Address, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial server: %w", err)
	}

	// 创建 StreamProcessor
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	tunnelStream := streamFactory.CreateStreamProcessor(conn, conn)

	// 发送 TunnelOpen
	req := &packet.TunnelOpenRequest{
		MappingID: mappingID,
		TunnelID:  tunnelID,
		SecretKey: secretKey,
	}

	reqData, _ := json.Marshal(req)
	openPkt := &packet.TransferPacket{
		PacketType: packet.TunnelOpen,
		TunnelID:   tunnelID,
		Payload:    reqData,
	}

	if _, err := tunnelStream.WritePacket(openPkt, false, 0); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("failed to send tunnel open: %w", err)
	}

	// 等待 TunnelOpenAck
	ackPkt, _, err := tunnelStream.ReadPacket()
	if err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("failed to read tunnel open ack: %w", err)
	}

	if ackPkt.PacketType != packet.TunnelOpenAck {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("unexpected packet type: %v", ackPkt.PacketType)
	}

	var ack packet.TunnelOpenAckResponse
	if err := json.Unmarshal(ackPkt.Payload, &ack); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("failed to unmarshal ack: %w", err)
	}

	if !ack.Success {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("tunnel open failed: %s", ack.Error)
	}

	return conn, tunnelStream, nil
}

// DialTunnel 建立隧道连接（供映射处理器使用）
func (c *TunnoxClient) DialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error) {
	return c.dialTunnel(tunnelID, mappingID, secretKey)
}

// handleTunnelOpenRequest 处理隧道打开请求（作为目标客户端）
func (c *TunnoxClient) handleTunnelOpenRequest(cmdBody string) {
	var req struct {
		TunnelID   string `json:"tunnel_id"`
		MappingID  string `json:"mapping_id"`
		SecretKey  string `json:"secret_key"`
		TargetHost string `json:"target_host"`
		TargetPort int    `json:"target_port"`
		Protocol   string `json:"protocol"` // tcp/udp

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

	transformConfig := &transform.TransformConfig{
		EnableCompression: req.EnableCompression,
		CompressionLevel:  req.CompressionLevel,
		EnableEncryption:  req.EnableEncryption,
		EncryptionMethod:  req.EncryptionMethod,
		EncryptionKey:     req.EncryptionKey,
	}

	// 根据协议类型分发
	protocol := req.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	switch protocol {
	case "tcp":
		go c.handleTCPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	case "udp":
		go c.handleUDPTargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	default:
		utils.Errorf("Client: unsupported protocol: %s", protocol)
	}
}

// handleTCPTargetTunnel 处理TCP目标端隧道
func (c *TunnoxClient) handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int, transformConfig *transform.TransformConfig) {
	// 1. 连接到目标服务
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		utils.Errorf("Client: failed to connect to target %s: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	// 2. 建立隧道连接
	tunnelConn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		utils.Errorf("Client: failed to dial tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	utils.Infof("Client: TCP tunnel %s established for target %s", tunnelID, targetAddr)

	// 3. 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 4. 创建转换器并启动双向转发
	transformer, _ := transform.NewTransformer(transformConfig)
	utils.BidirectionalCopy(targetConn, tunnelConn, &utils.BidirectionalCopyOptions{
		Transformer: transformer,
		LogPrefix:   fmt.Sprintf("Client[TCP-target][%s]", tunnelID),
	})
}

// handleUDPTargetTunnel 处理UDP目标端隧道
func (c *TunnoxClient) handleUDPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int, transformConfig *transform.TransformConfig) {
	// UDP 目标处理由 udp_target.go 实现
	HandleUDPTarget(c, tunnelID, mappingID, secretKey, targetHost, targetPort, transformConfig)
}

