package handler

import (
	"context"
	"encoding/json"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session/registry"
	streamprocessor "tunnox-core/internal/stream/processor"
)

// ControlConnectionInterface 控制连接接口（与session包兼容）
type ControlConnectionInterface interface {
	GetConnID() string
	GetClientID() int64
	GetUserID() string
	IsAuthenticated() bool
	GetStream() streamprocessor.StreamProcessor
}

// AuthHandler 认证处理器接口
type AuthHandler interface {
	HandleHandshake(conn ControlConnectionInterface, req *packet.HandshakeRequest) (*packet.HandshakeResponse, error)
	GetClientConfig(conn ControlConnectionInterface) (string, error)
}

// SessionManagerInterface SessionManager的最小接口（用于HandshakeHandler）
// ⚠️ 临时设计：HandshakeHandler目前需要访问SessionManager的部分方法
// TODO: 在后续优化中逐步减少对SessionManager的依赖
type SessionManagerInterface interface {
	// 连接查询
	GetConnectionByConnID(connID string) (*types.Connection, bool)
	GetControlConnectionByConnID(connID string) ControlConnectionInterface

	// 创建和注册方法
	CreateControlConnection(connID string, stream streamprocessor.StreamProcessor, remoteAddr net.Addr, protocol string) ControlConnectionInterface
	RegisterControlConnection(conn ControlConnectionInterface)
	RegisterConnectionState(ctx context.Context, connID string, clientID int64, nodeID string, protocol string, connType string) error

	// 协议相关
	GetNodeID() string
	Ctx() context.Context

	// 旧架构访问（待优化）
	UpdateClientIDIndex(clientID int64, conn ControlConnectionInterface) error
	HandleReconnect(clientID int64, newConnID string) error
}

// HandshakeHandler 握手处理器
type HandshakeHandler struct {
	// 新架构依赖
	clientRegistry *registry.ClientRegistry
	logger         corelog.Logger

	// ⚠️ 临时依赖（待后续优化移除）
	sessionManager SessionManagerInterface
	authHandler    AuthHandler
}

// HandshakeHandlerConfig 握手处理器配置
type HandshakeHandlerConfig struct {
	ClientRegistry *registry.ClientRegistry
	AuthHandler    AuthHandler
	SessionManager SessionManagerInterface
	Logger         corelog.Logger
}

