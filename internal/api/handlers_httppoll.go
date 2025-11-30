package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

const (
	httppollMaxRequestSize = 1024 * 1024 // 1MB
	httppollDefaultTimeout = 30          // 默认 30 秒
	httppollMaxTimeout     = 60          // 最大 60 秒
)

// HTTPPushRequest HTTP 推送请求
type HTTPPushRequest struct {
	Data      string `json:"data"`      // Base64 编码的数据
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
	Data      string `json:"data,omitempty"`      // Base64 编码的数据
	Seq       uint64 `json:"seq,omitempty"`       // 序列号
	Timeout   bool   `json:"timeout,omitempty"`   // 是否超时
	Timestamp int64  `json:"timestamp"`           // 时间戳
}

// httppollConnectionManager HTTP 长轮询连接管理器
type httppollConnectionManager struct {
	mu sync.RWMutex
	// clientID -> ServerHTTPLongPollingConn
	connections map[int64]*session.ServerHTTPLongPollingConn
	// connectionID -> ServerHTTPLongPollingConn (用于握手阶段，clientID=0)
	tempConnections map[string]*session.ServerHTTPLongPollingConn
	// clientIP -> ServerHTTPLongPollingConn (用于 clientID=0 时匹配 push 和 poll 请求)
	tempConnectionsByIP map[string]*session.ServerHTTPLongPollingConn
}

func newHTTPPollConnectionManager() *httppollConnectionManager {
	return &httppollConnectionManager{
		connections:          make(map[int64]*session.ServerHTTPLongPollingConn),
		tempConnections:      make(map[string]*session.ServerHTTPLongPollingConn),
		tempConnectionsByIP:  make(map[string]*session.ServerHTTPLongPollingConn),
	}
}

func (m *httppollConnectionManager) getOrCreate(clientID int64, ctx context.Context) *session.ServerHTTPLongPollingConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.connections[clientID]; exists {
		return conn
	}

	conn := session.NewServerHTTPLongPollingConn(ctx, clientID)
	m.connections[clientID] = conn
	return conn
}

func (m *httppollConnectionManager) getByClientID(clientID int64) *session.ServerHTTPLongPollingConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[clientID]
}

func (m *httppollConnectionManager) registerTemp(connID string, conn *session.ServerHTTPLongPollingConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tempConnections[connID] = conn
}

func (m *httppollConnectionManager) getTemp(connID string) *session.ServerHTTPLongPollingConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tempConnections[connID]
}

func (m *httppollConnectionManager) removeTemp(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tempConnections, connID)
}

func (m *httppollConnectionManager) migrateTempToClientID(connID string, clientID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.tempConnections[connID]
	if !exists {
		return
	}

	delete(m.tempConnections, connID)
	if clientID > 0 {
		// 更新连接的 clientID
		conn.UpdateClientID(clientID)
		
		// 如果已存在该 clientID 的连接，关闭旧的
		if oldConn, exists := m.connections[clientID]; exists && oldConn != conn {
			oldConn.Close()
		}
		m.connections[clientID] = conn
	}
}

// getByConnectionID 通过 ConnectionID 获取连接（用于握手后更新映射）
func (m *httppollConnectionManager) getByConnectionID(connID string) *session.ServerHTTPLongPollingConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tempConnections[connID]
}

// getFirstTemp 获取第一个临时连接（用于 clientID=0 时查找连接）
func (m *httppollConnectionManager) getFirstTemp() *session.ServerHTTPLongPollingConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 返回第一个临时连接（如果有多个，使用第一个）
	for _, conn := range m.tempConnections {
		return conn
	}
	return nil
}

// getByIP 通过客户端 IP 地址获取连接
func (m *httppollConnectionManager) getByIP(clientIP string) *session.ServerHTTPLongPollingConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tempConnectionsByIP[clientIP]
}

