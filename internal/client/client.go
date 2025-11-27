package client

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/client/mapping"
	"tunnox-core/internal/cloud/models"
	clientconfig "tunnox-core/internal/config"
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
	mappingHandlers map[string]MappingHandler
	mu              sync.RWMutex

	// 商业化控制：配额缓存
	cachedQuota      *models.UserQuota
	quotaCacheMu     sync.RWMutex
	quotaLastRefresh time.Time

	// 商业化控制：流量累计
	localTrafficStats map[string]*localMappingStats // mappingID -> stats
	trafficStatsMu    sync.RWMutex

	// 重连控制
	kicked     bool // 是否被踢下线
	authFailed bool // 是否认证失败
}

// localMappingStats 本地映射流量统计
type localMappingStats struct {
	bytesSent      int64
	bytesReceived  int64
	lastReportTime time.Time
	mu             sync.RWMutex
}

// NewClient 创建客户端
func NewClient(ctx context.Context, config *ClientConfig) *TunnoxClient {
	client := &TunnoxClient{
		ManagerBase:       dispose.NewManager("TunnoxClient", ctx),
		config:            config,
		mappingHandlers:   make(map[string]MappingHandler),
		localTrafficStats: make(map[string]*localMappingStats),
	}

	// 添加清理处理器
	client.AddCleanHandler(func() error {
		utils.Infof("Client: cleaning up client resources")

		// 关闭所有映射处理器
		client.mu.RLock()
		handlers := make([]MappingHandler, 0, len(client.mappingHandlers))
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

	protocol := c.config.Server.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	utils.Infof("Client: using %s transport for control connection", strings.ToUpper(protocol))

	// 1. 根据协议建立控制连接
	var (
		conn net.Conn
		err  error
	)
	switch strings.ToLower(protocol) {
	case "tcp":
		conn, err = net.DialTimeout("tcp", c.config.Server.Address, 10*time.Second)
	case "udp":
		conn, err = dialUDPControlConnection(c.config.Server.Address)
	case "websocket":
		conn, err = dialWebSocket(c.Ctx(), c.config.Server.Address, "/_tunnox")
	case "quic":
		conn, err = dialQUIC(c.Ctx(), c.config.Server.Address)
	default:
		return fmt.Errorf("unsupported server protocol: %s", protocol)
	}
	if err != nil {
		return fmt.Errorf("failed to dial server (%s): %w", protocol, err)
	}

	c.config.Server.Protocol = strings.ToLower(protocol)
	c.controlConn = conn

	// 2. 创建 Stream
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)

	// 记录连接信息用于调试
	localAddr := "unknown"
	remoteAddr := "unknown"
	if conn.LocalAddr() != nil {
		localAddr = conn.LocalAddr().String()
	}
	if conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	utils.Infof("Client: %s connection established - Local=%s, Remote=%s, controlStream=%p",
		strings.ToUpper(protocol), localAddr, remoteAddr, c.controlStream)

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

	// 6. 请求当前客户端的映射配置（默认依赖服务端推送，可按需启用）
	// go func() {
	// 	time.Sleep(200 * time.Millisecond) // 等待readLoop启动
	// 	c.requestMappingConfig()
	// }()

	return nil
}

// requestMappingConfig 请求当前客户端的映射配置
func (c *TunnoxClient) requestMappingConfig() {
	utils.Infof("Client: requestMappingConfig() started")

	// 稍微延迟，确保连接已稳定
	time.Sleep(100 * time.Millisecond)

	utils.Infof("Client: preparing ConfigGet request")

	// 构造请求
	cmd := &packet.CommandPacket{
		CommandType: packet.ConfigGet,
		CommandBody: "{}",
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	utils.Infof("Client: sending ConfigGet request via WritePacket")

	n, err := c.controlStream.WritePacket(pkt, false, 0)
	utils.Infof("Client: WritePacket returned: n=%d, err=%v", n, err)
	if err != nil {
		utils.Errorf("Client: failed to request mapping config: %v", err)
		return
	}

	utils.Infof("Client: ConfigGet request sent successfully, bytes=%d", n)
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

	utils.Debugf("Client: received response PacketType=%d, Payload len=%d", respPkt.PacketType, len(respPkt.Payload))
	if len(respPkt.Payload) > 0 {
		utils.Debugf("Client: Payload=%s", string(respPkt.Payload))
	}

	if respPkt.PacketType != packet.HandshakeResp {
		return fmt.Errorf("unexpected response type: %v", respPkt.PacketType)
	}

	var resp packet.HandshakeResponse
	if err := json.Unmarshal(respPkt.Payload, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal handshake response (payload='%s'): %w", string(respPkt.Payload), err)
	}

	if !resp.Success {
		// 认证失败，标记不重连
		if strings.Contains(resp.Error, "auth") || strings.Contains(resp.Error, "token") {
			c.authFailed = true
		}
		return fmt.Errorf("handshake failed: %s", resp.Error)
	}

	// 匿名模式下，服务器会返回分配的 ClientID
	if c.config.Anonymous && resp.Message != "" {
		var assignedClientID int64
		if _, err := fmt.Sscanf(resp.Message, "Anonymous client authenticated, client_id=%d", &assignedClientID); err == nil {
			c.config.ClientID = assignedClientID
		}
	}

	// 打印认证信息
	if c.config.Anonymous {
		utils.Infof("Client: authenticated as anonymous client, ClientID=%d, DeviceID=%s",
			c.config.ClientID, c.config.DeviceID)
	} else {
		utils.Infof("Client: authenticated successfully, ClientID=%d, Token=%s",
			c.config.ClientID, c.config.AuthToken)
	}

	return nil
}

// readLoop 读取循环（接收服务器命令）
func (c *TunnoxClient) readLoop() {
	utils.Infof("Client: readLoop started, controlStream=%p", c.controlStream)
	defer func() {
		utils.Infof("Client: readLoop exited, checking if should reconnect")
		// 读取循环退出，尝试重连
		if c.shouldReconnect() {
			go c.reconnect()
		}
	}()

	for {
		select {
		case <-c.Ctx().Done():
			utils.Infof("Client: readLoop stopped (context done)")
			return
		default:
		}

		utils.Debugf("Client: readLoop waiting for packet")
		pkt, _, err := c.controlStream.ReadPacket()
		if err != nil {
			if err != io.EOF {
				utils.Errorf("Client: failed to read packet: %v", err)
			} else {
				utils.Infof("Client: connection closed (EOF)")
			}
			return
		}

		utils.Infof("Client: received packet, type=%d", pkt.PacketType)

		// 处理不同类型的数据包
		switch pkt.PacketType & 0x3F {
		case packet.Heartbeat:
			// 心跳响应
			utils.Debugf("Client: heartbeat response received")
		case packet.JsonCommand:
			// 命令处理
			utils.Infof("Client: processing JsonCommand")
			c.handleCommand(pkt)
		default:
			utils.Warnf("Client: unknown packet type: %d", pkt.PacketType)
		}
	}
}

// handleKickCommand 处理踢下线命令
func (c *TunnoxClient) handleKickCommand(cmdBody string) {
	var kickInfo struct {
		Reason string `json:"reason"`
		Code   string `json:"code"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &kickInfo); err != nil {
		utils.Errorf("Client: failed to parse kick command: %v", err)
		kickInfo.Reason = "Unknown reason"
		kickInfo.Code = "UNKNOWN"
	}

	utils.Errorf("Client: KICKED BY SERVER - Reason: %s, Code: %s", kickInfo.Reason, kickInfo.Code)

	// 标记为被踢下线，禁止重连
	c.kicked = true

	// 停止客户端
	c.Stop()
}

// handleCommand 处理命令
func (c *TunnoxClient) handleCommand(pkt *packet.TransferPacket) {
	if pkt.CommandPacket == nil {
		utils.Warnf("Client: received command packet with nil CommandPacket")
		return
	}

	cmdType := pkt.CommandPacket.CommandType
	utils.Infof("Client: received command, type=%v", cmdType)

	switch cmdType {
	case packet.ConfigSet:
		// 接收服务器推送的配置
		c.handleConfigUpdate(pkt.CommandPacket.CommandBody)

	case packet.TunnelOpenRequestCmd:
		// 隧道打开请求（作为目标客户端）
		// 根据协议类型分发处理
		c.handleTunnelOpenRequest(pkt.CommandPacket.CommandBody)

	case packet.KickClient:
		// 踢下线命令
		c.handleKickCommand(pkt.CommandPacket.CommandBody)
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
	utils.Infof("Client: ✅ received ConfigSet from server, body length=%d", len(configBody))

	var configUpdate struct {
		Mappings []clientconfig.MappingConfig `json:"mappings"`
	}

	if err := json.Unmarshal([]byte(configBody), &configUpdate); err != nil {
		utils.Errorf("Client: failed to parse config update: %v", err)
		return
	}

	utils.Infof("Client: parsed %d mappings from ConfigSet", len(configUpdate.Mappings))

	// 构建新配置的映射ID集合
	newMappingIDs := make(map[string]bool)
	for i, mappingConfig := range configUpdate.Mappings {
		utils.Infof("Client: processing mapping[%d]: ID=%s, Protocol=%s, LocalPort=%d",
			i, mappingConfig.MappingID, mappingConfig.Protocol, mappingConfig.LocalPort)
		newMappingIDs[mappingConfig.MappingID] = true
		c.addOrUpdateMapping(mappingConfig)
	}

	// 删除不再存在的映射
	c.mu.Lock()
	for mappingID, handler := range c.mappingHandlers {
		if !newMappingIDs[mappingID] {
			utils.Infof("Client: removing mapping %s (no longer in config)", mappingID)
			handler.Stop()
			delete(c.mappingHandlers, mappingID)
		}
	}
	c.mu.Unlock()

	utils.Infof("Client: ✅ config updated successfully, total active mappings=%d", len(newMappingIDs))
}

// addOrUpdateMapping 添加或更新映射
func (c *TunnoxClient) addOrUpdateMapping(mappingCfg clientconfig.MappingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在，存在则先停止
	if handler, exists := c.mappingHandlers[mappingCfg.MappingID]; exists {
		utils.Infof("Client: updating mapping %s", mappingCfg.MappingID)
		handler.Stop()
		delete(c.mappingHandlers, mappingCfg.MappingID)
	}

	// ✅ 目标端配置（LocalPort==0）不需要启动监听
	if mappingCfg.LocalPort == 0 {
		utils.Debugf("Client: skipping mapping %s (target-side, no local listener needed)", mappingCfg.MappingID)
		return
	}

	// 根据协议类型创建适配器和处理器
	protocol := mappingCfg.Protocol
	if protocol == "" {
		protocol = "tcp" // 默认 TCP
	}

	// 创建协议适配器
	adapter, err := mapping.CreateAdapter(protocol, mappingCfg)
	if err != nil {
		utils.Errorf("Client: failed to create adapter: %v", err)
		return
	}

	// 创建映射处理器（使用BaseMappingHandler）
	handler := mapping.NewBaseMappingHandler(c, mappingCfg, adapter)

	if err := handler.Start(); err != nil {
		utils.Errorf("Client: ❌ failed to start %s mapping %s: %v", protocol, mappingCfg.MappingID, err)
		return
	}

	c.mappingHandlers[mappingCfg.MappingID] = handler
	utils.Infof("Client: ✅ %s mapping %s started successfully on port %d", protocol, mappingCfg.MappingID, mappingCfg.LocalPort)
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
	// 根据协议建立到服务器的连接
	var (
		conn net.Conn
		err  error
	)
	
	protocol := strings.ToLower(c.config.Server.Protocol)
	switch protocol {
	case "tcp", "":
		conn, err = net.DialTimeout("tcp", c.config.Server.Address, 10*time.Second)
	case "udp":
		conn, err = dialUDPControlConnection(c.config.Server.Address)
	case "websocket":
		conn, err = dialWebSocket(c.Ctx(), c.config.Server.Address, "/_tunnox")
	case "quic":
		conn, err = dialQUIC(c.Ctx(), c.config.Server.Address)
	default:
		return nil, nil, fmt.Errorf("unsupported server protocol: %s", protocol)
	}
	
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial server (%s): %w", protocol, err)
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
		EncryptionKey     string `json:"encryption_key"` // hex编码
		BandwidthLimit    int64  `json:"bandwidth_limit"`
	}

	if err := json.Unmarshal([]byte(cmdBody), &req); err != nil {
		utils.Errorf("Client: failed to parse tunnel open request: %v", err)
		return
	}

	// 创建Transform配置（只用于限速）
	transformConfig := &transform.TransformConfig{
		BandwidthLimit: req.BandwidthLimit,
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
	case "socks5":
		go c.handleSOCKS5TargetTunnel(req.TunnelID, req.MappingID, req.SecretKey, req.TargetHost, req.TargetPort, transformConfig)
	default:
		utils.Errorf("Client: unsupported protocol: %s", protocol)
	}
}

// handleTCPTargetTunnel 处理TCP目标端隧道
// transformConfig: Transform的限速配置
func (c *TunnoxClient) handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
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

// handleSOCKS5TargetTunnel 处理SOCKS5目标端隧道（与TCP流程一致）
func (c *TunnoxClient) handleSOCKS5TargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	utils.Infof("Client: handling SOCKS5 target tunnel, tunnel_id=%s, target=%s:%d", tunnelID, targetHost, targetPort)
	c.handleTCPTargetTunnel(tunnelID, mappingID, secretKey, targetHost, targetPort, transformConfig)
}

// handleUDPTargetTunnel 处理UDP目标端隧道
// transformConfig: Transform的限速配置
func (c *TunnoxClient) handleUDPTargetTunnel(tunnelID, mappingID, secretKey, targetHost string, targetPort int,
	transformConfig *transform.TransformConfig) {
	utils.Infof("Client: handling UDP target tunnel, tunnel_id=%s, target=%s:%d", tunnelID, targetHost, targetPort)

	// 1. 解析目标 UDP 地址
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		utils.Errorf("Client: failed to resolve UDP address %s: %v", targetAddr, err)
		return
	}

	// 2. 创建 UDP 连接到目标
	targetConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		utils.Errorf("Client: failed to connect to UDP target %s: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	utils.Infof("Client: connected to UDP target %s for tunnel %s", targetAddr, tunnelID)

	// 3. 建立隧道连接
	tunnelConn, tunnelStream, err := c.dialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		utils.Errorf("Client: failed to dial tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	utils.Infof("Client: UDP tunnel %s established successfully", tunnelID)

	// 4. 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 5. 启动 UDP 双向转发
	c.bidirectionalCopyUDPTarget(tunnelConn, targetConn, tunnelID, transformConfig)
}

// bidirectionalCopyUDPTarget UDP 双向转发（作为目标端）
func (c *TunnoxClient) bidirectionalCopyUDPTarget(tunnelConn net.Conn, targetConn *net.UDPConn, tunnelID string, transformConfig *transform.TransformConfig) {
	// 创建转换器
	transformer, err := transform.NewTransformer(transformConfig)
	if err != nil {
		utils.Errorf("Client: failed to create transformer: %v", err)
		return
	}

	// 包装读写器
	reader := io.Reader(tunnelConn)
	writer := io.Writer(tunnelConn)
	if transformer != nil {
		reader, _ = transformer.WrapReader(reader)
		writer, _ = transformer.WrapWriter(writer)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// 从隧道读取数据并发送到目标 UDP
	go func() {
		defer wg.Done()
		for {
			data, err := readLengthPrefixedPacket(reader)
			if err != nil {
				if err != io.EOF {
					utils.Errorf("UDPTarget[%s]: failed to read length from tunnel: %v", tunnelID, err)
				}
				return
			}

			// 发送到目标 UDP
			_, err = targetConn.Write(data)
			if err != nil {
				utils.Errorf("UDPTarget[%s]: failed to write to target: %v", tunnelID, err)
				return
			}
		}
	}()

	// 从目标 UDP 读取数据并发送到隧道
	go func() {
		defer wg.Done()
		buf := make([]byte, 65535)
		targetConn.SetReadDeadline(time.Now().Add(60 * time.Second))

		for {
			n, err := targetConn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					utils.Debugf("UDPTarget[%s]: read timeout, closing tunnel", tunnelID)
				} else {
					utils.Errorf("UDPTarget[%s]: failed to read from target: %v", tunnelID, err)
				}
				return
			}

			if n > 0 {
				// 重置超时
				targetConn.SetReadDeadline(time.Now().Add(60 * time.Second))

				if err := writeLengthPrefixedPacket(writer, buf[:n]); err != nil {
					utils.Errorf("UDPTarget[%s]: failed to write data to tunnel: %v", tunnelID, err)
					return
				}
			}
		}
	}()

	wg.Wait()
	utils.Infof("UDPTarget[%s]: tunnel closed", tunnelID)
}

const maxUDPPacketSize = 65535

func readLengthPrefixedPacket(reader io.Reader) ([]byte, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return nil, err
	}

	dataLen := binary.BigEndian.Uint32(lenBuf)
	if dataLen == 0 || dataLen > maxUDPPacketSize {
		return nil, fmt.Errorf("invalid data length: %d", dataLen)
	}

	data := make([]byte, dataLen)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	return data, nil
}

func writeLengthPrefixedPacket(writer io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > maxUDPPacketSize {
		return fmt.Errorf("invalid data length: %d", len(data))
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	if _, err := writer.Write(lenBuf); err != nil {
		return err
	}

	_, err := writer.Write(data)
	return err
}

// ============ ClientInterface 实现（商业化控制） ============

// CheckMappingQuota 检查映射配额
// 这个方法由BaseMappingHandler调用，用于在建立新连接前检查配额
func (c *TunnoxClient) CheckMappingQuota(mappingID string) error {
	// 获取用户配额
	_, err := c.GetUserQuota()
	if err != nil {
		// 获取配额失败，记录日志但不阻塞连接
		utils.Warnf("Client: failed to get quota for mapping %s: %v", mappingID, err)
		return nil
	}

	// 检查带宽限制（已在MappingConfig中单独配置，这里不重复检查）
	// 检查存储限制（如果需要）
	// 注意：连接数限制已在BaseMappingHandler.checkConnectionQuota中检查

	// 未来可以在这里添加更多业务限制检查
	// 例如：月流量限制、特定时段限制等

	utils.Debugf("Client: quota check passed for mapping %s", mappingID)
	return nil
}

// TrackTraffic 上报流量统计
// 这个方法由BaseMappingHandler定期调用（每30秒）
func (c *TunnoxClient) TrackTraffic(mappingID string, bytesSent, bytesReceived int64) error {
	if bytesSent == 0 && bytesReceived == 0 {
		return nil // 无流量，不处理
	}

	// 1. 本地累计（用于月流量检查和统计）
	c.trafficStatsMu.Lock()
	stats, exists := c.localTrafficStats[mappingID]
	if !exists {
		stats = &localMappingStats{
			lastReportTime: time.Now(),
		}
		c.localTrafficStats[mappingID] = stats
	}
	stats.mu.Lock()
	stats.bytesSent += bytesSent
	stats.bytesReceived += bytesReceived
	stats.lastReportTime = time.Now()
	totalSent := stats.bytesSent
	totalReceived := stats.bytesReceived
	stats.mu.Unlock()
	c.trafficStatsMu.Unlock()

	// 2. 记录日志
	utils.Debugf("Client: traffic stats for %s - period(sent=%d, recv=%d), total(sent=%d, recv=%d)",
		mappingID, bytesSent, bytesReceived, totalSent, totalReceived)

	// 3. 预留：可在此处将统计数据上报服务器
	// 可以通过控制连接发送JsonCommand类型的统计报告
	// 或者通过专门的统计上报接口

	return nil
}

// GetUserQuota 获取用户配额信息
// 这个方法由BaseMappingHandler调用，用于获取当前用户的配额限制
// 使用缓存机制，每5分钟刷新一次
func (c *TunnoxClient) GetUserQuota() (*models.UserQuota, error) {
	const quotaCacheDuration = 5 * time.Minute

	// 检查缓存是否有效
	c.quotaCacheMu.RLock()
	if c.cachedQuota != nil && time.Since(c.quotaLastRefresh) < quotaCacheDuration {
		quota := c.cachedQuota
		c.quotaCacheMu.RUnlock()
		return quota, nil
	}
	c.quotaCacheMu.RUnlock()

	// 缓存失效，需要刷新
	// 预留：未来可通过 JsonCommand 发送 QuotaQuery 请求，从服务器获取配额信息

	// 暂时使用默认配额
	defaultQuota := &models.UserQuota{
		MaxClientIDs:   10,
		MaxConnections: 100,
		BandwidthLimit: 0, // 0表示无限制
		StorageLimit:   0,
	}

	// 更新缓存
	c.quotaCacheMu.Lock()
	c.cachedQuota = defaultQuota
	c.quotaLastRefresh = time.Now()
	c.quotaCacheMu.Unlock()

	utils.Debugf("Client: quota refreshed - MaxConnections=%d, BandwidthLimit=%d",
		defaultQuota.MaxConnections, defaultQuota.BandwidthLimit)

	return defaultQuota, nil
}

// GetLocalTrafficStats 获取本地流量统计
// 用于调试和监控
func (c *TunnoxClient) GetLocalTrafficStats(mappingID string) (sent, received int64) {
	c.trafficStatsMu.RLock()
	defer c.trafficStatsMu.RUnlock()

	if stats, exists := c.localTrafficStats[mappingID]; exists {
		stats.mu.RLock()
		defer stats.mu.RUnlock()
		return stats.bytesSent, stats.bytesReceived
	}

	return 0, 0
}
