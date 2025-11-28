package client

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

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

	// ✅ 新连接需要先进行握手认证
	if err := c.sendHandshakeOnStream(tunnelStream); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("tunnel connection handshake failed: %w", err)
	}

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