// registerByIP 通过客户端 IP 地址注册连接
func (m *httppollConnectionManager) registerByIP(clientIP string, conn *session.ServerHTTPLongPollingConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tempConnectionsByIP[clientIP] = conn
}

// createMigrationCallback 创建迁移回调函数
// 当 clientID 从 0 变为非 0 时，自动调用此回调执行迁移
func (m *httppollConnectionManager) createMigrationCallback(connID string) func(string, int64, int64) {
	return func(actualConnID string, oldClientID, newClientID int64) {
		if oldClientID == 0 && newClientID > 0 {
			m.migrateTempToClientID(actualConnID, newClientID)
		}
	}
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
	// 1. 从 Header 获取 ClientID（允许为 0，用于握手阶段）
	clientIDStr := r.Header.Get("X-Client-ID")
	if clientIDStr == "" {
		s.respondError(w, http.StatusBadRequest, "missing X-Client-ID header")
		return
	}

	clientID, err := strconv.ParseInt(clientIDStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid X-Client-ID: %v", err))
		return
	}

	// 2. 获取 SessionManager
	if s.sessionMgr == nil {
		s.respondError(w, http.StatusInternalServerError, "SessionManager not available")
		return
	}

	// 3. 解析请求
	var req HTTPPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// 4. 验证 Base64 数据格式（不在这里解码，由适配层处理）
	if req.Data == "" {
		s.respondError(w, http.StatusBadRequest, "Empty data")
		return
	}

	// 5. 检查 Base64 数据大小（粗略估算：Base64 编码后大小约为原始数据的 4/3）
	// 为了安全，我们检查 Base64 字符串长度
	estimatedSize := len(req.Data) * 3 / 4
	if estimatedSize > httppollMaxRequestSize {
		s.respondError(w, http.StatusBadRequest, "Data too large")
		return
	}

	// 6. 获取或创建 HTTP 长轮询连接
	// 如果 clientID = 0，使用客户端 IP 地址来匹配 push 和 poll 请求
	var httppollConn *session.ServerHTTPLongPollingConn
	clientIP := s.getClientIP(r)
	if clientID == 0 {
		// 握手阶段：直接调用 getOrCreateHTTPLongPollingConn，它内部会处理 IP 匹配和锁
		utils.Debugf("HTTP long polling: [HANDLE_PUSH] getting or creating connection for IP %s, clientID=0", clientIP)
		httppollConn = s.getOrCreateHTTPLongPollingConn(clientID, r.Context(), clientIP)
		if httppollConn == nil {
			s.respondError(w, http.StatusServiceUnavailable, "Failed to create connection")
			return
		}
	} else {
		// 已认证：查找已存在的连接
		httppollConn = s.getHTTPLongPollingConn(clientID)
		if httppollConn == nil {
			// 连接不存在，创建新连接
			// 注意：迁移逻辑现在由适配层的回调自动处理，不需要在这里手动迁移
			httppollConn = s.getOrCreateHTTPLongPollingConn(clientID, r.Context(), "")
			if httppollConn == nil {
				s.respondError(w, http.StatusServiceUnavailable, "Failed to create connection")
				return
			}
		}
	}

	// 7. 将 Base64 数据推送到连接（触发 Read()）
	// 注意：PushData 现在接收 Base64 字符串，而不是解码后的字节
	utils.Infof("HTTP long polling: [HANDLE_PUSH] pushing Base64 data (len=%d) to connection, clientID=%d", 
		len(req.Data), clientID)
	if err := httppollConn.PushData(req.Data); err != nil {
		utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to push Base64 data: %v, clientID=%d", err, clientID)
		s.respondError(w, http.StatusServiceUnavailable, "Connection closed")
		return
	}
	utils.Debugf("HTTP long polling: [HANDLE_PUSH] Base64 data pushed successfully, clientID=%d", clientID)

	// 8. 返回 ACK（直接编码，不包装在 ResponseData 中，与 handleHTTPPoll 保持一致）
	resp := HTTPPushResponse{
		Success:   true,
		Ack:       req.Seq,
		Timestamp: time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleHTTPPoll 处理客户端长轮询
// GET /tunnox/v1/poll?timeout=30&since=0
func (s *ManagementAPIServer) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
	// 1. 从 Header 获取 ClientID（允许为 0，用于握手阶段）
	clientIDStr := r.Header.Get("X-Client-ID")
	if clientIDStr == "" {
		s.respondError(w, http.StatusBadRequest, "missing X-Client-ID header")
		return
	}

	clientID, err := strconv.ParseInt(clientIDStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid X-Client-ID: %v", err))
		return
	}

	// 2. 解析超时参数
	timeout := httppollDefaultTimeout
	if t := r.URL.Query().Get("timeout"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 && parsed <= httppollMaxTimeout {
			timeout = parsed
		}
	}

	// 3. 获取或创建 HTTP 长轮询连接
	// 对于 clientID=0，使用客户端 IP 地址来匹配 push 和 poll 请求
	var httppollConn *session.ServerHTTPLongPollingConn
	clientIP := s.getClientIP(r)
	if clientID == 0 {
		// 握手阶段：直接调用 getOrCreateHTTPLongPollingConn，它内部会处理 IP 匹配和锁
		utils.Debugf("HTTP long polling: [HANDLE_POLL] getting or creating connection for IP %s, clientID=0", clientIP)
		httppollConn = s.getOrCreateHTTPLongPollingConn(clientID, r.Context(), clientIP)
		if httppollConn == nil {
			utils.Errorf("HTTP long polling: [HANDLE_POLL] failed to create connection for clientID=0")
			s.respondError(w, http.StatusInternalServerError, "Failed to create connection")
			return
		}
	} else {
		// 已认证：查找已存在的连接
		httppollConn = s.getHTTPLongPollingConn(clientID)
		if httppollConn == nil {
			// 连接不存在，返回空响应
			// 注意：迁移逻辑现在由适配层的回调自动处理，不需要在这里手动迁移
			resp := HTTPPollResponse{
				Success:   true,
				Timeout:   true,
				Timestamp: time.Now().Unix(),
			}
			s.respondJSON(w, http.StatusOK, resp)
			return
		}
	}

	// 4. 长轮询：等待数据（触发 Write()）
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	utils.Debugf("HTTP long polling: [HANDLE_POLL] calling PollData, clientID=%d, timeout=%d", clientID, timeout)
	base64Data, err := httppollConn.PollData(ctx)
	utils.Debugf("HTTP long polling: [HANDLE_POLL] PollData returned, clientID=%d, base64DataLen=%d, err=%v", 
		clientID, len(base64Data), err)
	if err == context.DeadlineExceeded {
		// 超时，返回空响应
			resp := HTTPPollResponse{
				Success:   true,
				Timeout:   true,
				Timestamp: time.Now().Unix(),
			}
		// 直接返回 HTTPPollResponse，不使用 respondJSON（避免双重包装）
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
	if err != nil {
		// 对于 context canceled 或 EOF，返回超时响应而不是错误
		// 这些情况通常是正常的（客户端断开、连接关闭等），客户端会重试
		if err == context.Canceled || err == io.EOF {
			utils.Debugf("HTTP long polling: [HANDLE_POLL] %v, returning timeout response, clientID=%d", err, clientID)
			resp := HTTPPollResponse{
				Success:   true,
				Timeout:   true,
				Timestamp: time.Now().Unix(),
			}
			// 直接返回 HTTPPollResponse，不使用 respondJSON（避免双重包装）
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
			return
		}
		// 其他错误才返回 500
		utils.Errorf("HTTP long polling: [HANDLE_POLL] PollData failed: %v, clientID=%d", err, clientID)
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 5. 有数据，立即返回（PollData 已经返回 Base64 编码的数据）
			resp := HTTPPollResponse{
				Success:   true,
		Data:      base64Data,
		Seq:       0, // 序列号暂时不使用
				Timeout:   false,
				Timestamp: time.Now().Unix(),
			}
	// 直接返回 HTTPPollResponse，不使用 respondJSON（避免双重包装）
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}

// getOrCreateHTTPLongPollingConn 获取或创建 HTTP 长轮询连接
func (s *ManagementAPIServer) getOrCreateHTTPLongPollingConn(clientID int64, ctx context.Context, clientIP string) *session.ServerHTTPLongPollingConn {
	// 1. 获取连接管理器（懒加载）
	if s.httppollConnMgr == nil {
		s.httppollConnMgr = newHTTPPollConnectionManager()
	}

	// 2. 对于 clientID=0，先通过 IP 查找连接（带锁，确保线程安全）
	// 注意：必须在创建连接之前检查，避免并发创建多个连接
	if clientID == 0 && clientIP != "" {
		s.httppollConnMgr.mu.RLock()
		if conn := s.httppollConnMgr.tempConnectionsByIP[clientIP]; conn != nil {
			s.httppollConnMgr.mu.RUnlock()
			utils.Debugf("HTTP long polling: [getOrCreate] found existing connection by IP %s", clientIP)
			return conn
		}
		s.httppollConnMgr.mu.RUnlock()
	}

	// 3. 检查是否已存在连接（通过 clientID）
	if conn := s.httppollConnMgr.getByClientID(clientID); conn != nil {
		return conn
	}

	// 3. 创建新的 HTTP 长轮询连接（实现 net.Conn）
	// 使用 server 的 context 而不是请求的 context，避免请求结束后 context 被取消
	serverCtx := s.Ctx()
	if serverCtx == nil {
		serverCtx = context.Background()
	}
	httppollConn := session.NewServerHTTPLongPollingConn(serverCtx, clientID)

	// 4. 统一使用 CreateConnection，就像其他协议一样
	sessionMgrWithConn := getSessionManagerWithConnection(s.sessionMgr)
	if sessionMgrWithConn == nil {
		// 如果 SessionManager 不支持 CreateConnection，直接返回连接
		// 这种情况不应该发生，但为了健壮性保留
		utils.Warnf("SessionManager does not support CreateConnection, using direct connection")
		s.httppollConnMgr.mu.Lock()
		s.httppollConnMgr.connections[clientID] = httppollConn
		s.httppollConnMgr.mu.Unlock()
		return httppollConn
	}

	// 5. 创建连接（统一流程）
	conn, err := sessionMgrWithConn.CreateConnection(httppollConn, httppollConn)
	if err != nil {
		utils.Errorf("HTTP long polling: failed to create connection: %v", err)
		httppollConn.Close()
		return nil
	}
	
	utils.Debugf("HTTP long polling: created connection connID=%s for clientID=%d", conn.ID, clientID)

	// 6. 设置协议类型
	conn.Protocol = "httppoll"

	// 7. 启动读取循环（处理从客户端接收的数据包）
	go s.startHTTPLongPollingReadLoop(conn)

	// 8. 设置连接 ID（用于迁移回调）
	httppollConn.SetConnectionID(conn.ID)

	// 9. 如果是临时连接（clientID=0），注册到临时连接映射和 IP 映射，并注入迁移回调
	// 注意：必须在创建连接之后立即注册，使用写锁确保线程安全
	if clientID == 0 {
		s.httppollConnMgr.registerTemp(conn.ID, httppollConn)

		// 注入迁移回调：当 clientID 更新时自动触发迁移
		migrationCallback := s.httppollConnMgr.createMigrationCallback(conn.ID)
		httppollConn.SetMigrationCallback(migrationCallback)
		
		if clientIP != "" {
			// 使用写锁确保同一 IP 只创建一个连接
			s.httppollConnMgr.mu.Lock()
			// 双重检查：在注册之前再次检查，避免覆盖已存在的连接
			if existingConn := s.httppollConnMgr.tempConnectionsByIP[clientIP]; existingConn != nil {
				// 已存在连接，关闭新创建的连接，返回已存在的连接
				s.httppollConnMgr.mu.Unlock()
				utils.Debugf("HTTP long polling: [getOrCreate] found existing connection by IP %s (during registration), closing new connection", clientIP)
				httppollConn.Close()
				return existingConn
			}
			s.httppollConnMgr.tempConnectionsByIP[clientIP] = httppollConn
			s.httppollConnMgr.mu.Unlock()
			utils.Debugf("HTTP long polling: registered connection by IP %s, connID=%s", clientIP, conn.ID)
		}
	} else {
		// 10. 保存连接映射（用于后续查找）
		s.httppollConnMgr.mu.Lock()
		s.httppollConnMgr.connections[clientID] = httppollConn
		s.httppollConnMgr.mu.Unlock()
	}

	return httppollConn
}

// getClientIP 获取客户端 IP 地址
func (s *ManagementAPIServer) getClientIP(r *http.Request) string {
	// 优先使用 X-Forwarded-For（如果存在代理）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 可能包含多个 IP，取第一个
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	// 使用 X-Real-IP（如果存在）
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// 使用 RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// getHTTPLongPollingConn 获取 HTTP 长轮询连接
func (s *ManagementAPIServer) getHTTPLongPollingConn(clientID int64) *session.ServerHTTPLongPollingConn {
	if s.httppollConnMgr == nil {
		return nil
	}
	return s.httppollConnMgr.getByClientID(clientID)
}

// startHTTPLongPollingReadLoop 启动 HTTP 长轮询连接的读取循环
func (s *ManagementAPIServer) startHTTPLongPollingReadLoop(conn *types.Connection) {
	defer func() {
		if r := recover(); r != nil {
			utils.Errorf("HTTP long polling read loop panic: %v", r)
		}
	}()

	utils.Infof("HTTP long polling: [READ_LOOP] starting read loop for connection %s", conn.ID)

	for {
		select {
		case <-s.Ctx().Done():
			utils.Debugf("HTTP long polling: [READ_LOOP] context canceled for connection %s", conn.ID)
			return
		default:
		}

		utils.Debugf("HTTP long polling: [READ_LOOP] waiting for packet from connection %s", conn.ID)
		// 从 StreamProcessor 读取数据包
		pkt, bytesRead, err := conn.Stream.ReadPacket()
		if err != nil {
			if err == io.EOF {
				utils.Infof("HTTP long polling: [READ_LOOP] connection %s closed (EOF)", conn.ID)
				return
			}
			// 检查是否为超时错误（HTTP 长轮询的 Read 可能会超时）
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && netErr.Temporary() {
				utils.Debugf("HTTP long polling: [READ_LOOP] timeout for connection %s, continuing", conn.ID)
				// 超时错误，继续循环
				continue
			}
			utils.Errorf("HTTP long polling: [READ_LOOP] failed to read packet from connection %s: %v", conn.ID, err)
			return
		}

		utils.Infof("HTTP long polling: [READ_LOOP] received packet from connection %s, type=%d, bytes=%d", 
			conn.ID, pkt.PacketType, bytesRead)

		// 构造 StreamPacket
		streamPacket := &types.StreamPacket{
			ConnectionID: conn.ID,
			Packet:       pkt,
			Timestamp:    time.Now(),
		}

		utils.Debugf("HTTP long polling: [READ_LOOP] handling packet for connection %s", conn.ID)
		// 处理数据包
		if err := s.sessionMgr.(interface {
			HandlePacket(*types.StreamPacket) error
		}).HandlePacket(streamPacket); err != nil {
			utils.Errorf("HTTP long polling: [READ_LOOP] failed to handle packet for connection %s: %v", conn.ID, err)
		} else {
			utils.Debugf("HTTP long polling: [READ_LOOP] packet handled successfully for connection %s", conn.ID)
		}
	}
}
