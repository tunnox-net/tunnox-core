// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"net/http"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// handleError 处理错误
func (m *DomainProxyModule) handleError(w http.ResponseWriter, err error) {
	var statusCode int
	var message string

	switch {
	case coreerrors.Is(err, httpservice.ErrDomainNotFound):
		statusCode = http.StatusNotFound
		message = "Domain not found"
	case coreerrors.Is(err, httpservice.ErrClientOffline):
		statusCode = http.StatusServiceUnavailable
		message = "Backend service unavailable"
	case coreerrors.Is(err, httpservice.ErrProxyTimeout):
		statusCode = http.StatusGatewayTimeout
		message = "Request timeout"
	case coreerrors.Is(err, httpservice.ErrBaseDomainNotAllow):
		statusCode = http.StatusForbidden
		message = "Domain not allowed"
	default:
		statusCode = http.StatusInternalServerError
		message = "Internal server error"
	}

	corelog.Warnf("DomainProxyModule: error: %v", err)
	http.Error(w, message, statusCode)
}

// isHopByHopHeader 检查是否为 hop-by-hop 头
func isHopByHopHeader(header string) bool {
	hopByHopHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}
	return hopByHopHeaders[header]
}

// itoa 整数转字符串
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}

	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
