// Package management 提供 Management API 模块
// 包含用户、客户端、映射等管理接口
package management

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/health"
	"tunnox-core/internal/httpservice"

	"github.com/gorilla/mux"
)

// ManagementModule 管理 API 模块
type ManagementModule struct {
	*dispose.ServiceBase

	config       *httpservice.ManagementAPIModuleConfig
	deps         *httpservice.ModuleDependencies
	cloudControl managers.CloudControlAPI

	// 连接码服务
	connCodeService *services.ConnectionCodeService

	// 健康检查
	healthManager *health.HealthManager

	// pprof 自动抓取器
	pprofCapture *PProfCapture

	// 认证中间件
	authMiddleware mux.MiddlewareFunc
}

// NewManagementModule 创建管理 API 模块
func NewManagementModule(
	ctx context.Context,
	config *httpservice.ManagementAPIModuleConfig,
	cloudControl managers.CloudControlAPI,
	connCodeService *services.ConnectionCodeService,
	healthManager *health.HealthManager,
) *ManagementModule {
	m := &ManagementModule{
		ServiceBase:     dispose.NewService("ManagementModule", ctx),
		config:          config,
		cloudControl:    cloudControl,
		connCodeService: connCodeService,
		healthManager:   healthManager,
	}

	return m
}

// Name 返回模块名称
func (m *ManagementModule) Name() string {
	return "ManagementAPI"
}

// SetDependencies 注入依赖
func (m *ManagementModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
	if deps.CloudControl != nil {
		m.cloudControl = deps.CloudControl
	}
	if deps.HealthManager != nil {
		m.healthManager = deps.HealthManager
	}
}

// RegisterRoutes 注册路由
func (m *ManagementModule) RegisterRoutes(router *mux.Router) {
	// API 基础路径
	api := router.PathPrefix("/tunnox/v1").Subrouter()

	// 应用认证中间件
	if m.config.Auth.Type != "none" {
		api.Use(m.createAuthMiddleware())
	}

	// 用户管理路由
	api.HandleFunc("/users", m.handleCreateUser).Methods("POST")
	api.HandleFunc("/users/{user_id}", m.handleGetUser).Methods("GET")
	api.HandleFunc("/users/{user_id}", m.handleUpdateUser).Methods("PUT")
	api.HandleFunc("/users/{user_id}", m.handleDeleteUser).Methods("DELETE")
	api.HandleFunc("/users", m.handleListUsers).Methods("GET")
	api.HandleFunc("/users/{user_id}/clients", m.handleListUserClients).Methods("GET")
	api.HandleFunc("/users/{user_id}/mappings", m.handleListUserMappings).Methods("GET")

	// 客户端管理路由
	api.HandleFunc("/clients", m.handleListAllClients).Methods("GET")
	api.HandleFunc("/clients", m.handleCreateClient).Methods("POST")
	api.HandleFunc("/clients/{client_id}", m.handleGetClient).Methods("GET")
	api.HandleFunc("/clients/{client_id}", m.handleUpdateClient).Methods("PUT")
	api.HandleFunc("/clients/{client_id}", m.handleDeleteClient).Methods("DELETE")
	api.HandleFunc("/clients/{client_id}/disconnect", m.handleDisconnectClient).Methods("POST")
	api.HandleFunc("/clients/{client_id}/mappings", m.handleListClientMappings).Methods("GET")

	// 端口映射管理路由
	api.HandleFunc("/mappings", m.handleListAllMappings).Methods("GET")
	api.HandleFunc("/mappings", m.handleCreateMapping).Methods("POST")
	api.HandleFunc("/mappings/{mapping_id}", m.handleGetMapping).Methods("GET")
	api.HandleFunc("/mappings/{mapping_id}", m.handleUpdateMapping).Methods("PUT")
	api.HandleFunc("/mappings/{mapping_id}", m.handleDeleteMapping).Methods("DELETE")

	// HTTP 域名映射专用路由
	api.HandleFunc("/mappings/check-subdomain", m.handleCheckSubdomain).Methods("GET")

	// 统计查询路由
	api.HandleFunc("/stats", m.handleGetSystemStats).Methods("GET")

	// 节点管理路由
	api.HandleFunc("/nodes", m.handleListNodes).Methods("GET")
	api.HandleFunc("/nodes/{node_id}", m.handleGetNode).Methods("GET")

	// 注册 pprof 路由
	if m.config.PProf.Enabled {
		m.registerPProfRoutes(router)
	}

	corelog.Infof("ManagementModule: registered API routes at /tunnox/v1/*")
}

// Start 启动模块
func (m *ManagementModule) Start() error {
	// 初始化 pprof 自动抓取器
	if m.config.PProf.Enabled && m.config.PProf.AutoCapture {
		m.pprofCapture = NewPProfCapture(m.Ctx(), &m.config.PProf)
		if err := m.pprofCapture.Start(); err != nil {
			corelog.Warnf("ManagementModule: failed to start pprof capture: %v", err)
		}
		m.AddCleanHandler(func() error {
			return m.pprofCapture.Stop()
		})
	}

	corelog.Infof("ManagementModule: started")
	return nil
}

// Stop 停止模块
func (m *ManagementModule) Stop() error {
	corelog.Infof("ManagementModule: stopped")
	return nil
}

// createAuthMiddleware 创建认证中间件
func (m *ManagementModule) createAuthMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 获取 Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				m.respondError(w, http.StatusUnauthorized, "Missing authorization header")
				return
			}

			// 检查格式：Bearer <token>
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				m.respondError(w, http.StatusUnauthorized, "Invalid authorization header format")
				return
			}

			token := parts[1]

			switch m.config.Auth.Type {
			case "api_key", "bearer":
				if token != m.config.Auth.Secret {
					m.respondError(w, http.StatusUnauthorized, "Invalid API key")
					return
				}

			case "jwt":
				if m.cloudControl == nil {
					m.respondError(w, http.StatusInternalServerError, "JWT validation not configured")
					return
				}
				_, err := m.cloudControl.ValidateJWTToken(token)
				if err != nil {
					m.respondError(w, http.StatusUnauthorized, fmt.Sprintf("Invalid JWT token: %v", err))
					return
				}

			default:
				m.respondError(w, http.StatusInternalServerError, "Unknown auth type")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// registerPProfRoutes 注册 pprof 路由
func (m *ManagementModule) registerPProfRoutes(router *mux.Router) {
	pprofHandler := NewPProfHandler(true)

	var authMiddleware mux.MiddlewareFunc
	if m.config.Auth.Type != "none" {
		authMiddleware = m.createAuthMiddleware()
	}

	pprofHandler.RegisterRoutes(router, authMiddleware)
	corelog.Infof("ManagementModule: pprof enabled at /tunnox/v1/debug/pprof/")
}

// respondJSON 发送 JSON 响应
func (m *ManagementModule) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"success": statusCode >= 200 && statusCode < 300,
		"data":    data,
	}

	json.NewEncoder(w).Encode(response)
}

// respondError 发送错误响应
func (m *ManagementModule) respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"success": false,
		"error":   message,
	}

	json.NewEncoder(w).Encode(response)
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
