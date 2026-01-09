package httpservice

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/health"

	"github.com/gorilla/mux"
)

// HTTPService 统一 HTTP 服务
// 管理所有 HTTP 模块，提供统一的入口
type HTTPService struct {
	*dispose.ManagerBase

	config  *HTTPServiceConfig
	router  *mux.Router
	server  *http.Server
	modules []HTTPModule
	deps    *ModuleDependencies

	// 域名注册表
	domainRegistry *DomainRegistry

	// 健康检查
	healthManager *health.HealthManager

	// JWT 验证函数（可选）
	validateJWT func(token string) (*JWTClaims, error)
}

// NewHTTPService 创建统一 HTTP 服务
func NewHTTPService(
	ctx context.Context,
	config *HTTPServiceConfig,
	cloudControl managers.CloudControlAPI,
	stor storage.Storage,
	healthManager *health.HealthManager,
) *HTTPService {
	if config == nil {
		config = DefaultHTTPServiceConfig()
	}

	s := &HTTPService{
		ManagerBase:   dispose.NewManager("HTTPService", ctx),
		config:        config,
		router:        mux.NewRouter(),
		modules:       make([]HTTPModule, 0),
		healthManager: healthManager,
	}

	// 初始化域名注册表
	if config.Modules.DomainProxy.Enabled {
		s.domainRegistry = NewDomainRegistry(config.Modules.DomainProxy.BaseDomains)
	}

	// 初始化依赖
	s.deps = &ModuleDependencies{
		CloudControl:   cloudControl,
		Storage:        stor,
		HealthManager:  healthManager,
		DomainRegistry: s.domainRegistry,
	}

	// 创建 HTTP 服务器
	maxHeaderBytes := int(config.MaxHeaderBytes)
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = 1 << 20 // 默认 1MB
	}
	s.server = &http.Server{
		Addr:           config.ListenAddr,
		Handler:        s.router,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: maxHeaderBytes,
	}

	// 添加清理处理器
	s.AddCleanHandler(func() error {
		corelog.Infof("HTTPService: shutting down...")
		shutdownCtx, cancel := context.WithTimeout(s.Ctx(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	})

	return s
}

// SetSessionManager 设置会话管理器
func (s *HTTPService) SetSessionManager(sessionMgr SessionManagerInterface) {
	s.deps.SessionMgr = sessionMgr
}

// SetHTTPDomainMappingRepo 设置 HTTP 域名映射仓库
func (s *HTTPService) SetHTTPDomainMappingRepo(repo repos.IHTTPDomainMappingRepository) {
	s.deps.HTTPDomainMappingRepo = repo
}

func (s *HTTPService) SetWebhookManager(mgr managers.WebhookManagerAPI) {
	s.deps.WebhookManager = mgr
}

// SetJWTValidator 设置 JWT 验证函数
func (s *HTTPService) SetJWTValidator(validator func(token string) (*JWTClaims, error)) {
	s.validateJWT = validator
}

// RegisterModule 注册模块
func (s *HTTPService) RegisterModule(module HTTPModule) {
	if module == nil {
		return
	}

	// 注入依赖
	module.SetDependencies(s.deps)

	// 添加到模块列表
	s.modules = append(s.modules, module)

	corelog.Infof("HTTPService: registered module %s", module.Name())
}

// GetDomainRegistry 获取域名注册表
func (s *HTTPService) GetDomainRegistry() *DomainRegistry {
	return s.domainRegistry
}

// GetDependencies 获取依赖（供模块使用）
func (s *HTTPService) GetDependencies() *ModuleDependencies {
	return s.deps
}

// Start 启动服务
func (s *HTTPService) Start() error {
	corelog.Infof("HTTPService: starting on %s", s.config.ListenAddr)

	// 注册通用中间件
	s.router.Use(loggingMiddleware)
	s.router.Use(corsMiddleware(&s.config.CORS))
	// 注册请求体大小限制中间件
	if s.config.MaxBodySize > 0 {
		s.router.Use(bodySizeLimitMiddleware(s.config.MaxBodySize))
	}

	// 注册首页（防止防火墙因无响应而拉黑）
	s.registerLandingPage()

	// 注册健康检查端点（不需要认证）
	s.registerHealthRoutes()

	// 注册各模块路由
	for _, module := range s.modules {
		corelog.Infof("HTTPService: registering routes for module %s", module.Name())
		module.RegisterRoutes(s.router)
	}

	// 启动各模块
	for _, module := range s.modules {
		if err := module.Start(); err != nil {
			corelog.Errorf("HTTPService: failed to start module %s: %v", module.Name(), err)
			return err
		}
		corelog.Infof("HTTPService: started module %s", module.Name())
	}

	// 启动 HTTP 服务器
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			corelog.Errorf("HTTPService: ListenAndServe error: %v", err)
		}
	}()

	s.logEndpoints()

	return nil
}

