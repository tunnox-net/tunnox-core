package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/health"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/gorilla/mux"
)

// ControlConnectionAccessor 控制连接访问器接口（用于API层）
type ControlConnectionAccessor interface {
	GetConnID() string
	GetRemoteAddr() string
	GetStream() stream.PackageStreamer
}

// controlConnectionAdapter 适配器，将 session.ControlConnectionInterface 转换为 api.ControlConnectionAccessor
type controlConnectionAdapter struct {
	conn session.ControlConnectionInterface
}

func (a *controlConnectionAdapter) GetConnID() string {
	if a.conn == nil {
		return ""
	}
	return a.conn.GetConnID()
}

func (a *controlConnectionAdapter) GetRemoteAddr() string {
	if a.conn == nil {
		return ""
	}
	addr := a.conn.GetRemoteAddr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

func (a *controlConnectionAdapter) GetStream() stream.PackageStreamer {
	if a.conn == nil {
		return nil
	}
	return a.conn.GetStream()
}

// adaptControlConnection 将 session.ControlConnectionInterface 适配为 api.ControlConnectionAccessor
func adaptControlConnection(conn session.ControlConnectionInterface) ControlConnectionAccessor {
	if conn == nil {
		return nil
	}
	return &controlConnectionAdapter{conn: conn}
}

// SessionManager 接口（避免循环依赖）
type SessionManager interface {
	GetControlConnectionInterface(clientID int64) ControlConnectionAccessor
	BroadcastConfigPush(clientID int64, configBody string) error
	GetNodeID() string // 获取当前节点ID
	// GetTunnelBridgeByConnectionID 通过 ConnectionID 查找 tunnel bridge（优先使用）
	GetTunnelBridgeByConnectionID(connID string) session.TunnelBridgeAccessor
	// GetTunnelBridgeByMappingID 通过 mappingID 查找 tunnel bridge（向后兼容）
	GetTunnelBridgeByMappingID(mappingID string, clientID int64) session.TunnelBridgeAccessor
}

// apiSessionManagerAdapter 适配器，将 session.SessionManager 适配为 api.SessionManager
type apiSessionManagerAdapter struct {
	sessionMgr *session.SessionManager
}

func (a *apiSessionManagerAdapter) GetControlConnectionInterface(clientID int64) ControlConnectionAccessor {
	if a.sessionMgr == nil {
		return nil
	}
	conn := a.sessionMgr.GetControlConnectionInterface(clientID)
	return adaptControlConnection(conn)
}

func (a *apiSessionManagerAdapter) BroadcastConfigPush(clientID int64, configBody string) error {
	if a.sessionMgr == nil {
		return fmt.Errorf("session manager is nil")
	}
	return a.sessionMgr.BroadcastConfigPush(clientID, configBody)
}

func (a *apiSessionManagerAdapter) GetNodeID() string {
	if a.sessionMgr == nil {
		return ""
	}
	return a.sessionMgr.GetNodeID()
}

func (a *apiSessionManagerAdapter) GetTunnelBridgeByConnectionID(connID string) session.TunnelBridgeAccessor {
	if a.sessionMgr == nil {
		return nil
	}
	return a.sessionMgr.GetTunnelBridgeByConnectionID(connID)
}

func (a *apiSessionManagerAdapter) GetTunnelBridgeByMappingID(mappingID string, clientID int64) session.TunnelBridgeAccessor {
	if a.sessionMgr == nil {
		return nil
	}
	return a.sessionMgr.GetTunnelBridgeByMappingID(mappingID, clientID)
}

// CreateConnection 创建连接（实现 SessionManagerWithConnection 接口）
func (a *apiSessionManagerAdapter) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	if a.sessionMgr == nil {
		return nil, fmt.Errorf("session manager is nil")
	}
	// session.SessionManager 直接实现了 CreateConnection 方法
	return a.sessionMgr.CreateConnection(reader, writer)
}

// GetConnection 获取连接（实现 SessionManagerWithConnection 接口）
func (a *apiSessionManagerAdapter) GetConnection(connID string) (*types.Connection, bool) {
	if a.sessionMgr == nil {
		return nil, false
	}
	// session.SessionManager 直接实现了 GetConnection 方法
	return a.sessionMgr.GetConnection(connID)
}

