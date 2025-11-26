package server

import (
	"encoding/json"
	"fmt"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/config"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// ServerAuthHandler 服务器认证处理器
type ServerAuthHandler struct {
	cloudControl managers.CloudControlAPI
}

// NewServerAuthHandler 创建认证处理器
func NewServerAuthHandler(cloudControl managers.CloudControlAPI) *ServerAuthHandler {
	return &ServerAuthHandler{
		cloudControl: cloudControl,
	}
}

// HandleHandshake 处理握手请求
func (h *ServerAuthHandler) HandleHandshake(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	utils.Debugf("ServerAuthHandler: handling handshake for connection %s, ClientID=%d", conn.ConnID, req.ClientID)

	var clientID int64

	// 处理匿名客户端（ClientID == 0 表示匿名）
	if req.ClientID == 0 {
		// 生成匿名客户端凭据
		anonClient, err := h.cloudControl.GenerateAnonymousCredentials()
		if err != nil {
			utils.Errorf("ServerAuthHandler: failed to generate anonymous credentials: %v", err)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   err.Error(),
			}, err
		}
		clientID = anonClient.ID
		utils.Infof("ServerAuthHandler: generated anonymous client ID: %d", clientID)
	} else {
		// 验证注册客户端
		authResp, err := h.cloudControl.Authenticate(&models.AuthRequest{
			ClientID: req.ClientID,
			AuthCode: req.Token,
		})
		if err != nil {
			utils.Errorf("ServerAuthHandler: authentication failed for client %d: %v", req.ClientID, err)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Authentication failed",
			}, fmt.Errorf("authentication failed: %w", err)
		}
		
		if !authResp.Success {
			return &packet.HandshakeResponse{
				Success: false,
				Error:   authResp.Message,
			}, fmt.Errorf("authentication failed: %s", authResp.Message)
		}
		
		clientID = req.ClientID
		utils.Infof("ServerAuthHandler: authenticated registered client ID: %d", clientID)
	}

	// 更新 ClientConnection 的 ClientID
	conn.ClientID = clientID
	conn.Authenticated = true // 设置为已认证

	// 构造握手响应
	message := "Handshake successful"
	if req.ClientID == 0 {
		// 匿名客户端，在 Message 中返回分配的 ClientID
		message = fmt.Sprintf("Anonymous client authenticated, client_id=%d", clientID)
	}

	response := &packet.HandshakeResponse{
		Success: true,
		Message: message,
	}

	return response, nil
}

// GetClientConfig 获取客户端配置
func (h *ServerAuthHandler) GetClientConfig(conn *session.ClientConnection) (string, error) {
	// 获取客户端的所有映射
	mappings, err := h.cloudControl.GetClientPortMappings(conn.ClientID)
	if err != nil {
		return "", fmt.Errorf("failed to get client mappings: %w", err)
	}

	// 转换为客户端配置格式（使用共享的config.MappingConfig）
	configs := make([]config.MappingConfig, 0, len(mappings))
	for _, m := range mappings {
		// ✅ 处理源端映射：需要监听本地端口
		if m.SourceClientID == conn.ClientID {
			configs = append(configs, config.MappingConfig{
				MappingID:         m.ID,
				SecretKey:         m.SecretKey,
				LocalPort:         m.SourcePort,
				TargetHost:        m.TargetHost,
				TargetPort:        m.TargetPort,
				Protocol:          string(m.Protocol),
				EnableCompression: m.Config.EnableCompression,
				CompressionLevel:  m.Config.CompressionLevel,
				EnableEncryption:  m.Config.EnableEncryption,
				EncryptionMethod:  m.Config.EncryptionMethod,
				EncryptionKey:     m.Config.EncryptionKey,
			})
		}
		// ✅ 处理目标端映射：需要准备接收TunnelOpen请求
		// 目标端不需要监听端口，所以LocalPort设为0
		if m.TargetClientID == conn.ClientID && m.TargetClientID != m.SourceClientID {
			configs = append(configs, config.MappingConfig{
				MappingID:         m.ID,
				SecretKey:         m.SecretKey,
				LocalPort:         0, // 目标端不监听
				TargetHost:        m.TargetHost,
				TargetPort:        m.TargetPort,
				Protocol:          string(m.Protocol),
				EnableCompression: m.Config.EnableCompression,
				CompressionLevel:  m.Config.CompressionLevel,
				EnableEncryption:  m.Config.EnableEncryption,
				EncryptionMethod:  m.Config.EncryptionMethod,
				EncryptionKey:     m.Config.EncryptionKey,
			})
		}
	}

	// 序列化为JSON
	configData := struct {
		Mappings []config.MappingConfig `json:"mappings"`
	}{Mappings: configs}

	jsonBytes, err := json.Marshal(configData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	return string(jsonBytes), nil
}

// ServerTunnelHandler 服务器隧道处理器
type ServerTunnelHandler struct {
	cloudControl managers.CloudControlAPI
}

// NewServerTunnelHandler 创建隧道处理器
func NewServerTunnelHandler(cloudControl managers.CloudControlAPI) *ServerTunnelHandler {
	return &ServerTunnelHandler{
		cloudControl: cloudControl,
	}
}

// HandleTunnelOpen 处理隧道打开请求
func (h *ServerTunnelHandler) HandleTunnelOpen(conn *session.ClientConnection, req *packet.TunnelOpenRequest) error {
	utils.Debugf("ServerTunnelHandler: handling tunnel open for connection %s, MappingID=%s, TunnelID=%s",
		conn.ConnID, req.MappingID, req.TunnelID)

	// 验证 MappingID 和 SecretKey
	mapping, err := h.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		utils.Errorf("ServerTunnelHandler: mapping not found %s: %v", req.MappingID, err)
		return fmt.Errorf("mapping not found: %w", err)
	}

	// 验证 SecretKey
	if mapping.SecretKey != req.SecretKey {
		utils.Warnf("ServerTunnelHandler: invalid secret key for mapping %s from client %d", 
			req.MappingID, conn.ClientID)
		return fmt.Errorf("invalid secret key")
	}

	// 验证客户端是否有权限访问此映射（必须是目标客户端）
	if mapping.TargetClientID != conn.ClientID {
		utils.Warnf("ServerTunnelHandler: client %d not authorized for mapping %s (target: %d)", 
			conn.ClientID, req.MappingID, mapping.TargetClientID)
		return fmt.Errorf("not authorized")
	}

	utils.Infof("ServerTunnelHandler: tunnel opened successfully - TunnelID=%s, Mapping=%s, Client=%d", 
		req.TunnelID, req.MappingID, conn.ClientID)
	return nil
}

