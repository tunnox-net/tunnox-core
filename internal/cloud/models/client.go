package models

import (
	"time"
	"tunnox-core/internal/cloud/configs"
)

// Client 客户端完整视图（聚合对象）
//
// 用途：
// - 对外API返回
// - 内部业务逻辑
//
// 注意：
// - 此对象不直接存储
// - 由Service层从ClientConfig + ClientRuntimeState + ClientToken聚合而来
//
// 数据来源：
// - 配置部分：从ClientConfig
// - 状态部分：从ClientRuntimeState
// - Token部分：从ClientToken
type Client struct {
	// ========== 配置部分（持久化） ==========
	ID               int64                `json:"id"`
	UserID           string               `json:"user_id"`
	Name             string               `json:"name"`
	AuthCode         string               `json:"auth_code"`
	SecretKey        string               `json:"secret_key"`
	Type             ClientType           `json:"type"`
	Config           configs.ClientConfig `json:"config"`
	FirstConnectedAt *time.Time           `json:"first_connected_at,omitempty"` // 首次连接时间（激活时间）
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`

	// ========== 状态部分（运行时） ==========
	NodeID    string       `json:"node_id"`
	Status    ClientStatus `json:"status"`
	IPAddress string       `json:"ip_address"`
	IPRegion  string       `json:"ip_region,omitempty"` // IP 所在地区（GeoIP 解析）
	LastSeen  *time.Time   `json:"last_seen,omitempty"`
	Version   string       `json:"version,omitempty"`

	// ========== Token部分（运行时） ==========
	JWTToken       string     `json:"jwt_token,omitempty"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	RefreshToken   string     `json:"refresh_token,omitempty"`
	TokenID        string     `json:"token_id,omitempty"`
}

// FromConfigAndState 从配置和状态聚合Client
//
// 参数：
//   - cfg: 客户端配置（必需）
//   - state: 运行时状态（可选，nil表示离线）
//   - token: Token信息（可选，nil表示无Token）
//
// 返回：
//   - *Client: 聚合后的完整Client对象
func FromConfigAndState(cfg *ClientConfig, state *ClientRuntimeState, token *ClientToken) *Client {
	if cfg == nil {
		return nil
	}

	client := &Client{
		// 配置部分（来自ClientConfig）
		ID:               cfg.ID,
		UserID:           cfg.UserID,
		Name:             cfg.Name,
		AuthCode:         cfg.AuthCode,
		SecretKey:        cfg.SecretKey,
		Type:             cfg.Type,
		Config:           cfg.Config,
		FirstConnectedAt: cfg.FirstConnectedAt,
		CreatedAt:        cfg.CreatedAt,
		UpdatedAt:        cfg.UpdatedAt,

		// 默认离线状态，使用 Config 中保存的最后 IP
		Status:    ClientStatusOffline,
		IPAddress: cfg.LastIPAddress,
		IPRegion:  cfg.LastIPRegion,
	}

	// 填充状态（可能为空）
	if state != nil && state.IsOnline() {
		client.NodeID = state.NodeID
		client.Status = state.Status
		client.IPAddress = state.IPAddress
		client.LastSeen = &state.LastSeen
		client.Version = state.Version
	}

	// 填充Token（可能为空）
	if token != nil && token.IsValid() {
		client.JWTToken = token.JWTToken
		client.TokenExpiresAt = &token.TokenExpiresAt
		client.RefreshToken = token.RefreshToken
		client.TokenID = token.TokenID
	}

	return client
}

// IsOnline 判断客户端是否在线
func (c *Client) IsOnline() bool {
	return c.Status == ClientStatusOnline
}

// IsAnonymous 判断是否为匿名客户端
func (c *Client) IsAnonymous() bool {
	return c.Type == ClientTypeAnonymous
}

// IsRegistered 判断是否为注册客户端
func (c *Client) IsRegistered() bool {
	return c.Type == ClientTypeRegistered
}

// GetID 实现GenericEntity接口
func (c *Client) GetID() int64 {
	return c.ID
}
