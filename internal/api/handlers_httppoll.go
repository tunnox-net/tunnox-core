package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

const (
	httppollMaxRequestSize = 1024 * 1024 // 1MB
	httppollDefaultTimeout = 30          // 默认 30 秒
	httppollMaxTimeout     = 60          // 最大 60 秒
)

// HTTPPushRequest HTTP 推送请求
type HTTPPushRequest struct {
	Data      string `json:"data"`      // Base64 编码的数据流
	Seq       uint64 `json:"seq"`       // 序列号
	Timestamp int64  `json:"timestamp"` // 时间戳
}

// HTTPPushResponse HTTP 推送响应
type HTTPPushResponse struct {
	Success   bool   `json:"success"`
	Ack       uint64 `json:"ack"`       // 确认的序列号
	Timestamp int64  `json:"timestamp"` // 时间戳
}

// HTTPPollResponse HTTP 轮询响应
type HTTPPollResponse struct {
	Success   bool   `json:"success"`
	Data      string `json:"data,omitempty"`    // Base64 编码的数据流
	Seq       uint64 `json:"seq,omitempty"`     // 序列号
	Timeout   bool   `json:"timeout,omitempty"` // 是否超时
	Timestamp int64  `json:"timestamp"`         // 时间戳
}

// SessionManagerWithConnection 扩展的 SessionManager 接口
type SessionManagerWithConnection interface {
	SessionManager
	CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
	GetConnection(connID string) (*types.Connection, bool)
}

// getSessionManagerWithConnection 获取支持 CreateConnection 的 SessionManager
func getSessionManagerWithConnection(sm SessionManager) SessionManagerWithConnection {
	// 尝试直接类型断言
	if smc, ok := sm.(SessionManagerWithConnection); ok {
		return smc
	}
	// 尝试通过接口组合获取
	type createConn interface {
		CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
		GetConnection(connID string) (*types.Connection, bool)
	}
	if cc, ok := sm.(createConn); ok {
		return &sessionManagerAdapter{
			SessionManager: sm,
			createConn:     cc,
		}
	}
	return nil
}

// sessionManagerAdapter 适配器
type sessionManagerAdapter struct {
	SessionManager
	createConn interface {
		CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error)
		GetConnection(connID string) (*types.Connection, bool)
	}
}

func (a *sessionManagerAdapter) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	return a.createConn.CreateConnection(reader, writer)
}

func (a *sessionManagerAdapter) GetConnection(connID string) (*types.Connection, bool) {
	return a.createConn.GetConnection(connID)
}

