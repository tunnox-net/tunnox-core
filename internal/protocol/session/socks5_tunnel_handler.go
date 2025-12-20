// Package session SOCKS5 隧道请求处理
// 处理 ClientA 发起的 SOCKS5 隧道创建请求
package session

import (
	"encoding/json"
	"fmt"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// SOCKS5TunnelRequest SOCKS5 隧道请求（从 ClientA 发送）
type SOCKS5TunnelRequest struct {
	TunnelID       string `json:"tunnel_id"`
	MappingID      string `json:"mapping_id"`
	TargetClientID int64  `json:"target_client_id"`
	TargetHost     string `json:"target_host"` // 动态目标地址
	TargetPort     int    `json:"target_port"` // 动态目标端口
	Protocol       string `json:"protocol"`
}

// HandleSOCKS5TunnelRequest 处理 SOCKS5 隧道请求
// 由 ClientA 发起，Server 转发到 ClientB
func (s *SessionManager) HandleSOCKS5TunnelRequest(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return fmt.Errorf("command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 1. 解析 SOCKS5 隧道请求
	var req SOCKS5TunnelRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		corelog.Errorf("SOCKS5TunnelHandler: failed to parse request: %v", err)
		return fmt.Errorf("invalid SOCKS5 tunnel request: %w", err)
	}

	corelog.Infof("SOCKS5TunnelHandler: received request - TunnelID=%s, MappingID=%s, Target=%s:%d, TargetClientID=%d",
		req.TunnelID, req.MappingID, req.TargetHost, req.TargetPort, req.TargetClientID)

	// 2. 验证映射
	if s.cloudControl == nil {
		return fmt.Errorf("cloud control not configured")
	}

	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		corelog.Errorf("SOCKS5TunnelHandler: mapping not found %s: %v", req.MappingID, err)
		return fmt.Errorf("mapping not found: %w", err)
	}

	// 3. 验证请求来源是 ListenClientID
	sourceClientID := s.getClientIDFromConnection(connPacket.ConnectionID)
	if sourceClientID != mapping.ListenClientID {
		corelog.Warnf("SOCKS5TunnelHandler: client %d not authorized (expected %d)",
			sourceClientID, mapping.ListenClientID)
		return fmt.Errorf("client not authorized for this mapping")
	}

	// 4. 查找目标客户端的控制连接
	targetControlConn := s.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		// 尝试跨服务器转发
		if s.bridgeManager != nil {
			corelog.Infof("SOCKS5TunnelHandler: target client %d not on this server, broadcasting",
				mapping.TargetClientID)
			// 构造 TunnelOpenRequest 用于跨服务器转发
			tunnelReq := &packet.TunnelOpenRequest{
				TunnelID:   req.TunnelID,
				MappingID:  req.MappingID,
				SecretKey:  mapping.SecretKey,
				TargetHost: req.TargetHost, // 动态目标
				TargetPort: req.TargetPort, // 动态端口
			}
			if err := s.bridgeManager.BroadcastTunnelOpen(tunnelReq, mapping.TargetClientID); err != nil {
				corelog.Errorf("SOCKS5TunnelHandler: failed to broadcast: %v", err)
				return fmt.Errorf("failed to reach target client")
			}
			return nil
		}
		corelog.Errorf("SOCKS5TunnelHandler: target client %d not connected", mapping.TargetClientID)
		return fmt.Errorf("target client not connected")
	}

	// 5. 构造 TunnelOpenRequest 命令（包含动态目标地址）
	cmdBody := map[string]interface{}{
		"tunnel_id":          req.TunnelID,
		"mapping_id":         req.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        req.TargetHost, // 动态目标（来自 SOCKS5 协议）
		"target_port":        req.TargetPort, // 动态端口（来自 SOCKS5 协议）
		"protocol":           "socks5",
		"enable_compression": mapping.Config.EnableCompression,
		"compression_level":  mapping.Config.CompressionLevel,
		"enable_encryption":  mapping.Config.EnableEncryption,
		"encryption_method":  mapping.Config.EncryptionMethod,
		"encryption_key":     mapping.Config.EncryptionKey,
		"bandwidth_limit":    mapping.Config.BandwidthLimit,
	}

	cmdBodyJSON, err := json.Marshal(cmdBody)
	if err != nil {
		corelog.Errorf("SOCKS5TunnelHandler: failed to marshal command: %v", err)
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// 6. 发送 TunnelOpenRequest 到目标客户端
	tunnelCmd := &packet.CommandPacket{
		CommandType: packet.TunnelOpenRequestCmd,
		CommandBody: string(cmdBodyJSON),
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: tunnelCmd,
	}

	if _, err := targetControlConn.Stream.WritePacket(pkt, false, 0); err != nil {
		corelog.Errorf("SOCKS5TunnelHandler: failed to send to target client %d: %v",
			mapping.TargetClientID, err)
		return fmt.Errorf("failed to send to target client: %w", err)
	}

	corelog.Infof("SOCKS5TunnelHandler: sent TunnelOpenRequest to client %d for tunnel %s, target=%s:%d",
		mapping.TargetClientID, req.TunnelID, req.TargetHost, req.TargetPort)

	return nil
}

// getClientIDFromConnection 从连接中获取客户端ID
func (s *SessionManager) getClientIDFromConnection(connID string) int64 {
	// 尝试从控制连接获取
	s.controlConnLock.RLock()
	if controlConn, exists := s.controlConnMap[connID]; exists {
		s.controlConnLock.RUnlock()
		return controlConn.ClientID
	}
	s.controlConnLock.RUnlock()

	// 尝试从连接的 Stream 获取
	conn := s.getConnectionByConnID(connID)
	if conn != nil && conn.Stream != nil {
		if streamWithClientID, ok := conn.Stream.(interface {
			GetClientID() int64
		}); ok {
			return streamWithClientID.GetClientID()
		}
	}

	return 0
}
