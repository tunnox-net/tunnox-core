package client

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/stream"
)

// dialTunnel 建立隧道连接（通用方法）
func (c *TunnoxClient) dialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error) {
	return c.dialTunnelWithTarget(tunnelID, mappingID, secretKey, "", 0)
}

// dialTunnelWithTarget 建立隧道连接（支持 SOCKS5 动态目标地址）
func (c *TunnoxClient) dialTunnelWithTarget(tunnelID, mappingID, secretKey, targetHost string, targetPort int) (net.Conn, stream.PackageStreamer, error) {
	// 根据协议建立到服务器的连接
	var (
		conn  net.Conn
		err   error
		token string // HTTP 长轮询使用的 token
	)

	protocol := strings.ToLower(c.config.Server.Protocol)
	switch protocol {
	case "tcp", "":
		// TCP 连接使用 DialContext 以支持 context 取消
		dialer := &net.Dialer{
			Timeout: 10 * time.Second,
		}
		conn, err = dialer.DialContext(c.Ctx(), "tcp", c.config.Server.Address)
	case "websocket":
		conn, err = dialWebSocket(c.Ctx(), c.config.Server.Address)
	case "quic":
		conn, err = dialQUIC(c.Ctx(), c.config.Server.Address)
	case "kcp":
		conn, err = dialKCP(c.Ctx(), c.config.Server.Address)
	case "httppoll", "http-long-polling", "httplp":
		// HTTP 长轮询使用 AuthToken 或 SecretKey
		token = c.config.AuthToken
		if token == "" && c.config.Anonymous {
			token = c.config.SecretKey
		}
		// 隧道连接需要传入 mappingID
		corelog.Infof("Client: dialing HTTP long polling tunnel connection, mappingID=%s, clientID=%d", mappingID, c.config.ClientID)
		conn, err = dialHTTPLongPolling(c.Ctx(), c.config.Server.Address, c.config.ClientID, token, c.GetInstanceID(), mappingID)
		// 保存 token 供后续使用
		_ = token
	default:
		return nil, nil, fmt.Errorf("unsupported server protocol: %s", protocol)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial server (%s): %w", protocol, err)
	}

	// 创建 StreamProcessor
	// HTTP 长轮询协议直接使用 HTTPStreamProcessor，不需要通过 CreateStreamProcessor
	var tunnelStream stream.PackageStreamer
	if protocol == "httppoll" || protocol == "http-long-polling" || protocol == "httplp" {
		// 对于 HTTP 长轮询，conn 是 HTTPLongPollingConn，需要转换为 HTTPStreamProcessor
		if httppollConn, ok := conn.(*HTTPLongPollingConn); ok {
			// 创建 HTTPStreamProcessor
			baseURL := httppollConn.baseURL
			pushURL := baseURL + "/tunnox/v1/push"
			pollURL := baseURL + "/tunnox/v1/poll"
			tunnelStream = httppoll.NewStreamProcessor(c.Ctx(), baseURL, pushURL, pollURL, c.config.ClientID, token, c.GetInstanceID(), mappingID)
			// 设置 ConnectionID（如果已从握手响应中获取）
			if httppollConn.connectionID != "" {
				tunnelStream.(*httppoll.StreamProcessor).SetConnectionID(httppollConn.connectionID)
			}
		} else {
			// 回退到默认方式
			streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
			tunnelStream = streamFactory.CreateStreamProcessor(conn, conn)
		}
	} else {
		streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
		tunnelStream = streamFactory.CreateStreamProcessor(conn, conn)
	}

	// ✅ HTTP 长轮询连接已经通过 HTTP 请求认证，不需要再次握手
	// 其他协议（TCP/UDP/WebSocket/QUIC）需要握手
	if protocol != "httppoll" && protocol != "http-long-polling" && protocol != "httplp" {
		// ✅ 新连接需要先进行握手认证（标识为隧道连接）
		if err := c.sendHandshakeOnStream(tunnelStream, "tunnel"); err != nil {
			tunnelStream.Close()
			conn.Close()
			return nil, nil, fmt.Errorf("tunnel connection handshake failed: %w", err)
		}
	}

	// 发送 TunnelOpen（包含 SOCKS5 动态目标地址）
	req := &packet.TunnelOpenRequest{
		MappingID:  mappingID,
		TunnelID:   tunnelID,
		SecretKey:  secretKey,
		TargetHost: targetHost, // SOCKS5 动态目标地址
		TargetPort: targetPort, // SOCKS5 动态目标端口
	}

	reqData, _ := json.Marshal(req)
	corelog.Infof("Client: sending TunnelOpen, tunnelID=%s, mappingID=%s, targetHost=%s, targetPort=%d, payloadLen=%d",
		tunnelID, mappingID, targetHost, targetPort, len(reqData))
	openPkt := &packet.TransferPacket{
		PacketType: packet.TunnelOpen,
		TunnelID:   tunnelID,
		Payload:    reqData,
	}

	if _, err := tunnelStream.WritePacket(openPkt, true, 0); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("failed to send tunnel open: %w", err)
	}

	// 等待 TunnelOpenAck（忽略心跳包）
	corelog.Infof("Client: waiting for TunnelOpenAck, tunnelID=%s, mappingID=%s", tunnelID, mappingID)
	var ackPkt *packet.TransferPacket
	timeout := time.After(30 * time.Second)
	for {
		select {
		case <-timeout:
			tunnelStream.Close()
			conn.Close()
			return nil, nil, fmt.Errorf("timeout waiting for tunnel open ack (30s)")
		default:
		}

		pkt, bytesRead, err := tunnelStream.ReadPacket()
		if err != nil {
			corelog.Errorf("Client: failed to read packet while waiting for TunnelOpenAck: %v", err)
			tunnelStream.Close()
			conn.Close()
			return nil, nil, fmt.Errorf("failed to read tunnel open ack: %w", err)
		}

		// 忽略心跳包和TunnelOpen包（可能是其他连接的TunnelOpen）
		baseType := pkt.PacketType & 0x3F
		corelog.Debugf("Client: received packet while waiting for TunnelOpenAck, type=%d (base=%d), bytesRead=%d", pkt.PacketType, baseType, bytesRead)
		if baseType == packet.Heartbeat {
			corelog.Debugf("Client: ignoring heartbeat packet while waiting for TunnelOpenAck")
			// 对于 HTTP 长轮询连接，心跳包已经被 ReadPacket 消耗，不需要恢复
			continue
		}
		if baseType == packet.TunnelOpen {
			corelog.Debugf("Client: ignoring TunnelOpen packet while waiting for TunnelOpenAck")
			// 对于 HTTP 长轮询连接，TunnelOpen 包已经被 ReadPacket 消耗，不需要恢复
			continue
		}

		// 检查是否是 TunnelOpenAck
		if baseType == packet.TunnelOpenAck {
			corelog.Infof("Client: received TunnelOpenAck, tunnelID=%s, mappingID=%s", tunnelID, mappingID)
			ackPkt = pkt
			break
		}

		// 收到其他类型的包（可能是 MySQL 握手包等原始数据）
		// 对于 HTTP 长轮询连接，这些数据应该被保留在 readBuffer 中，供流模式使用
		// 但由于 ReadPacket 已经消耗了数据，我们需要将数据放回 readBuffer
		if httppollConn, ok := conn.(interface {
			Unread(data []byte)
		}); ok {
			// 尝试从 Payload 恢复数据（如果 Payload 存在）
			if len(pkt.Payload) > 0 {
				// 构造完整的数据包：包类型(1字节) + 包体大小(4字节) + 包体
				restoreData := make([]byte, 1+4+len(pkt.Payload))
				restoreData[0] = byte(pkt.PacketType)
				binary.BigEndian.PutUint32(restoreData[1:5], uint32(len(pkt.Payload)))
				copy(restoreData[5:], pkt.Payload)
				corelog.Infof("Client: restoring %d bytes to readBuffer (packet type=%d, payload len=%d), tunnelID=%s, mappingID=%s",
					len(restoreData), baseType, len(pkt.Payload), tunnelID, mappingID)
				httppollConn.Unread(restoreData)
			} else {
				// 如果没有 Payload，至少恢复包类型和包体大小
				restoreData := make([]byte, 5)
				restoreData[0] = byte(pkt.PacketType)
				binary.BigEndian.PutUint32(restoreData[1:5], 0)
				corelog.Infof("Client: restoring packet header (5 bytes) to readBuffer, tunnelID=%s, mappingID=%s", tunnelID, mappingID)
				httppollConn.Unread(restoreData)
			}
			continue
		}

		// 收到其他类型的包，返回错误
		corelog.Errorf("Client: unexpected packet type: %v (expected TunnelOpenAck), baseType=%d", pkt.PacketType, baseType)
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("unexpected packet type: %v (expected TunnelOpenAck)", pkt.PacketType)
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

	// ✅ 对于 HTTP 长轮询连接，切换到流模式（不再解析数据包格式，直接转发原始数据）
	if protocol == "httppoll" || protocol == "http-long-polling" || protocol == "httplp" {
		if httppollConn, ok := conn.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			corelog.Infof("Client: switching HTTP long polling tunnel connection to stream mode, tunnelID=%s, mappingID=%s", tunnelID, mappingID)
			httppollConn.SetStreamMode(true)
		}
	}

	return conn, tunnelStream, nil
}

// DialTunnel 建立隧道连接（供映射处理器使用）
func (c *TunnoxClient) DialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error) {
	return c.dialTunnel(tunnelID, mappingID, secretKey)
}