// handleHTTPPush 处理客户端推送数据
// POST /tunnox/v1/push
func (s *ManagementAPIServer) handleHTTPPush(w http.ResponseWriter, r *http.Request) {
	// 1. 获取并解码 X-Tunnel-Package（必须）
	packageHeader := r.Header.Get("X-Tunnel-Package")
	if packageHeader == "" {
		s.respondError(w, http.StatusBadRequest, "missing X-Tunnel-Package header")
		return
	}

	// 2. 解码控制包
	pkg, err := httppoll.DecodeTunnelPackage(packageHeader)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode tunnel package: %v", err))
		return
	}

	// 3. 获取 ConnectionID（必须）
	connID := pkg.ConnectionID
	if connID == "" {
		s.respondError(w, http.StatusBadRequest, "missing connection_id in tunnel package")
		return
	}

	// 4. 验证 ConnectionID 格式
	if !httppoll.ValidateConnectionID(connID) {
		s.respondError(w, http.StatusBadRequest, "invalid connection_id format")
		return
	}

	// 5. 获取或创建连接
	if s.httppollRegistry == nil {
		s.httppollRegistry = httppoll.NewConnectionRegistry()
	}

	streamProcessor := s.httppollRegistry.Get(connID)
	if streamProcessor == nil {
		// 新连接：创建并注册（在创建时就注册，避免查找时序问题）
		streamProcessor = s.createHTTPLongPollingConnection(connID, pkg, r.Context())
		if streamProcessor == nil {
			s.respondError(w, http.StatusServiceUnavailable, "Failed to create connection")
			return
		}
		s.httppollRegistry.Register(connID, streamProcessor)
		utils.Infof("HTTP long polling: [HANDLE_PUSH] created and registered new connection, connID=%s, clientID=%d, mappingID=%s", connID, pkg.ClientID, pkg.MappingID)
	} else {
		// 已存在的连接：更新 clientID 和 mappingID（如果需要）
		if pkg.ClientID > 0 {
			streamProcessor.UpdateClientID(pkg.ClientID)
		}
		if pkg.MappingID != "" {
			streamProcessor.SetMappingID(pkg.MappingID)
		}
	}

	// 6. 处理 Push 请求
	var pushReq HTTPPushRequest
	if err := json.NewDecoder(r.Body).Decode(&pushReq); err == nil {
		// 处理数据流
		if err := streamProcessor.PushData(pushReq.Data); err != nil {
			utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to push data: %v, connID=%s", err, connID)
			s.respondError(w, http.StatusServiceUnavailable, "Connection closed")
			return
		}
	}

	// 7. 处理控制包（如果有 type 字段）
	var responsePkg *httppoll.TunnelPackage
	if pkg.Type != "" {
		responsePkg = s.handleControlPackage(streamProcessor, pkg)
	}

	// 8. 返回响应（如果有控制包响应，放在 X-Tunnel-Package 中）
	if responsePkg != nil {
		// 设置响应包的连接信息
		responsePkg.ConnectionID = connID
		responsePkg.ClientID = streamProcessor.GetClientID()
		responsePkg.MappingID = streamProcessor.GetMappingID()
		responsePkg.TunnelType = pkg.TunnelType
		encodedPkg, err := httppoll.EncodeTunnelPackage(responsePkg)
		if err == nil {
			w.Header().Set("X-Tunnel-Package", encodedPkg)
		}
	}

	// 9. 返回 ACK
	resp := HTTPPushResponse{
		Success:   true,
		Ack:       pushReq.Seq,
		Timestamp: time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleHTTPPoll 处理客户端长轮询
// GET /tunnox/v1/poll?timeout=30
func (s *ManagementAPIServer) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
	// 1. 获取并解码 X-Tunnel-Package（必须）
	packageHeader := r.Header.Get("X-Tunnel-Package")
	if packageHeader == "" {
		s.respondError(w, http.StatusBadRequest, "missing X-Tunnel-Package header")
		return
	}

	// 2. 解码控制包
	pkg, err := httppoll.DecodeTunnelPackage(packageHeader)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode tunnel package: %v", err))
		return
	}

	// 3. 获取 ConnectionID（必须）
	connID := pkg.ConnectionID
	if connID == "" {
		s.respondError(w, http.StatusBadRequest, "missing connection_id in tunnel package")
		return
	}

	// 4. 验证 ConnectionID 格式
	if !httppoll.ValidateConnectionID(connID) {
		s.respondError(w, http.StatusBadRequest, "invalid connection_id format")
		return
	}

	// 5. 查找连接（必须存在，因为 Push 请求已经创建并注册）
	if s.httppollRegistry == nil {
		s.respondError(w, http.StatusNotFound, "connection not found, please reconnect")
		return
	}

	streamProcessor := s.httppollRegistry.Get(connID)
	if streamProcessor == nil {
		// 连接不存在，可能是 Poll 请求先于 Push 请求到达（不应该发生）
		// 或者连接已关闭，返回错误让客户端重新建立连接
		utils.Warnf("HTTP long polling: [HANDLE_POLL] connection not found, connID=%s", connID)
		s.respondError(w, http.StatusNotFound, "connection not found, please reconnect")
		return
	}

	// 6. 更新 clientID 和 mappingID（如果需要，与 Push 请求保持一致）
	if pkg.ClientID > 0 {
		streamProcessor.UpdateClientID(pkg.ClientID)
	}
	if pkg.MappingID != "" {
		streamProcessor.SetMappingID(pkg.MappingID)
	}

	// 7. 解析超时参数
	timeout := httppollDefaultTimeout
	if t := r.URL.Query().Get("timeout"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 && parsed <= httppollMaxTimeout {
			timeout = parsed
		}
	}

	// 8. 长轮询：等待数据
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	base64Data, responsePkg, err := streamProcessor.HandlePollRequest(ctx)
	if err == context.DeadlineExceeded {
		// 超时，返回空响应
		resp := HTTPPollResponse{
			Success:   true,
			Timeout:   true,
			Timestamp: time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
		return
	}
	if err != nil {
		// 对于 context canceled 或 EOF，返回超时响应而不是错误
		if err == context.Canceled || err == io.EOF {
			utils.Debugf("HTTP long polling: [HANDLE_POLL] %v, returning timeout response, connID=%s", err, connID)
			resp := HTTPPollResponse{
				Success:   true,
				Timeout:   true,
				Timestamp: time.Now().Unix(),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		// 其他错误才返回 500
		utils.Errorf("HTTP long polling: [HANDLE_POLL] PollData failed: %v, connID=%s", err, connID)
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 9. 检查是否有控制包响应（如 TunnelOpenAck）
	if responsePkg != nil {
		encodedPkg, err := httppoll.EncodeTunnelPackage(responsePkg)
		if err == nil {
			w.Header().Set("X-Tunnel-Package", encodedPkg)
		}
	}

	// 10. 返回响应
	resp := HTTPPollResponse{
		Success:   true,
		Data:      base64Data,
		Seq:       0, // 序列号暂时不使用
		Timeout:   false,
		Timestamp: time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// httppollStreamAdapter HTTP 长轮询流适配器
// 用于将 ServerStreamProcessor 适配为 io.Reader/io.Writer，以便在 SessionManager 中注册
type httppollStreamAdapter struct {
	streamProcessor *httppoll.ServerStreamProcessor
}

func (a *httppollStreamAdapter) Read(p []byte) (int, error) {
	// HTTP 长轮询是无状态的，不通过 Read 读取数据
	// 数据通过 Push 请求和 Poll 响应处理
	return 0, io.EOF
}

func (a *httppollStreamAdapter) Write(p []byte) (int, error) {
	// HTTP 长轮询是无状态的，不通过 Write 写入数据
	// 数据通过 Push 请求和 Poll 响应处理
	return len(p), nil
}

func (a *httppollStreamAdapter) GetConnectionID() string {
	return a.streamProcessor.GetConnectionID()
}

// createHTTPLongPollingConnection 创建 HTTP 长轮询连接
func (s *ManagementAPIServer) createHTTPLongPollingConnection(connID string, pkg *httppoll.TunnelPackage, ctx context.Context) *httppoll.ServerStreamProcessor {
	// 1. 获取 SessionManager
	if s.sessionMgr == nil {
		utils.Errorf("HTTP long polling: SessionManager not available")
		return nil
	}

	// 2. 确定连接类型
	connType := pkg.TunnelType
	if connType == "" {
		// 根据包类型推断
		if pkg.Type == "TunnelOpen" {
			connType = "data"
		} else {
			connType = "control"
		}
	}

	// 3. 使用 server 的 context 而不是请求的 context，避免请求结束后 context 被取消
	serverCtx := s.Ctx()
	if serverCtx == nil {
		serverCtx = context.Background()
	}

	clientID := pkg.ClientID
	if clientID == 0 {
		// 握手阶段，clientID 为 0
		clientID = 0
	}

	// 4. 创建 HTTP 长轮询流处理器（使用新的 ServerStreamProcessor）
	streamProcessor := httppoll.NewServerStreamProcessor(serverCtx, connID, clientID, pkg.MappingID)

	// 5. 在 SessionManager 中注册连接（用于握手等流程）
	// 创建适配器，让 ServerStreamProcessor 可以作为 reader/writer 传递给 CreateConnection
	adapter := &httppollStreamAdapter{streamProcessor: streamProcessor}
	sessionMgrWithConn := getSessionManagerWithConnection(s.sessionMgr)
	if sessionMgrWithConn != nil {
		_, err := sessionMgrWithConn.CreateConnection(adapter, adapter)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to create connection in SessionManager: %v", err)
			// 即使注册失败，也返回 streamProcessor，因为连接管理主要通过 ConnectionRegistry
		} else {
			utils.Infof("HTTP long polling: registered connection in SessionManager, connID=%s", connID)
		}
	}

	utils.Infof("HTTP long polling: created stream processor connID=%s for clientID=%d, mappingID=%s", connID, clientID, pkg.MappingID)

	return streamProcessor
}
	
// handleControlPackage 处理控制包
func (s *ManagementAPIServer) handleControlPackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage) *httppoll.TunnelPackage {
	if s.sessionMgr == nil {
		return nil
	}

	// 获取连接对应的 Connection 对象
	connID := streamProcessor.GetConnectionID()
	sessionMgrWithConn := getSessionManagerWithConnection(s.sessionMgr)
	if sessionMgrWithConn == nil {
		return nil
	}

	typesConn, exists := sessionMgrWithConn.GetConnection(connID)
	if !exists || typesConn == nil {
		utils.Warnf("HTTP long polling: connection not found in SessionManager, connID=%s", connID)
		return nil
	}

	// 根据包类型处理
	switch pkg.Type {
	case "Handshake":
		return s.handleHandshakePackage(streamProcessor, pkg, typesConn)
	case "JsonCommand":
		return s.handleJsonCommandPackage(streamProcessor, pkg, typesConn)
	case "TunnelOpen":
		return s.handleTunnelOpenPackage(streamProcessor, pkg, typesConn)
	default:
		utils.Warnf("HTTP long polling: unknown control package type: %s", pkg.Type)
		return nil
	}
}

// handleHandshakePackage 处理握手包
func (s *ManagementAPIServer) handleHandshakePackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage, typesConn *types.Connection) *httppoll.TunnelPackage {
	// 解析 HandshakeRequest
	dataBytes, err := json.Marshal(pkg.Data)
	if err != nil {
		utils.Errorf("HTTP long polling: failed to marshal handshake data: %v", err)
		return nil
	}

	var handshakeReq packet.HandshakeRequest
	if err := json.Unmarshal(dataBytes, &handshakeReq); err != nil {
		utils.Errorf("HTTP long polling: failed to unmarshal handshake request: %v", err)
		return nil
	}

	// 获取 ConnectionID（应该已经由 createHTTPLongPollingConnection 生成）
	connID := streamProcessor.GetConnectionID()
	if connID == "" {
		// 如果还没有 ConnectionID，生成一个
		uuid, err := utils.GenerateUUID()
		if err != nil {
			utils.Errorf("HTTP long polling: failed to generate connection ID: %v", err)
			return &httppoll.TunnelPackage{
				Type: "HandshakeResponse",
				Data: &packet.HandshakeResponse{
					Success: false,
					Error:   fmt.Sprintf("failed to generate connection ID: %v", err),
				},
			}
		}
		connID = "conn_" + uuid[:8]
		streamProcessor.SetConnectionID(connID)
		utils.Infof("HTTP long polling: generated connection ID: %s", connID)
	}

	// 构造 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet: &packet.TransferPacket{
			PacketType: packet.Handshake,
			Payload:    dataBytes,
		},
		Timestamp: time.Now(),
	}

	// 处理数据包（通过 SessionManager）
	if handler, ok := s.sessionMgr.(interface {
		HandlePacket(*types.StreamPacket) error
	}); ok {
		if err := handler.HandlePacket(streamPacket); err != nil {
			utils.Errorf("HTTP long polling: failed to handle handshake packet: %v", err)
			return &httppoll.TunnelPackage{
				ConnectionID: connID,
				Type:         "HandshakeResponse",
				Data: &packet.HandshakeResponse{
					Success: false,
					Error:   err.Error(),
				},
			}
		}
	}

	// 从 SessionManager 获取握手响应（通过控制连接）
	var handshakeResp *packet.HandshakeResponse
	if controlConn := s.getControlConnectionByConnID(connID); controlConn != nil {
		// 等待握手完成（通过轮询控制连接状态）
		// 注意：这里简化处理，实际应该通过事件或回调获取响应
		// 暂时返回空响应，让客户端通过 Poll 获取响应
		// TODO: 实现握手响应的异步获取机制
	}

	// 如果还没有响应，构造一个临时响应（包含 ConnectionID）
	if handshakeResp == nil {
		handshakeResp = &packet.HandshakeResponse{
			Success:      true,
			Message:      "Handshake in progress",
			ConnectionID: connID, // 服务端分配的 ConnectionID
		}
	} else {
		// 确保响应中包含 ConnectionID
		handshakeResp.ConnectionID = connID
	}

	// 构建响应 TunnelPackage
	return &httppoll.TunnelPackage{
		ConnectionID: connID,
		ClientID:     streamProcessor.GetClientID(),
		TunnelType:   "control",
		Type:         "HandshakeResponse",
		Data:         handshakeResp,
	}
}

