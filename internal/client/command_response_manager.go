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
	handleStartTime := time.Now()

	// 忽略压缩/加密标志，只检查基础类型
	if !pkt.PacketType.IsCommandResp() || pkt.CommandPacket == nil {
		utils.Debugf("[CMD_TRACE] [CLIENT] [RESP_HANDLE_FAILED] Reason=not_CommandResp_or_no_CommandPacket, Time=%s",
			time.Now().Format("15:04:05.000"))
		return false
	}

	commandID := pkt.CommandPacket.CommandId
	if commandID == "" {
		utils.Debugf("[CMD_TRACE] [CLIENT] [RESP_HANDLE_FAILED] Reason=CommandID_empty, Time=%s",
			time.Now().Format("15:04:05.000"))
		return false
	}

	utils.Infof("[CMD_TRACE] [CLIENT] [RESP_HANDLE_START] CommandID=%s, Time=%s",
		commandID, handleStartTime.Format("15:04:05.000"))

	m.mu.RLock()
	responseChan, exists := m.pendingRequests[commandID]
	m.mu.RUnlock()

	if !exists {
		utils.Debugf("[CMD_TRACE] [CLIENT] [RESP_HANDLE_FAILED] CommandID=%s, Reason=no_pending_request, Time=%s",
			commandID, time.Now().Format("15:04:05.000"))
		return false // 不是我们等待的响应
	}

	// 注意：不能通过尝试接收来检查通道是否关闭，因为这会消费掉响应
	// 通道关闭检查在发送前进行（通过检查 pendingRequests）

	utils.Infof("[CMD_TRACE] [CLIENT] [RESP_HANDLE_FOUND] CommandID=%s, Time=%s",
		commandID, time.Now().Format("15:04:05.000"))

	// 解析响应（服务端返回的格式：{"success": true, "data": {...}, "command_id": "...", "request_id": "..."}）
	// 注意：在解析过程中，responseChan 可能被 UnregisterRequest 关闭，需要在发送前再次检查
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

	// 发送响应到通道（再次检查通道是否仍然在 pendingRequests 中，防止竞态）
	m.mu.RLock()
	_, stillExists := m.pendingRequests[commandID]
	m.mu.RUnlock()

	if !stillExists {
		utils.Debugf("[CMD_TRACE] [CLIENT] [RESP_HANDLE_FAILED] CommandID=%s, Reason=request_unregistered, Time=%s",
			commandID, time.Now().Format("15:04:05.000"))
		return false
	}

	// 发送响应到通道
	select {
	case responseChan <- resp:
		handleDuration := time.Since(handleStartTime)
		utils.Infof("[CMD_TRACE] [CLIENT] [RESP_HANDLE_COMPLETE] CommandID=%s, Success=%v, HandleDuration=%v, Time=%s",
			commandID, resp.Success, handleDuration, time.Now().Format("15:04:05.000"))
		return true
	default:
		utils.Warnf("[CMD_TRACE] [CLIENT] [RESP_HANDLE_FAILED] CommandID=%s, Reason=channel_full, Time=%s",
			commandID, time.Now().Format("15:04:05.000"))
		utils.Warnf("Response channel is full for command %s", commandID)
		return false
	}
}

// WaitForResponse 等待响应（带超时）
func (m *CommandResponseManager) WaitForResponse(commandID string, responseChan chan *CommandResponse) (*CommandResponse, error) {
	waitStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [CLIENT] [WAIT_START] CommandID=%s, Time=%s",
		commandID, waitStartTime.Format("15:04:05.000"))
	timeout := time.After(m.timeout)

	select {
	case resp := <-responseChan:
		waitDuration := time.Since(waitStartTime)
		if resp == nil {
			utils.Errorf("[CMD_TRACE] [CLIENT] [WAIT_FAILED] CommandID=%s, WaitDuration=%v, Reason=channel_closed, Time=%s",
				commandID, waitDuration, time.Now().Format("15:04:05.000"))
			return nil, fmt.Errorf("response channel closed")
		}
		utils.Infof("[CMD_TRACE] [CLIENT] [WAIT_COMPLETE] CommandID=%s, WaitDuration=%v, Success=%v, Time=%s",
			commandID, waitDuration, resp.Success, time.Now().Format("15:04:05.000"))
		return resp, nil
	case <-timeout:
		waitDuration := time.Since(waitStartTime)
		utils.Errorf("[CMD_TRACE] [CLIENT] [WAIT_TIMEOUT] CommandID=%s, WaitDuration=%v, Timeout=%v, Time=%s",
			commandID, waitDuration, m.timeout, time.Now().Format("15:04:05.000"))
		m.UnregisterRequest(commandID)
		return nil, fmt.Errorf("command timeout after %v", m.timeout)
	}
}
