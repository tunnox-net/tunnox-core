package client

import (
	"encoding/json"
	"fmt"

	"tunnox-core/internal/client/command"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令类型别名（向后兼容）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

type (
	HTTPDomainBaseDomainInfo = command.HTTPDomainBaseDomainInfo
	GetBaseDomainsResponse   = command.GetBaseDomainsResponse
	CheckSubdomainRequest    = command.CheckSubdomainRequest
	CheckSubdomainResponse   = command.CheckSubdomainResponse
	GenSubdomainRequest      = command.GenSubdomainRequest
	GenSubdomainResponse     = command.GenSubdomainResponse
	CreateHTTPDomainRequest  = command.CreateHTTPDomainRequest
	CreateHTTPDomainResponse = command.CreateHTTPDomainResponse
	HTTPDomainMappingInfo    = command.HTTPDomainMappingInfo
	ListHTTPDomainsResponse  = command.ListHTTPDomainsResponse
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令发送方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetBaseDomains 获取可用的基础域名列表
func (c *TunnoxClient) GetBaseDomains() (*GetBaseDomainsResponse, error) {
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.HTTPDomainGetBaseDomains,
		RequestBody: nil,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp GetBaseDomainsResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("get base domains failed: %s", resp.Error)
	}

	return &resp, nil
}

// CheckSubdomain 检查子域名可用性
func (c *TunnoxClient) CheckSubdomain(req *CheckSubdomainRequest) (*CheckSubdomainResponse, error) {
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.HTTPDomainCheckSubdomain,
		RequestBody: req,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp CheckSubdomainResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("check subdomain failed: %s", resp.Error)
	}

	return &resp, nil
}

// GenSubdomain 生成随机子域名
func (c *TunnoxClient) GenSubdomain(baseDomain string) (*GenSubdomainResponse, error) {
	req := &GenSubdomainRequest{
		BaseDomain: baseDomain,
	}

	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.HTTPDomainGenSubdomain,
		RequestBody: req,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp GenSubdomainResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("generate subdomain failed: %s", resp.Error)
	}

	return &resp, nil
}

// CreateHTTPDomain 创建 HTTP 域名映射
func (c *TunnoxClient) CreateHTTPDomain(req *CreateHTTPDomainRequest) (*CreateHTTPDomainResponse, error) {
	corelog.Infof("Client.CreateHTTPDomain: creating domain %s.%s -> %s",
		req.Subdomain, req.BaseDomain, req.TargetURL)

	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.HTTPDomainCreate,
		RequestBody: req,
		EnableTrace: true,
	})
	if err != nil {
		return nil, err
	}

	var resp CreateHTTPDomainResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		corelog.Errorf("Client.CreateHTTPDomain: failed to parse response data: %v, Data=%s", err, cmdResp.Data)
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("create HTTP domain failed: %s", resp.Error)
	}

	corelog.Infof("Client.CreateHTTPDomain: success, MappingID=%s, Domain=%s", resp.MappingID, resp.FullDomain)
	return &resp, nil
}

// ListHTTPDomains 列出 HTTP 域名映射
func (c *TunnoxClient) ListHTTPDomains() (*ListHTTPDomainsResponse, error) {
	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.HTTPDomainList,
		RequestBody: nil,
		EnableTrace: false,
	})
	if err != nil {
		return nil, err
	}

	var resp ListHTTPDomainsResponse
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response data: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("list HTTP domains failed: %s", resp.Error)
	}

	return &resp, nil
}

// DeleteHTTPDomain 删除 HTTP 域名映射
func (c *TunnoxClient) DeleteHTTPDomain(mappingID string) error {
	req := struct {
		MappingID string `json:"mapping_id"`
	}{
		MappingID: mappingID,
	}

	cmdResp, err := c.sendCommandAndWaitResponse(&CommandRequest{
		CommandType: packet.HTTPDomainDelete,
		RequestBody: req,
		EnableTrace: true,
	})
	if err != nil {
		return err
	}

	var resp struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(cmdResp.Data), &resp); err != nil {
		return fmt.Errorf("failed to parse response data: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete HTTP domain failed: %s", resp.Error)
	}

	corelog.Infof("Client.DeleteHTTPDomain: deleted mapping %s", mappingID)
	return nil
}
