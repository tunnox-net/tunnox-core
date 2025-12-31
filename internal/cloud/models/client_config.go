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
// - 认证信息：AuthCode, SecretKey
// - 配置信息：Config（带宽、端口等）
// - 时间戳：CreatedAt, UpdatedAt
type ClientConfig struct {
	ID        int64                `json:"id"`         // 客户端ID（8位数字）
	UserID    string               `json:"user_id"`    // 所属用户ID（匿名用户为空）
	Name      string               `json:"name"`       // 客户端名称
	AuthCode  string               `json:"auth_code"`  // 认证码
	SecretKey string               `json:"secret_key"` // 密钥
	Type      ClientType           `json:"type"`       // 客户端类型（registered/anonymous）
	Config    configs.ClientConfig `json:"config"`     // 客户端配置
	CreatedAt time.Time            `json:"created_at"` // 创建时间
	UpdatedAt time.Time            `json:"updated_at"` // 更新时间
}

// GetID 实现GenericEntity接口
func (c *ClientConfig) GetID() string {
	return fmt.Sprintf("%d", c.ID)
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
