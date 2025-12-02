package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"tunnox-core/internal/packet"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// Connect 连接到服务器并建立指令连接
func (c *TunnoxClient) Connect() error {
	// 如果配置中没有地址，使用自动连接
	if c.config.Server.Address == "" {
		return c.connectWithAutoDetection()
	}

	utils.Infof("Client: connecting to server %s", c.config.Server.Address)

	protocol := c.config.Server.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	utils.Infof("Client: using %s transport for control connection", strings.ToUpper(protocol))

	// 1. 根据协议建立控制连接
	var (
		conn  net.Conn
		err   error
		token string // HTTP 长轮询使用的 token
	)
	switch strings.ToLower(protocol) {
	case "tcp":
		conn, err = net.DialTimeout("tcp", c.config.Server.Address, 10*time.Second)
	case "udp":
		conn, err = dialUDPControlConnection(c.config.Server.Address)
	case "websocket":
		conn, err = dialWebSocket(c.Ctx(), c.config.Server.Address)
	case "quic":
		conn, err = dialQUIC(c.Ctx(), c.config.Server.Address)
	case "httppoll", "http-long-polling", "httplp":
		// HTTP 长轮询使用 AuthToken 或 SecretKey
		token = c.config.AuthToken
		if token == "" && c.config.Anonymous {
			token = c.config.SecretKey
		}
		// 首次握手时，对于匿名客户端，必须使用 clientID=0
		// 对于已注册客户端，使用配置的 clientID
		clientID := c.config.ClientID
		if c.config.Anonymous {
			// 匿名客户端首次握手，强制使用 0（不管配置文件中是否有保存的 ClientID）
			clientID = 0
		}
		conn, err = dialHTTPLongPolling(c.Ctx(), c.config.Server.Address, clientID, token, c.GetInstanceID(), "")
	default:
		return fmt.Errorf("unsupported server protocol: %s", protocol)
	}
	if err != nil {
		return fmt.Errorf("failed to dial server (%s): %w", protocol, err)
	}

	c.config.Server.Protocol = strings.ToLower(protocol)

	// 使用锁保护连接状态
	c.mu.Lock()
	c.controlConn = conn
	// 2. 创建 Stream
	// HTTP 长轮询协议直接使用 HTTPStreamProcessor，不需要通过 CreateStreamProcessor
	if protocol == "httppoll" || protocol == "http-long-polling" || protocol == "httplp" {
		// 对于 HTTP 长轮询，conn 是 HTTPLongPollingConn，需要转换为 HTTPStreamProcessor
		if httppollConn, ok := conn.(*HTTPLongPollingConn); ok {
			// 创建 HTTPStreamProcessor
			baseURL := httppollConn.baseURL
			pushURL := baseURL + "/tunnox/v1/push"
			pollURL := baseURL + "/tunnox/v1/poll"
			c.controlStream = httppoll.NewStreamProcessor(c.Ctx(), baseURL, pushURL, pollURL, c.config.ClientID, token, c.GetInstanceID(), "")
			// ✅ 重要：设置客户端生成的临时 ConnectionID（用于初始握手）
			// 服务端会在握手响应中分配正式的 ConnectionID，然后会更新这个值
			if httppollConn.connectionID != "" {
				c.controlStream.(*httppoll.StreamProcessor).SetConnectionID(httppollConn.connectionID)
				utils.Debugf("Client: set initial ConnectionID from HTTPLongPollingConn: %s", httppollConn.connectionID)
			} else {
				utils.Warnf("Client: HTTPLongPollingConn has empty connectionID")
			}
		} else {
			// 回退到默认方式
			streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
			c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
		}
	} else {
		streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
		c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
	}
	c.mu.Unlock()

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
		// 握手失败，清理连接资源
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// 4. 启动读取循环（接收服务器命令）
	// ✅ 防止重复启动 readLoop
	if !c.readLoopRunning.CompareAndSwap(false, true) {
		utils.Warnf("Client: readLoop already running, skipping")
	} else {
		go func() {
			defer c.readLoopRunning.Store(false)
			c.readLoop()
		}()
	}

	// 5. 启动心跳循环
	// ✅ 防止重复启动 heartbeatLoop
	if !c.heartbeatLoopRunning.CompareAndSwap(false, true) {
		utils.Debugf("Client: heartbeatLoop already running, skipping")
	} else {
		go func() {
			defer c.heartbeatLoopRunning.Store(false)
			c.heartbeatLoop()
		}()
	}

	utils.Infof("Client: control connection established successfully")

	return nil
}

// Disconnect 断开与服务器的连接
func (c *TunnoxClient) Disconnect() error {
	utils.Infof("Client: disconnecting from server")

	// 使用锁保护连接状态
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭控制流和连接
	if c.controlStream != nil {
		c.controlStream.Close()
		c.controlStream = nil
	}

	if c.controlConn != nil {
		c.controlConn.Close()
		c.controlConn = nil
	}

	utils.Infof("Client: disconnected successfully")
	return nil
}

