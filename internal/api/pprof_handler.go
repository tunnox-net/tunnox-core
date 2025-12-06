package api

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/gorilla/mux"
)

// PProfHandler pprof 标准化处理器
// 提供统一的 pprof 接口，并支持权限保护
type PProfHandler struct {
	enabled bool
	router  *mux.Router
}

// NewPProfHandler 创建 pprof 处理器
func NewPProfHandler(enabled bool) *PProfHandler {
	return &PProfHandler{
		enabled: enabled,
		router:  mux.NewRouter(),
	}
}

// RegisterRoutes 注册 pprof 路由
// 路径：/tunnox/v1/debug/pprof/
func (h *PProfHandler) RegisterRoutes(router *mux.Router, authMiddleware mux.MiddlewareFunc) {
	if !h.enabled {
		return
	}

	// 创建 pprof 子路由
	pprofRouter := router.PathPrefix("/tunnox/v1/debug/pprof").Subrouter()

	// 应用认证中间件（必须认证才能访问 pprof）
	if authMiddleware != nil {
		pprofRouter.Use(authMiddleware)
	}

	// 注册标准 pprof 路由
	h.registerStandardRoutes(pprofRouter)
}

// registerStandardRoutes 注册标准 pprof 路由
func (h *PProfHandler) registerStandardRoutes(router *mux.Router) {
	// 索引页面：列出所有可用的 profile
	router.HandleFunc("/", pprof.Index).Methods("GET")

	// CPU profile（需要采样时间，通过 seconds 参数指定）
	router.HandleFunc("/profile", h.handleProfile).Methods("GET")

	// 堆内存 profile
	router.HandleFunc("/heap", pprof.Handler("heap").ServeHTTP).Methods("GET")

	// Goroutine profile
	router.HandleFunc("/goroutine", pprof.Handler("goroutine").ServeHTTP).Methods("GET")

	// 内存分配 profile
	router.HandleFunc("/allocs", pprof.Handler("allocs").ServeHTTP).Methods("GET")

	// 阻塞 profile（需要先启用：runtime.SetBlockProfileRate(1)）
	router.HandleFunc("/block", pprof.Handler("block").ServeHTTP).Methods("GET")

	// 互斥锁 profile（需要先启用：runtime.SetMutexProfileFraction(1)）
	router.HandleFunc("/mutex", pprof.Handler("mutex").ServeHTTP).Methods("GET")

	// 命令行工具（go tool pprof）
	router.HandleFunc("/cmdline", pprof.Cmdline).Methods("GET")

	// 符号表
	router.HandleFunc("/symbol", pprof.Symbol).Methods("GET", "POST")

	// 追踪（需要采样时间，通过 seconds 参数指定）
	router.HandleFunc("/trace", h.handleTrace).Methods("GET")
}

// handleProfile 处理 CPU profile 请求
// 支持 seconds 参数指定采样时间（默认 30 秒）
func (h *PProfHandler) handleProfile(w http.ResponseWriter, r *http.Request) {
	// 从查询参数获取采样时间
	seconds := r.URL.Query().Get("seconds")
	if seconds == "" {
		seconds = "30" // 默认 30 秒
	}

	// 设置采样时间
	var duration int
	if _, err := fmt.Sscanf(seconds, "%d", &duration); err != nil || duration <= 0 {
		http.Error(w, "Invalid seconds parameter", http.StatusBadRequest)
		return
	}

	// 限制最大采样时间为 300 秒（5 分钟），防止资源耗尽
	if duration > 300 {
		duration = 300
	}

	// 调用标准 pprof.Profile
	pprof.Profile(w, r)
}

// handleTrace 处理 trace 请求
// 支持 seconds 参数指定追踪时间（默认 1 秒）
func (h *PProfHandler) handleTrace(w http.ResponseWriter, r *http.Request) {
	// 从查询参数获取追踪时间
	seconds := r.URL.Query().Get("seconds")
	if seconds == "" {
		seconds = "1" // 默认 1 秒
	}

	// 设置追踪时间
	var duration int
	if _, err := fmt.Sscanf(seconds, "%d", &duration); err != nil || duration <= 0 {
		http.Error(w, "Invalid seconds parameter", http.StatusBadRequest)
		return
	}

	// 限制最大追踪时间为 10 秒，防止资源耗尽
	if duration > 10 {
		duration = 10
	}

	// 调用标准 pprof.Trace
	pprof.Trace(w, r)
}

// EnableBlockProfile 启用阻塞 profile
// 需要在应用启动时调用
func EnableBlockProfile() {
	runtime.SetBlockProfileRate(1) // 记录所有阻塞事件
}

// EnableMutexProfile 启用互斥锁 profile
// 需要在应用启动时调用
func EnableMutexProfile() {
	runtime.SetMutexProfileFraction(1) // 记录所有互斥锁事件
}

