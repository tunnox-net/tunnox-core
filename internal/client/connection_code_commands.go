package client

import (
	"encoding/json"
	"fmt"
	"time"

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
	ActivatedBy   *int64 `json:"activated_by,omitempty"`
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
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.ConnectionCodeGenerate,
		RequestBody: req,
		EnableTrace: true,
	})
	if err != nil {
		return nil, err
	}

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
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.ConnectionCodeList,
		RequestBody: nil,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp ListConnectionCodesResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	return &resp, nil
}

// ActivateConnectionCode 通过指令通道激活连接码
func (c *TunnoxClient) ActivateConnectionCode(req *ActivateConnectionCodeRequest) (*ActivateConnectionCodeResponse, error) {
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.ConnectionCodeActivate,
		RequestBody: req,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp ActivateConnectionCodeResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

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
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.MappingList,
		RequestBody: req,
		EnableTrace: true,
	})
	if err != nil {
		return nil, err
	}

	var resp ListMappingsResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	c.updateTrafficStatsFromMappings(resp.Mappings)

	return &resp, nil
}

// updateTrafficStatsFromMappings 从服务端返回的映射列表更新本地流量统计
func (c *TunnoxClient) updateTrafficStatsFromMappings(mappings []MappingInfoCmd) {
	c.trafficStatsMu.Lock()
	defer c.trafficStatsMu.Unlock()

	for _, m := range mappings {
		stats, exists := c.localTrafficStats[m.MappingID]
		if !exists {
			stats = &localMappingStats{
				lastReportTime: time.Now(),
			}
			c.localTrafficStats[m.MappingID] = stats
		}
		stats.mu.Lock()
		stats.bytesSent = m.BytesSent
		stats.bytesReceived = m.BytesReceived
		stats.lastReportTime = time.Now()
		stats.mu.Unlock()
	}
}

// GetMappingRequest 获取映射详情请求
type GetMappingRequest struct {
	MappingID string `json:"mapping_id"`
}

// GetMappingResponseCmd 获取映射详情响应（通过指令通道）
type GetMappingResponseCmd struct {
	Mapping MappingInfoCmd `json:"mapping"`
}

// GetMapping 通过指令通道获取映射详情
func (c *TunnoxClient) GetMapping(mappingID string) (*MappingInfoCmd, error) {
	req := &GetMappingRequest{MappingID: mappingID}
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.MappingGet,
		RequestBody: req,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp GetMappingResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	c.updateTrafficStatsFromMappings([]MappingInfoCmd{resp.Mapping})

	return &resp.Mapping, nil
}

// DeleteMappingRequest 删除映射请求
type DeleteMappingRequest struct {
	MappingID string `json:"mapping_id"`
}

// DeleteMapping 通过指令通道删除映射
func (c *TunnoxClient) DeleteMapping(mappingID string) error {
	req := &DeleteMappingRequest{MappingID: mappingID}
	_, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.MappingDelete,
		RequestBody: req,
		EnableTrace: false,
	})
	if err != nil {
		return err
	}

	c.RemoveMapping(mappingID)
	return nil
}
