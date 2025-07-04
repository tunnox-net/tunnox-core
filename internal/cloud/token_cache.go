package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TokenCacheManager Token缓存管理器
type TokenCacheManager struct {
	storage Storage
}

// TokenInfo 缓存中的Token信息
type TokenInfo struct {
	ClientID   string    `json:"client_id"`
	UserID     string    `json:"user_id,omitempty"`
	ClientType string    `json:"client_type"`
	NodeID     string    `json:"node_id,omitempty"`
	TokenID    string    `json:"token_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// RefreshTokenInfo 缓存中的刷新Token信息
type RefreshTokenInfo struct {
	ClientID  string    `json:"client_id"`
	TokenID   string    `json:"token_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewTokenCacheManager 创建Token缓存管理器
func NewTokenCacheManager(storage Storage) *TokenCacheManager {
	return &TokenCacheManager{
		storage: storage,
	}
}

// StoreAccessToken 存储访问Token信息
func (m *TokenCacheManager) StoreAccessToken(ctx context.Context, token string, info *TokenInfo) error {
	key := fmt.Sprintf("%s:access_token:%s", KeyPrefixToken, token)
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal token info failed: %w", err)
	}

	// 设置过期时间与Token过期时间一致
	ttl := time.Until(info.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("token already expired")
	}

	return m.storage.Set(ctx, key, string(data), ttl)
}

// GetAccessTokenInfo 获取访问Token信息
func (m *TokenCacheManager) GetAccessTokenInfo(ctx context.Context, token string) (*TokenInfo, error) {
	key := fmt.Sprintf("%s:access_token:%s", KeyPrefixToken, token)

	value, err := m.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("token not found: %w", err)
	}

	data, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("invalid token data type")
	}

	var info TokenInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, fmt.Errorf("unmarshal token info failed: %w", err)
	}

	return &info, nil
}

// StoreRefreshToken 存储刷新Token信息
func (m *TokenCacheManager) StoreRefreshToken(ctx context.Context, refreshToken string, info *RefreshTokenInfo) error {
	key := fmt.Sprintf("%s:refresh_token:%s", KeyPrefixToken, refreshToken)
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal refresh token info failed: %w", err)
	}

	// 设置过期时间与刷新Token过期时间一致
	ttl := time.Until(info.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("refresh token already expired")
	}

	return m.storage.Set(ctx, key, string(data), ttl)
}

// GetRefreshTokenInfo 获取刷新Token信息
func (m *TokenCacheManager) GetRefreshTokenInfo(ctx context.Context, refreshToken string) (*RefreshTokenInfo, error) {
	key := fmt.Sprintf("%s:refresh_token:%s", KeyPrefixToken, refreshToken)

	value, err := m.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("refresh token not found: %w", err)
	}

	data, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("invalid refresh token data type")
	}

	var info RefreshTokenInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, fmt.Errorf("unmarshal refresh token info failed: %w", err)
	}

	return &info, nil
}

// RevokeAccessToken 撤销访问Token
func (m *TokenCacheManager) RevokeAccessToken(ctx context.Context, token string) error {
	key := fmt.Sprintf("%s:access_token:%s", KeyPrefixToken, token)
	return m.storage.Delete(ctx, key)
}

// RevokeRefreshToken 撤销刷新Token
func (m *TokenCacheManager) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	key := fmt.Sprintf("%s:refresh_token:%s", KeyPrefixToken, refreshToken)
	return m.storage.Delete(ctx, key)
}

// RevokeTokenByID 通过Token ID撤销所有相关Token
func (m *TokenCacheManager) RevokeTokenByID(ctx context.Context, tokenID string) error {
	// 这里可以实现更复杂的逻辑来查找和撤销所有相关的Token
	// 目前简单实现，实际使用时可能需要维护Token ID到Token的映射
	return nil
}

// IsTokenRevoked 检查Token是否被撤销
func (m *TokenCacheManager) IsTokenRevoked(ctx context.Context, token string) (bool, error) {
	key := fmt.Sprintf("%s:access_token:%s", KeyPrefixToken, token)
	exists, err := m.storage.Exists(ctx, key)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// IsRefreshTokenRevoked 检查刷新Token是否被撤销
func (m *TokenCacheManager) IsRefreshTokenRevoked(ctx context.Context, refreshToken string) (bool, error) {
	key := fmt.Sprintf("%s:refresh_token:%s", KeyPrefixToken, refreshToken)
	exists, err := m.storage.Exists(ctx, key)
	if err != nil {
		return false, err
	}
	return !exists, nil
}
