package httpproxy

import "encoding/json"

// Message HTTP 代理跨节点消息
type Message struct {
	RequestID string          `json:"request_id"`
	ClientID  int64           `json:"client_id"`
	Request   json.RawMessage `json:"request"`
}

// ResponseMessage HTTP 代理响应跨节点消息
type ResponseMessage struct {
	RequestID string          `json:"request_id"`
	Response  json.RawMessage `json:"response,omitempty"`
	Error     string          `json:"error,omitempty"`
}
