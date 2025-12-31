package handler

import (
	"encoding/json"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	streamprocessor "tunnox-core/internal/stream/processor"
)

// TunnelHandler 隧道处理器接口
type TunnelHandler interface {
	HandleTunnelOpen(conn ControlConnectionInterface, req *packet.TunnelOpenRequest) error
}

// CloudControlAPI 云控管理接口
type CloudControlAPI interface {
	GetPortMapping(mappingID string) (*PortMapping, error)
}

// PortMapping 端口映射配置
type PortMapping struct {
	MappingID       string
	ListenClientID  int64
	TargetClientID  int64
	TargetHost      string
	TargetPort      int
	Protocol        string
	SecretKey       string
	Config          MappingConfig
}

// MappingConfig 映射配置
type MappingConfig struct {
	EnableCompression bool
	CompressionLevel  int
	EnableEncryption  bool
	EncryptionMethod  string
	EncryptionKey     string
	BandwidthLimit    int64
}

// TunnelBridgeHandler 隧道桥接处理器接口（用于调用bridge相关方法）
type TunnelBridgeHandler interface {
	HandleExistingBridge(connPacket *types.StreamPacket, conn *types.Connection, req *packet.TunnelOpenRequest, bridge interface{}) error
	HandleSourceBridge(conn *types.Connection, req *packet.TunnelOpenRequest, netConn net.Conn) error
	HandleTargetBridge(conn *types.Connection, req *packet.TunnelOpenRequest, netConn net.Conn) error
	CleanupTunnelFromControlConn(connPacket *types.StreamPacket, conn *types.Connection, req *packet.TunnelOpenRequest)
}

// TunnelRoutingTable 隧道路由表接口
type TunnelRoutingTable interface {
	LookupWaitingTunnel(ctx interface{}, tunnelID string) (interface{}, error)
}

// CrossNodeHandler 跨节点处理器接口
type CrossNodeHandler interface {
	HandleCrossNodeTargetConnection(req *packet.TunnelOpenRequest, conn *types.Connection, netConn net.Conn) error
}

// TunnelOpenManagerInterface SessionManager的隧道打开最小接口
// ⚠️ 临时设计：TunnelOpenHandler目前需要访问SessionManager的部分方法
// TODO: 在后续优化中逐步减少对SessionManager的依赖
type TunnelOpenManagerInterface interface {
	// 连接查询
	GetConnectionByConnID(connID string) (*types.Connection, bool)
	GetControlConnectionByConnID(connID string) ControlConnectionInterface
	GetControlConnectionByClientID(clientID int64) ControlConnectionInterface

	// Bridge查询（临时）
	GetTunnelBridge(tunnelID string) (interface{}, bool)

	// 辅助方法
	ExtractNetConn(conn *types.Connection) net.Conn
	ExtractClientID(stream streamprocessor.StreamProcessor, netConn net.Conn) int64

	// 旧架构访问（待优化）
	RemoveFromControlConnMap(connID string, clientConn ControlConnectionInterface)
}

// TunnelOpenHandler 隧道打开处理器
type TunnelOpenHandler struct {
	// 核心依赖
	tunnelHandler    TunnelHandler
	cloudControl     CloudControlAPI
	logger           corelog.Logger

	// 桥接处理器
	bridgeHandler    TunnelBridgeHandler

	// 跨节点处理
	tunnelRouting    TunnelRoutingTable
	crossNodeHandler CrossNodeHandler

	// ⚠️ 临时依赖（待后续优化移除）
	sessionManager   TunnelOpenManagerInterface
}

// TunnelOpenHandlerConfig 隧道打开处理器配置
type TunnelOpenHandlerConfig struct {
	TunnelHandler    TunnelHandler
	CloudControl     CloudControlAPI
	BridgeHandler    TunnelBridgeHandler
	TunnelRouting    TunnelRoutingTable
	CrossNodeHandler CrossNodeHandler
	SessionManager   TunnelOpenManagerInterface
	Logger           corelog.Logger
}

