// Package httppoll 提供 HTTP 长轮询传输模块
package httppoll

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/protocol/session"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// HTTPPollModule HTTP 长轮询传输模块
type HTTPPollModule struct {
	*dispose.ServiceBase

	config   *httpservice.HTTPPollModuleConfig
	deps     *httpservice.ModuleDependencies
	registry *httppoll.ConnectionRegistry
	session  session.Session
}

// NewHTTPPollModule 创建 HTTP 长轮询模块
func NewHTTPPollModule(ctx context.Context, config *httpservice.HTTPPollModuleConfig) *HTTPPollModule {
	return &HTTPPollModule{
		ServiceBase: dispose.NewService("HTTPPollModule", ctx),
		config:      config,
		registry:    httppoll.NewConnectionRegistry(),
	}
}

// Name 返回模块名称
func (m *HTTPPollModule) Name() string {
	return "HTTPPoll"
}

// SetDependencies 注入依赖
func (m *HTTPPollModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
}

// SetSession 设置会话管理器
func (m *HTTPPollModule) SetSession(sess session.Session) {
	m.session = sess
}

// RegisterRoutes 注册路由
func (m *HTTPPollModule) RegisterRoutes(router *mux.Router) {
	if !m.config.Enabled {
		return
	}
	router.HandleFunc("/_tunnox/v1/push", m.handlePush).Methods("POST")
	router.HandleFunc("/_tunnox/v1/poll", m.handlePoll).Methods("GET", "POST")
	corelog.Infof("HTTPPollModule: registered routes")
}

