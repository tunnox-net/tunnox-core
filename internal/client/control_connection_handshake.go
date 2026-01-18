package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// ProtocolVersion 当前客户端协议版本
// V3 = 挑战-响应认证（SecretKey 不在网络传输）
const ProtocolVersion = "3.0"

// computeChallengeResponse 计算挑战响应
//
// 使用 HMAC-SHA256(secretKey, challenge) 计算响应
// 与服务端 SecretKeyManager.ComputeResponse 算法一致
func computeChallengeResponse(secretKey, challenge string) string {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(challenge))
	return hex.EncodeToString(h.Sum(nil))
}

// sendHandshake 发送握手请求（使用控制连接）
func (c *TunnoxClient) sendHandshake() error {
	corelog.Infof("Client: sendHandshake called, controlStream=%p", c.controlStream)
	return c.sendHandshakeOnStream(c.controlStream, "control", c.config.Server.Protocol)
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
	// 优先使用用户指定的配置文件路径，这样凭据会保存到用户的配置文件中
	configMgr := NewConfigManagerWithPath(c.configFilePath)
	// 只有在需要保存服务器配置时才允许更新服务器地址和协议
	if err := configMgr.SaveConfigWithOptions(c.config, shouldSaveServerConfig); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to save config")
	}

	corelog.Infof("Client: connection config saved (ClientID=%d, protocol=%s, address=%s, configFile=%s)",
		c.config.ClientID, c.config.Server.Protocol, c.config.Server.Address, c.configFilePath)
	return nil
}

