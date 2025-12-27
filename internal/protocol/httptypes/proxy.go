package httptypes

// HTTPProxyRequest HTTP 代理请求（客户端和服务端共用）
// 规范要求:
// - 纯类型定义,无业务逻辑
// - 使用强类型struct,不用interface{}
// - 所有导出类型必须有注释
// - JSON tag使用下划线命名
type HTTPProxyRequest struct {
	RequestID string            `json:"request_id"`     // 请求ID（用于关联响应）
	Method    string            `json:"method"`         // HTTP 方法
	URL       string            `json:"url"`            // 完整 URL: http://local.com:9334/api/users
	Headers   map[string]string `json:"headers"`        // 请求头(强类型)
	Body      []byte            `json:"body,omitempty"` // 请求体（小请求才有）
	Timeout   int               `json:"timeout"`        // 超时秒数
}

// HTTPProxyResponse HTTP 代理响应
// 规范要求: 同HTTPProxyRequest
type HTTPProxyResponse struct {
	RequestID  string            `json:"request_id"`      // 请求ID（关联请求）
	StatusCode int               `json:"status_code"`     // HTTP 状态码
	Headers    map[string]string `json:"headers"`         // 响应头(强类型)
	Body       []byte            `json:"body,omitempty"`  // 响应体
	Error      string            `json:"error,omitempty"` // 错误信息
}

// WSProxyTarget WebSocket 代理目标
type WSProxyTarget struct {
	URL     string            `json:"url"`     // 目标 WebSocket URL
	Headers map[string]string `json:"headers"` // 请求头
}

// HTTPTunnelRequest HTTP 隧道请求
// 用于大文件上传、流式传输等场景
type HTTPTunnelRequest struct {
	TunnelID  string `json:"tunnel_id"`  // 隧道ID
	MappingID string `json:"mapping_id"` // 映射ID
	TargetURL string `json:"target_url"` // 目标URL
	Method    string `json:"method"`     // HTTP 方法
}

// WebSocketTunnelRequest WebSocket 隧道请求
type WebSocketTunnelRequest struct {
	TunnelID  string            `json:"tunnel_id"`  // 隧道ID
	MappingID string            `json:"mapping_id"` // 映射ID
	TargetURL string            `json:"target_url"` // 目标 WebSocket URL
	Headers   map[string]string `json:"headers"`    // 请求头
}