// NewTunnelOpenHandler 创建隧道打开处理器
func NewTunnelOpenHandler(config *TunnelOpenHandlerConfig) *TunnelOpenHandler {
	if config == nil {
		config = &TunnelOpenHandlerConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &TunnelOpenHandler{
		tunnelHandler:    config.TunnelHandler,
		cloudControl:     config.CloudControl,
		bridgeHandler:    config.BridgeHandler,
		tunnelRouting:    config.TunnelRouting,
		crossNodeHandler: config.CrossNodeHandler,
		sessionManager:   config.SessionManager,
		logger:           logger,
	}
}

// HandlePacket 处理隧道打开数据包
func (h *TunnelOpenHandler) HandlePacket(connPacket *types.StreamPacket) error {
	return h.handleTunnelOpen(connPacket)
}

// handleTunnelOpen 处理隧道打开请求
// 这个方法处理两种情况：
// 1. 源端客户端发起的隧道连接（需要创建bridge并通知目标端）
// 2. 目标端客户端响应的隧道连接（连接到已有的bridge）
func (h *TunnelOpenHandler) handleTunnelOpen(connPacket *types.StreamPacket) error {
	if h.tunnelHandler == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "tunnel handler not configured")
	}

	// 获取底层连接
	conn, exists := h.sessionManager.GetConnectionByConnID(connPacket.ConnectionID)
	if !exists || conn == nil {
		return coreerrors.Newf(coreerrors.CodeNotFound, "connection not found: %s", connPacket.ConnectionID)
	}

	// 解析隧道打开请求（从 Payload）
	req := &packet.TunnelOpenRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			h.logger.Errorf("Failed to parse tunnel open request: %v", err)
			h.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
				TunnelID: "",
				Success:  false,
				Error:    "invalid tunnel open request format: " + err.Error(),
			})
			return coreerrors.Wrap(err, coreerrors.CodeInvalidPacket, "invalid tunnel open request format")
		}
	}

	// 设置 mappingID
	h.setMappingIDOnConnection(conn, req.MappingID)

	// 检查是否已有bridge（目标端连接或源端重连）
	bridge, exists := h.sessionManager.GetTunnelBridge(req.TunnelID)
	if exists {
		return h.bridgeHandler.HandleExistingBridge(connPacket, conn, req, bridge)
	}

	// 检查跨节点路由
	if h.tunnelRouting != nil {
		_, err := h.tunnelRouting.LookupWaitingTunnel(nil, req.TunnelID)
		if err == nil {
			netConn := h.sessionManager.ExtractNetConn(conn)
			return h.crossNodeHandler.HandleCrossNodeTargetConnection(req, conn, netConn)
		}
		// 忽略 TunnelNotFound 和 TunnelExpired 错误
	}

	// 获取或创建控制连接
	clientConn := h.findOrCreateControlConnection(connPacket, conn, req)
	if clientConn == nil {
		return coreerrors.Newf(coreerrors.CodeNotFound, "control connection not found: %s", connPacket.ConnectionID)
	}

	// 调用隧道处理器
	if err := h.tunnelHandler.HandleTunnelOpen(clientConn, req); err != nil {
		h.logger.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
		h.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    err.Error(),
		})
		return err
	}

	// 清理控制连接映射
	h.sessionManager.RemoveFromControlConnMap(connPacket.ConnectionID, clientConn)

	h.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	// 设置映射ID
	h.setMappingIDAfterAuth(conn, req.MappingID, clientConn)

	// 处理源端/目标端连接
	netConn := h.sessionManager.ExtractNetConn(conn)
	isSourceClient := h.isSourceClient(conn, req, clientConn, netConn)

	if isSourceClient {
		if err := h.bridgeHandler.HandleSourceBridge(conn, req, netConn); err != nil {
			return err
		}
	} else {
		if err := h.bridgeHandler.HandleTargetBridge(conn, req, netConn); err != nil {
			return err
		}
	}

	// 清理
	h.bridgeHandler.CleanupTunnelFromControlConn(connPacket, conn, req)

	return coreerrors.New(coreerrors.CodeTunnelModeSwitch, "tunnel source connected, switching to stream mode")
}

// setMappingIDOnConnection 设置连接的 mappingID
func (h *TunnelOpenHandler) setMappingIDOnConnection(conn *types.Connection, mappingID string) {
	if mappingID == "" || conn == nil || conn.Stream == nil {
		return
	}
	reader := conn.Stream.GetReader()
	if mappingConn, ok := reader.(interface {
		GetClientID() int64
		SetMappingID(mappingID string)
	}); ok {
		clientID := mappingConn.GetClientID()
		if clientID > 0 {
			mappingConn.SetMappingID(mappingID)
		}
	}
}