// getControlConnectionByConnID 通过 ConnectionID 获取控制连接
func (s *ManagementAPIServer) getControlConnectionByConnID(connID string) interface{} {
	// 通过 SessionManager 获取控制连接
	if sm, ok := s.sessionMgr.(interface {
		GetControlConnectionByConnID(connID string) interface{}
	}); ok {
		return sm.GetControlConnectionByConnID(connID)
	}
	return nil
}

// handleJsonCommandPackage 处理 JSON 命令包
func (s *ManagementAPIServer) handleJsonCommandPackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage, typesConn *types.Connection) *httppoll.TunnelPackage {
	// 解析 JsonCommand
	dataBytes, err := json.Marshal(pkg.Data)
	if err != nil {
		utils.Errorf("HTTP long polling: failed to marshal command data: %v", err)
		return nil
	}

	// 构造 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: streamProcessor.GetConnectionID(),
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			Payload:    dataBytes,
		},
		Timestamp: time.Now(),
	}

	// 处理数据包（通过 SessionManager）
	if handler, ok := s.sessionMgr.(interface {
		HandlePacket(*types.StreamPacket) error
	}); ok {
		if err := handler.HandlePacket(streamPacket); err != nil {
			utils.Errorf("HTTP long polling: failed to handle command packet: %v", err)
			return nil
		}
	}

	// 命令响应通过 Poll 获取
	return nil
}

