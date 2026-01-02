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

	var clientID int64
	var authError error
	var generatedClient *models.Client // 保存生成的客户端，避免后续重复查询

	// 4. 认证客户端
	// 统一认证模型：
	// - ClientID == 0 且 Token == "new-client"：首次连接，分配新凭据
	// - ClientID > 0 且 Token 是 SecretKey：使用持久化凭据认证
	isFirstConnection := req.ClientID == 0 && (req.Token == "new-client" || strings.HasPrefix(req.Token, "anonymous:"))
	if isFirstConnection {
		// 首次握手：生成新凭据
		anonClient, err := h.cloudControl.GenerateAnonymousCredentials()
		if err != nil {
			authError = err
			corelog.Errorf("ServerAuthHandler: failed to generate credentials: %v", err)
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
		generatedClient = anonClient // 保存生成的客户端，供后续返回凭据使用
		corelog.Infof("ServerAuthHandler: generated new client ID: %d", clientID)
	} else {
		// 注册客户端或匿名客户端重新认证（使用ClientID+Token）
		// 对于匿名客户端，Token是SecretKey
		// 对于注册客户端，Token是AuthToken

		// 先获取客户端信息以判断类型
		client, err := h.cloudControl.GetClient(req.ClientID)
		if err != nil || client == nil {
			// 客户端不存在：如果是持久化凭据重新认证（Token不是"new-client"且长度合理），自动生成新客户端
			// 这通常发生在服务端重启导致数据丢失，或客户端配置的ID无效时
			if req.Token != "new-client" && !strings.HasPrefix(req.Token, "anonymous:") && len(req.Token) >= 16 {
				// 可能是客户端的SecretKey，自动生成新客户端
				corelog.Warnf("ServerAuthHandler: client %d not found, auto-generating new anonymous client (likely server restart or invalid config)", req.ClientID)
				anonClient, genErr := h.cloudControl.GenerateAnonymousCredentials()
				if genErr != nil {
					authError = fmt.Errorf("failed to generate new anonymous client: %w", genErr)
					corelog.Errorf("ServerAuthHandler: failed to generate anonymous credentials: %v", genErr)
					if h.bruteForceProtector != nil {
						h.bruteForceProtector.RecordFailure(ip)
					}
					return &packet.HandshakeResponse{
						Success: false,
						Error:   "Client not found and failed to generate new client",
					}, authError
				}
				clientID = anonClient.ID
				// 注意：这里不直接返回，继续执行后续逻辑以返回新凭据
			} else {
				// 注册客户端或首次匿名握手，返回错误
				authError = fmt.Errorf("client not found")
				corelog.Errorf("ServerAuthHandler: client %d not found: %v", req.ClientID, err)
				if h.bruteForceProtector != nil {
					h.bruteForceProtector.RecordFailure(ip)
				}
				return &packet.HandshakeResponse{
					Success: false,
					Error:   "Client not found",
				}, authError
			}
		} else {
			// 统一使用 SecretKey 验证（匿名客户端和注册客户端都使用 SecretKey）
			// AuthCode 已废弃，现在使用连接码机制
			if client.SecretKey != req.Token {
				authError = fmt.Errorf("invalid secret key")
				corelog.Warnf("ServerAuthHandler: invalid secret key for client %d (type=%s)", req.ClientID, client.Type)
				if h.bruteForceProtector != nil {
					h.bruteForceProtector.RecordFailure(ip)
				}
				return &packet.HandshakeResponse{
					Success: false,
					Error:   "Invalid credentials",
				}, authError
			}
			clientID = req.ClientID
			corelog.Infof("ServerAuthHandler: client %d authenticated via SecretKey", clientID)
		}
	}

	// 5. 认证成功，清除失败记录
	if h.bruteForceProtector != nil {
		h.bruteForceProtector.RecordSuccess(ip)
	}

	// 6. 更新 ClientConnection 的 ClientID
	conn.SetClientID(clientID)
	conn.SetAuthenticated(true) // 设置为已认证

	// 7. 客户端连接：更新完整运行时状态（包含 connID）
	nodeID := h.sessionMgr.GetNodeID()
	if nodeID == "" {
		corelog.Warnf("ServerAuthHandler: NodeID not set in SessionManager, using empty string")
	}
	connID := conn.GetConnID()
	protocol := conn.GetProtocol()
	if protocol == "" {
		protocol = req.Protocol // 使用请求中的协议作为备选
	}
	version := req.Version
	if err := h.cloudControl.ConnectClient(clientID, nodeID, connID, ip, protocol, version); err != nil {
		corelog.Warnf("ServerAuthHandler: failed to connect client %d: %v", clientID, err)
		// 不返回错误，只记录警告，握手仍然成功
	}

	// 构造握手响应
	response := &packet.HandshakeResponse{
		Success: true,
		Message: "Handshake successful",
	}

	// 首次握手或重新分配凭据：返回分配的凭据
	// 判断条件：
	// 1. 首次握手（ClientID==0）- 此时 clientID 是新生成的
	// 2. 新生成的客户端（clientID != req.ClientID 且 clientID > 0）
	// 3. 重新认证 - 需要返回 SecretKey 供客户端更新
	isNewClient := clientID != req.ClientID && clientID > 0

	corelog.Debugf("ServerAuthHandler: handshake response check - isFirstConnection=%v, isNewClient=%v, req.ClientID=%d, clientID=%d",
		isFirstConnection, isNewClient, req.ClientID, clientID)

	if isFirstConnection || isNewClient {
		// 使用已生成的客户端信息（避免重复查询可能失败的问题）
		var anonClient *models.Client
		if generatedClient != nil {
			// 首次连接时，直接使用生成的客户端
			anonClient = generatedClient
		} else {
			// 重新分配凭据时（isNewClient），需要查询
			var err error
			anonClient, err = h.cloudControl.GetClient(clientID)
			if err != nil {
				corelog.Warnf("ServerAuthHandler: failed to get anonymous client %d: %v", clientID, err)
			}
		}

		if anonClient == nil {
			corelog.Warnf("ServerAuthHandler: anonymous client %d not found (nil)", clientID)
			response.Message = fmt.Sprintf("Anonymous client authenticated, client_id=%d", clientID)
		} else {
			response.ClientID = clientID
			response.SecretKey = anonClient.SecretKey
			corelog.Infof("ServerAuthHandler: returning ClientID=%d and SecretKey in handshake response", clientID)
			if clientID != req.ClientID {
				// 自动生成的新客户端，提示客户端更新配置
				response.Message = fmt.Sprintf("Client ID updated: %d -> %d (please update your config)", req.ClientID, clientID)
			} else {
				response.Message = fmt.Sprintf("Anonymous client authenticated, client_id=%d", clientID)
			}
		}
	} else {
		corelog.Debugf("ServerAuthHandler: not returning ClientID/SecretKey - isFirstConnection=%v, isNewClient=%v",
			isFirstConnection, isNewClient)
	}

	return response, nil
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
