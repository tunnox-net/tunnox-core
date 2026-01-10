package session

import (
	"encoding/json"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// sendTunnelOpenResponse 发送隧道打开响应
func (s *SessionManager) sendTunnelOpenResponse(conn ControlConnectionInterface, resp *packet.TunnelOpenAckResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal tunnel open response")
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.TunnelOpenAck,
		Payload:    respData,
	}

	// 发送响应
	if _, err := conn.GetStream().WritePacket(respPacket, false, 0); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write tunnel open response")
	}

	return nil
}

// sendTunnelOpenResponseDirect 直接发送隧道打开响应（使用types.Connection）
func (s *SessionManager) sendTunnelOpenResponseDirect(conn *types.Connection, resp *packet.TunnelOpenAckResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal tunnel open response")
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.TunnelOpenAck,
		Payload:    respData,
	}

	// 发送响应
	if _, err := conn.Stream.WritePacket(respPacket, true, 0); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write tunnel open response")
	}

	return nil
}

// notifyTargetClientToOpenTunnel 通知目标客户端建立隧道连接
func (s *SessionManager) notifyTargetClientToOpenTunnel(req *packet.TunnelOpenRequest) {
	// 1. 获取映射配置
	if s.cloudControl == nil {
		corelog.Errorf("Tunnel[%s]: CloudControl not configured, cannot notify target client", req.TunnelID)
		return
	}

	corelog.Infof("Tunnel[%s]: notifyTargetClientToOpenTunnel - req.TargetHost=%s, req.TargetPort=%d, req.MappingID=%s",
		req.TunnelID, req.TargetHost, req.TargetPort, req.MappingID)

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		corelog.Errorf("Tunnel[%s]: failed to get mapping %s: %v", req.TunnelID, req.MappingID, err)
		return
	}

	// 2. 找到目标客户端的控制连接（本地或跨服务器）
	targetControlConn := s.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		// 某些协议可能没有注册为控制连接，尝试通过 connMap 查找
		allConns := s.ListConnections()
		for _, c := range allConns {
			if c.Stream != nil {
				reader := c.Stream.GetReader()
				if clientIDConn, ok := reader.(interface {
					GetClientID() int64
				}); ok {
					connClientID := clientIDConn.GetClientID()
					if connClientID == mapping.TargetClientID {
						// 找到目标客户端的连接，创建临时控制连接
						var remoteAddr net.Addr
						if c.RawConn != nil {
							remoteAddr = c.RawConn.RemoteAddr()
						}
						protocol := c.Protocol
						tempConn := NewControlConnection(c.ID, c.Stream, remoteAddr, protocol)
						tempConn.SetClientID(mapping.TargetClientID)
						tempConn.SetAuthenticated(true)
						// 注册为控制连接（临时）
						s.RegisterControlConnection(tempConn)
						targetControlConn = tempConn
						break
					}
				}
			}
		}
	}
	if targetControlConn == nil {
		// 本地未找到，尝试跨服务器转发
		if s.bridgeManager != nil {
			s.bridgeManager.BroadcastTunnelOpen(req, mapping.TargetClientID)
		}
		return
	}

	// 3. 构造TunnelOpenRequest命令
	// 对于 SOCKS5 协议，使用请求中的动态目标地址
	// 对于其他协议，使用映射配置中的固定目标地址
	targetHost := mapping.TargetHost
	targetPort := mapping.TargetPort
	// 支持 "socks5" 和 "socks" 两种写法
	isSocks5 := mapping.Protocol == "socks5" || mapping.Protocol == "socks"
	if isSocks5 && req.TargetHost != "" {
		targetHost = req.TargetHost
		targetPort = req.TargetPort
	}

	cmdBody := map[string]interface{}{
		"tunnel_id":          req.TunnelID,
		"mapping_id":         req.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        targetHost,
		"target_port":        targetPort,
		"protocol":           string(mapping.Protocol),
		"enable_compression": mapping.Config.EnableCompression,
		"compression_level":  mapping.Config.CompressionLevel,
		"enable_encryption":  mapping.Config.EnableEncryption,
		"encryption_method":  mapping.Config.EncryptionMethod,
		"encryption_key":     mapping.Config.EncryptionKey,
		"bandwidth_limit":    mapping.Config.BandwidthLimit,
	}

	corelog.Infof("Tunnel[%s]: sending to target client %d - protocol=%s, isSocks5=%v, target_host=%s, target_port=%d",
		req.TunnelID, mapping.TargetClientID, mapping.Protocol, isSocks5, targetHost, targetPort)

	cmdBodyJSON, err := json.Marshal(cmdBody)
	if err != nil {
		corelog.Errorf("Tunnel[%s]: failed to marshal command body: %v", req.TunnelID, err)
		return
	}

	// 4. 通过控制连接发送命令
	cmd := &packet.CommandPacket{
		CommandType: packet.TunnelOpenRequestCmd, // 60
		CommandBody: string(cmdBodyJSON),
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	_, err = targetControlConn.Stream.WritePacket(pkt, false, 0)
	if err != nil {
		corelog.Errorf("Tunnel[%s]: failed to send tunnel open request to target client %d: %v",
			req.TunnelID, mapping.TargetClientID, err)
	}
}
