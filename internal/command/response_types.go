package command

// MessageResponse 消息响应
type MessageResponse struct {
	Message string `json:"message"`
}

// RPCRequest RPC请求
type RPCRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// RPCResponse RPC响应
type RPCResponse struct {
	Method string      `json:"method"`
	Result interface{} `json:"result"`
}

