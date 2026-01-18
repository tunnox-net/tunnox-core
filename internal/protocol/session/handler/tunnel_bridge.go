package handler

import (
	"encoding/json"
	"net"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// TunnelConnection 统一接口连接（用于桥接）
type TunnelConnection interface {
	GetConnID() string
	GetNetConn() net.Conn
	GetStream() stream.PackageStreamer
	GetClientID() int64
	GetMappingID() string
	GetTunnelID() string
}

// TunnelBridge 隧道桥接接口
type TunnelBridge interface {
	SetSourceConnection(conn TunnelConnection)
	SetTargetConnection(conn TunnelConnection)
}

// BridgeManager 桥接管理器接口
type BridgeManager interface {
	StartSourceBridge(req *packet.TunnelOpenRequest, sourceConn net.Conn, sourceStream stream.PackageStreamer) error
	HandleCrossNodeTargetConnection(req *packet.TunnelOpenRequest, conn *types.Connection, netConn net.Conn) error
	BroadcastTunnelOpen(req *packet.TunnelOpenRequest, targetClientID int64) error
}

// TunnelBridgeManagerInterface SessionManager的隧道桥接最小接口
type TunnelBridgeManagerInterface interface {
	// Bridge查询
	GetTunnelBridge(tunnelID string) (TunnelBridge, bool)

	// 辅助方法
	ExtractNetConn(conn *types.Connection) net.Conn
	ExtractClientID(stream stream.PackageStreamer, netConn net.Conn) int64
	CreateTunnelConnection(connID string, netConn net.Conn, streamProc stream.PackageStreamer, clientID int64, mappingID string, tunnelID string) TunnelConnection

	// 控制连接管理（临时）
	RemoveFromControlConnMap(connID string)
	RemoveFromConnMap(connID string)

	// 跨节点处理
	GetBridgeManager() BridgeManager
	GetCloudControl() CloudControlAPI
}

// TunnelBridgeHandlerImpl 隧道桥接处理器实现
type TunnelBridgeHandlerImpl struct {
	sessionManager TunnelBridgeManagerInterface
	cloudControl   CloudControlAPI
	bridgeManager  BridgeManager
	logger         corelog.Logger
}

// TunnelBridgeHandlerConfig 隧道桥接处理器配置
type TunnelBridgeHandlerConfig struct {
	SessionManager TunnelBridgeManagerInterface
	CloudControl   CloudControlAPI
	BridgeManager  BridgeManager
	Logger         corelog.Logger
}

// NewTunnelBridgeHandler 创建隧道桥接处理器
func NewTunnelBridgeHandler(config *TunnelBridgeHandlerConfig) *TunnelBridgeHandlerImpl {
	if config == nil {
		config = &TunnelBridgeHandlerConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &TunnelBridgeHandlerImpl{
		sessionManager: config.SessionManager,
		cloudControl:   config.CloudControl,
		bridgeManager:  config.BridgeManager,
		logger:         logger,
	}
}

// HandleExistingBridge 处理已有bridge的隧道连接（目标端连接或源端重连）
func (h *TunnelBridgeHandlerImpl) HandleExistingBridge(
	connPacket *types.StreamPacket,
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	bridgeInterface interface{},
) error {
	bridge, ok := bridgeInterface.(TunnelBridge)
	if !ok {
		return coreerrors.New(coreerrors.CodeInvalidParam, "invalid bridge type")
	}

	// ✅ 隧道连接（有 MappingID）不应该被注册为控制连接
	// 由于现在在 Handshake 中已经通过 ConnectionType 识别，这里只需要清理可能的误注册
	if req.MappingID != "" {
		h.sessionManager.RemoveFromControlConnMap(connPacket.ConnectionID)
	}

	h.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	netConn := h.sessionManager.ExtractNetConn(conn)
	// 如果无法提取 net.Conn，尝试从 Stream 创建数据转发器（通过接口抽象）
	if netConn == nil && conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		writer := conn.Stream.GetWriter()
		_ = reader
		_ = writer
	}

	// ✅ 判断是源端还是目标端连接，更新对应的连接
	// 通过 cloudControl 获取映射配置，判断 clientID 是源端还是目标端
	netConn = h.sessionManager.ExtractNetConn(conn)
	var isSourceClient bool
	if h.cloudControl != nil && req.MappingID != "" {
		mapping, err := h.cloudControl.GetPortMapping(req.MappingID)
		if err == nil {
			// 从连接中获取 clientID（使用 extractClientID 函数，支持多种方式）
			connClientID := h.sessionManager.ExtractClientID(conn.Stream, netConn)
			// 如果 extractClientID 返回 0，稍后从控制连接获取（clientConn 在后面定义）
			isSourceClient = (connClientID == mapping.ListenClientID)
		}
	}

	// 创建统一接口连接
	var connStream stream.PackageStreamer
	var connID string
	if conn != nil {
		connStream = conn.Stream
		connID = conn.ID
	}
	clientID := h.sessionManager.ExtractClientID(connStream, netConn)
	tunnelConn := h.sessionManager.CreateTunnelConnection(connID, netConn, connStream, clientID, req.MappingID, req.TunnelID)

	if isSourceClient {
		// 源端重连，更新 sourceConn
		bridge.SetSourceConnection(tunnelConn)
	} else {
		// 目标端连接
		bridge.SetTargetConnection(tunnelConn)
	}

	// ✅ 切换到流模式（通过接口调用，协议无关）
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if streamModeConn, ok := reader.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			streamModeConn.SetStreamMode(true)
		}
	}

	// ✅ 判断是否应该保留在 connMap（通过接口判断，协议无关）
	shouldKeep := false
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if keepConn, ok := reader.(interface {
			ShouldKeepInConnMap() bool
		}); ok {
			shouldKeep = keepConn.ShouldKeepInConnMap()
		}
	}

	if !shouldKeep && req.MappingID != "" {
		h.sessionManager.RemoveFromConnMap(connPacket.ConnectionID)
	}

	return coreerrors.New(coreerrors.CodeTunnelModeSwitch, "tunnel connected to existing bridge, switching to stream mode")
}