// sendHandshakeOnStream 在指定的stream上发送握手请求（用于隧道连接）
//
// connectionType: "control" 表示控制连接，"tunnel" 表示隧道连接
// protocol: 使用的传输协议（tcp/websocket/quic/kcp）
//
// V3 协议（挑战-响应认证）流程：
//  1. 客户端发送 ClientID（首次连接时 ClientID=0, Token="new-client"）
//  2. 服务端返回：
//     - 首次连接：Success=true, ClientID=xxx, SecretKey=xxx
//     - 已有客户端：NeedResponse=true, Challenge=xxx
//  3. 客户端计算 ChallengeResponse = HMAC-SHA256(SecretKey, Challenge)
//  4. 客户端发送 ClientID + ChallengeResponse
//  5. 服务端验证并返回 Success=true
func (c *TunnoxClient) sendHandshakeOnStream(stream stream.PackageStreamer, connectionType string, protocol string) error {
	isControlConnection := connectionType == "control"
	corelog.Infof("Client: sendHandshakeOnStream called, stream=%p, streamType=%T, connectionType=%s, protocol=%s, isControl=%v", stream, stream, connectionType, protocol, isControlConnection)

	// ========================================
	// 阶段1：发送初始握手请求
	// ========================================
	var req *packet.HandshakeRequest

	if c.config.ClientID > 0 && c.config.SecretKey != "" {
		// 已有凭据：发送 ClientID 和 Token（兼容 legacy 认证）
		// Token 字段用于 legacy 明文认证，ChallengeResponse 用于新版 challenge-response 认证
		req = &packet.HandshakeRequest{
			ClientID:       c.config.ClientID,
			Token:          c.config.SecretKey, // 兼容 legacy 认证
			Version:        ProtocolVersion,
			Protocol:       protocol,
			ConnectionType: connectionType,
			// ChallengeResponse 留空，等待服务端返回 Challenge（如果是新版认证）
		}
		corelog.Infof("Client: sending handshake phase 1 (existing client, ClientID=%d)", c.config.ClientID)
	} else {
		// 首次连接：请求分配凭据
		req = &packet.HandshakeRequest{
			ClientID:       0,
			Token:          "new-client", // 标识首次连接
			Version:        ProtocolVersion,
			Protocol:       protocol,
			ConnectionType: connectionType,
		}
		corelog.Infof("Client: sending handshake phase 1 (new client)")
	}

	// 发送阶段1请求
	resp, err := c.sendHandshakeRequest(stream, req)
	if err != nil {
		return err
	}

	// ========================================
	// 阶段2：处理挑战-响应（如果需要）
	// ========================================
	if resp.NeedResponse && resp.Challenge != "" {
		corelog.Infof("Client: received challenge, computing response...")

		// 计算挑战响应
		if c.config.SecretKey == "" {
			c.authFailed = true
			return coreerrors.New(coreerrors.CodeAuthFailed, "no SecretKey available for challenge response")
		}
		challengeResponse := computeChallengeResponse(c.config.SecretKey, resp.Challenge)

		// 发送阶段2请求
		req2 := &packet.HandshakeRequest{
			ClientID:          c.config.ClientID,
			Version:           ProtocolVersion,
			Protocol:          protocol,
			ConnectionType:    connectionType,
			ChallengeResponse: challengeResponse,
		}
		corelog.Infof("Client: sending handshake phase 2 (challenge response)")

		resp, err = c.sendHandshakeRequest(stream, req2)
		if err != nil {
			return err
		}
	}

	// ========================================
	// 阶段3：处理最终响应
	// ========================================
	if !resp.Success {
		// 认证失败，标记不重连
		c.authFailed = true
		return coreerrors.Newf(coreerrors.CodeAuthFailed, "handshake failed: %s", resp.Error)
	}

	// 首次连接时，服务器会返回分配的凭据（仅对控制连接有用）
	if isControlConnection && resp.ClientID > 0 && resp.SecretKey != "" {
		// 更新本地凭据
		c.config.ClientID = resp.ClientID
		c.config.SecretKey = resp.SecretKey
		corelog.Infof("Client: received credentials - ClientID=%d, SecretKey=*** (save this key, it will only be shown once!)", resp.ClientID)

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
	if isControlConnection {
		if err := c.saveConnectionConfig(); err != nil {
			corelog.Warnf("Client: failed to save connection config: %v", err)
		}
	}

	return nil
}

// sendHandshakeRequest 发送握手请求并等待响应
func (c *TunnoxClient) sendHandshakeRequest(stream stream.PackageStreamer, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	reqData, _ := json.Marshal(req)
	// 调试：打印发送的 JSON
	corelog.Infof("Client: sendHandshakeRequest sending JSON: %s", string(reqData))
	handshakePkt := &packet.TransferPacket{
		PacketType: packet.Handshake,
		Payload:    reqData,
	}

	if _, err := stream.WritePacket(handshakePkt, true, 0); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send handshake")
	}

	// 等待握手响应（忽略心跳包）
	var respPkt *packet.TransferPacket
	for {
		pkt, _, err := stream.ReadPacket()
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read handshake response")
		}
		if pkt == nil {
			corelog.Debugf("Client: ReadPacket returned nil packet, continuing to wait for handshake response")
			continue
		}

		// 忽略压缩/加密标志，只检查基础类型
		baseType := pkt.PacketType & 0x3F
		if baseType == packet.Heartbeat {
			corelog.Debugf("Client: received heartbeat during handshake, ignoring")
			continue
		}

		if baseType == packet.HandshakeResp {
			respPkt = pkt
			break
		}

		return nil, coreerrors.Newf(coreerrors.CodeInvalidPacket, "unexpected response type: %v (expected HandshakeResp)", pkt.PacketType)
	}

	corelog.Debugf("Client: received response PacketType=%d, Payload len=%d", respPkt.PacketType, len(respPkt.Payload))
	if len(respPkt.Payload) > 0 {
		corelog.Debugf("Client: Payload=%s", string(respPkt.Payload))
	}

	var resp packet.HandshakeResponse
	if err := json.Unmarshal(respPkt.Payload, &resp); err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeInvalidData, "failed to unmarshal handshake response (payload='%s')", string(respPkt.Payload))
	}

	// 检查非挑战响应的失败
	if !resp.Success && !resp.NeedResponse {
		// 认证失败，标记不重连
		if strings.Contains(resp.Error, "auth") || strings.Contains(resp.Error, "token") ||
			strings.Contains(resp.Error, "credential") || strings.Contains(resp.Error, "expired") {
			c.authFailed = true
		}
		return nil, coreerrors.Newf(coreerrors.CodeAuthFailed, "handshake failed: %s", resp.Error)
	}

	return &resp, nil
}