// IsConnected 检查是否连接到服务器
func (c *TunnoxClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.controlConn != nil && c.controlStream != nil
}

// Reconnect 重新连接到服务器
func (c *TunnoxClient) Reconnect() error {
	// ✅ 防止重复重连：如果已有重连在进行，直接返回
	if !c.reconnecting.CompareAndSwap(false, true) {
		utils.Debugf("Client: reconnect already in progress, skipping Reconnect() call")
		return nil
	}
	defer c.reconnecting.Store(false)

	utils.Infof("Client: attempting to reconnect...")

	// 先断开旧连接
	c.Disconnect()

	// 建立新连接
	return c.Connect()
}

// sendHandshake 发送握手请求（使用控制连接）
func (c *TunnoxClient) sendHandshake() error {
	return c.sendHandshakeOnStream(c.controlStream, "control")
}

// saveAnonymousCredentials 保存匿名客户端凭据到配置文件
func (c *TunnoxClient) saveAnonymousCredentials() error {
	if !c.config.Anonymous || c.config.ClientID == 0 {
		return nil // 非匿名客户端或无ClientID，无需保存
	}

	// 使用ConfigManager保存配置
	configMgr := NewConfigManager()
	if err := configMgr.SaveConfig(c.config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	utils.Infof("Client: anonymous credentials saved to config file")
	return nil
}

// sendHandshakeOnStream 在指定的stream上发送握手请求（用于隧道连接）
func (c *TunnoxClient) sendHandshakeOnStream(stream stream.PackageStreamer, connectionType string) error {
	var req *packet.HandshakeRequest

	// 认证策略：
	// 1. 匿名客户端 + 有ClientID和SecretKey → 使用ClientID+SecretKey认证
	// 2. 匿名客户端 + 无ClientID → 首次握手，服务端分配凭据
	// 3. 注册客户端 → 使用ClientID+AuthToken认证
	if c.config.Anonymous {
		if c.config.ClientID > 0 && c.config.SecretKey != "" {
			// 匿名客户端使用持久化凭据重新认证
			req = &packet.HandshakeRequest{
				ClientID:       c.config.ClientID,
				Token:          c.config.SecretKey, // ✅ 使用SecretKey而不是DeviceID
				Version:        "2.0",
				Protocol:       c.config.Server.Protocol,
				ConnectionType: connectionType, // ✅ 标识连接类型
			}
		} else {
			// 首次匿名握手，请求分配凭据
			req = &packet.HandshakeRequest{
				ClientID:       0,
				Token:          fmt.Sprintf("anonymous:%s", c.config.DeviceID),
				Version:        "2.0",
				Protocol:       c.config.Server.Protocol,
				ConnectionType: connectionType, // ✅ 标识连接类型
			}
		}
	} else {
		// 注册客户端使用AuthToken
		req = &packet.HandshakeRequest{
			ClientID:       c.config.ClientID,
			Token:          c.config.AuthToken,
			Version:        "2.0",
			Protocol:       c.config.Server.Protocol,
			ConnectionType: connectionType, // ✅ 标识连接类型
		}
	}

	reqData, _ := json.Marshal(req)
	handshakePkt := &packet.TransferPacket{
		PacketType: packet.Handshake,
		Payload:    reqData,
	}

	if _, err := stream.WritePacket(handshakePkt, true, 0); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// 等待握手响应（忽略心跳包）
	var respPkt *packet.TransferPacket
	for {
		pkt, _, err := stream.ReadPacket()
		if err != nil {
			return fmt.Errorf("failed to read handshake response: %w", err)
		}
		if pkt == nil {
			// ReadPacket 返回 nil 但没有错误，可能是超时或空响应，继续等待
			utils.Debugf("Client: ReadPacket returned nil packet, continuing to wait for handshake response")
			continue
		}

		// 忽略压缩/加密标志，只检查基础类型
		baseType := pkt.PacketType & 0x3F
		if baseType == packet.Heartbeat {
			// 收到心跳包，继续等待握手响应
			utils.Debugf("Client: received heartbeat during handshake, ignoring")
			continue
		}

		if baseType == packet.HandshakeResp {
			respPkt = pkt
			break
		}

		// 收到其他类型的包，返回错误
		return fmt.Errorf("unexpected response type: %v (expected HandshakeResp)", pkt.PacketType)
	}

	utils.Debugf("Client: received response PacketType=%d, Payload len=%d", respPkt.PacketType, len(respPkt.Payload))
	if len(respPkt.Payload) > 0 {
		utils.Debugf("Client: Payload=%s", string(respPkt.Payload))
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

	// ✅ 更新 ConnectionID（如果服务端返回了 ConnectionID）
	if resp.ConnectionID != "" {
		// 对于 HTTP 长轮询，更新 HTTPStreamProcessor 的 ConnectionID
		if httppollStream, ok := stream.(*httppoll.StreamProcessor); ok {
			httppollStream.SetConnectionID(resp.ConnectionID)
			utils.Infof("Client: received ConnectionID from server: %s", resp.ConnectionID)
		}
	}

	// 匿名模式下，服务器会返回分配的凭据（仅对控制连接有用）
	if c.config.Anonymous && stream == c.controlStream {
		// ✅ 优先使用结构化字段
		if resp.ClientID > 0 && resp.SecretKey != "" {
			c.config.ClientID = resp.ClientID
			c.config.SecretKey = resp.SecretKey
			utils.Infof("Client: received anonymous credentials - ClientID=%d, SecretKey=***", resp.ClientID)

			// 保存凭据到配置文件（供下次启动使用）
			if err := c.saveAnonymousCredentials(); err != nil {
				utils.Warnf("Client: failed to save anonymous credentials: %v", err)
			}

			// ✅ 更新 HTTPStreamProcessor 的 clientID
			if httppollStream, ok := stream.(*httppoll.StreamProcessor); ok {
				httppollStream.UpdateClientID(resp.ClientID)
				utils.Debugf("Client: updated HTTPStreamProcessor clientID to %d", resp.ClientID)
			}
		} else if resp.Message != "" {
			// 兼容旧版本：从Message解析ClientID
			var assignedClientID int64
			if _, err := fmt.Sscanf(resp.Message, "Anonymous client authenticated, client_id=%d", &assignedClientID); err == nil {
				c.config.ClientID = assignedClientID
				// ✅ 更新 HTTPStreamProcessor 的 clientID
				if httppollStream, ok := stream.(*httppoll.StreamProcessor); ok {
					httppollStream.UpdateClientID(assignedClientID)
					utils.Debugf("Client: updated HTTPStreamProcessor clientID to %d", assignedClientID)
				}
			}
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

	// ✅ 握手成功后，请求映射配置（仅对控制连接）
	// 这样客户端重启后能自动恢复映射列表
	// 延迟一小段时间，确保连接完全稳定后再发送请求
	if stream == c.controlStream && c.config.ClientID > 0 {
		go func() {
			// 等待 500ms，确保连接稳定
			time.Sleep(500 * time.Millisecond)
			c.requestMappingConfig()
		}()
	}

	return nil
}

// readLoop 读取循环（接收服务器命令）
func (c *TunnoxClient) readLoop() {
	defer func() {
		if c.shouldReconnect() {
			go c.reconnect()
		}
	}()

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
			c.mu.Lock()
			if c.controlStream != nil {
				c.controlStream.Close()
				c.controlStream = nil
			}
			if c.controlConn != nil {
				c.controlConn.Close()
				c.controlConn = nil
			}
			c.mu.Unlock()
			return
		}

		switch pkt.PacketType & 0x3F {
		case packet.Heartbeat:
		case packet.CommandResp:
			if c.commandResponseManager != nil && c.commandResponseManager.HandleResponse(pkt) {
				continue
			}
		case packet.JsonCommand:
			c.handleCommand(pkt)
		case packet.TunnelOpen:
			// ✅ TunnelOpen 应该由隧道连接处理，控制连接忽略它
			utils.Debugf("Client: ignoring TunnelOpen in control connection read loop")
		case packet.TunnelOpenAck:
			// ✅ TunnelOpenAck 应该由隧道连接处理，控制连接忽略它
			utils.Debugf("Client: ignoring TunnelOpenAck in control connection read loop")
		default:
			utils.Warnf("Client: unknown packet type: %d", pkt.PacketType)
		}
	}
}

// heartbeatLoop 心跳循环
func (c *TunnoxClient) heartbeatLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx().Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				utils.Errorf("Client: failed to send heartbeat: %v", err)
				return
			}
		}
	}
}

