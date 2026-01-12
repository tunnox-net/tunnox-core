package client

import (
	"encoding/json"
	"net"
	"strings"
	"time"

	"tunnox-core/internal/client/transport"
	coreerrors "tunnox-core/internal/core/errors"
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
	return c.dialTunnelWithTargetNetwork(tunnelID, mappingID, secretKey, targetHost, targetPort, "")
}

// dialTunnelWithTargetNetwork 建立隧道连接（支持 SOCKS5 动态目标地址和传输层协议）
// targetNetwork: "tcp"（默认）或 "udp"（用于 SOCKS5 UDP ASSOCIATE）
func (c *TunnoxClient) dialTunnelWithTargetNetwork(tunnelID, mappingID, secretKey, targetHost string, targetPort int, targetNetwork string) (net.Conn, stream.PackageStreamer, error) {
	corelog.Debugf("Client[%s]: dialTunnelWithTarget START, tunnelID=%s, mappingID=%s", tunnelID, tunnelID, mappingID)

	// 根据协议建立到服务器的连接
	var (
		conn net.Conn
		err  error
	)

	protocol := strings.ToLower(c.config.Server.Protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	corelog.Debugf("Client[%s]: about to dial server, protocol=%s, address=%s", tunnelID, protocol, c.config.Server.Address)

	// 检查协议是否可用
	if !transport.IsProtocolAvailable(protocol) {
		availableProtocols := transport.GetAvailableProtocolNames()
		return nil, nil, coreerrors.Newf(coreerrors.CodeNotConfigured, "protocol %q is not available (compiled protocols: %v)", protocol, availableProtocols)
	}

	// 使用统一的协议注册表拨号
	conn, err = transport.Dial(c.Ctx(), protocol, c.config.Server.Address)

	if err != nil {
		corelog.Errorf("Client[%s]: dial server failed: %v", tunnelID, err)
		return nil, nil, coreerrors.Wrapf(err, coreerrors.CodeNetworkError, "failed to dial server (%s)", protocol)
	}
	corelog.Debugf("Client[%s]: dial server SUCCESS, local=%s, remote=%s", tunnelID, conn.LocalAddr(), conn.RemoteAddr())

	// 创建 StreamProcessor
	corelog.Debugf("Client[%s]: creating StreamProcessor", tunnelID)
	streamFactory := stream.NewDefaultStreamFactory(c.Ctx())
	tunnelStream := streamFactory.CreateStreamProcessor(conn, conn)
	corelog.Debugf("Client[%s]: StreamProcessor created", tunnelID)

	// ✅ 新连接需要先进行握手认证（标识为隧道连接）
	if err := c.sendHandshakeOnStream(tunnelStream, "tunnel", protocol); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeHandshakeFailed, "tunnel connection handshake failed")
	}

	req := &packet.TunnelOpenRequest{
		MappingID:     mappingID,
		TunnelID:      tunnelID,
		SecretKey:     secretKey,
		TargetHost:    targetHost,
		TargetPort:    targetPort,
		TargetNetwork: targetNetwork,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal tunnel open request")
	}
	openPkt := &packet.TransferPacket{
		PacketType: packet.TunnelOpen,
		TunnelID:   tunnelID,
		Payload:    reqData,
	}

	if _, err := tunnelStream.WritePacket(openPkt, true, 0); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send tunnel open")
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
			return nil, nil, coreerrors.New(coreerrors.CodeTimeout, "timeout waiting for tunnel open ack (30s)")
		default:
		}

		pkt, bytesRead, err := tunnelStream.ReadPacket()
		if err != nil {
			corelog.Errorf("Client: failed to read packet while waiting for TunnelOpenAck: %v", err)
			tunnelStream.Close()
			conn.Close()
			return nil, nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read tunnel open ack")
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
		return nil, nil, coreerrors.Newf(coreerrors.CodeInvalidPacket, "unexpected packet type: %v (expected TunnelOpenAck)", pkt.PacketType)
	}

	var ack packet.TunnelOpenAckResponse
	if err := json.Unmarshal(ackPkt.Payload, &ack); err != nil {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal ack")
	}

	if !ack.Success {
		tunnelStream.Close()
		conn.Close()
		return nil, nil, coreerrors.Newf(coreerrors.CodeTunnelError, "tunnel open failed: %s", ack.Error)
	}

	return conn, tunnelStream, nil
}

// DialTunnel 建立隧道连接（供映射处理器使用）
func (c *TunnoxClient) DialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error) {
	return c.dialTunnel(tunnelID, mappingID, secretKey)
}
