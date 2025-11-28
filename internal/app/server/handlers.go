package server

import (
	"encoding/json"
	"fmt"
	"net"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/config"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/security"
	"tunnox-core/internal/utils"
)

// ServerAuthHandler 服务器认证处理器
type ServerAuthHandler struct {
	cloudControl        managers.CloudControlAPI
	sessionMgr          *session.SessionManager       // 用于获取NodeID
	bruteForceProtector *security.BruteForceProtector // 暴力破解防护
	ipManager           *security.IPManager           // IP黑白名单管理
	rateLimiter         *security.RateLimiter         // 速率限制器
}

// NewServerAuthHandler 创建认证处理器
func NewServerAuthHandler(cloudControl managers.CloudControlAPI, sessionMgr *session.SessionManager, bruteForceProtector *security.BruteForceProtector, ipManager *security.IPManager, rateLimiter *security.RateLimiter) *ServerAuthHandler {
	return &ServerAuthHandler{
		cloudControl:        cloudControl,
		sessionMgr:          sessionMgr,
		bruteForceProtector: bruteForceProtector,
		ipManager:           ipManager,
		rateLimiter:         rateLimiter,
	}
}

// HandleHandshake 处理握手请求
func (h *ServerAuthHandler) HandleHandshake(conn *session.ClientConnection, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	utils.Debugf("ServerAuthHandler: handling handshake for connection %s, ClientID=%d", conn.ConnID, req.ClientID)

	// 提取IP地址
	ip := extractIP(conn.RemoteAddr)

	// 1. IP黑白名单检查（最高优先级）
	if h.ipManager != nil {
		if allowed, reason := h.ipManager.IsAllowed(ip); !allowed {
			utils.Warnf("ServerAuthHandler: blocked IP %s (blacklisted): %s", ip, reason)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Access denied",
			}, fmt.Errorf("IP blacklisted: %s", reason)
		}
	}

	// 2. 暴力破解防护检查
	if h.bruteForceProtector != nil {
		if banned, reason := h.bruteForceProtector.IsBanned(ip); banned {
			utils.Warnf("ServerAuthHandler: blocked banned IP %s: %s", ip, reason)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Access denied: too many failed attempts",
			}, fmt.Errorf("IP banned: %s", reason)
		}
	}

	// 3. 匿名客户端速率限制检查
	if req.ClientID == 0 && h.rateLimiter != nil {
		if !h.rateLimiter.AllowIP(ip) {
			utils.Warnf("ServerAuthHandler: rate limit exceeded for IP %s", ip)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Rate limit exceeded, please try again later",
			}, fmt.Errorf("rate limit exceeded for IP: %s", ip)
		}
	}

	var clientID int64
	var authError error

	// 4. 处理匿名客户端（ClientID == 0 表示匿名）
	if req.ClientID == 0 {
		// 生成匿名客户端凭据
		anonClient, err := h.cloudControl.GenerateAnonymousCredentials()
		if err != nil {
			authError = err
			utils.Errorf("ServerAuthHandler: failed to generate anonymous credentials: %v", err)
			// 记录失败
			if h.bruteForceProtector != nil {
				h.bruteForceProtector.RecordFailure(ip)
			}
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
			authError = err
			utils.Errorf("ServerAuthHandler: authentication failed for client %d: %v", req.ClientID, err)
			// 记录失败
			if h.bruteForceProtector != nil {
				h.bruteForceProtector.RecordFailure(ip)
			}
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Authentication failed",
			}, fmt.Errorf("authentication failed: %w", err)
		}

		if !authResp.Success {
			authError = fmt.Errorf("authentication failed: %s", authResp.Message)
			// 记录失败
			if h.bruteForceProtector != nil {
				h.bruteForceProtector.RecordFailure(ip)
			}
			return &packet.HandshakeResponse{
				Success: false,
				Error:   authResp.Message,
			}, authError
		}

		clientID = req.ClientID
		utils.Infof("ServerAuthHandler: authenticated registered client ID: %d", clientID)
	}

	// 5. 认证成功，清除失败记录
	if h.bruteForceProtector != nil {
		h.bruteForceProtector.RecordSuccess(ip)
	}

	// 6. 更新 ClientConnection 的 ClientID
	conn.ClientID = clientID
	conn.Authenticated = true // 设置为已认证

	// 7. 更新客户端状态为在线
	nodeID := h.sessionMgr.GetNodeID()
	if nodeID == "" {
		utils.Warnf("ServerAuthHandler: NodeID not set in SessionManager, using empty string")
	}
	if err := h.cloudControl.UpdateClientStatus(clientID, models.ClientStatusOnline, nodeID); err != nil {
		utils.Warnf("ServerAuthHandler: failed to update client %d status to online: %v", clientID, err)
		// 不返回错误，只记录警告，握手仍然成功
	} else {
		utils.Infof("ServerAuthHandler: client %d status updated to online on node %s", clientID, nodeID)
	}

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
	cloudControl    managers.CloudControlAPI
	connCodeService *services.ConnectionCodeService
}

