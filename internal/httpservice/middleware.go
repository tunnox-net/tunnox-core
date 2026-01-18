package httpservice

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	corelog "tunnox-core/internal/core/log"
)

// APIResponse 泛型统一响应结构
// 使用泛型确保 Data 字段的类型安全
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// MessageResponse 简单消息响应结构
// 用于替代 map[string]string{"message": "..."}
type MessageResponse struct {
	Message string `json:"message"`
}

// SubdomainCheckResponse 子域名检查响应结构
type SubdomainCheckResponse struct {
	Available  bool   `json:"available"`
	FullDomain string `json:"full_domain"`
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

// ReadyResponse 就绪检查响应
type ReadyResponse struct {
	Ready  bool   `json:"ready"`
	Status string `json:"status"`
}

// JWTClaims JWT 令牌声明
// 用于 JWT 验证函数的返回值
type JWTClaims struct {
	UserID    string `json:"user_id,omitempty"`
	ClientID  int64  `json:"client_id,omitempty"`
	ExpiresAt int64  `json:"exp,omitempty"`
	IssuedAt  int64  `json:"iat,omitempty"`
}

// loggingMiddleware 日志中间件
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 调用下一个处理器
		next.ServeHTTP(w, r)

		// 记录日志
		corelog.Debugf("HTTP: %s %s - %s", r.Method, r.RequestURI, time.Since(start))
	})
}

// corsMiddleware CORS 中间件
func corsMiddleware(config *CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if config == nil || !config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")

			// 检查 origin 是否允许
			allowed := false
			for _, allowedOrigin := range config.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
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
}

// bodySizeLimitMiddleware 请求体大小限制中间件
// 防止恶意客户端发送超大请求导致内存耗尽
func bodySizeLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			// 使用 http.MaxBytesReader 限制请求体大小
			// 超过限制时会返回 413 Request Entity Too Large
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			next.ServeHTTP(w, r)
		})
	}
}

// respondJSON 发送 JSON 响应
// 使用泛型函数确保类型安全
func respondJSON[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := APIResponse[T]{
		Success: statusCode >= 200 && statusCode < 300,
		Data:    data,
	}

	json.NewEncoder(w).Encode(response)
}
