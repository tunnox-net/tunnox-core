package utils

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"tunnox-core/internal/constants"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDMiddleware 请求ID中间件
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(constants.HTTPHeaderXRequestID)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		c.Set("request_id", requestID)
		c.Header(constants.HTTPHeaderXRequestID, requestID)
		c.Next()
	}
}

// LoggingMiddleware 日志中间件
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := c.GetString("request_id")

		// 记录请求开始
		logEntry := WithContext(c.Request.Context()).
			WithRequest(c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent()).
			WithField(constants.LogFieldRequestID, requestID)

		logEntry.Infof(constants.LogMsgHTTPRequestReceived, c.Request.Method, c.Request.URL.Path)

		// 处理请求
		c.Next()

		// 记录请求完成
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		logEntry = logEntry.WithDuration(duration).WithField(constants.LogFieldStatusCode, statusCode)

		if statusCode >= 400 {
			logEntry.Warnf(constants.LogMsgHTTPRequestFailed, c.Request.Method, c.Request.URL.Path, statusCode)
		} else {
			logEntry.Infof(constants.LogMsgHTTPRequestCompleted, c.Request.Method, c.Request.URL.Path, statusCode)
		}
	}
}

// CORSMiddleware CORS中间件
func CORSMiddleware() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{
		constants.HTTPMethodGET,
		constants.HTTPMethodPOST,
		constants.HTTPMethodPUT,
		constants.HTTPMethodDELETE,
		constants.HTTPMethodPATCH,
		"OPTIONS",
	}
	config.AllowHeaders = []string{
		constants.HTTPHeaderContentType,
		constants.HTTPHeaderAuthorization,
		constants.HTTPHeaderXRequestID,
		constants.HTTPHeaderXForwardedFor,
		constants.HTTPHeaderXRealIP,
		constants.HTTPHeaderAccept,
		constants.HTTPHeaderUserAgent,
	}
	config.ExposeHeaders = []string{
		constants.HTTPHeaderXRequestID,
		constants.HTTPHeaderContentType,
	}
	config.AllowCredentials = true
	config.MaxAge = 12 * time.Hour

	return cors.New(config)
}

// RecoveryMiddleware 恢复中间件
func RecoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		requestID := c.GetString("request_id")

		logEntry := WithContext(c.Request.Context()).
			WithRequest(c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent()).
			WithField(constants.LogFieldRequestID, requestID).
			WithError(fmt.Errorf("panic: %v", recovered))

		logEntry.Errorf(constants.LogMsgErrorInternalServer, recovered)

		SendInternalError(c, "Internal server error", fmt.Errorf("panic: %v", recovered))
	})
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(limit, window, context.Background())

	return func(c *gin.Context) {
		if !limiter.Allow() {
			SendError(c, constants.HTTPStatusTooManyRequests, constants.ResponseMsgTooManyRequests, nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		done := make(chan struct{})
		go func() {
			c.Next()
			done <- struct{}{}
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			SendError(c, http.StatusRequestTimeout, "Request timeout", ctx.Err())
			c.Abort()
			return
		}
	}
}

// SizeLimitMiddleware 大小限制中间件
func SizeLimitMiddleware(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}

// SecurityHeadersMiddleware 安全头部中间件
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// MetricsMiddleware 指标中间件
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// 这里可以添加指标收集逻辑
		// 例如：Prometheus metrics, 自定义指标等

		logEntry := WithContext(c.Request.Context()).
			WithRequest(c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent()).
			WithDuration(duration).
			WithField(constants.LogFieldStatusCode, statusCode)

		// 记录慢请求
		if duration > 5*time.Second {
			logEntry.Warnf(constants.LogMsgPerformanceSlow, c.Request.URL.Path, duration)
		}
	}
}