// HandleSourceBridge 处理源端连接：创建新的bridge
func (h *TunnelBridgeHandlerImpl) HandleSourceBridge(
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	netConn net.Conn,
) error {
	// 源端连接：创建新的bridge
	var sourceConn net.Conn
	var sourceStream stream.PackageStreamer
	if conn != nil {
		sourceConn = netConn // 可能为 nil（某些协议不支持 net.Conn）
		sourceStream = conn.Stream
		// 如果 net.Conn 为 nil，尝试从 Stream 创建数据转发器（通过接口抽象）
		if netConn == nil && sourceStream != nil {
			reader := sourceStream.GetReader()
			writer := sourceStream.GetWriter()
			_ = reader
			_ = writer
		}
	}

	// ✅ 切换到流模式（通过接口调用，协议无关）
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if streamModeConn, ok := reader.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			streamModeConn.SetStreamMode(true)
		}
	}

	bridgeManager := h.sessionManager.GetBridgeManager()
	if bridgeManager == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "bridge manager not configured")
	}

	if err := bridgeManager.StartSourceBridge(req, sourceConn, sourceStream); err != nil {
		h.logger.Errorf("Tunnel[%s]: failed to start bridge: %v", req.TunnelID, err)
		return err
	}
	return nil
}

// HandleTargetBridge 处理目标端连接：查找已存在的bridge并设置target连接
func (h *TunnelBridgeHandlerImpl) HandleTargetBridge(
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
	netConn net.Conn,
) error {
	bridge, exists := h.sessionManager.GetTunnelBridge(req.TunnelID)
	if !exists {
		// 本地未找到 Bridge，尝试跨节点转发
		bridgeManager := h.sessionManager.GetBridgeManager()
		if bridgeManager != nil {
			if err := bridgeManager.HandleCrossNodeTargetConnection(req, conn, netConn); err != nil {
				h.logger.Errorf("Tunnel[%s]: cross-node forwarding failed: %v", req.TunnelID, err)
				return coreerrors.Wrapf(err, coreerrors.CodeNotFound, "bridge not found for tunnel %s", req.TunnelID)
			}
			return nil
		}
		return coreerrors.Newf(coreerrors.CodeNotFound, "bridge not found for tunnel %s", req.TunnelID)
	}

	// 创建统一接口连接
	var connStream stream.PackageStreamer
	var connID string
	if conn != nil {
		connStream = conn.Stream
		connID = conn.ID
	}
	clientID := h.sessionManager.ExtractClientID(connStream, netConn)
	tunnelConn := h.sessionManager.CreateTunnelConnection(connID, netConn, connStream, clientID, req.MappingID, req.TunnelID)

	// 设置目标端连接
	bridge.SetTargetConnection(tunnelConn)

	// 切换到流模式（通过接口调用，协议无关）
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if streamModeConn, ok := reader.(interface {
			SetStreamMode(streamMode bool)
		}); ok {
			streamModeConn.SetStreamMode(true)
		}
	}
	return nil
}

// CleanupTunnelFromControlConn 清理控制连接中的隧道引用
func (h *TunnelBridgeHandlerImpl) CleanupTunnelFromControlConn(
	connPacket *types.StreamPacket,
	conn *types.Connection,
	req *packet.TunnelOpenRequest,
) {
	if req.MappingID == "" {
		return
	}

	h.sessionManager.RemoveFromControlConnMap(connPacket.ConnectionID)

	// ✅ 判断是否应该保留在 connMap（通过接口判断，协议无关）
	shouldKeep := false
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		if keepConn, ok := reader.(interface {
			ShouldKeepInConnMap() bool
		}); ok {
			shouldKeep = keepConn.ShouldKeepInConnMap()
		}
	}

	if !shouldKeep && req.MappingID != "" {
		h.sessionManager.RemoveFromConnMap(connPacket.ConnectionID)
	}
}

// sendTunnelOpenResponseDirect 直接发送隧道打开响应（使用types.Connection）
func (h *TunnelBridgeHandlerImpl) sendTunnelOpenResponseDirect(conn *types.Connection, resp *packet.TunnelOpenAckResponse) error {
	if conn == nil || conn.Stream == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "connection or stream is nil")
	}

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
