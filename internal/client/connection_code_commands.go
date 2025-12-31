package client

import (
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/client/command"
	clientconfig "tunnox-core/internal/config"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// 类型别名 - 为了向后兼容，保留在 client 包中
type (
	GenerateConnectionCodeRequest  = command.GenerateConnectionCodeRequest
	GenerateConnectionCodeResponse = command.GenerateConnectionCodeResponse
	ListConnectionCodesResponseCmd = command.ListConnectionCodesResponse
	ConnectionCodeInfoCmd          = command.ConnectionCodeInfo
	ActivateConnectionCodeRequest  = command.ActivateConnectionCodeRequest
	ActivateConnectionCodeResponse = command.ActivateConnectionCodeResponse
)

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
		corelog.Errorf("Client.GenerateConnectionCode: failed to parse response data: %v, Data=%s", err, cmdResp.Data)
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to parse response data")
	}

	corelog.Infof("Client.GenerateConnectionCode: success, Code=%s", resp.Code)
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to parse response data")
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to parse response data")
	}

	if resp.MappingID != "" {
		// 解析监听地址
		_, port, err := parseListenAddress(resp.ListenAddress)
		if err != nil {
			corelog.Warnf("Client.ActivateConnectionCode: failed to parse listen address %q: %v", resp.ListenAddress, err)
			// 继续返回响应，即使解析失败
			return &resp, nil
		}

		// 解析目标地址以确定协议
		_, _, protocol, err := parseTargetAddress(resp.TargetAddress)
		if err != nil {
			corelog.Warnf("Client.ActivateConnectionCode: failed to parse target address %q: %v", resp.TargetAddress, err)
			protocol = "tcp" // 默认TCP
		}

		// 创建映射配置
		mappingCfg := clientconfig.MappingConfig{
			MappingID:      resp.MappingID,
			Protocol:       protocol,
			LocalPort:      port,
			TargetHost:     "",                  // 目标地址由服务端管理
			TargetPort:     0,                   // 目标端口由服务端管理
			TargetClientID: resp.TargetClientID, // SOCKS5 需要目标客户端ID
			SecretKey:      resp.SecretKey,      // SOCKS5 需要密钥
			MaxConnections: 100,
			BandwidthLimit: 0, // 无限制
		}

		// 根据协议类型分发到正确的处理器
		if protocol == "socks5" && port > 0 {
			c.addOrUpdateSOCKS5Mapping(mappingCfg)
			corelog.Infof("Client.ActivateConnectionCode: SOCKS5 mapping handler started for %s on %s", resp.MappingID, resp.ListenAddress)
		} else {
			c.addOrUpdateMapping(mappingCfg)
			corelog.Infof("Client.ActivateConnectionCode: mapping handler started for %s on %s", resp.MappingID, resp.ListenAddress)
		}
	}

	return &resp, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 映射列表命令 - 类型别名
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

type (
	ListMappingsRequest     = command.ListMappingsRequest
	ListMappingsResponseCmd = command.ListMappingsResponse
	MappingInfoCmd          = command.MappingInfo
	GetMappingRequest       = command.GetMappingRequest
	GetMappingResponseCmd   = command.GetMappingResponse
	DeleteMappingRequest    = command.DeleteMappingRequest
)

// ListMappings 通过指令通道列出映射
// 使用 TunnoxClient 自身的 context，遵循 dispose 层次结构
func (c *TunnoxClient) ListMappings(req *ListMappingsRequest) (*ListMappingsResponseCmd, error) {
	return c.ListMappingsWithContext(c.Ctx(), req)
}

// ListMappingsWithContext 通过指令通道列出映射（支持context取消）
func (c *TunnoxClient) ListMappingsWithContext(ctx context.Context, req *ListMappingsRequest) (*ListMappingsResponseCmd, error) {
	cmdResp, err := c.sendCommandAndWaitResponseWithContext(ctx, &CommandRequest{
		CommandType: packet.MappingList,
		RequestBody: req,
		EnableTrace: true,
	})
	if err != nil {
		return nil, err
	}

	var resp ListMappingsResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to parse response data")
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

// GetMapping 通过指令通道获取映射详情
// 使用 TunnoxClient 自身的 context，遵循 dispose 层次结构
func (c *TunnoxClient) GetMapping(mappingID string) (*MappingInfoCmd, error) {
	return c.GetMappingWithContext(c.Ctx(), mappingID)
}

// GetMappingWithContext 通过指令通道获取映射详情（支持context取消）
func (c *TunnoxClient) GetMappingWithContext(ctx context.Context, mappingID string) (*MappingInfoCmd, error) {
	req := &GetMappingRequest{MappingID: mappingID}
	cmdResp, err := c.sendCommandAndWaitResponseWithContext(ctx, &CommandRequest{
		CommandType: packet.MappingGet,
		RequestBody: req,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp GetMappingResponseCmd
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to parse response data")
	}

	c.updateTrafficStatsFromMappings([]MappingInfoCmd{resp.Mapping})

	return &resp.Mapping, nil
}

// DeleteMapping 通过指令通道删除映射
// 使用 TunnoxClient 自身的 context，遵循 dispose 层次结构
func (c *TunnoxClient) DeleteMapping(mappingID string) error {
	return c.DeleteMappingWithContext(c.Ctx(), mappingID)
}

// DeleteMappingWithContext 通过指令通道删除映射（支持context取消）
func (c *TunnoxClient) DeleteMappingWithContext(ctx context.Context, mappingID string) error {
	req := &DeleteMappingRequest{MappingID: mappingID}
	_, err := c.sendCommandAndWaitResponseWithContext(ctx, &CommandRequest{
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
