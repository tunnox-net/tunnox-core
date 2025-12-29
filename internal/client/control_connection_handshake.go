package client

import (
	"encoding/json"
	"fmt"
	"strings"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// sendHandshake 发送握手请求（使用控制连接）
func (c *TunnoxClient) sendHandshake() error {
	corelog.Infof("Client: sendHandshake called, controlStream=%p", c.controlStream)
	return c.sendHandshakeOnStream(c.controlStream, "control")
}

// saveConnectionConfig 保存连接配置到配置文件
// 连接成功后保存配置，供下次启动使用
// 保存条件：
// 1. 命令行参数指定了服务器地址/协议，或使用了自动连接检测
// 2. 获取到了 ClientID 和 SecretKey（首次认证成功后）
func (c *TunnoxClient) saveConnectionConfig() error {
	// 检查是否需要保存配置
	// 条件1: 命令行参数指定或使用自动连接
	shouldSaveServerConfig := c.serverAddressFromCLI || c.serverProtocolFromCLI || c.usedAutoConnection

	// 条件2: 有 ClientID 和 SecretKey（需要保存以便重连时使用）
	shouldSaveCredentials := c.config.ClientID > 0 && c.config.SecretKey != ""

	if !shouldSaveServerConfig && !shouldSaveCredentials {
		return nil // 无需保存
	}

	// 使用ConfigManager保存配置
	configMgr := NewConfigManager()
	// 只有在需要保存服务器配置时才允许更新服务器地址和协议
	if err := configMgr.SaveConfigWithOptions(c.config, shouldSaveServerConfig); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	corelog.Infof("Client: connection config saved (ClientID=%d, protocol=%s, address=%s)",
		c.config.ClientID, c.config.Server.Protocol, c.config.Server.Address)
	return nil
}

// sendHandshakeOnStream 在指定的stream上发送握手请求（用于隧道连接）
func (c *TunnoxClient) sendHandshakeOnStream(stream stream.PackageStreamer, connectionType string) error {
	corelog.Infof("Client: sendHandshakeOnStream called, stream=%p, streamType=%T, connectionType=%s", stream, stream, connectionType)

	var req *packet.HandshakeRequest

	// 统一认证策略：
	// 1. 有 ClientID + SecretKey → 使用这两个字段认证
	// 2. 无 ClientID → 首次握手，请求服务端分配凭据
	if c.config.ClientID > 0 && c.config.SecretKey != "" {
		// 使用持久化凭据认证
		req = &packet.HandshakeRequest{
			ClientID:       c.config.ClientID,
			Token:          c.config.SecretKey,
			Version:        "2.0",
			Protocol:       c.config.Server.Protocol,
			ConnectionType: connectionType,
		}
	} else {
		// 首次握手，请求分配凭据
		req = &packet.HandshakeRequest{
			ClientID:       0,
			Token:          "new-client", // 标识首次连接
			Version:        "2.0",
			Protocol:       c.config.Server.Protocol,
			ConnectionType: connectionType,
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
			corelog.Debugf("Client: ReadPacket returned nil packet, continuing to wait for handshake response")
			continue
		}

		// 忽略压缩/加密标志，只检查基础类型
		baseType := pkt.PacketType & 0x3F
		if baseType == packet.Heartbeat {
			// 收到心跳包，继续等待握手响应
			corelog.Debugf("Client: received heartbeat during handshake, ignoring")
			continue
		}

		if baseType == packet.HandshakeResp {
			respPkt = pkt
			break
		}

		// 收到其他类型的包，返回错误
		return fmt.Errorf("unexpected response type: %v (expected HandshakeResp)", pkt.PacketType)
	}

	corelog.Debugf("Client: received response PacketType=%d, Payload len=%d", respPkt.PacketType, len(respPkt.Payload))
	if len(respPkt.Payload) > 0 {
		corelog.Debugf("Client: Payload=%s", string(respPkt.Payload))
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

	// 首次连接时，服务器会返回分配的凭据（仅对控制连接有用）
	if stream == c.controlStream && resp.ClientID > 0 && resp.SecretKey != "" {
		// 更新本地凭据
		c.config.ClientID = resp.ClientID
		c.config.SecretKey = resp.SecretKey
		corelog.Infof("Client: received credentials - ClientID=%d, SecretKey=***", resp.ClientID)

		// 更新 SOCKS5Manager 的 clientID
		if c.socks5Manager != nil {
			c.socks5Manager.SetClientID(resp.ClientID)
			corelog.Debugf("Client: updated SOCKS5Manager clientID to %d", resp.ClientID)
		}
	}

	// 打印认证信息
	corelog.Infof("Client: authenticated successfully, ClientID=%d", c.config.ClientID)

	// 握手成功后保存配置
	// 仅对控制连接保存，避免隧道连接重复保存
	if stream == c.controlStream {
		if err := c.saveConnectionConfig(); err != nil {
			corelog.Warnf("Client: failed to save connection config: %v", err)
		}
	}

	// ✅ 握手成功后不再主动请求映射配置
	// 服务端会在握手成功后通过 pushConfigToClient 主动推送配置
	// 移除客户端主动请求逻辑，避免 ConfigSet 重复发送
	// 详见：packet_handler_handshake.go:166

	return nil
}