// sendHeartbeat 发送心跳包
func (c *TunnoxClient) sendHeartbeat() error {
	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return fmt.Errorf("control stream is nil")
	}

	heartbeatPkt := &packet.TransferPacket{
		PacketType: packet.Heartbeat,
		Payload:    []byte{},
	}
	_, err := controlStream.WritePacket(heartbeatPkt, true, 0)
	if err != nil {
		// 心跳失败，可能连接已断开，清理连接状态
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()
	}
	return err
}

// requestMappingConfig 请求当前客户端的映射配置
func (c *TunnoxClient) requestMappingConfig() {
	if !c.configRequesting.CompareAndSwap(false, true) {
		return
	}
	defer c.configRequesting.Store(false)

	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return
	}

	commandID, err := utils.GenerateRandomString(16)
	if err != nil {
		utils.Errorf("Client: failed to generate command ID: %v", err)
		return
	}

	responseChan := c.commandResponseManager.RegisterRequest(commandID)
	defer c.commandResponseManager.UnregisterRequest(commandID)

	cmd := &packet.CommandPacket{
		CommandType: packet.ConfigGet,
		CommandBody: "{}",
		CommandId:   commandID,
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	// 重试发送请求（最多3次，每次间隔1秒）
	var writeErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(time.Second)
			utils.Debugf("Client: retrying mapping config request (attempt %d/3)", retry+1)
		}
		_, writeErr = controlStream.WritePacket(pkt, true, 0)
		if writeErr == nil {
			break
		}
		utils.Warnf("Client: failed to request mapping config (attempt %d/3): %v", retry+1, writeErr)
	}

	if writeErr != nil {
		utils.Errorf("Client: failed to request mapping config after 3 attempts: %v", writeErr)
		return
	}

	select {
	case resp := <-responseChan:
		if !resp.Success {
			utils.Errorf("Client: ConfigGet failed: %s", resp.Error)
			return
		}
		c.handleConfigUpdate(resp.Data)
	case <-time.After(30 * time.Second):
		utils.Errorf("Client: ConfigGet request timeout after 30s")
	case <-c.Ctx().Done():
	}
}

