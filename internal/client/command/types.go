// Package command 提供客户端命令处理的类型定义和工具
package command

import "tunnox-core/internal/packet"

// Request 命令请求参数
type Request struct {
	CommandType packet.CommandType
	RequestBody interface{}
	EnableTrace bool
}

// ResponseData 命令响应数据（简化版本，用于方法返回）
type ResponseData struct {
	Success bool
	Data    string
	Error   string
}

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

// ListConnectionCodesResponse 连接码列表响应（通过指令通道）
type ListConnectionCodesResponse struct {
	Codes []ConnectionCodeInfo `json:"codes"`
	Total int                  `json:"total"`
}

// ConnectionCodeInfo 连接码信息（通过指令通道）
type ConnectionCodeInfo struct {
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
	MappingID      string `json:"mapping_id"`
	TargetAddress  string `json:"target_address"`
	ListenAddress  string `json:"listen_address"`
	ExpiresAt      string `json:"expires_at"`
	TargetClientID int64  `json:"target_client_id"` // SOCKS5 映射需要目标客户端ID
	SecretKey      string `json:"secret_key"`       // SOCKS5 映射需要密钥
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

// ListMappingsResponse 列出映射响应（通过指令通道）
type ListMappingsResponse struct {
	Mappings []MappingInfo `json:"mappings"`
	Total    int           `json:"total"`
}

// MappingInfo 映射信息（通过指令通道）
type MappingInfo struct {
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

// GetMappingRequest 获取映射详情请求
type GetMappingRequest struct {
	MappingID string `json:"mapping_id"`
}

// GetMappingResponse 获取映射详情响应（通过指令通道）
type GetMappingResponse struct {
	Mapping MappingInfo `json:"mapping"`
}

// DeleteMappingRequest 删除映射请求
type DeleteMappingRequest struct {
	MappingID string `json:"mapping_id"`
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令请求/响应类型
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