// Stop 停止服务
func (s *HTTPService) Stop() error {
	corelog.Infof("HTTPService: stopping...")

	// 停止各模块（逆序）
	for i := len(s.modules) - 1; i >= 0; i-- {
		module := s.modules[i]
		if err := module.Stop(); err != nil {
			corelog.Warnf("HTTPService: failed to stop module %s: %v", module.Name(), err)
		}
	}

	// 关闭服务
	return s.Close()
}

// registerHealthRoutes 注册健康检查路由
func (s *HTTPService) registerHealthRoutes() {
	healthRouter := s.router.PathPrefix("/tunnox/v1").Subrouter()
	healthRouter.HandleFunc("/health", s.handleHealth).Methods("GET")
	healthRouter.HandleFunc("/healthz", s.handleHealthz).Methods("GET")
	healthRouter.HandleFunc("/ready", s.handleReady).Methods("GET")
}

// handleHealth 简单健康检查
func (s *HTTPService) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.healthManager == nil {
		respondJSON(w, http.StatusOK, HealthResponse{
			Status: "ok",
			Time:   time.Now().Format(time.RFC3339),
		})
		return
	}

	info := s.healthManager.GetHealthInfo()

	var statusCode int
	switch info.Status {
	case health.HealthStatusHealthy:
		statusCode = http.StatusOK
	case health.HealthStatusDraining:
		statusCode = http.StatusServiceUnavailable
	case health.HealthStatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	default:
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(info)
}

// handleHealthz 增强的健康检查
func (s *HTTPService) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if s.healthManager == nil {
		respondJSON(w, http.StatusOK, HealthResponse{
			Status: "ok",
			Time:   time.Now().Format(time.RFC3339),
		})
		return
	}

	info := s.healthManager.GetHealthInfo()

	var statusCode int
	switch info.Status {
	case health.HealthStatusHealthy:
		statusCode = http.StatusOK
	default:
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(info)
}

// handleReady 就绪检查
func (s *HTTPService) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.healthManager == nil || s.healthManager.IsAcceptingConnections() {
		respondJSON(w, http.StatusOK, ReadyResponse{
			Ready:  true,
			Status: "accepting_connections",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(ReadyResponse{
		Ready:  false,
		Status: string(s.healthManager.GetStatus()),
	})
}

// registerLandingPage 注册首页（防止防火墙因无响应而拉黑）
func (s *HTTPService) registerLandingPage() {
	s.router.HandleFunc("/", s.handleLandingPage).Methods("GET", "HEAD")
}

// handleLandingPage 返回简单的欢迎页面
func (s *HTTPService) handleLandingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>Tunnox Gateway</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
               display: flex; justify-content: center; align-items: center;
               min-height: 100vh; margin: 0; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }
        .container { text-align: center; color: white; padding: 40px; }
        h1 { font-size: 3em; margin-bottom: 10px; }
        p { font-size: 1.2em; opacity: 0.9; }
        .status { display: inline-block; width: 12px; height: 12px; background: #4ade80;
                  border-radius: 50%; margin-right: 8px; animation: pulse 2s infinite; }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Tunnox</h1>
        <p><span class="status"></span>Gateway Online</p>
        <p style="font-size: 0.9em; opacity: 0.7; margin-top: 30px;">Secure Tunnel Service</p>
    </div>
</body>
</html>`))
}

// logEndpoints 打印端点信息
func (s *HTTPService) logEndpoints() {
	corelog.Infof("HTTPService: API base path: http://%s/tunnox/v1", s.config.ListenAddr)
	corelog.Infof("HTTPService: Health endpoints:")
	corelog.Infof("  - GET http://%s/tunnox/v1/health", s.config.ListenAddr)
	corelog.Infof("  - GET http://%s/tunnox/v1/healthz", s.config.ListenAddr)
	corelog.Infof("  - GET http://%s/tunnox/v1/ready", s.config.ListenAddr)

	for _, module := range s.modules {
		corelog.Infof("HTTPService: Module %s enabled", module.Name())
	}
}

// GetRouter 获取路由器（供测试使用）
func (s *HTTPService) GetRouter() *mux.Router {
	return s.router
}

// GetConfig 获取配置
func (s *HTTPService) GetConfig() *HTTPServiceConfig {
	return s.config
}
