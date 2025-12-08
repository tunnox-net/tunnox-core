package registry

import "net/http"

// HTTPRouter HTTP 路由接口（依赖倒置原则）
// 协议层依赖这个抽象接口，而不是具体的 HTTP 服务器实现
type HTTPRouter interface {
	// RegisterRoute 注册路由
	// method: HTTP 方法（GET, POST, PUT, DELETE 等）
	// path: 路由路径（如 "/tunnox/v1/push"）
	// handler: HTTP 请求处理器
	RegisterRoute(method, path string, handler http.HandlerFunc) error

	// RegisterRouteWithMiddleware 注册带中间件的路由
	// method: HTTP 方法
	// path: 路由路径
	// handler: HTTP 请求处理器
	// middlewares: 中间件列表（按顺序应用）
	RegisterRouteWithMiddleware(method, path string, handler http.HandlerFunc, middlewares ...func(http.Handler) http.Handler) error
}

