package client

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tunnox-core/internal/packet"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/stream"
)

// sendHandshake 发送握手请求（使用控制连接）
func (c *TunnoxClient) sendHandshake() error {
	return c.sendHandshakeOnStream(c.controlStream, "control")
}

// saveAnonymousCredentials 保存匿名客户端凭据到配置文件
// 注意：只保存 ClientID 和 SecretKey，不保存 Server.Address 和 Server.Protocol
// 这些字段应该由配置文件或命令行参数指定，不应该被自动连接覆盖
// 只有在命令行参数中指定了服务器地址或协议且连接成功时，才允许更新服务器配置
func (c *TunnoxClient) saveAnonymousCredentials() error {
	if !c.config.Anonymous || c.config.ClientID == 0 {
		return nil // 非匿名客户端或无ClientID，无需保存
	}

	// 使用ConfigManager保存配置
	// 只有在命令行参数中指定了服务器地址或协议时，才允许更新服务器配置
	configMgr := NewConfigManager()
	allowUpdateServerConfig := c.serverAddressFromCLI || c.serverProtocolFromCLI
	if err := configMgr.SaveConfigWithOptions(c.config, allowUpdateServerConfig); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	corelog.Infof("Client: anonymous credentials saved to config file")
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

	// ✅ 更新 ConnectionID（如果服务端返回了 ConnectionID）
	if resp.ConnectionID != "" {
		// 对于 HTTP 长轮询，更新 HTTPStreamProcessor 的 ConnectionID
		if httppollStream, ok := stream.(*httppoll.StreamProcessor); ok {
			httppollStream.SetConnectionID(resp.ConnectionID)
			corelog.Infof("Client: received ConnectionID from server: %s", resp.ConnectionID)
		}
	}

	// 匿名模式下，服务器会返回分配的凭据（仅对控制连接有用）
	if c.config.Anonymous && stream == c.controlStream {
		// ✅ 优先使用结构化字段
		if resp.ClientID > 0 && resp.SecretKey != "" {
			c.config.ClientID = resp.ClientID
			c.config.SecretKey = resp.SecretKey
			corelog.Infof("Client: received anonymous credentials - ClientID=%d, SecretKey=***", resp.ClientID)

			// 保存凭据到配置文件（供下次启动使用）
			if err := c.saveAnonymousCredentials(); err != nil {
				corelog.Warnf("Client: failed to save anonymous credentials: %v", err)
			}

			// ✅ 更新 HTTPStreamProcessor 的 clientID
			if httppollStream, ok := stream.(*httppoll.StreamProcessor); ok {
				httppollStream.UpdateClientID(resp.ClientID)
				corelog.Debugf("Client: updated HTTPStreamProcessor clientID to %d", resp.ClientID)
			}
		} else if resp.Message != "" {
			// 兼容旧版本：从Message解析ClientID
			var assignedClientID int64
			if _, err := fmt.Sscanf(resp.Message, "Anonymous client authenticated, client_id=%d", &assignedClientID); err == nil {
				c.config.ClientID = assignedClientID
				// ✅ 更新 HTTPStreamProcessor 的 clientID
				if httppollStream, ok := stream.(*httppoll.StreamProcessor); ok {
					httppollStream.UpdateClientID(assignedClientID)
					corelog.Debugf("Client: updated HTTPStreamProcessor clientID to %d", assignedClientID)
				}
			}
		}
	}

	// 打印认证信息
	if c.config.Anonymous {
		corelog.Infof("Client: authenticated as anonymous client, ClientID=%d, DeviceID=%s",
			c.config.ClientID, c.config.DeviceID)
	} else {
		corelog.Infof("Client: authenticated successfully, ClientID=%d, Token=%s",
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

