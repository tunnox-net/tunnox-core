package server

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	cloudutils "tunnox-core/internal/cloud/utils"
	"tunnox-core/internal/config"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/security"
)

// ServerAuthHandler 服务器认证处理器
type ServerAuthHandler struct {
	cloudControl        managers.CloudControlAPI
	sessionMgr          *session.SessionManager       // 用于获取NodeID
	bruteForceProtector *security.BruteForceProtector // 暴力破解防护
	ipManager           *security.IPManager           // IP黑白名单管理
	rateLimiter         *security.RateLimiter         // 速率限制器
	secretKeyMgr        *security.SecretKeyManager    // SecretKey 管理器（挑战-响应认证）
}

// NewServerAuthHandler 创建认证处理器
func NewServerAuthHandler(
	cloudControl managers.CloudControlAPI,
	sessionMgr *session.SessionManager,
	bruteForceProtector *security.BruteForceProtector,
	ipManager *security.IPManager,
	rateLimiter *security.RateLimiter,
	secretKeyMgr *security.SecretKeyManager,
) *ServerAuthHandler {
	return &ServerAuthHandler{
		cloudControl:        cloudControl,
		sessionMgr:          sessionMgr,
		bruteForceProtector: bruteForceProtector,
		ipManager:           ipManager,
		rateLimiter:         rateLimiter,
		secretKeyMgr:        secretKeyMgr,
	}
}

// HandleHandshake 处理握手请求（挑战-响应认证）
//
// 认证流程：
// 1. 首次连接（ClientID == 0, Token == "new-client"）：
//   - 生成新的匿名客户端凭据
//   - 返回 ClientID 和 SecretKey（仅此一次）
//
// 2. 已有客户端认证（ClientID > 0）：
//   - 阶段一：客户端发送 ClientID（无 ChallengeResponse）
//     → 服务端返回随机挑战
//   - 阶段二：客户端发送 HMAC(SecretKey, Challenge)
//     → 服务端验证响应，完成认证
func (h *ServerAuthHandler) HandleHandshake(conn session.ControlConnectionInterface, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error) {
	// 提取IP地址
	remoteAddr := conn.GetRemoteAddr()
	var ip string
	if remoteAddr != nil {
		ip = extractIP(remoteAddr)
	}

	// 1. IP黑白名单检查（最高优先级）
	if h.ipManager != nil {
		if allowed, reason := h.ipManager.IsAllowed(ip); !allowed {
			corelog.Warnf("ServerAuthHandler: blocked IP %s (blacklisted): %s", ip, reason)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Access denied",
			}, fmt.Errorf("IP blacklisted: %s", reason)
		}
	}

	// 2. 暴力破解防护检查
	if h.bruteForceProtector != nil {
		if banned, reason := h.bruteForceProtector.IsBanned(ip); banned {
			corelog.Warnf("ServerAuthHandler: blocked banned IP %s: %s", ip, reason)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Access denied: too many failed attempts",
			}, fmt.Errorf("IP banned: %s", reason)
		}
	}

	// 3. 匿名客户端速率限制检查
	if req.ClientID == 0 && h.rateLimiter != nil {
		if !h.rateLimiter.AllowIP(ip) {
			corelog.Warnf("ServerAuthHandler: rate limit exceeded for IP %s", ip)
			return &packet.HandshakeResponse{
				Success: false,
				Error:   "Rate limit exceeded, please try again later",
			}, fmt.Errorf("rate limit exceeded for IP: %s", ip)
		}
	}

	// ============================================================
	// 4. 首次连接：分配新凭据
	// ============================================================
	isFirstConnection := req.ClientID == 0 && (req.Token == "new-client" || strings.HasPrefix(req.Token, "anonymous:"))
	if isFirstConnection {
		return h.handleFirstConnection(conn, req, ip)
	}

	// ============================================================
	// 5. 已有客户端：挑战-响应认证
	// ============================================================

	// 获取客户端配置（包含加密的 SecretKey）
	config, err := h.cloudControl.GetClientConfig(req.ClientID)
	if err != nil || config == nil {
		corelog.Warnf("ServerAuthHandler: client %d not found", req.ClientID)
		if h.bruteForceProtector != nil {
			h.bruteForceProtector.RecordFailure(ip)
		}
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Client not found",
		}, fmt.Errorf("client %d not found", req.ClientID)
	}

	// 检查凭据是否过期（匿名客户端 30 天未绑定用户）
	if config.IsExpired() {
		corelog.Warnf("ServerAuthHandler: client %d credentials expired", req.ClientID)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Credentials expired. Please register a new client.",
		}, fmt.Errorf("client %d credentials expired", req.ClientID)
	}

	// ============================================================
	// 5.1 阶段一：发送挑战
	// ============================================================
	if req.ChallengeResponse == "" {
		return h.handleChallengePhase1(conn, req, config, ip)
	}

	// ============================================================
	// 5.2 阶段二：验证响应
	// ============================================================
	return h.handleChallengePhase2(conn, req, config, ip)
}

