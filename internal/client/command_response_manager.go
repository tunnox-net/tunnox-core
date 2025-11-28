package client

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// CommandResponseManager 命令响应管理器
// 用于管理客户端发送的命令请求和对应的响应
type CommandResponseManager struct {
	pendingRequests map[string]chan *CommandResponse
	mu              sync.RWMutex
	timeout         time.Duration
}

// CommandResponse 命令响应
type CommandResponse struct {
	Success   bool                   `json:"success"`
	Data      string                 `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	CommandId string                 `json:"command_id,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	RawData   map[string]interface{} `json:"-"` // 原始数据（用于解析）
}

// NewCommandResponseManager 创建命令响应管理器
func NewCommandResponseManager() *CommandResponseManager {
	return &CommandResponseManager{
		pendingRequests: make(map[string]chan *CommandResponse),
		timeout:         30 * time.Second,
	}
}

// RegisterRequest 注册请求，返回响应通道
func (m *CommandResponseManager) RegisterRequest(commandID string) chan *CommandResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	responseChan := make(chan *CommandResponse, 1)
	m.pendingRequests[commandID] = responseChan
	return responseChan
}

// UnregisterRequest 注销请求
func (m *CommandResponseManager) UnregisterRequest(commandID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ch, exists := m.pendingRequests[commandID]; exists {
		close(ch)
		delete(m.pendingRequests, commandID)
	}
}

// HandleResponse 处理响应数据包
func (m *CommandResponseManager) HandleResponse(pkt *packet.TransferPacket) bool {
	if pkt.PacketType != packet.CommandResp || pkt.CommandPacket == nil {
		utils.Debugf("CommandResponseManager: packet is not CommandResp or has no CommandPacket")
		return false
	}

	commandID := pkt.CommandPacket.CommandId
	if commandID == "" {
		utils.Debugf("CommandResponseManager: CommandID is empty")
		return false
	}

	utils.Debugf("CommandResponseManager: handling response, CommandID=%s", commandID)

	m.mu.RLock()
	responseChan, exists := m.pendingRequests[commandID]
	m.mu.RUnlock()

	if !exists {
		utils.Debugf("CommandResponseManager: no pending request found for CommandID=%s", commandID)
		return false // 不是我们等待的响应
	}

	utils.Debugf("CommandResponseManager: found pending request for CommandID=%s", commandID)

	// 解析响应（服务端返回的格式：{"success": true, "data": {...}, "command_id": "...", "request_id": "..."}）
	var rawData map[string]interface{}
	if err := json.Unmarshal([]byte(pkt.CommandPacket.CommandBody), &rawData); err != nil {
		utils.Warnf("Failed to parse command response: %v", err)
		resp := &CommandResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to parse response: %v", err),
		}
		select {
		case responseChan <- resp:
		default:
		}
		return false
	}

	resp := &CommandResponse{
		RawData: rawData,
	}

	if success, ok := rawData["success"].(bool); ok {
		resp.Success = success
	}

	if data, ok := rawData["data"].(string); ok {
		resp.Data = data
	} else if dataObj, exists := rawData["data"]; exists {
		// 如果 data 是对象，序列化为 JSON 字符串
		if dataBytes, err := json.Marshal(dataObj); err == nil {
			resp.Data = string(dataBytes)
		}
	}

	if errMsg, ok := rawData["error"].(string); ok {
		resp.Error = errMsg
	}

	if cmdID, ok := rawData["command_id"].(string); ok {
		resp.CommandId = cmdID
	}

	if reqID, ok := rawData["request_id"].(string); ok {
		resp.RequestID = reqID
	}

	// 确保 CommandId 和 RequestID 设置
	if resp.CommandId == "" {
		resp.CommandId = commandID
	}
	if resp.RequestID == "" {
		resp.RequestID = pkt.CommandPacket.Token
	}

	// 发送响应到通道
	select {
	case responseChan <- resp:
		return true
	default:
		utils.Warnf("Response channel is full for command %s", commandID)
		return false
	}
}

// WaitForResponse 等待响应（带超时）
func (m *CommandResponseManager) WaitForResponse(commandID string, responseChan chan *CommandResponse) (*CommandResponse, error) {
	timeout := time.After(m.timeout)

	select {
	case resp := <-responseChan:
		if resp == nil {
			return nil, fmt.Errorf("response channel closed")
		}
		return resp, nil
	case <-timeout:
		m.UnregisterRequest(commandID)
		return nil, fmt.Errorf("command timeout after %v", m.timeout)
	}
}
