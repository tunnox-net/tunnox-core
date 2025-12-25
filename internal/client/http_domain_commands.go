package client

import (
	"encoding/json"
	"fmt"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令请求/响应类型（客户端使用）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainBaseDomainInfo 基础域名信息
type HTTPDomainBaseDomainInfo struct {
	Domain      string `json:"domain"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// GetBaseDomainsResponse 获取基础域名列表响应
type GetBaseDomainsResponse struct {
	Success     bool                       `json:"success"`
	BaseDomains []HTTPDomainBaseDomainInfo `json:"base_domains"`
	Error       string                     `json:"error,omitempty"`
}

// CheckSubdomainRequest 检查子域名可用性请求
type CheckSubdomainRequest struct {
	Subdomain  string `json:"subdomain"`
	BaseDomain string `json:"base_domain"`
}

// CheckSubdomainResponse 检查子域名可用性响应
type CheckSubdomainResponse struct {
	Success    bool   `json:"success"`
	Available  bool   `json:"available"`
	FullDomain string `json:"full_domain"`
	Error      string `json:"error,omitempty"`
}

// GenSubdomainRequest 生成随机子域名请求
type GenSubdomainRequest struct {
	BaseDomain string `json:"base_domain"`
}

// GenSubdomainResponse 生成随机子域名响应
type GenSubdomainResponse struct {
	Success    bool   `json:"success"`
	Subdomain  string `json:"subdomain"`
	FullDomain string `json:"full_domain"`
	Error      string `json:"error,omitempty"`
}

// CreateHTTPDomainRequest 创建 HTTP 域名映射请求
type CreateHTTPDomainRequest struct {
	TargetURL   string `json:"target_url"`
	Subdomain   string `json:"subdomain"`
	BaseDomain  string `json:"base_domain"`
	MappingTTL  int    `json:"mapping_ttl,omitempty"`
	Description string `json:"description,omitempty"`
}

// CreateHTTPDomainResponse 创建 HTTP 域名映射响应
type CreateHTTPDomainResponse struct {
	Success    bool   `json:"success"`
	MappingID  string `json:"mapping_id"`
	FullDomain string `json:"full_domain"`
	TargetURL  string `json:"target_url"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// HTTPDomainMappingInfo HTTP 域名映射信息
type HTTPDomainMappingInfo struct {
	MappingID  string `json:"mapping_id"`
	FullDomain string `json:"full_domain"`
	TargetURL  string `json:"target_url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// ListHTTPDomainsResponse 列出 HTTP 域名映射响应
type ListHTTPDomainsResponse struct {
	Success  bool                    `json:"success"`
	Mappings []HTTPDomainMappingInfo `json:"mappings"`
	Total    int                     `json:"total"`
	Error    string                  `json:"error,omitempty"`
}

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