// handleFirstConnection 处理首次连接（分配新凭据）
func (h *ServerAuthHandler) handleFirstConnection(conn session.ControlConnectionInterface, req *packet.HandshakeRequest, ip string) (*packet.HandshakeResponse, error) {
	// 生成新的匿名客户端凭据
	anonClient, err := h.cloudControl.GenerateAnonymousCredentials()
	if err != nil {
		corelog.Errorf("ServerAuthHandler: failed to generate credentials: %v", err)
		if h.bruteForceProtector != nil {
			h.bruteForceProtector.RecordFailure(ip)
		}
		return &packet.HandshakeResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	corelog.Infof("ServerAuthHandler: generated new client ID: %d", anonClient.ID)

	// 认证成功，清除失败记录
	if h.bruteForceProtector != nil {
		h.bruteForceProtector.RecordSuccess(ip)
	}

	// 更新连接状态
	conn.SetClientID(anonClient.ID)
	conn.SetAuthenticated(true)

	// 更新运行时状态
	h.updateClientRuntimeState(conn, anonClient.ID, req, ip)

	// 返回凭据（SecretKey 仅此一次返回）
	return &packet.HandshakeResponse{
		Success:   true,
		Message:   fmt.Sprintf("New client registered, client_id=%d", anonClient.ID),
		ClientID:  anonClient.ID,
		SecretKey: anonClient.SecretKeyPlaintext, // 仅返回一次的明文 SecretKey
	}, nil
}

// handleChallengePhase1 处理挑战阶段一：发送挑战
func (h *ServerAuthHandler) handleChallengePhase1(conn session.ControlConnectionInterface, req *packet.HandshakeRequest, config *models.ClientConfig, ip string) (*packet.HandshakeResponse, error) {
	// 检查 SecretKeyManager 是否可用
	if h.secretKeyMgr == nil {
		corelog.Errorf("ServerAuthHandler: SecretKeyManager not configured")
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Server configuration error",
		}, fmt.Errorf("SecretKeyManager not configured")
	}

	// 检查客户端是否有加密的 SecretKey
	if config.SecretKeyEncrypted == "" {
		// 兼容旧数据：如果没有加密的 SecretKey，使用明文 SecretKey 验证（回退到旧逻辑）
		if config.SecretKey != "" {
			corelog.Warnf("ServerAuthHandler: client %d using legacy plaintext SecretKey (needs migration)", req.ClientID)
			return h.handleLegacyAuth(conn, req, config, ip)
		}
		corelog.Errorf("ServerAuthHandler: client %d has no SecretKey configured", req.ClientID)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Client credentials not configured",
		}, fmt.Errorf("client %d has no SecretKey", req.ClientID)
	}

	// 生成随机挑战
	challenge, err := h.secretKeyMgr.GenerateChallenge()
	if err != nil {
		corelog.Errorf("ServerAuthHandler: failed to generate challenge: %v", err)
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Server error",
		}, err
	}

	// 存储挑战到连接（用于阶段二验证）
	conn.SetPendingChallenge(challenge)

	corelog.Debugf("ServerAuthHandler: sending challenge to client %d", req.ClientID)

	// 返回挑战
	return &packet.HandshakeResponse{
		Success:      false,         // 认证尚未完成
		NeedResponse: true,          // 需要客户端响应
		Challenge:    challenge,     // 发送挑战
		Message:      "Challenge sent, please respond with HMAC",
	}, nil
}

// handleChallengePhase2 处理挑战阶段二：验证响应
func (h *ServerAuthHandler) handleChallengePhase2(conn session.ControlConnectionInterface, req *packet.HandshakeRequest, config *models.ClientConfig, ip string) (*packet.HandshakeResponse, error) {
	// 获取待验证的挑战
	challenge := conn.GetPendingChallenge()
	if challenge == "" {
		corelog.Warnf("ServerAuthHandler: no pending challenge for client %d", req.ClientID)
		if h.bruteForceProtector != nil {
			h.bruteForceProtector.RecordFailure(ip)
		}
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "No pending challenge. Please start authentication again.",
		}, fmt.Errorf("no pending challenge")
	}

	// 清除挑战（防止重放攻击）
	conn.ClearPendingChallenge()

	// 验证响应
	if !h.secretKeyMgr.VerifyResponse(config.SecretKeyEncrypted, challenge, req.ChallengeResponse) {
		corelog.Warnf("ServerAuthHandler: challenge-response verification failed for client %d", req.ClientID)
		if h.bruteForceProtector != nil {
			h.bruteForceProtector.RecordFailure(ip)
		}
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Invalid credentials",
		}, fmt.Errorf("challenge-response verification failed")
	}

	corelog.Infof("ServerAuthHandler: client %d authenticated via challenge-response", req.ClientID)

	// 认证成功，清除失败记录
	if h.bruteForceProtector != nil {
		h.bruteForceProtector.RecordSuccess(ip)
	}

	// 更新连接状态
	conn.SetClientID(req.ClientID)
	conn.SetAuthenticated(true)

	// 更新运行时状态
	h.updateClientRuntimeState(conn, req.ClientID, req, ip)

	return &packet.HandshakeResponse{
		Success: true,
		Message: "Authentication successful",
	}, nil
}

