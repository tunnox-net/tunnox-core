package httpservice

import (
	"encoding/json"
	"io"
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

// maxBytesErrorMiddleware 处理请求体过大错误的中间件
// 捕获 http.MaxBytesReader 返回的错误并返回友好的错误响应
func maxBytesErrorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// limitedReader 带限制的 Reader 包装器
type limitedReader struct {
	r         io.ReadCloser
	remaining int64
}

func (l *limitedReader) Read(p []byte) (n int, err error) {
	if l.remaining <= 0 {
		return 0, &http.MaxBytesError{Limit: 0}
	}
	if int64(len(p)) > l.remaining {
		p = p[0:l.remaining]
	}
	n, err = l.r.Read(p)
	l.remaining -= int64(n)
	return
}

func (l *limitedReader) Close() error {
	return l.r.Close()
}

// authMiddleware 认证中间件
func authMiddleware(config *AuthConfig, validateJWT func(token string) (*JWTClaims, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if config == nil || config.Type == "none" {
				next.ServeHTTP(w, r)
				return
			}

			// 获取 Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondError(w, http.StatusUnauthorized, "Missing authorization header")
				return
			}

			// 检查格式：Bearer <token>
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				respondError(w, http.StatusUnauthorized, "Invalid authorization header format")
				return
			}

			token := parts[1]

			switch config.Type {
			case "api_key", "bearer":
				// API Key 或 Bearer Token 认证
				if token != config.Secret {
					respondError(w, http.StatusUnauthorized, "Invalid API key")
					return
				}

			case "jwt":
				// JWT 认证
				if validateJWT == nil {
					respondError(w, http.StatusInternalServerError, "JWT validation not configured")
					return
				}
				_, err := validateJWT(token)
				if err != nil {
					respondError(w, http.StatusUnauthorized, "Invalid JWT token: "+err.Error())
					return
				}

			default:
				respondError(w, http.StatusInternalServerError, "Unknown auth type")
				return
			}

			// 认证成功，继续处理
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

// respondError 发送错误响应
func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Success: false,
		Error:   message,
	}

	json.NewEncoder(w).Encode(response)
}

// respondSuccess 发送成功响应
// 使用泛型函数确保类型安全
func respondSuccess[T any](w http.ResponseWriter, data T) {
	respondJSON(w, http.StatusOK, data)
}
