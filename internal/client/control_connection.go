package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

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

	// 使用锁保护连接状态
	c.mu.Lock()
	c.controlConn = conn
	// 2. 创建 Stream
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
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
	go c.readLoop()

	// 5. 启动心跳循环
	go c.heartbeatLoop()

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

	if _, err := stream.WritePacket(handshakePkt, false, 0); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// 等待握手响应
	respPkt, _, err := stream.ReadPacket()
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
		} else if resp.Message != "" {
			// 兼容旧版本：从Message解析ClientID
			var assignedClientID int64
			if _, err := fmt.Sscanf(resp.Message, "Anonymous client authenticated, client_id=%d", &assignedClientID); err == nil {
				c.config.ClientID = assignedClientID
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
	if stream == c.controlStream && c.config.ClientID > 0 {
		go c.requestMappingConfig()
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
			// 读取失败，清理连接状态
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

		utils.Infof("Client: received packet, type=%d", pkt.PacketType)

		// 处理不同类型的数据包
		switch pkt.PacketType & 0x3F {
		case packet.Heartbeat:
			// 心跳响应
			utils.Debugf("Client: heartbeat response received")
		case packet.CommandResp:
			// ✅ 命令响应（通过指令通道返回的响应）
			if c.commandResponseManager != nil {
				if handled := c.commandResponseManager.HandleResponse(pkt); handled {
					utils.Debugf("Client: command response handled by response manager")
					continue
				}
			}
			utils.Debugf("Client: unhandled command response")
		case packet.JsonCommand:
			// 命令处理（服务器推送的命令）
			utils.Infof("Client: processing JsonCommand")
			c.handleCommand(pkt)
		default:
			utils.Warnf("Client: unknown packet type: %d", pkt.PacketType)
		}
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