// handleTunnelOpenPackage 处理隧道打开包
func (s *ManagementAPIServer) handleTunnelOpenPackage(streamProcessor *httppoll.ServerStreamProcessor, pkg *httppoll.TunnelPackage, typesConn *types.Connection) *httppoll.TunnelPackage {
	// 解析 TunnelOpenRequest
	dataBytes, err := json.Marshal(pkg.Data)
	if err != nil {
		utils.Errorf("HTTP long polling: failed to marshal tunnel open data: %v", err)
		return nil
	}

	var tunnelOpenReq packet.TunnelOpenRequest
	if err := json.Unmarshal(dataBytes, &tunnelOpenReq); err != nil {
		utils.Errorf("HTTP long polling: failed to unmarshal tunnel open request: %v", err)
		return nil
	}

	// 设置 mappingID
	if tunnelOpenReq.MappingID != "" {
		streamProcessor.SetMappingID(tunnelOpenReq.MappingID)
	}

	// 构造 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: streamProcessor.GetConnectionID(),
		Packet: &packet.TransferPacket{
			PacketType: packet.TunnelOpen,
			Payload:    dataBytes,
		},
		Timestamp: time.Now(),
	}

	// 处理数据包（通过 SessionManager）
	if handler, ok := s.sessionMgr.(interface {
		HandlePacket(*types.StreamPacket) error
	}); ok {
		if err := handler.HandlePacket(streamPacket); err != nil {
			utils.Errorf("HTTP long polling: failed to handle tunnel open packet: %v", err)
			return &httppoll.TunnelPackage{
				Type: "TunnelOpenAck",
				Data: &packet.TunnelOpenAckResponse{
					TunnelID: tunnelOpenReq.TunnelID,
					Success:  false,
					Error:    err.Error(),
				},
			}
		}
	}

	// TunnelOpenAck 响应通过 Poll 获取
	return nil
}

// isSourceClientForMapping 判断是否是源端客户端（用于更新 bridge 的 sourceConn）
func (s *ManagementAPIServer) isSourceClientForMapping(mappingID string, clientID int64) bool {
	if s.cloudControl == nil || mappingID == "" || clientID == 0 {
		return false
	}
	
	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		return false
	}
	
	listenClientID := mapping.ListenClientID
	if listenClientID == 0 {
		listenClientID = mapping.SourceClientID
	}
	
	return clientID == listenClientID
}
