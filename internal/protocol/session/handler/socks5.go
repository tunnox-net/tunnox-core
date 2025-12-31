package handler

import (
	"encoding/json"

	coreerrors "tunnox-core/internal/core/errors"
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

// SOCKS5ManagerInterface SessionManager的SOCKS5最小接口
type SOCKS5ManagerInterface interface {
	// 连接查询
	GetControlConnectionByClientID(clientID int64) ControlConnectionInterface
	GetConnectionByConnID(connID string) (*types.Connection, bool)

	// 客户端ID获取
	GetClientIDFromConnection(connID string) int64

	// 云控管理
	GetCloudControl() CloudControlAPI

	// 跨节点桥接
	GetBridgeManager() BridgeManager
}

// SOCKS5Handler SOCKS5隧道处理器
type SOCKS5Handler struct {
	sessionManager SOCKS5ManagerInterface
	cloudControl   CloudControlAPI
	bridgeManager  BridgeManager
	logger         corelog.Logger
}

// SOCKS5HandlerConfig SOCKS5处理器配置
type SOCKS5HandlerConfig struct {
	SessionManager SOCKS5ManagerInterface
	CloudControl   CloudControlAPI
	BridgeManager  BridgeManager
	Logger         corelog.Logger
}

// NewSOCKS5Handler 创建SOCKS5处理器
func NewSOCKS5Handler(config *SOCKS5HandlerConfig) *SOCKS5Handler {
	if config == nil {
		config = &SOCKS5HandlerConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &SOCKS5Handler{
		sessionManager: config.SessionManager,
		cloudControl:   config.CloudControl,
		bridgeManager:  config.BridgeManager,
		logger:         logger,
	}
}

// HandlePacket 处理SOCKS5隧道请求数据包
func (h *SOCKS5Handler) HandlePacket(connPacket *types.StreamPacket) error {
	return h.handleSOCKS5TunnelRequest(connPacket)
}

// handleSOCKS5TunnelRequest 处理 SOCKS5 隧道请求
// 由 ClientA 发起，Server 转发到 ClientB
func (h *SOCKS5Handler) handleSOCKS5TunnelRequest(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 1. 解析 SOCKS5 隧道请求
	var req SOCKS5TunnelRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		h.logger.Errorf("SOCKS5TunnelHandler: failed to parse request: %v", err)
		return coreerrors.Wrap(err, coreerrors.CodeInvalidPacket, "invalid SOCKS5 tunnel request")
	}

	h.logger.Infof("SOCKS5TunnelHandler: received request - TunnelID=%s, MappingID=%s, Target=%s:%d, TargetClientID=%d",
		req.TunnelID, req.MappingID, req.TargetHost, req.TargetPort, req.TargetClientID)

	// 2. 验证映射
	if h.cloudControl == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "cloud control not configured")
	}

	mapping, err := h.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		h.logger.Errorf("SOCKS5TunnelHandler: mapping not found %s: %v", req.MappingID, err)
		return coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found")
	}

	// 3. 验证请求来源是 ListenClientID
	sourceClientID := h.sessionManager.GetClientIDFromConnection(connPacket.ConnectionID)
	if sourceClientID != mapping.ListenClientID {
		h.logger.Warnf("SOCKS5TunnelHandler: client %d not authorized (expected %d)",
			sourceClientID, mapping.ListenClientID)
		return coreerrors.New(coreerrors.CodeUnauthorized, "client not authorized for this mapping")
	}

	// 4. 查找目标客户端的控制连接
	targetControlConn := h.sessionManager.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		// 尝试跨服务器转发
		if h.bridgeManager != nil {
			h.logger.Infof("SOCKS5TunnelHandler: target client %d not on this server, broadcasting",
				mapping.TargetClientID)
			// 构造 TunnelOpenRequest 用于跨服务器转发
			tunnelReq := &packet.TunnelOpenRequest{
				TunnelID:   req.TunnelID,
				MappingID:  req.MappingID,
				SecretKey:  mapping.SecretKey,
				TargetHost: req.TargetHost, // 动态目标
				TargetPort: req.TargetPort, // 动态端口
			}
			if err := h.bridgeManager.BroadcastTunnelOpen(tunnelReq, mapping.TargetClientID); err != nil {
				h.logger.Errorf("SOCKS5TunnelHandler: failed to broadcast: %v", err)
				return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to reach target client")
			}
			return nil
		}
		h.logger.Errorf("SOCKS5TunnelHandler: target client %d not connected", mapping.TargetClientID)
		return coreerrors.New(coreerrors.CodeClientOffline, "target client not connected")
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
		h.logger.Errorf("SOCKS5TunnelHandler: failed to marshal command: %v", err)
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal command")
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

	if _, err := targetControlConn.GetStream().WritePacket(pkt, false, 0); err != nil {
		h.logger.Errorf("SOCKS5TunnelHandler: failed to send to target client %d: %v",
			mapping.TargetClientID, err)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send to target client")
	}

	h.logger.Infof("SOCKS5TunnelHandler: sent TunnelOpenRequest to client %d for tunnel %s, target=%s:%d",
		mapping.TargetClientID, req.TunnelID, req.TargetHost, req.TargetPort)

	return nil
}
