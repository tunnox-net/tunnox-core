package models

import (
	"fmt"
	"time"
)

// ClientToken 客户端JWT Token信息
//
// 存储：仅缓存（Redis/Memory），不持久化到数据库
// 键：tunnox:runtime:client:token:{client_id}
// TTL：Token过期时间（自动过期）
// 特点：临时数据，服务重启后需要重新登录
//
// 包含字段：
// - Token信息：JWTToken, TokenID, RefreshToken
// - 过期时间：TokenExpiresAt
type ClientToken struct {
	ClientID       int64     `json:"client_id"`        // 客户端ID
	JWTToken       string    `json:"jwt_token"`        // JWT Token
	TokenID        string    `json:"token_id"`         // Token唯一标识（用于撤销）
	RefreshToken   string    `json:"refresh_token"`    // 刷新Token
	TokenExpiresAt time.Time `json:"token_expires_at"` // Token过期时间
}

// IsExpired 判断Token是否已过期
func (t *ClientToken) IsExpired() bool {
	return time.Now().After(t.TokenExpiresAt)
}

// IsValid 判断Token是否有效
func (t *ClientToken) IsValid() bool {
	return !t.IsExpired() && t.JWTToken != ""
}

// TTL 获取Token剩余有效时间
func (t *ClientToken) TTL() time.Duration {
	if t.IsExpired() {
		return 0
	}
	return time.Until(t.TokenExpiresAt)
}

// Validate 验证Token有效性
func (t *ClientToken) Validate() error {
	if t.ClientID <= 0 {
		return fmt.Errorf("invalid client ID: %d", t.ClientID)
	}

	if t.JWTToken == "" {
		return fmt.Errorf("jwt_token is required")
	}

	if t.TokenID == "" {
		return fmt.Errorf("token_id is required")
	}

	if t.TokenExpiresAt.IsZero() {
		return fmt.Errorf("token_expires_at is required")
	}

	return nil
}
