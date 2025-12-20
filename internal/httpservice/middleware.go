package httpservice

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	corelog "tunnox-core/internal/core/log"
)

// ResponseData 统一响应结构
type ResponseData struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
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

// authMiddleware 认证中间件
func authMiddleware(config *AuthConfig, validateJWT func(token string) (interface{}, error)) func(http.Handler) http.Handler {
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
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ResponseData{
		Success: statusCode >= 200 && statusCode < 300,
		Data:    data,
	}

	json.NewEncoder(w).Encode(response)
}

// respondError 发送错误响应
func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ResponseData{
		Success: false,
		Error:   message,
	}

	json.NewEncoder(w).Encode(response)
}

// respondSuccess 发送成功响应
func respondSuccess(w http.ResponseWriter, data interface{}) {
	respondJSON(w, http.StatusOK, data)
}
