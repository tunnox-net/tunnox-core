package server

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

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

	// 4. 认证客户端
	if req.ClientID == 0 && strings.HasPrefix(req.Token, "anonymous:") {
		// 首次匿名握手：生成新凭据
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
		// 注册客户端或匿名客户端重新认证（使用ClientID+Token）
		// 对于匿名客户端，Token是SecretKey
		// 对于注册客户端，Token是AuthToken

		// 先获取客户端信息以判断类型
		client, err := h.cloudControl.GetClient(req.ClientID)
		if err != nil || client == nil {
			// ✅ 客户端不存在：如果是匿名客户端（Token不是"anonymous:"开头且长度合理），自动生成新客户端
			// 这通常发生在服务端重启导致数据丢失，或客户端配置的ID无效时
			if !strings.HasPrefix(req.Token, "anonymous:") && len(req.Token) >= 16 {
				// 可能是匿名客户端的SecretKey，自动生成新匿名客户端
				utils.Warnf("ServerAuthHandler: client %d not found, auto-generating new anonymous client (likely server restart or invalid config)", req.ClientID)
				anonClient, genErr := h.cloudControl.GenerateAnonymousCredentials()
				if genErr != nil {
					authError = fmt.Errorf("failed to generate new anonymous client: %w", genErr)
					utils.Errorf("ServerAuthHandler: failed to generate anonymous credentials: %v", genErr)
					if h.bruteForceProtector != nil {
						h.bruteForceProtector.RecordFailure(ip)
					}
					return &packet.HandshakeResponse{
						Success: false,
						Error:   "Client not found and failed to generate new client",
					}, authError
				}
				clientID = anonClient.ID
				utils.Infof("ServerAuthHandler: auto-generated new anonymous client ID: %d (replacing invalid client %d)", clientID, req.ClientID)
				// 注意：这里不直接返回，继续执行后续逻辑以返回新凭据
			} else {
				// 注册客户端或首次匿名握手，返回错误
				authError = fmt.Errorf("client not found")
				utils.Errorf("ServerAuthHandler: client %d not found: %v", req.ClientID, err)
				if h.bruteForceProtector != nil {
					h.bruteForceProtector.RecordFailure(ip)
				}
				return &packet.HandshakeResponse{
					Success: false,
					Error:   "Client not found",
				}, authError
			}
		} else {

			// 根据客户端类型验证凭据
			var authResp *models.AuthResponse
			if client.Type == models.ClientTypeAnonymous {
				// 匿名客户端：验证SecretKey
				if client.SecretKey != req.Token {
					authError = fmt.Errorf("invalid secret key")
					utils.Warnf("ServerAuthHandler: invalid secret key for anonymous client %d", req.ClientID)
					if h.bruteForceProtector != nil {
						h.bruteForceProtector.RecordFailure(ip)
					}
					return &packet.HandshakeResponse{
						Success: false,
						Error:   "Invalid credentials",
					}, authError
				}
				// SecretKey正确，更新状态
				authResp = &models.AuthResponse{
					Success: true,
					Client:  client,
					Message: "Anonymous client re-authenticated",
				}
				utils.Infof("ServerAuthHandler: anonymous client %d re-authenticated with SecretKey", req.ClientID)
				clientID = req.ClientID
			} else {
				// 注册客户端：使用Authenticate验证AuthCode
				authResp, err = h.cloudControl.Authenticate(&models.AuthRequest{
					ClientID: req.ClientID,
					AuthCode: req.Token,
				})
				if err != nil {
					authError = err
					utils.Errorf("ServerAuthHandler: authentication failed for client %d: %v", req.ClientID, err)
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
					if h.bruteForceProtector != nil {
						h.bruteForceProtector.RecordFailure(ip)
					}
					return &packet.HandshakeResponse{
						Success: false,
						Error:   authResp.Message,
					}, authError
				}
				utils.Infof("ServerAuthHandler: registered client %d authenticated", req.ClientID)
				clientID = req.ClientID
			}
		}
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
	response := &packet.HandshakeResponse{
		Success: true,
		Message: "Handshake successful",
	}

	// ✅ 匿名客户端首次握手（ClientID==0）或自动生成新客户端：返回分配的凭据
	// 判断条件：首次握手（ClientID==0）或新生成的客户端（clientID != req.ClientID）
	if (req.ClientID == 0 && strings.HasPrefix(req.Token, "anonymous:")) || (clientID != req.ClientID && clientID > 0) {
		// 获取匿名客户端信息（包含SecretKey）
		anonClient, err := h.cloudControl.GetClient(clientID)
		if err != nil {
			utils.Warnf("ServerAuthHandler: failed to get anonymous client %d: %v", clientID, err)
			response.Message = fmt.Sprintf("Anonymous client authenticated, client_id=%d", clientID)
		} else if anonClient == nil {
			utils.Warnf("ServerAuthHandler: anonymous client %d not found (nil)", clientID)
			response.Message = fmt.Sprintf("Anonymous client authenticated, client_id=%d", clientID)
		} else {
			response.ClientID = clientID
			response.SecretKey = anonClient.SecretKey
			if clientID != req.ClientID {
				// 自动生成的新客户端，提示客户端更新配置
				response.Message = fmt.Sprintf("Client ID updated: %d -> %d (please update your config)", req.ClientID, clientID)
				utils.Infof("ServerAuthHandler: returning new credentials for auto-generated anonymous client %d (replacing invalid client %d, SecretKey=%s)", clientID, req.ClientID, anonClient.SecretKey)
			} else {
				response.Message = fmt.Sprintf("Anonymous client authenticated, client_id=%d", clientID)
				utils.Infof("ServerAuthHandler: returning credentials for anonymous client %d (SecretKey=%s)", clientID, anonClient.SecretKey)
			}
		}
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
		// ✅ 统一使用 ListenClientID（向后兼容：如果为 0 则使用 SourceClientID）
		listenClientID := m.ListenClientID
		if listenClientID == 0 {
			listenClientID = m.SourceClientID
		}

		// ✅ 处理源端映射：需要监听本地端口
		if listenClientID == conn.ClientID {
			configs = append(configs, config.MappingConfig{
				MappingID:         m.ID,
				SecretKey:         m.SecretKey,
				LocalPort:         m.SourcePort, // 从 ListenAddress 解析，或直接使用 SourcePort
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
		if m.TargetClientID == conn.ClientID && m.TargetClientID != listenClientID {
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
	utils.Infof("ServerTunnelHandler: tunnel open - ConnID=%s, MappingID=%s, TunnelID=%s, SecretKey=%v, ResumeToken=%v",
		conn.ConnID, req.MappingID, req.TunnelID, req.SecretKey != "", req.ResumeToken != "")

	// ✨ Phase 2: 优先级0 - 检查是否是恢复请求
	if req.ResumeToken != "" {
		return h.resumeTunnel(conn, req)
	}

	// 1. 验证ClientID
	if conn.ClientID == 0 {
		utils.Warnf("ServerTunnelHandler: client not authenticated for connection %s", conn.ConnID)
		return fmt.Errorf("client not authenticated")
	}

	var authMethod string
	var mapping *models.PortMapping

	// 2. 优先级1: MappingID验证（新设计，推荐）
	if req.MappingID != "" && req.SecretKey == "" {
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

		// 验证SecretKey
		if err := h.validateWithSecretKey(req.SecretKey, portMapping); err != nil {
			utils.Warnf("ServerTunnelHandler: secret key validation failed for mapping %s",
				req.MappingID)
			return fmt.Errorf("invalid secret key")
		}

		// ✅ 验证客户端是否有权限使用这个mapping
		// 检查SourceClientID或TargetClientID是否匹配
		if portMapping.SourceClientID != conn.ClientID && portMapping.TargetClientID != conn.ClientID {
			utils.Warnf("ServerTunnelHandler: client %d not authorized for mapping %s (source=%d, target=%d)",
				conn.ClientID, req.MappingID, portMapping.SourceClientID, portMapping.TargetClientID)
			return fmt.Errorf("client not authorized for this mapping")
		}

		utils.Infof("ServerTunnelHandler: tunnel opened with secret key - TunnelID=%s, MappingID=%s, Client=%d",
			req.TunnelID, req.MappingID, conn.ClientID)

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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Phase 2: 隧道恢复逻辑
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// resumeTunnel 恢复中断的隧道
//
// 当客户端发送带有ResumeToken的TunnelOpenRequest时调用。
//
// 流程：
// 1. 验证ResumeToken并加载隧道状态
// 2. 恢复隧道元数据（MappingID等）
// 3. 恢复缓冲区状态（如果启用序列号）
// 4. 返回成功，客户端可继续传输
func (h *ServerTunnelHandler) resumeTunnel(conn *session.ClientConnection, req *packet.TunnelOpenRequest) error {
	utils.Infof("ServerTunnelHandler: attempting to resume tunnel %s for client %d",
		req.TunnelID, conn.ClientID)

	// 需要SessionManager支持
	sessionMgr, ok := h.cloudControl.(interface {
		ValidateTunnelResumeToken(string) (*session.TunnelState, error)
	})
	if !ok {
		utils.Errorf("ServerTunnelHandler: session manager does not support tunnel resumption")
		return fmt.Errorf("tunnel resumption not supported")
	}

	// 1. 验证ResumeToken并加载隧道状态
	tunnelState, err := sessionMgr.ValidateTunnelResumeToken(req.ResumeToken)
	if err != nil {
		utils.Warnf("ServerTunnelHandler: failed to validate resume token for tunnel %s: %v",
			req.TunnelID, err)
		return fmt.Errorf("invalid resume token: %w", err)
	}

	// 2. 验证TunnelID匹配
	if tunnelState.TunnelID != req.TunnelID {
		utils.Warnf("ServerTunnelHandler: tunnel ID mismatch (token=%s, request=%s)",
			tunnelState.TunnelID, req.TunnelID)
		return fmt.Errorf("tunnel ID mismatch")
	}

	// 3. （可选）验证MappingID权限
	if h.connCodeService != nil && tunnelState.MappingID != "" {
		_, err := h.connCodeService.ValidateMapping(tunnelState.MappingID, conn.ClientID)
		if err != nil {
			utils.Warnf("ServerTunnelHandler: mapping validation failed during resume for %s: %v",
				tunnelState.MappingID, err)
			return fmt.Errorf("mapping validation failed: %w", err)
		}
	}

	// 4. 记录恢复成功
	utils.Infof("ServerTunnelHandler: tunnel resumed successfully - TunnelID=%s, MappingID=%s, Client=%d",
		tunnelState.TunnelID, tunnelState.MappingID, conn.ClientID)

	// TODO: 如果需要恢复缓冲区状态，在这里实现
	// if tunnelState.BufferedPackets != nil && len(tunnelState.BufferedPackets) > 0 {
	//     // 恢复发送缓冲区
	//     session.RestoreToSendBuffer(tunnelConn.sendBuffer, tunnelState.BufferedPackets)
	// }

	return nil
}
