package client

import (
	"bytes"
	"io"
	"net/http"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// HTTPProxyExecutor HTTP 代理执行器
// 用于 Client 端执行 Server 端发来的 HTTP 代理请求
type HTTPProxyExecutor struct {
	// HTTP 客户端（带连接池）
	httpClient *http.Client

	// 配置
	config *HTTPProxyConfig
}

// HTTPProxyConfig HTTP 代理配置
type HTTPProxyConfig struct {
	Enabled         bool     `json:"enabled"`
	AllowedHosts    []string `json:"allowed_hosts"`     // 允许访问的内网地址（CIDR）
	DeniedHosts     []string `json:"denied_hosts"`      // 禁止访问的地址
	DefaultTimeout  int      `json:"default_timeout"`   // 超时秒数
	MaxResponseSize int64    `json:"max_response_size"` // 最大响应体大小
}

// NewHTTPProxyExecutor 创建 HTTP 代理执行器
func NewHTTPProxyExecutor(config *HTTPProxyConfig) *HTTPProxyExecutor {
	if config == nil {
		config = &HTTPProxyConfig{
			Enabled:         true,
			DefaultTimeout:  30,
			MaxResponseSize: 10 * 1024 * 1024, // 10MB
		}
	}

	timeout := time.Duration(config.DefaultTimeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &HTTPProxyExecutor{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		config: config,
	}
}

// Execute 执行 HTTP 代理请求
func (e *HTTPProxyExecutor) Execute(req *httpservice.HTTPProxyRequest) (*httpservice.HTTPProxyResponse, error) {
	if !e.config.Enabled {
		return nil, coreerrors.New(coreerrors.CodeForbidden, "HTTP proxy is disabled")
	}

	// TODO: 验证目标地址是否允许访问
	// if !e.isAllowed(req.URL) {
	// 	return nil, coreerrors.New(coreerrors.CodeForbidden, "target host not allowed")
	// }

	// 创建 HTTP 请求
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to create HTTP request")
	}

	// 设置请求头
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// 设置超时
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout == 0 {
		timeout = time.Duration(e.config.DefaultTimeout) * time.Second
	}

	// 创建带超时的客户端
	client := &http.Client{
		Timeout:   timeout,
		Transport: e.httpClient.Transport,
	}

	corelog.Debugf("HTTPProxyExecutor: executing request %s %s", req.Method, req.URL)

	// 执行请求
	httpResp, err := client.Do(httpReq)
	if err != nil {
		corelog.Warnf("HTTPProxyExecutor: request failed: %v", err)
		return &httpservice.HTTPProxyResponse{
			RequestID: req.RequestID,
			Error:     err.Error(),
		}, nil
	}
	defer httpResp.Body.Close()

	// 读取响应体
	maxSize := e.config.MaxResponseSize
	if maxSize == 0 {
		maxSize = 10 * 1024 * 1024 // 10MB
	}

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, maxSize))
	if err != nil {
		corelog.Warnf("HTTPProxyExecutor: failed to read response body: %v", err)
		return &httpservice.HTTPProxyResponse{
			RequestID: req.RequestID,
			Error:     "failed to read response body: " + err.Error(),
		}, nil
	}

	// 提取响应头
	headers := make(map[string]string)
	for key, values := range httpResp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	corelog.Debugf("HTTPProxyExecutor: request completed, status=%d, body_size=%d", httpResp.StatusCode, len(body))

	return &httpservice.HTTPProxyResponse{
		RequestID:  req.RequestID,
		StatusCode: httpResp.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

// UpdateConfig 更新配置
func (e *HTTPProxyExecutor) UpdateConfig(config *HTTPProxyConfig) {
	e.config = config

	// 更新超时
	timeout := time.Duration(config.DefaultTimeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	e.httpClient.Timeout = timeout
}