// NewServerTunnelHandler 创建隧道处理器
func NewServerTunnelHandler(cloudControl managers.CloudControlAPI, connCodeService *services.ConnectionCodeService) *ServerTunnelHandler {
	return &ServerTunnelHandler{
		cloudControl:    cloudControl,
		connCodeService: connCodeService,
	}
}

// HandleTunnelOpen 处理隧道打开请求
//
// 验证优先级：
//  1. MappingID - 验证隧道映射权限（新设计）
//  2. SecretKey - 传统密钥验证（向后兼容）
func (h *ServerTunnelHandler) HandleTunnelOpen(conn *session.ClientConnection, req *packet.TunnelOpenRequest) error {
	utils.Debugf("ServerTunnelHandler: handling tunnel open for connection %s, MappingID=%s, TunnelID=%s",
		conn.ConnID, req.MappingID, req.TunnelID)

	// 1. 验证ClientID
	if conn.ClientID == 0 {
		utils.Warnf("ServerTunnelHandler: client not authenticated for connection %s", conn.ConnID)
		return fmt.Errorf("client not authenticated")
	}

	var authMethod string
	var mapping *models.TunnelMapping

	// 2. 优先级1: MappingID验证（新设计，推荐）
	if req.MappingID != "" {
		authMethod = "mapping_id"

		// 验证隧道映射权限
		if h.connCodeService == nil {
			utils.Errorf("ServerTunnelHandler: connection code service not available")
			return fmt.Errorf("connection code service not available")
		}

		validatedMapping, err := h.connCodeService.ValidateMapping(req.MappingID, conn.ClientID)
		if err != nil {
			utils.Warnf("ServerTunnelHandler: mapping validation failed for %s (client %d): %v",
				req.MappingID, conn.ClientID, err)
			return fmt.Errorf("mapping validation failed: %w", err)
		}

		mapping = validatedMapping

		// 记录映射使用（统计）
		if err := h.connCodeService.RecordMappingUsage(req.MappingID); err != nil {
			utils.Warnf("ServerTunnelHandler: failed to record mapping usage for %s: %v", req.MappingID, err)
			// 不返回错误，只记录警告
		}

		utils.Infof("ServerTunnelHandler: tunnel opened with mapping - TunnelID=%s, MappingID=%s, Client=%d",
			req.TunnelID, req.MappingID, conn.ClientID)

	} else if req.SecretKey != "" {
		// 优先级2: SecretKey验证（向后兼容，用于旧版API调用）
		authMethod = "secret_key"

		// 从旧的PortMapping获取（保持向后兼容）
		portMapping, err := h.cloudControl.GetPortMapping(req.MappingID)
		if err != nil {
			utils.Errorf("ServerTunnelHandler: port mapping not found %s: %v", req.MappingID, err)
			return fmt.Errorf("mapping not found: %w", err)
		}

		if err := h.validateWithSecretKey(req.SecretKey, portMapping); err != nil {
			utils.Warnf("ServerTunnelHandler: secret key validation failed for mapping %s",
				req.MappingID)
			return fmt.Errorf("invalid secret key")
		}

		utils.Infof("ServerTunnelHandler: tunnel opened with secret key - TunnelID=%s, MappingID=%s",
			req.TunnelID, req.MappingID)

	} else {
		// 无有效凭证
		utils.Warnf("ServerTunnelHandler: no valid credentials provided for connection %s",
			conn.ConnID)
		return fmt.Errorf("authentication required: either mapping_id or secret_key must be provided")
	}

	utils.Infof("ServerTunnelHandler: tunnel opened successfully - TunnelID=%s, AuthMethod=%s, Client=%d",
		req.TunnelID, authMethod, conn.ClientID)

	// 存储mapping信息到conn（如果需要后续使用）
	_ = mapping // 暂时未使用，但保留以备后续扩展

	return nil
}

// validateWithSecretKey 使用秘钥验证（传统方式，向后兼容）
func (h *ServerTunnelHandler) validateWithSecretKey(secretKey string, mapping *models.PortMapping) error {
	if mapping.SecretKey != secretKey {
		return fmt.Errorf("invalid secret key")
	}
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// extractIP 从net.Addr中提取IP地址
func extractIP(addr interface{}) string {
	if addr == nil {
		return "unknown"
	}

	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP.String()
	case *net.UDPAddr:
		return v.IP.String()
	default:
		// 尝试解析字符串格式（如 "192.168.1.1:12345"）
		if addrStr, ok := addr.(net.Addr); ok {
			host, _, err := net.SplitHostPort(addrStr.String())
			if err == nil {
				return host
			}
			return addrStr.String()
		}
		return fmt.Sprintf("%v", addr)
	}
}