// findOrCreateControlConnection 查找或创建控制连接
func (h *TunnelOpenHandler) findOrCreateControlConnection(
	connPacket *types.StreamPacket,
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
) ControlConnectionInterface {
	clientConn := h.sessionManager.GetControlConnectionByConnID(connPacket.ConnectionID)
	if clientConn != nil {
		return clientConn
	}

	if conn == nil || conn.Stream == nil {
		h.logger.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
		return nil
	}

	// 尝试从 Stream 直接获取 clientID
	var clientID int64
	if streamWithClientID, ok := conn.Stream.(interface {
		GetClientID() int64
	}); ok {
		clientID = streamWithClientID.GetClientID()
	} else {
		type streamProcessorGetter interface {
			GetStreamProcessor() interface {
				GetClientID() int64
				GetConnectionID() string
				GetMappingID() string
			}
		}
		if adapter, ok := conn.Stream.(streamProcessorGetter); ok {
			streamProc := adapter.GetStreamProcessor()
			if streamProc != nil {
				clientID = streamProc.GetClientID()
			}
		}
	}

	// 如果获取到 clientID，尝试通过 clientID 查找控制连接
	if clientID > 0 {
		clientConn = h.sessionManager.GetControlConnectionByClientID(clientID)
		if clientConn != nil {
			return clientConn
		}
	}

	// 尝试创建临时控制连接
	reader := conn.Stream.GetReader()
	if tempConn, ok := reader.(interface {
		CanCreateTemporaryControlConn() bool
		GetClientID() int64
	}); ok && tempConn.CanCreateTemporaryControlConn() {
		// ⚠️ 注意：这里直接创建了ControlConnection，可能需要通过SessionManager创建
		// 但为了保持与原代码一致，暂时这样实现
		// TODO: 考虑添加CreateTemporaryControlConnection方法到TunnelOpenManagerInterface
		newConn := &controlConnectionImpl{
			connID:        conn.ID,
			stream:        conn.Stream,
			clientID:      clientID,
			authenticated: false,
		}

		if clientID > 0 {
			newConn.clientID = clientID
			newConn.authenticated = true
		} else {
			tempClientID := tempConn.GetClientID()
			if tempClientID > 0 {
				newConn.clientID = tempClientID
				newConn.authenticated = true
			}
		}
		return newConn
	}

	h.logger.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
	h.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  false,
		Error:    "connection not found or not authenticated",
	})
	return nil
}

// setMappingIDAfterAuth 认证后设置映射ID
func (h *TunnelOpenHandler) setMappingIDAfterAuth(conn *types.Connection, mappingID string, clientConn ControlConnectionInterface) {
	if mappingID == "" || !clientConn.IsAuthenticated() || clientConn.GetClientID() <= 0 {
		return
	}
	if conn == nil || conn.Stream == nil {
		return
	}
	reader := conn.Stream.GetReader()
	if mappingConn, ok := reader.(interface {
		SetMappingID(mappingID string)
	}); ok {
		mappingConn.SetMappingID(mappingID)
	}
}

// isSourceClient 判断是否为源端客户端
func (h *TunnelOpenHandler) isSourceClient(
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	clientConn ControlConnectionInterface,
	netConn net.Conn,
) bool {
	if h.cloudControl == nil || req.MappingID == "" {
		return false
	}
	mapping, err := h.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		return false
	}
	connClientID := h.sessionManager.ExtractClientID(conn.Stream, netConn)
	if connClientID == 0 && clientConn != nil && clientConn.IsAuthenticated() {
		connClientID = clientConn.GetClientID()
	}
	return connClientID == mapping.ListenClientID
}

// sendTunnelOpenResponseDirect 直接发送隧道打开响应（使用types.Connection）
func (h *TunnelOpenHandler) sendTunnelOpenResponseDirect(conn *types.Connection, resp *packet.TunnelOpenAckResponse) error {
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

// controlConnectionImpl 临时控制连接实现（用于创建临时连接）
type controlConnectionImpl struct {
	connID        string
	stream        streamprocessor.StreamProcessor
	clientID      int64
	authenticated bool
}

func (c *controlConnectionImpl) GetConnID() string {
	return c.connID
}

func (c *controlConnectionImpl) GetClientID() int64 {
	return c.clientID
}

func (c *controlConnectionImpl) GetUserID() string {
	return ""
}

func (c *controlConnectionImpl) IsAuthenticated() bool {
	return c.authenticated
}

func (c *controlConnectionImpl) GetStream() streamprocessor.StreamProcessor {
	return c.stream
}