// HandlePacket 处理数据包（用于握手等流程）
func (a *apiSessionManagerAdapter) HandlePacket(connPacket *types.StreamPacket) error {
	if a.sessionMgr == nil {
		return fmt.Errorf("session manager is nil")
	}
	// session.SessionManager 直接实现了 HandlePacket 方法
	return a.sessionMgr.HandlePacket(connPacket)
}

// AdaptSessionManager 将 session.SessionManager 适配为 api.SessionManager
func AdaptSessionManager(sessionMgr *session.SessionManager) SessionManager {
	if sessionMgr == nil {
		return nil
	}
	return &apiSessionManagerAdapter{sessionMgr: sessionMgr}
}

// ManagementAPIServer Management API 服务器
type ManagementAPIServer struct {
	*dispose.ManagerBase

	config       *APIConfig
	cloudControl managers.CloudControlAPI
	router       *mux.Router
	server       *http.Server
	sessionMgr   SessionManager // 用于推送配置给客户端

	// 新的连接码系统
	connCodeHandlers *ConnectionCodeHandlers

	// 健康检查
	healthManager *health.HealthManager

	// pprof 自动抓取器
	pprofCapture *PProfCapture

	// HTTP 长轮询连接注册表
	httppollRegistry *httppoll.ConnectionRegistry
}

// APIConfig API 配置
type APIConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenAddr string `yaml:"listen_addr"`

	// 认证配置
	Auth AuthConfig `yaml:"auth"`

	// CORS配置
	CORS CORSConfig `yaml:"cors"`

	// 限流配置
	RateLimit RateLimitConfig `yaml:"rate_limit"`

	// PProf 配置
	PProf PProfConfig `yaml:"pprof"`
}

// PProfConfig PProf 性能分析配置
type PProfConfig struct {
	Enabled     bool   `yaml:"enabled"`      // 是否启用 pprof
	DataDir     string `yaml:"data_dir"`     // pprof 数据保存目录
	Retention   int    `yaml:"retention"`    // 保留分钟数（默认10分钟）
	AutoCapture bool   `yaml:"auto_capture"` // 是否自动抓取（默认true）
}

// AuthConfig 认证配置
type AuthConfig struct {
	Type   string `yaml:"type"`   // api_key / jwt / none
	Secret string `yaml:"secret"` // API 密钥或 JWT 密钥
}

// CORSConfig CORS 配置
type CORSConfig struct {
	Enabled        bool     `yaml:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second"`
	Burst             int  `yaml:"burst"`
}

