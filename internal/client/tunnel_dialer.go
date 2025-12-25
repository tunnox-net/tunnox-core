package client

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
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
		conn net.Conn
		err  error
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
	default:
		return nil, nil, fmt.Errorf("unsupported server protocol: %s", protocol)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial server (%s): %w", protocol, err)
	}

	// 创建 StreamProcessor
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	tunnelStream := streamFactory.CreateStreamProcessor(conn, conn)

	// ✅ 新连接需要先进行握手认证（标识为隧道连接）
	if err := c.sendHandshakeOnStream(tunnelStream, "tunnel"); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("tunnel connection handshake failed: %w", err)
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

	return conn, tunnelStream, nil
}

// DialTunnel 建立隧道连接（供映射处理器使用）
func (c *TunnoxClient) DialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error) {
	return c.dialTunnel(tunnelID, mappingID, secretKey)
}
