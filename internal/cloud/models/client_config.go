package models

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/configs"
	coreerrors "tunnox-core/internal/core/errors"
)

// ClientConfig 客户端持久化配置
//
// 存储：数据库 + 缓存（通过HybridStorage）
// 键：tunnox:persist:client:config:{client_id}
// 特点：慢变化，需要持久保存
//
// 包含字段：
// - 基础信息：ID, UserID, Name, Type
// - 认证信息：AuthCode, SecretKeyEncrypted（AES-GCM加密）
// - 配置信息：Config（带宽、端口等）
// - 时间戳：CreatedAt, UpdatedAt
type ClientConfig struct {
	ID                 int64                `json:"id"`                             // 客户端ID（8位数字）
	UserID             string               `json:"user_id"`                        // 所属用户ID（匿名用户为空）
	Name               string               `json:"name"`                           // 客户端名称
	AuthCode           string               `json:"auth_code"`                      // 认证码
	SecretKey          string               `json:"secret_key,omitempty"`           // [废弃] 明文密钥，仅用于数据迁移
	SecretKeyEncrypted string               `json:"secret_key_encrypted,omitempty"` // AES-GCM 加密的 SecretKey
	SecretKeyVersion   int                  `json:"secret_key_version"`             // 密钥版本号（重置时+1）
	Type               ClientType           `json:"type"`                           // 客户端类型（registered/anonymous）
	Config             configs.ClientConfig `json:"config"`                         // 客户端配置
	ExpiresAt          *time.Time           `json:"expires_at,omitempty"`           // 凭据过期时间（未绑定用户时有效）
	FirstConnectedAt   *time.Time           `json:"first_connected_at,omitempty"`   // 首次连接时间（激活时间）
	LastIPAddress      string               `json:"last_ip_address,omitempty"`      // 最后连接 IP 地址（离线时保留）
	LastIPRegion       string               `json:"last_ip_region,omitempty"`       // 最后连接 IP 所在地区（离线时保留）
	CreatedAt          time.Time            `json:"created_at"`                     // 创建时间
	UpdatedAt          time.Time            `json:"updated_at"`                     // 更新时间
}

// GetID 实现 Entity 接口
func (c *ClientConfig) GetID() string {
	return fmt.Sprintf("%d", c.ID)
}

// GetUserID 实现 UserOwnedEntity 接口
// 返回配置所属的用户 ID，匿名客户端返回空字符串
func (c *ClientConfig) GetUserID() string {
	return c.UserID
}

// Validate 验证配置有效性
func (c *ClientConfig) Validate() error {
	if c.ID <= 0 {
		return coreerrors.Newf(coreerrors.CodeValidationError, "invalid client ID: %d", c.ID)
	}

	if c.AuthCode == "" {
		return coreerrors.New(coreerrors.CodeValidationError, "auth code is required")
	}

	if c.Type != ClientTypeRegistered && c.Type != ClientTypeAnonymous {
		return coreerrors.Newf(coreerrors.CodeValidationError, "invalid client type: %s", c.Type)
	}

	return nil
}

// IsAnonymous 判断是否为匿名客户端
func (c *ClientConfig) IsAnonymous() bool {
	return c.Type == ClientTypeAnonymous
}

// IsRegistered 判断是否为注册客户端
func (c *ClientConfig) IsRegistered() bool {
	return c.Type == ClientTypeRegistered
}

// IsExpired 判断凭据是否已过期
func (c *ClientConfig) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false // 未设置过期时间 = 永不过期
	}
	return time.Now().After(*c.ExpiresAt)
}

// HasEncryptedKey 判断是否已迁移到加密存储
func (c *ClientConfig) HasEncryptedKey() bool {
	return c.SecretKeyEncrypted != ""
}

// NeedsMigration 判断是否需要迁移（旧数据使用明文存储）
func (c *ClientConfig) NeedsMigration() bool {
	return c.SecretKey != "" && c.SecretKeyEncrypted == ""
}
