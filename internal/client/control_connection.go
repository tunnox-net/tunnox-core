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