// NewHandshakeHandler 创建握手处理器
func NewHandshakeHandler(config *HandshakeHandlerConfig) *HandshakeHandler {
	if config == nil {
		config = &HandshakeHandlerConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &HandshakeHandler{
		clientRegistry: config.ClientRegistry,
		authHandler:    config.AuthHandler,
		sessionManager: config.SessionManager,
		logger:         logger,
	}
}

// HandlePacket 处理握手数据包
func (h *HandshakeHandler) HandlePacket(connPacket *types.StreamPacket) error {
	return h.handleHandshake(connPacket)
}

// handleHandshake 处理握手请求
func (h *HandshakeHandler) handleHandshake(connPacket *types.StreamPacket) error {
	if h.authHandler == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "auth handler not configured")
	}

	// 解析握手请求（从 Payload）
	req := &packet.HandshakeRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			h.logger.Errorf("Failed to parse handshake request: %v", err)
			return coreerrors.Wrap(err, coreerrors.CodeInvalidPacket, "invalid handshake request format")
		}
	}

	isControlConnection := req.ConnectionType != "tunnel"
	if req.ConnectionType == "" {
		isControlConnection = true
	}

	var clientConn ControlConnectionInterface
	if isControlConnection {
		// 获取或创建控制连接
		existingConn := h.sessionManager.GetControlConnectionByConnID(connPacket.ConnectionID)
		if existingConn != nil {
			clientConn = existingConn
		} else {
			// 获取底层连接信息
			conn, exists := h.sessionManager.GetConnectionByConnID(connPacket.ConnectionID)
			if !exists || conn == nil {
				return coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", connPacket.ConnectionID)
			}

			// 创建控制连接
			enforcedProtocol := conn.Protocol
			if enforcedProtocol == "" {
				enforcedProtocol = "tcp" // 默认协议
			}
			// 从连接中提取远程地址
			var remoteAddr net.Addr
			if conn.RawConn != nil {
				remoteAddr = conn.RawConn.RemoteAddr()
			}

			// ⚠️ 通过SessionManager创建ControlConnection（避免循环依赖）
			newConn := h.sessionManager.CreateControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
			h.sessionManager.RegisterControlConnection(newConn)
			clientConn = newConn
		}
	} else {
		conn, exists := h.sessionManager.GetConnectionByConnID(connPacket.ConnectionID)
		if !exists || conn == nil {
			return coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", connPacket.ConnectionID)
		}
		enforcedProtocol := conn.Protocol
		if enforcedProtocol == "" {
			enforcedProtocol = "tcp"
		}
		var remoteAddr net.Addr
		if conn.RawConn != nil {
			remoteAddr = conn.RawConn.RemoteAddr()
		}

		// ⚠️ 通过SessionManager创建ControlConnection
		newConn := h.sessionManager.CreateControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
		// ⚠️ 注意：这里是隧道连接的握手，不应该注册为控制连接
		// 但原代码这样做了，暂时保持一致
		h.sessionManager.RegisterControlConnection(newConn)
		clientConn = newConn
	}

	// 调用 authHandler 处理
	resp, err := h.authHandler.HandleHandshake(clientConn, req)
	if err != nil {
		h.logger.Errorf("Handshake failed for connection %s: %v", connPacket.ConnectionID, err)
		// 发送失败响应
		h.sendHandshakeResponse(clientConn, &packet.HandshakeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return err
	}

	// 发送成功响应
	if err := h.sendHandshakeResponse(clientConn, resp); err != nil {
		h.logger.Errorf("Failed to send handshake response: %v", err)
		return err
	}

	// 调试日志
	h.logger.Infof("Handshake: after sendHandshakeResponse - isControlConnection=%v, isAuthenticated=%v, clientID=%d, connID=%s",
		isControlConnection, clientConn.IsAuthenticated(), clientConn.GetClientID(), connPacket.ConnectionID)

	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		h.logger.Infof("Handshake: updating clientIDIndexMap for client %d, isControlConnection=%v, isAuthenticated=%v",
			clientConn.GetClientID(), isControlConnection, clientConn.IsAuthenticated())

		// ⚠️ 处理重连逻辑（委托给SessionManager）
		if err := h.sessionManager.HandleReconnect(clientConn.GetClientID(), clientConn.GetConnID()); err != nil {
			h.logger.Warnf("Failed to handle reconnect: %v", err)
		}

		// 更新 clientIDIndex
		if err := h.sessionManager.UpdateClientIDIndex(clientConn.GetClientID(), clientConn); err != nil {
			h.logger.Warnf("Failed to update clientID index: %v", err)
		}

		// ✅ 登记客户端位置到 Redis（用于跨节点查询）
		conn, exists := h.sessionManager.GetConnectionByConnID(connPacket.ConnectionID)
		protocol := "tcp"
		if exists && conn != nil && conn.Protocol != "" {
			protocol = conn.Protocol
		}
		if err := h.sessionManager.RegisterConnectionState(
			h.sessionManager.Ctx(),
			connPacket.ConnectionID,
			clientConn.GetClientID(),
			h.sessionManager.GetNodeID(),
			protocol,
			"control",
		); err != nil {
			h.logger.Warnf("Failed to register connection state: %v", err)
		} else {
			h.logger.Infof("Registered client %d location (node=%s, connID=%s)",
				clientConn.GetClientID(), h.sessionManager.GetNodeID(), connPacket.ConnectionID)
		}

		// 协议特定的握手后处理（通过统一的回调接口）
		if exists && conn != nil && conn.Stream != nil {
			reader := conn.Stream.GetReader()
			if handshakeHandler, ok := reader.(interface{ OnHandshakeComplete(clientID int64) }); ok {
				handshakeHandler.OnHandshakeComplete(clientConn.GetClientID())
			}
		}
	} else {
		h.logger.Warnf("Handshake: NOT updating clientIDIndexMap - isControlConnection=%v, isAuthenticated=%v, clientID=%d",
			isControlConnection, clientConn.IsAuthenticated(), clientConn.GetClientID())
	}

	h.logger.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, clientConn.GetClientID())

	// ✅ 握手成功后，主动推送客户端的映射配置
	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		go h.pushConfigToClient(clientConn)
	}

	return nil
}

