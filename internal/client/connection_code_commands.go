package client

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	clientconfig "tunnox-core/internal/config"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码命令请求/响应类型
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateConnectionCodeRequest 生成连接码请求
type GenerateConnectionCodeRequest struct {
	TargetAddress string `json:"target_address"`        // 目标地址（如 tcp://192.168.1.10:8080）
	ActivationTTL int    `json:"activation_ttl"`        // 激活有效期（秒）
	MappingTTL    int    `json:"mapping_ttl"`           // 映射有效期（秒）
	Description   string `json:"description,omitempty"` // 描述（可选）
}

// GenerateConnectionCodeResponse 生成连接码响应
type GenerateConnectionCodeResponse struct {
	Code          string `json:"code"`
	TargetAddress string `json:"target_address"`
	ExpiresAt     string `json:"expires_at"`
	Description   string `json:"description,omitempty"`
}

// ListConnectionCodesResponseCmd 连接码列表响应（通过指令通道）
type ListConnectionCodesResponseCmd struct {
	Codes []ConnectionCodeInfoCmd `json:"codes"`
	Total int                     `json:"total"`
}

// ConnectionCodeInfoCmd 连接码信息（通过指令通道）
type ConnectionCodeInfoCmd struct {
	Code          string `json:"code"`
	TargetAddress string `json:"target_address"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Activated     bool   `json:"activated"`
	Description   string `json:"description,omitempty"`
}

// ActivateConnectionCodeRequest 激活连接码请求
type ActivateConnectionCodeRequest struct {
	Code          string `json:"code"`
	ListenAddress string `json:"listen_address"` // 监听地址（如 127.0.0.1:8888）
}

// ActivateConnectionCodeResponse 激活连接码响应
type ActivateConnectionCodeResponse struct {
	MappingID     string `json:"mapping_id"`
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	ExpiresAt     string `json:"expires_at"`
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码命令发送方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateConnectionCode 通过指令通道生成连接码
func (c *TunnoxClient) GenerateConnectionCode(req *GenerateConnectionCodeRequest) (*GenerateConnectionCodeResponse, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection not established, please connect to server first")
	}

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建命令包
	cmdID, _ := utils.GenerateRandomString(16)
	cmdPkt := &packet.CommandPacket{
		CommandType: packet.ConnectionCodeGenerate,
		CommandId:   cmdID,
		CommandBody: string(reqBody),
	}

	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	// 注册请求
	responseChan := c.commandResponseManager.RegisterRequest(cmdPkt.CommandId)
	defer c.commandResponseManager.UnregisterRequest(cmdPkt.CommandId)

	// 发送命令前再次检查连接状态
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection is closed, please reconnect to server")
	}

	// 发送命令
	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return nil, fmt.Errorf("control stream is nil")
	}

	_, err = controlStream.WritePacket(transferPkt, false, 0)
	if err != nil {
		utils.Errorf("Client.GenerateConnectionCode: failed to send command: %v", err)
		// 发送失败，清理连接状态
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()

		// 检查是否是流已关闭的错误
		errMsg := err.Error()
		if strings.Contains(errMsg, "stream is closed") ||
			strings.Contains(errMsg, "stream closed") ||
			strings.Contains(errMsg, "ErrStreamClosed") {
			return nil, fmt.Errorf("control connection is closed, please reconnect to server")
		}
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// 等待响应
	cmdResp, err := c.commandResponseManager.WaitForResponse(cmdPkt.CommandId, responseChan)
	if err != nil {
		return nil, err
	}

	if !cmdResp.Success {
		return nil, fmt.Errorf("command failed: %s", cmdResp.Error)
	}

	// 解析响应数据
	var resp GenerateConnectionCodeResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		utils.Errorf("Client.GenerateConnectionCode: failed to parse response data: %v, Data=%s", err, cmdResp.Data)
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	utils.Infof("Client.GenerateConnectionCode: success, Code=%s", resp.Code)
	return &resp, nil
}

// ListConnectionCodes 通过指令通道列出连接码
func (c *TunnoxClient) ListConnectionCodes() (*ListConnectionCodesResponseCmd, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection not established, please connect to server first")
	}

	// 创建命令包
	cmdID, err := utils.GenerateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate command ID: %w", err)
	}
	cmdPkt := &packet.CommandPacket{
		CommandType: packet.ConnectionCodeList,
		CommandId:   cmdID,
		CommandBody: "{}",
	}

	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	// 注册请求
	responseChan := c.commandResponseManager.RegisterRequest(cmdPkt.CommandId)
	defer c.commandResponseManager.UnregisterRequest(cmdPkt.CommandId)

	// 发送命令前再次检查连接状态（双重检查）
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection is closed, please reconnect to server")
	}

	// 发送命令
	_, err = c.controlStream.WritePacket(transferPkt, false, 0)
	if err != nil {
		// 发送失败，清理连接状态
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()

		// 检查是否是流已关闭的错误
		errMsg := err.Error()
		if strings.Contains(errMsg, "stream is closed") ||
			strings.Contains(errMsg, "stream closed") ||
			strings.Contains(errMsg, "ErrStreamClosed") {
			return nil, fmt.Errorf("control connection is closed, please reconnect to server")
		}
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// 等待响应
	cmdResp, err := c.commandResponseManager.WaitForResponse(cmdPkt.CommandId, responseChan)
	if err != nil {
		return nil, err
	}

	if !cmdResp.Success {
		return nil, fmt.Errorf("command failed: %s", cmdResp.Error)
	}

	// 解析响应数据
	var resp ListConnectionCodesResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	return &resp, nil
}

// ActivateConnectionCode 通过指令通道激活连接码
func (c *TunnoxClient) ActivateConnectionCode(req *ActivateConnectionCodeRequest) (*ActivateConnectionCodeResponse, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection not established, please connect to server first")
	}

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建命令包
	cmdID, _ := utils.GenerateRandomString(16)
	cmdPkt := &packet.CommandPacket{
		CommandType: packet.ConnectionCodeActivate,
		CommandId:   cmdID,
		CommandBody: string(reqBody),
	}

	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	// 注册请求
	responseChan := c.commandResponseManager.RegisterRequest(cmdPkt.CommandId)
	defer c.commandResponseManager.UnregisterRequest(cmdPkt.CommandId)

	// 发送命令前再次检查连接状态（双重检查）
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection is closed, please reconnect to server")
	}

	// 发送命令
	_, err = c.controlStream.WritePacket(transferPkt, false, 0)
	if err != nil {
		// 发送失败，清理连接状态
		c.mu.Lock()
		if c.controlStream != nil {
			c.controlStream.Close()
			c.controlStream = nil
		}
		if c.controlConn != nil {
			c.controlConn.Close()
			c.controlConn = nil
		}
		c.mu.Unlock()

		// 检查是否是流已关闭的错误
		errMsg := err.Error()
		if strings.Contains(errMsg, "stream is closed") ||
			strings.Contains(errMsg, "stream closed") ||
			strings.Contains(errMsg, "ErrStreamClosed") {
			return nil, fmt.Errorf("control connection is closed, please reconnect to server")
		}
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// 等待响应
	cmdResp, err := c.commandResponseManager.WaitForResponse(cmdPkt.CommandId, responseChan)
	if err != nil {
		return nil, err
	}

	if !cmdResp.Success {
		return nil, fmt.Errorf("command failed: %s", cmdResp.Error)
	}

	// 解析响应数据
	var resp ActivateConnectionCodeResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	// ✅ 激活成功后，自动创建并启动映射处理器
	if resp.MappingID != "" {
		// 解析监听地址
		_, port, err := parseListenAddress(resp.ListenAddress)
		if err != nil {
			utils.Warnf("Client.ActivateConnectionCode: failed to parse listen address %q: %v", resp.ListenAddress, err)
			// 继续返回响应，即使解析失败
			return &resp, nil
		}

		// 解析目标地址以确定协议
		_, _, protocol, err := parseTargetAddress(resp.TargetAddress)
		if err != nil {
			utils.Warnf("Client.ActivateConnectionCode: failed to parse target address %q: %v", resp.TargetAddress, err)
			protocol = "tcp" // 默认TCP
		}

		// 创建映射配置
		mappingCfg := clientconfig.MappingConfig{
			MappingID:      resp.MappingID,
			Protocol:       protocol,
			LocalPort:      port,
			TargetHost:     "", // 目标地址由服务端管理
			TargetPort:     0,  // 目标端口由服务端管理
			SecretKey:      "", // SecretKey由服务端管理，客户端不需要
			MaxConnections: 100,
			BandwidthLimit: 0, // 无限制
		}

		// 启动映射处理器
		c.addOrUpdateMapping(mappingCfg)
		utils.Infof("Client.ActivateConnectionCode: mapping handler started for %s on %s", resp.MappingID, resp.ListenAddress)
	}

	return &resp, nil
}

// parseListenAddress 解析监听地址 "127.0.0.1:8888" -> ("127.0.0.1", 8888, nil)
func parseListenAddress(addr string) (string, int, error) {
	if addr == "" {
		return "", 0, fmt.Errorf("listen address is empty")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid listen address format %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port in listen address %q: %w", addr, err)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port %d out of range [1, 65535]", port)
	}
	return host, port, nil
}

// parseTargetAddress 解析目标地址 "tcp://10.51.22.69:3306" -> ("10.51.22.69", 3306, "tcp", nil)
func parseTargetAddress(addr string) (string, int, string, error) {
	if addr == "" {
		return "", 0, "", fmt.Errorf("target address is empty")
	}

	// 解析 URL 格式：tcp://host:port
	parsedURL, err := url.Parse(addr)
	if err != nil || parsedURL.Scheme == "" {
		// 如果不是URL格式，尝试直接解析为 host:port
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid target address format %q: %w", addr, err)
		}
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid port in target address %q: %w", addr, err)
		}
		if portNum < 1 || portNum > 65535 {
			return "", 0, "", fmt.Errorf("port %d out of range [1, 65535]", portNum)
		}
		return host, portNum, "tcp", nil // 默认协议为tcp
	}

	// 从 URL 解析
	protocol := strings.ToLower(parsedURL.Scheme)
	if protocol == "" {
		protocol = "tcp"
	}
	host := parsedURL.Hostname()
	if host == "" {
		return "", 0, "", fmt.Errorf("missing host in target address %q", addr)
	}
	portStr := parsedURL.Port()
	if portStr == "" {
		return "", 0, "", fmt.Errorf("missing port in target address %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid port in target address %q: %w", addr, err)
	}
	if port < 1 || port > 65535 {
		return "", 0, "", fmt.Errorf("port %d out of range [1, 65535]", port)
	}
	return host, port, protocol, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 映射列表命令
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ListMappingsRequest 列出映射请求
type ListMappingsRequest struct {
	Direction string `json:"direction,omitempty"` // outbound | inbound
	Type      string `json:"type,omitempty"`      // 映射类型过滤
	Status    string `json:"status,omitempty"`    // 状态过滤
}

// ListMappingsResponseCmd 列出映射响应（通过指令通道）
type ListMappingsResponseCmd struct {
	Mappings []MappingInfoCmd `json:"mappings"`
	Total    int              `json:"total"`
}

// MappingInfoCmd 映射信息（通过指令通道）
type MappingInfoCmd struct {
	MappingID     string `json:"mapping_id"`
	Type          string `json:"type"` // outbound | inbound
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	Status        string `json:"status"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
}

// ListMappings 通过指令通道列出映射
func (c *TunnoxClient) ListMappings(req *ListMappingsRequest) (*ListMappingsResponseCmd, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection not established, please connect to server first")
	}

	// 序列化请求
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 生成命令ID
	cmdID, err := utils.GenerateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate command ID: %w", err)
	}

	// 构造命令包
	cmdPkt := &packet.CommandPacket{
		CommandType: packet.MappingList,
		CommandId:   cmdID,
		CommandBody: string(reqBody),
	}

	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	// 注册响应通道
	responseChan := c.commandResponseManager.RegisterRequest(cmdID)
	defer c.commandResponseManager.UnregisterRequest(cmdID)

	// 发送命令
	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return nil, fmt.Errorf("control stream is nil")
	}

	_, err = controlStream.WritePacket(transferPkt, false, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// 等待响应
	cmdResp, err := c.commandResponseManager.WaitForResponse(cmdID, responseChan)
	if err != nil {
		return nil, err
	}

	if !cmdResp.Success {
		return nil, fmt.Errorf("command failed: %s", cmdResp.Error)
	}

	// 解析响应数据
	var resp ListMappingsResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	return &resp, nil
}
