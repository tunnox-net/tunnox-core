package api

import (
	"time"
	
	"tunnox-core/internal/cloud/models"
)

// ====================
// 列表响应类型
// ====================

// ClientListResponse 客户端列表响应
type ClientListResponse struct {
	Clients []*models.Client `json:"clients"`
	Total   int              `json:"total"`
}

// MappingListResponse 映射列表响应
type MappingListResponse struct {
	Mappings []*models.PortMapping `json:"mappings"`
	Total    int                   `json:"total"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Users []*models.User `json:"users"`
	Total int            `json:"total"`
}

// ConnectionListResponse 连接列表响应
type ConnectionListResponse struct {
	Connections interface{} `json:"connections"` // 实际类型依赖于CloudControl返回值
	Total       int         `json:"total"`
	MappingID   string      `json:"mapping_id,omitempty"`
	ClientID    int64       `json:"client_id,omitempty"`
}

// NodeListResponse 节点列表响应
type NodeListResponse struct {
	Nodes []*models.NodeServiceInfo `json:"nodes"`
	Total int                       `json:"total"`
}

// ====================
// 认证响应类型
// ====================

// LoginResponse 登录响应
type LoginResponse struct {
	Success   bool           `json:"success"`
	Token     string         `json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	Client    *models.Client `json:"client"`
	Message   string         `json:"message"`
}

// RefreshTokenResponse 刷新令牌响应
type RefreshTokenResponse struct {
	Success   bool      `json:"success"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Message   string    `json:"message"`
}

// ValidateTokenResponse 验证令牌响应
type ValidateTokenResponse struct {
	Success   bool           `json:"success"`
	Client    *models.Client `json:"client"`
	ExpiresAt time.Time      `json:"expires_at"`
	Message   string         `json:"message"`
}

// ClaimClientResponse 认领客户端响应
type ClaimClientResponse struct{
	ClientID  int64  `json:"client_id"`
	AuthToken string `json:"auth_token"`
}

// ====================
// 统计响应类型
// ====================

// StatsResponse 统计数据响应（通用）
type StatsResponse struct {
	TimeRange string      `json:"time_range"`
	Data      interface{} `json:"data"` // 保留interface{}因为统计数据结构多样
}