// pushConfigToClient 推送配置给客户端
func (h *HandshakeHandler) pushConfigToClient(conn ControlConnectionInterface) {
	if h.authHandler == nil {
		h.logger.Warnf("HandshakeHandler: authHandler is nil, cannot push config to client %d", conn.GetClientID())
		return
	}

	configBody, err := h.authHandler.GetClientConfig(conn)
	if err != nil {
		h.logger.Errorf("HandshakeHandler: failed to get config for client %d: %v", conn.GetClientID(), err)
		return
	}

	// 检查配置是否为空（没有映射）
	if configBody == "" || configBody == `{"mappings":[]}` || configBody == `{"mappings":null}` {
		h.logger.Debugf("HandshakeHandler: skipping empty config push to client %d (no mappings yet)", conn.GetClientID())
		return
	}

	// 发送配置
	responseCmd := &packet.CommandPacket{
		CommandType: packet.ConfigSet,
		CommandBody: configBody,
	}

	responsePacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: responseCmd,
	}

	stream := conn.GetStream()
	if stream == nil {
		h.logger.Errorf("HandshakeHandler: stream is nil for client %d", conn.GetClientID())
		return
	}

	if _, err := stream.WritePacket(responsePacket, true, 0); err != nil {
		h.logger.Errorf("HandshakeHandler: failed to push config to client %d: %v", conn.GetClientID(), err)
		return
	}

	h.logger.Infof("HandshakeHandler: pushed config to client %d (%d bytes)", conn.GetClientID(), len(configBody))
}

// sendHandshakeResponse 发送握手响应
func (h *HandshakeHandler) sendHandshakeResponse(conn ControlConnectionInterface, resp *packet.HandshakeResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal handshake response")
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.HandshakeResp,
		Payload:    respData,
	}

	// 发送响应
	h.logger.Infof("Sending handshake response to connection %s, ClientID=%d", conn.GetConnID(), conn.GetClientID())

	stream := conn.GetStream()
	if stream == nil {
		h.logger.Errorf("Failed to send handshake response: stream is nil for connection %s", conn.GetConnID())
		return coreerrors.New(coreerrors.CodeConnectionError, "stream is nil")
	}

	// 调试：检查 stream 类型
	h.logger.Infof("sendHandshakeResponse: stream type=%T, connID=%s", stream, conn.GetConnID())
	type streamProcessorGetter interface {
		GetStreamProcessor() interface {
			GetClientID() int64
			GetConnectionID() string
			GetMappingID() string
		}
	}
	if adapter, ok := stream.(streamProcessorGetter); ok {
		sp := adapter.GetStreamProcessor()
		if sp != nil {
			h.logger.Infof("sendHandshakeResponse: adapter contains streamProcessor type=%T, connID=%s, clientID=%d", sp, conn.GetConnID(), sp.GetClientID())
		}
	}

	if _, err := stream.WritePacket(respPacket, true, 0); err != nil {
		h.logger.Errorf("Failed to write handshake response to connection %s: %v", conn.GetConnID(), err)
		return err
	}

	h.logger.Infof("Handshake response written successfully to connection %s", conn.GetConnID())
	return nil
}

