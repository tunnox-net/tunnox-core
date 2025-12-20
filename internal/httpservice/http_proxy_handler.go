package httpservice

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/command"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// HTTPProxyHandler HTTP 代理命令处理器（Server 端）
// 用于接收 Client 端的 HTTP 代理响应
type HTTPProxyHandler struct {
	*command.BaseCommandHandler[HTTPProxyResponse, struct{}]

	// 等待响应的请求
	pendingRequests map[string]chan *HTTPProxyResponse
	pendingMu       sync.RWMutex

	// 请求超时
	defaultTimeout time.Duration
}

// NewHTTPProxyHandler 创建 HTTP 代理命令处理器
func NewHTTPProxyHandler() *HTTPProxyHandler {
	h := &HTTPProxyHandler{
		BaseCommandHandler: command.NewBaseCommandHandler[HTTPProxyResponse, struct{}](
			packet.HTTPProxyResponse,
			command.DirectionOneway,
			command.Simplex,
		),
		pendingRequests: make(map[string]chan *HTTPProxyResponse),
		defaultTimeout:  30 * time.Second,
	}
	return h
}

// Handle 处理 HTTP 代理响应
func (h *HTTPProxyHandler) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
	resp, err := h.ParseRequest(ctx)
	if err != nil {
		corelog.Errorf("HTTPProxyHandler: failed to parse response: %v", err)
		return nil, err
	}

	// 查找等待的请求
	h.pendingMu.RLock()
	ch, exists := h.pendingRequests[resp.RequestID]
	h.pendingMu.RUnlock()

	if !exists {
		corelog.Warnf("HTTPProxyHandler: no pending request for ID %s", resp.RequestID)
		return nil, nil
	}

	// 发送响应
	select {
	case ch <- resp:
		corelog.Debugf("HTTPProxyHandler: response sent for request %s", resp.RequestID)
	default:
		corelog.Warnf("HTTPProxyHandler: response channel full for request %s", resp.RequestID)
	}

	return nil, nil
}

// RegisterPendingRequest 注册等待响应的请求
func (h *HTTPProxyHandler) RegisterPendingRequest(requestID string) chan *HTTPProxyResponse {
	ch := make(chan *HTTPProxyResponse, 1)

	h.pendingMu.Lock()
	h.pendingRequests[requestID] = ch
	h.pendingMu.Unlock()

	return ch
}

// UnregisterPendingRequest 注销等待响应的请求
func (h *HTTPProxyHandler) UnregisterPendingRequest(requestID string) {
	h.pendingMu.Lock()
	delete(h.pendingRequests, requestID)
	h.pendingMu.Unlock()
}

// WaitForResponse 等待响应
func (h *HTTPProxyHandler) WaitForResponse(requestID string, timeout time.Duration) (*HTTPProxyResponse, error) {
	ch := h.RegisterPendingRequest(requestID)
	defer h.UnregisterPendingRequest(requestID)

	if timeout == 0 {
		timeout = h.defaultTimeout
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return nil, coreerrors.New(coreerrors.CodeTimeout, "HTTP proxy request timeout")
	}
}

// HTTPProxyRequestHandler HTTP 代理请求处理器（Client 端）
// 用于处理 Server 端发来的 HTTP 代理请求
type HTTPProxyRequestHandler struct {
	*command.BaseCommandHandler[HTTPProxyRequest, HTTPProxyResponse]

	// HTTP 代理执行器
	executor HTTPProxyExecutor
}

// HTTPProxyExecutor HTTP 代理执行器接口
type HTTPProxyExecutor interface {
	Execute(req *HTTPProxyRequest) (*HTTPProxyResponse, error)
}

// NewHTTPProxyRequestHandler 创建 HTTP 代理请求处理器
func NewHTTPProxyRequestHandler(executor HTTPProxyExecutor) *HTTPProxyRequestHandler {
	h := &HTTPProxyRequestHandler{
		BaseCommandHandler: command.NewBaseCommandHandler[HTTPProxyRequest, HTTPProxyResponse](
			packet.HTTPProxyRequest,
			command.DirectionDuplex,
			command.DuplexMode,
		),
		executor: executor,
	}
	return h
}

// Handle 处理 HTTP 代理请求
func (h *HTTPProxyRequestHandler) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
	req, err := h.ParseRequest(ctx)
	if err != nil {
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	if h.executor == nil {
		return h.CreateErrorResponse(
			coreerrors.New(coreerrors.CodeNotConfigured, "HTTP proxy executor not configured"),
			ctx.RequestID,
		), nil
	}

	// 执行代理请求
	resp, err := h.executor.Execute(req)
	if err != nil {
		corelog.Warnf("HTTPProxyRequestHandler: proxy request failed: %v", err)
		return h.CreateErrorResponse(err, ctx.RequestID), nil
	}

	// 返回响应
	return h.CreateSuccessResponse(resp, ctx.RequestID), nil
}

// CreateSuccessResponse 创建成功响应
func (h *HTTPProxyRequestHandler) CreateSuccessResponse(data *HTTPProxyResponse, requestID string) *types.CommandResponse {
	response := &types.CommandResponse{
		Success:   true,
		RequestID: requestID,
	}

	if data != nil {
		if jsonData, err := json.Marshal(data); err == nil {
			response.Data = string(jsonData)
		} else {
			response.Error = fmt.Sprintf("failed to marshal response: %v", err)
		}
	}

	return response
}