// handlePush 处理 Push 请求
func (m *HTTPPollModule) handlePush(w http.ResponseWriter, r *http.Request) {
	var req httppoll.TunnelPackage

	if xTunnelPackage := r.Header.Get("X-Tunnel-Package"); xTunnelPackage != "" {
		pkg, err := httppoll.DecodeTunnelPackage(xTunnelPackage)
		if err != nil {
			http.Error(w, "Invalid X-Tunnel-Package header", http.StatusBadRequest)
			return
		}
		req = *pkg
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	connID := req.ConnectionID
	if connID == "" {
		connID = uuid.New().String()
	}

	sp := m.registry.GetOrCreate(connID, func() *httppoll.ServerStreamProcessor {
		return httppoll.NewServerStreamProcessor(m.Ctx(), connID, req.ClientID, req.MappingID)
	})

	if sp == nil {
		http.Error(w, "Failed to create connection", http.StatusInternalServerError)
		return
	}

	var dataStr string
	if req.Data != nil {
		switch v := req.Data.(type) {
		case string:
			dataStr = v
		case map[string]interface{}:
			dataBytes, _ := json.Marshal(v)
			dataStr = string(dataBytes)
		}
	}

	if dataStr == "" && req.Type == "" && r.Body != nil {
		var bodyData struct {
			Data      string `json:"data"`
			Timestamp int64  `json:"timestamp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&bodyData); err == nil && bodyData.Data != "" {
			dataStr = bodyData.Data
		}
	}

	pushReq := &httppoll.HTTPPushRequest{Data: dataStr}

	responsePkg, err := sp.HandlePushRequest(&req, pushReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Type != "" && m.session != nil {
		pkt, err := httppoll.TunnelPackageToTransferPacket(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		needAcceptConnection := pkt.PacketType == packet.Handshake || (pkt.PacketType&0x3F) == packet.TunnelOpen
		if needAcceptConnection {
			httpPollConn := NewHTTPPollConn(sp, r.RemoteAddr)
			streamConn, err := m.session.AcceptConnection(httpPollConn, httpPollConn)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			sp.SetConnectionID(streamConn.ID)
			connID = streamConn.ID
			m.registry.Register(connID, sp)
		}

		streamPacket := &types.StreamPacket{
			ConnectionID: connID,
			Packet:       pkt,
			Timestamp:    time.Now(),
		}

		m.session.HandlePacket(streamPacket)

		if pkt.PacketType == packet.Handshake || (pkt.PacketType&0x3F) == packet.TunnelOpen {
			responsePkg = sp.WaitForControlPacket(r.Context(), 5*time.Second)
		}
	}

	resp := httppoll.TunnelPackage{
		ConnectionID: connID,
		ClientID:     sp.GetClientID(),
		MappingID:    sp.GetMappingID(),
		RequestID:    req.RequestID,
	}

	if responsePkg != nil {
		resp.Type = responsePkg.Type
		resp.Data = responsePkg.Data
	}

	encodedResp, err := httppoll.EncodeTunnelPackage(&resp)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Tunnel-Package", encodedResp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handlePoll 处理 Poll 请求
func (m *HTTPPollModule) handlePoll(w http.ResponseWriter, r *http.Request) {
	var req httppoll.TunnelPackage

	if xTunnelPackage := r.Header.Get("X-Tunnel-Package"); xTunnelPackage != "" {
		pkg, err := httppoll.DecodeTunnelPackage(xTunnelPackage)
		if err != nil {
			http.Error(w, "Invalid X-Tunnel-Package header", http.StatusBadRequest)
			return
		}
		req = *pkg
	} else if r.Method == "POST" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	} else {
		req.ConnectionID = r.URL.Query().Get("connection_id")
		req.RequestID = r.URL.Query().Get("request_id")
		req.TunnelType = r.URL.Query().Get("tunnel_type")
	}

	connID := req.ConnectionID
	if connID == "" {
		http.Error(w, "Missing connection_id", http.StatusBadRequest)
		return
	}

	sp := m.registry.Get(connID)
	if sp == nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	timeout := time.Duration(m.config.DefaultTimeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	requestID := req.RequestID
	if requestID == "" {
		requestID = uuid.New().String()
	}
	tunnelType := req.TunnelType
	if tunnelType == "" {
		tunnelType = "control"
	}

	dataStr, responsePkg, err := sp.HandlePollRequest(ctx, requestID, tunnelType)
	if err != nil {
		if err == context.DeadlineExceeded || err == context.Canceled {
			timeoutResp := httppoll.CreateTimeoutResponse()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(timeoutResp)
			return
		}
		if err == io.EOF {
			http.Error(w, "Connection closed", http.StatusGone)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if dataStr != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(dataStr))
		return
	}

	if responsePkg != nil {
		resp := httppoll.TunnelPackage{
			ConnectionID: connID,
			ClientID:     sp.GetClientID(),
			MappingID:    sp.GetMappingID(),
			Type:         responsePkg.Type,
			Data:         responsePkg.Data,
			RequestID:    requestID,
		}
		encodedResp, err := httppoll.EncodeTunnelPackage(&resp)
		if err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Tunnel-Package", encodedResp)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	timeoutResp := httppoll.CreateTimeoutResponse()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeoutResp)
}

// Start 启动模块
func (m *HTTPPollModule) Start() error {
	return nil
}

// Stop 停止模块
func (m *HTTPPollModule) Stop() error {
	return nil
}

// GetRegistry 获取连接注册表
func (m *HTTPPollModule) GetRegistry() *httppoll.ConnectionRegistry {
	return m.registry
}

// HTTPPollConn 包装 ServerStreamProcessor 实现 io.Reader、io.Writer 和 stream.PackageStreamer 接口
// 这样 StreamManager.CreateStream 可以直接使用 ServerStreamProcessor，而不是创建新的 StreamProcessor
type HTTPPollConn struct {
	sp         *httppoll.ServerStreamProcessor
	remoteAddr string
}

// NewHTTPPollConn 创建 HTTPPollConn
func NewHTTPPollConn(sp *httppoll.ServerStreamProcessor, remoteAddr string) *HTTPPollConn {
	return &HTTPPollConn{
		sp:         sp,
		remoteAddr: remoteAddr,
	}
}

// GetConnectionID 返回连接 ID（用于 SessionManager.CreateConnection）
func (c *HTTPPollConn) GetConnectionID() string {
	return c.sp.GetConnectionID()
}

// Read 实现 io.Reader
func (c *HTTPPollConn) Read(p []byte) (int, error) {
	data, err := c.sp.ReadAvailable(len(p))
	if err != nil {
		return 0, err
	}
	n := copy(p, data)
	return n, nil
}

// Write 实现 io.Writer
func (c *HTTPPollConn) Write(p []byte) (int, error) {
	err := c.sp.WriteExact(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// ============================================================================
// 实现 stream.PackageStreamer 接口（委托给 ServerStreamProcessor）
// 这样 StreamManager.CreateStream 可以直接使用 HTTPPollConn，而不是创建新的 StreamProcessor
// ============================================================================

// GetReader 实现 stream.StreamReader
func (c *HTTPPollConn) GetReader() io.Reader {
	return c.sp.GetReader()
}

// GetWriter 实现 stream.StreamWriter
func (c *HTTPPollConn) GetWriter() io.Writer {
	return c.sp.GetWriter()
}

// ReadPacket 实现 stream.PackageStreamer
func (c *HTTPPollConn) ReadPacket() (*packet.TransferPacket, int, error) {
	return c.sp.ReadPacket()
}

// WritePacket 实现 stream.PackageStreamer
func (c *HTTPPollConn) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	return c.sp.WritePacket(pkt, useCompression, rateLimitBytesPerSecond)
}

// ReadExact 实现 stream.PackageStreamer
func (c *HTTPPollConn) ReadExact(length int) ([]byte, error) {
	return c.sp.ReadExact(length)
}

// ReadAvailable 实现 StreamDataForwarder 接口
// 读取可用数据（不等待完整长度），用于 Bridge 数据转发
func (c *HTTPPollConn) ReadAvailable(maxLength int) ([]byte, error) {
	return c.sp.ReadAvailable(maxLength)
}

// WriteExact 实现 stream.PackageStreamer
func (c *HTTPPollConn) WriteExact(data []byte) error {
	return c.sp.WriteExact(data)
}

// Close 实现 stream.PackageStreamer
func (c *HTTPPollConn) Close() {
	c.sp.Close()
}

// ============================================================================
// 实现 HTTPPoll 协议特定的接口，用于 Session 层查找控制连接
// ============================================================================

// GetClientID 返回客户端 ID（用于 Session 层通过 clientID 查找控制连接）
func (c *HTTPPollConn) GetClientID() int64 {
	return c.sp.GetClientID()
}

// GetMappingID 返回映射 ID
func (c *HTTPPollConn) GetMappingID() string {
	return c.sp.GetMappingID()
}

// CanCreateTemporaryControlConn 返回是否可以创建临时控制连接
// HTTPPoll 协议中，每个隧道连接使用独立的 connectionID，需要通过 clientID 查找控制连接
// 如果找不到，可以创建临时控制连接
func (c *HTTPPollConn) CanCreateTemporaryControlConn() bool {
	return true
}