// NewManagementAPIServer 创建 Management API 服务器
func NewManagementAPIServer(
	ctx context.Context,
	config *APIConfig,
	cloudControl managers.CloudControlAPI,
	connCodeService *services.ConnectionCodeService,
	healthManager *health.HealthManager,
) *ManagementAPIServer {
	s := &ManagementAPIServer{
		ManagerBase:   dispose.NewManager("ManagementAPIServer", ctx),
		config:        config,
		cloudControl:  cloudControl,
		router:        mux.NewRouter(),
		healthManager: healthManager,
	}

	// 初始化连接码handlers（如果提供了service）
	if connCodeService != nil {
		s.connCodeHandlers = NewConnectionCodeHandlers(connCodeService)
	}

	// 注册路由
	s.registerRoutes()

	// 注册 HTTP 长轮询路由（独立路径）
	s.registerHTTPLongPollingRoutes()

	// 注册 pprof 性能分析路由
	s.registerPProfRoutes()

	// 初始化 pprof 自动抓取器
	if config.PProf.Enabled && config.PProf.AutoCapture {
		s.pprofCapture = NewPProfCapture(ctx, &config.PProf)
		s.AddCleanHandler(func() error {
			return s.pprofCapture.Stop()
		})
	}

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Addr:         config.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 添加清理处理器
	s.AddCleanHandler(func() error {
		utils.Infof("ManagementAPIServer: shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	})

	return s
}

// Start 启动服务器
func (s *ManagementAPIServer) Start() error {
	utils.Infof("ManagementAPIServer: starting on %s", s.config.ListenAddr)
	utils.Infof("ManagementAPIServer: API base path: http://%s/tunnox/v1", s.config.ListenAddr)
	utils.Infof("ManagementAPIServer: HTTP long polling endpoints:")
	utils.Infof("  - POST http://%s/tunnox/v1/push (client -> server)", s.config.ListenAddr)
	utils.Infof("  - GET  http://%s/tunnox/v1/poll (server -> client)", s.config.ListenAddr)
	if s.config.PProf.Enabled {
		utils.Infof("ManagementAPIServer: pprof enabled at http://%s/tunnox/v1/debug/pprof/", s.config.ListenAddr)
		if s.config.PProf.AutoCapture && s.pprofCapture != nil {
			if err := s.pprofCapture.Start(); err != nil {
				utils.Warnf("ManagementAPIServer: failed to start pprof capture: %v", err)
			}
		}
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.Errorf("ManagementAPIServer: ListenAndServe error: %v", err)
		}
	}()

	return nil
}

// registerRoutes 注册所有路由
func (s *ManagementAPIServer) registerRoutes() {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 健康检查端点（统一到 /tunnox/v1，不需要认证）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	healthRouter := s.router.PathPrefix("/tunnox/v1").Subrouter()
	healthRouter.HandleFunc("/health", s.handleHealth).Methods("GET")
	healthRouter.HandleFunc("/ready", s.handleReady).Methods("GET")

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// API 路由（需要认证）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// API 基础路径（统一使用 /tunnox/v1）
	api := s.router.PathPrefix("/tunnox/v1").Subrouter()

	// 应用中间件
	api.Use(s.loggingMiddleware)
	api.Use(s.corsMiddleware)
	if s.config.Auth.Type != "none" {
		api.Use(s.authMiddleware)
	}

	// 用户管理路由
	api.HandleFunc("/users", s.handleCreateUser).Methods("POST")
	api.HandleFunc("/users/{user_id}", s.handleGetUser).Methods("GET")
	api.HandleFunc("/users/{user_id}", s.handleUpdateUser).Methods("PUT")
	api.HandleFunc("/users/{user_id}", s.handleDeleteUser).Methods("DELETE")
	api.HandleFunc("/users", s.handleListUsers).Methods("GET")
	api.HandleFunc("/users/{user_id}/clients", s.handleListUserClients).Methods("GET")
	api.HandleFunc("/users/{user_id}/mappings", s.handleListUserMappings).Methods("GET")

	// 客户端管理路由
	api.HandleFunc("/clients", s.handleListAllClients).Methods("GET") // 新增：列出所有客户端
	api.HandleFunc("/clients", s.handleCreateClient).Methods("POST")
	api.HandleFunc("/clients/{client_id}", s.handleGetClient).Methods("GET")
	api.HandleFunc("/clients/{client_id}", s.handleUpdateClient).Methods("PUT")
	api.HandleFunc("/clients/{client_id}", s.handleDeleteClient).Methods("DELETE")
	api.HandleFunc("/clients/{client_id}/disconnect", s.handleDisconnectClient).Methods("POST")
	api.HandleFunc("/clients/{client_id}/mappings", s.handleListClientMappings).Methods("GET")
	api.HandleFunc("/clients/{client_id}/connections", s.handleListClientConnections).Methods("GET") // 新增：客户端连接
	api.HandleFunc("/clients/claim", s.handleClaimClient).Methods("POST")

	// 批量客户端操作
	api.HandleFunc("/clients/batch/disconnect", s.handleBatchDisconnectClients).Methods("POST") // 新增：批量下线

	// 端口映射管理路由
	api.HandleFunc("/mappings", s.handleListAllMappings).Methods("GET") // 新增：列出所有映射
	api.HandleFunc("/mappings", s.handleCreateMapping).Methods("POST")
	api.HandleFunc("/mappings/{mapping_id}", s.handleGetMapping).Methods("GET")
	api.HandleFunc("/mappings/{mapping_id}", s.handleUpdateMapping).Methods("PUT")
	api.HandleFunc("/mappings/{mapping_id}", s.handleDeleteMapping).Methods("DELETE")
	api.HandleFunc("/mappings/{mapping_id}/connections", s.handleListMappingConnections).Methods("GET") // 新增：映射连接

	// 批量映射操作
	api.HandleFunc("/mappings/batch/delete", s.handleBatchDeleteMappings).Methods("POST") // 新增：批量删除
	api.HandleFunc("/mappings/batch/update", s.handleBatchUpdateMappings).Methods("POST") // 新增：批量更新

	// 连接码管理路由（新的授权系统）
	if s.connCodeHandlers != nil {
		api.HandleFunc("/connection-codes", s.connCodeHandlers.HandleCreateConnectionCode).Methods("POST")
		api.HandleFunc("/connection-codes/{code}/activate", s.connCodeHandlers.HandleActivateConnectionCode).Methods("POST")
		api.HandleFunc("/connection-codes/{code}", s.connCodeHandlers.HandleRevokeConnectionCode).Methods("DELETE")
		api.HandleFunc("/connection-codes", s.connCodeHandlers.HandleListConnectionCodes).Methods("GET")

		// 隧道映射管理路由（新系统）
		api.HandleFunc("/tunnel-mappings", s.connCodeHandlers.HandleListMappings).Methods("GET")
		api.HandleFunc("/tunnel-mappings/{id}", s.connCodeHandlers.HandleRevokeMapping).Methods("DELETE")
	}

	// 统计查询路由
	api.HandleFunc("/stats/users/{user_id}", s.handleGetUserStats).Methods("GET")
	api.HandleFunc("/stats/clients/{client_id}", s.handleGetClientStats).Methods("GET")
	api.HandleFunc("/stats/system", s.handleGetSystemStats).Methods("GET")
	api.HandleFunc("/stats/traffic", s.handleGetTrafficStats).Methods("GET")        // 新增：流量时序统计
	api.HandleFunc("/stats/connections", s.handleGetConnectionStats).Methods("GET") // 新增：连接时序统计

	// 节点管理路由
	api.HandleFunc("/nodes", s.handleListNodes).Methods("GET")
	api.HandleFunc("/nodes/{node_id}", s.handleGetNode).Methods("GET")

	// 搜索路由
	api.HandleFunc("/search/users", s.handleSearchUsers).Methods("GET")       // 新增：搜索用户
	api.HandleFunc("/search/clients", s.handleSearchClients).Methods("GET")   // 新增：搜索客户端
	api.HandleFunc("/search/mappings", s.handleSearchMappings).Methods("GET") // 新增：搜索映射

	// 连接管理路由
	api.HandleFunc("/connections", s.handleListAllConnections).Methods("GET")           // 新增：列出所有连接
	api.HandleFunc("/connections/{conn_id}", s.handleCloseConnection).Methods("DELETE") // 新增：关闭连接

	// 认证路由
	api.HandleFunc("/auth/login", s.handleLogin).Methods("POST")           // 新增：登录
	api.HandleFunc("/auth/refresh", s.handleRefreshToken).Methods("POST")  // 新增：刷新token
	api.HandleFunc("/auth/revoke", s.handleRevokeToken).Methods("POST")    // 新增：撤销token
	api.HandleFunc("/auth/validate", s.handleValidateToken).Methods("GET") // 新增：验证token

	// 配额管理路由
	api.HandleFunc("/users/{user_id}/quota", s.handleGetUserQuota).Methods("GET")    // 新增：获取配额
	api.HandleFunc("/users/{user_id}/quota", s.handleUpdateUserQuota).Methods("PUT") // 新增：更新配额

	// 注意：健康检查端点已在 registerRoutes() 开头单独注册为免认证，不在此处重复注册
}

// registerHTTPLongPollingRoutes 注册 HTTP 长轮询路由（统一到 /tunnox/v1）
// 注意：HTTP Long Polling 端点不需要 HTTP 层面的认证，认证在握手阶段进行（与其他传输协议一致）
func (s *ManagementAPIServer) registerHTTPLongPollingRoutes() {
	// HTTP 长轮询端点（用于客户端连接，统一到 /tunnox/v1）
	longPollRouter := s.router.PathPrefix("/tunnox/v1").Subrouter()

	// 不应用认证中间件，认证在握手阶段进行
	// 应用 CORS 中间件
	longPollRouter.Use(s.corsMiddleware)

	// 注册长轮询端点
	longPollRouter.HandleFunc("/push", s.handleHTTPPush).Methods("POST")
	longPollRouter.HandleFunc("/poll", s.handleHTTPPoll).Methods("GET")
}

// registerPProfRoutes 注册 pprof 性能分析路由（统一到 /tunnox/v1）
func (s *ManagementAPIServer) registerPProfRoutes() {
	if !s.config.PProf.Enabled {
		return
	}

	// pprof 路由需要认证（如果配置了认证），统一到 /tunnox/v1/debug/pprof
	pprofRouter := s.router.PathPrefix("/tunnox/v1/debug/pprof").Subrouter()

	// 应用认证中间件（如果需要）
	if s.config.Auth.Type != "none" {
		pprofRouter.Use(s.authMiddleware)
	}

	// 注册 pprof 路由（使用 http.DefaultServeMux，它已经注册了所有 pprof 路由）
	pprofRouter.PathPrefix("/").Handler(http.DefaultServeMux)

	utils.Infof("ManagementAPIServer: pprof enabled at http://%s/tunnox/v1/debug/pprof/", s.config.ListenAddr)
}

// ResponseData 统一响应结构
type ResponseData struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// respondJSON 发送 JSON 响应（使用统一响应格式）
func (s *ManagementAPIServer) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	helper := NewResponseHelper()
	if statusCode >= 200 && statusCode < 300 {
		helper.Success(w, statusCode, data)
	} else {
		// 对于非成功状态码，如果data是字符串，作为错误消息
		if msg, ok := data.(string); ok {
			helper.Error(w, statusCode, msg)
		} else {
			helper.Success(w, statusCode, data)
		}
	}
}

// respondError 发送错误响应（使用统一响应格式）
func (s *ManagementAPIServer) respondError(w http.ResponseWriter, statusCode int, message string) {
	helper := NewResponseHelper()
	helper.Error(w, statusCode, message)
}

// handleHealth 健康检查
func (s *ManagementAPIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.healthManager == nil {
		// 如果没有配置HealthManager，返回简单的OK
		s.respondJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
		return
	}

	info := s.healthManager.GetHealthInfo()

	// 根据状态返回不同的HTTP状态码
	var statusCode int
	switch info.Status {
	case health.HealthStatusHealthy:
		statusCode = http.StatusOK // 200
	case health.HealthStatusDraining:
		statusCode = http.StatusServiceUnavailable // 503 (告诉负载均衡器不要路由新请求)
	case health.HealthStatusUnhealthy:
		statusCode = http.StatusServiceUnavailable // 503
	default:
		statusCode = http.StatusInternalServerError // 500
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(info)
}

// getInt64PathVar 获取路径参数（int64）
func getInt64PathVar(r *http.Request, key string) (int64, error) {
	vars := mux.Vars(r)
	str := vars[key]
	if str == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	val, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", key, err)
	}
	return val, nil
}

// getStringPathVar 获取路径参数（string）
func getStringPathVar(r *http.Request, key string) (string, error) {
	vars := mux.Vars(r)
	str := vars[key]
	if str == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return str, nil
}

// parseJSONBody 解析 JSON 请求体
func parseJSONBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %v", err)
	}
	return nil
}