// connectWithAutoDetection 使用自动连接检测连接到服务器
func (c *TunnoxClient) connectWithAutoDetection() error {
	connector := NewAutoConnector(c.Ctx(), c)
	defer connector.Close()

	endpoint, err := connector.ConnectWithAutoDetection(c.Ctx())
	if err != nil {
		return fmt.Errorf("auto connection failed: %w", err)
	}

	// 更新配置
	c.config.Server.Protocol = endpoint.Protocol
	c.config.Server.Address = endpoint.Address

	utils.Infof("Client: auto-detected server endpoint - %s://%s", endpoint.Protocol, endpoint.Address)

	// 使用选中的端点建立控制连接
	return c.connectWithEndpoint(endpoint.Protocol, endpoint.Address)
}

// connectWithEndpoint 使用指定的协议和地址建立控制连接
func (c *TunnoxClient) connectWithEndpoint(protocol, address string) error {
	utils.Infof("Client: connecting to server %s://%s", protocol, address)

	var (
		conn net.Conn
		err  error
	)
	switch strings.ToLower(protocol) {
	case "tcp":
		conn, err = net.DialTimeout("tcp", address, 10*time.Second)
		if err == nil {
			// 使用接口而不是具体类型
			SetKeepAliveIfSupported(conn, true)
		}
	case "udp":
		conn, err = dialUDPControlConnection(address)
	case "websocket":
		conn, err = dialWebSocket(c.Ctx(), address)
	case "quic":
		conn, err = dialQUIC(c.Ctx(), address)
	case "httppoll", "http-long-polling", "httplp":
		// HTTP 长轮询使用 AuthToken 或 SecretKey
		token := c.config.AuthToken
		if token == "" && c.config.Anonymous {
			token = c.config.SecretKey
		}
		// 首次握手时，对于匿名客户端，必须使用 clientID=0
		clientID := c.config.ClientID
		if c.config.Anonymous {
			clientID = 0
		}
		conn, err = dialHTTPLongPolling(c.Ctx(), address, clientID, token, c.GetInstanceID(), "")
	default:
		return fmt.Errorf("unsupported server protocol: %s", protocol)
	}
	if err != nil {
		return fmt.Errorf("failed to dial server (%s): %w", protocol, err)
	}

	c.config.Server.Protocol = strings.ToLower(protocol)

	// 使用锁保护连接状态
	c.mu.Lock()
	c.controlConn = conn
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
	c.mu.Unlock()

	// 记录连接信息
	localAddr := "unknown"
	remoteAddr := "unknown"
	if conn.LocalAddr() != nil {
		localAddr = conn.LocalAddr().String()
	}
	if conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	utils.Infof("Client: %s connection established - Local=%s, Remote=%s",
		strings.ToUpper(protocol), localAddr, remoteAddr)

	// 发送握手请求
	if err := c.sendHandshake(); err != nil {
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// 启动读取循环
	if !c.readLoopRunning.CompareAndSwap(false, true) {
		utils.Warnf("Client: readLoop already running, skipping")
	} else {
		go func() {
			defer c.readLoopRunning.Store(false)
			c.readLoop()
		}()
	}

	// 启动心跳循环
	if !c.heartbeatLoopRunning.CompareAndSwap(false, true) {
		utils.Debugf("Client: heartbeatLoop already running, skipping")
	} else {
		go func() {
			defer c.heartbeatLoopRunning.Store(false)
			c.heartbeatLoop()
		}()
	}

	utils.Infof("Client: control connection established successfully")
	return nil
}