// handleLegacyAuth 处理旧版认证（明文 SecretKey，用于数据迁移过渡期）
func (h *ServerAuthHandler) handleLegacyAuth(conn session.ControlConnectionInterface, req *packet.HandshakeRequest, config *models.ClientConfig, ip string) (*packet.HandshakeResponse, error) {
	// 旧版使用 Token 字段传递 SecretKey
	if config.SecretKey != req.Token {
		corelog.Warnf("ServerAuthHandler: legacy auth failed for client %d", req.ClientID)
		if h.bruteForceProtector != nil {
			h.bruteForceProtector.RecordFailure(ip)
		}
		return &packet.HandshakeResponse{
			Success: false,
			Error:   "Invalid credentials",
		}, fmt.Errorf("invalid secret key")
	}

	corelog.Infof("ServerAuthHandler: client %d authenticated via legacy SecretKey", req.ClientID)

	// 认证成功，清除失败记录
	if h.bruteForceProtector != nil {
		h.bruteForceProtector.RecordSuccess(ip)
	}

	// 更新连接状态
	conn.SetClientID(req.ClientID)
	conn.SetAuthenticated(true)

	// 更新运行时状态
	h.updateClientRuntimeState(conn, req.ClientID, req, ip)

	return &packet.HandshakeResponse{
		Success: true,
		Message: "Authentication successful (legacy mode - please update client)",
	}, nil
}

// updateClientRuntimeState 更新客户端运行时状态
func (h *ServerAuthHandler) updateClientRuntimeState(conn session.ControlConnectionInterface, clientID int64, req *packet.HandshakeRequest, ip string) {
	nodeID := h.sessionMgr.GetNodeID()
	if nodeID == "" {
		corelog.Warnf("ServerAuthHandler: NodeID not set in SessionManager")
	}

	connID := conn.GetConnID()
	protocol := conn.GetProtocol()
	if protocol == "" {
		protocol = req.Protocol
	}
	version := req.Version

	if err := h.cloudControl.ConnectClient(clientID, nodeID, connID, ip, protocol, version); err != nil {
		corelog.Warnf("ServerAuthHandler: failed to update runtime state for client %d: %v", clientID, err)
	}
}

// GetClientConfig 获取客户端配置
func (h *ServerAuthHandler) GetClientConfig(conn session.ControlConnectionInterface) (string, error) {
	// 获取客户端的所有映射
	mappings, err := h.cloudControl.GetClientPortMappings(conn.GetClientID())
	if err != nil {
		return "", fmt.Errorf("failed to get client mappings: %w", err)
	}

	// 转换为客户端配置格式（使用共享的config.MappingConfig）
	configs := make([]config.MappingConfig, 0, len(mappings))
	for _, m := range mappings {
		// ✅ 处理源端映射：需要监听本地端口
		if m.ListenClientID == conn.GetClientID() {
			// 从 ListenAddress 解析端口
			localPort := m.SourcePort
			if localPort == 0 && m.ListenAddress != "" {
				_, port, err := cloudutils.ParseListenAddress(m.ListenAddress)
				if err == nil {
					localPort = port
				}
			}

			cfg := config.MappingConfig{
				MappingID:         m.ID,
				SecretKey:         m.SecretKey,
				LocalPort:         localPort,
				TargetHost:        m.TargetHost,
				TargetPort:        m.TargetPort,
				Protocol:          string(m.Protocol),
				EnableCompression: m.Config.EnableCompression,
				CompressionLevel:  m.Config.CompressionLevel,
				EnableEncryption:  m.Config.EnableEncryption,
				EncryptionMethod:  m.Config.EncryptionMethod,
				EncryptionKey:     m.Config.EncryptionKey,
			}

			// SOCKS5 映射需要 TargetClientID
			if m.Protocol == models.ProtocolSOCKS {
				cfg.TargetClientID = m.TargetClientID
			}

			configs = append(configs, cfg)
		}
		// ✅ 处理目标端映射：需要准备接收TunnelOpen请求
		// 目标端不需要监听端口，所以LocalPort设为0
		if m.TargetClientID == conn.GetClientID() && m.TargetClientID != m.ListenClientID {
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

// extractIP 从net.Addr中提取IP地址
func extractIP(addr net.Addr) string {
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
		host, _, err := net.SplitHostPort(addr.String())
		if err == nil {
			return host
		}
		return addr.String()
	}
}