// loggingMiddleware 日志中间件
func (s *ManagementAPIServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 调用下一个处理器
		next.ServeHTTP(w, r)

		// 记录日志
		utils.Debugf("API: %s %s - %s", r.Method, r.RequestURI, time.Since(start))
	})
}

// corsMiddleware CORS 中间件
func (s *ManagementAPIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.config.CORS.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")

		// 检查 origin 是否允许
		allowed := false
		for _, allowedOrigin := range s.config.CORS.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(s.config.CORS.AllowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(s.config.CORS.AllowedHeaders, ", "))
			w.Header().Set("Access-Control-Max-Age", "3600")
		}

		// 处理预检请求
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware 认证中间件
func (s *ManagementAPIServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 获取 Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.respondError(w, http.StatusUnauthorized, "Missing authorization header")
			return
		}

		// 检查格式：Bearer <token>
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.respondError(w, http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		token := parts[1]

		switch s.config.Auth.Type {
		case "api_key", "bearer":
			// API Key 或 Bearer Token 认证（两者使用相同的验证逻辑）
			if token != s.config.Auth.Secret {
				s.respondError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

		case "jwt":
			// JWT 认证
			_, err := s.cloudControl.ValidateJWTToken(token)
			if err != nil {
				s.respondError(w, http.StatusUnauthorized, fmt.Sprintf("Invalid JWT token: %v", err))
				return
			}

		default:
			s.respondError(w, http.StatusInternalServerError, "Unknown auth type")
			return
		}

		// 认证成功，继续处理
		next.ServeHTTP(w, r)
	})
}

// handleReady 就绪检查端点
//
// GET /ready
//
// 检查服务器是否准备好接受新连接
// 用于Kubernetes等容器编排系统的readiness probe
func (s *ManagementAPIServer) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.healthManager == nil || s.healthManager.IsAcceptingConnections() {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"ready":  true,
			"status": "accepting_connections",
		})
		return
	}

	// 不接受新连接
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":  false,
		"status": s.healthManager.GetStatus(),
	})
}
