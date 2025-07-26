package managers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"
)

// TokenCacheManager Token缓存管理器
type TokenCacheManager struct {
	*dispose.ResourceBase
	storage storage.Storage
}

// NewTokenCacheManager 创建新的Token缓存管理器
func NewTokenCacheManager(storage storage.Storage) *TokenCacheManager {
	manager := &TokenCacheManager{
		ResourceBase: dispose.NewResourceBase("TokenCacheManager"),
		storage:      storage,
	}
	manager.Initialize(context.Background())
	return manager
}

// StoreAccessToken 存储访问Token信息
func (m *TokenCacheManager) StoreAccessToken(ctx context.Context, token string, info *TokenInfo) error {
	key := fmt.Sprintf("%s:access_token:%s", constants.KeyPrefixToken, token)
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal token info failed: %w", err)
	}

	// 设置过期时间与Token过期时间一致
	ttl := time.Until(info.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("token already expired")
	}

	return m.storage.Set(key, string(data), ttl)
}

// GetAccessTokenInfo 获取访问Token信息
func (m *TokenCacheManager) GetAccessTokenInfo(ctx context.Context, token string) (*TokenInfo, error) {
	key := fmt.Sprintf("%s:access_token:%s", constants.KeyPrefixToken, token)

	value, err := m.storage.Get(key)
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
	key := fmt.Sprintf("%s:refresh_token:%s", constants.KeyPrefixToken, refreshToken)
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal refresh token info failed: %w", err)
	}

	// 设置过期时间与刷新Token过期时间一致
	ttl := time.Until(info.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("refresh token already expired")
	}

	return m.storage.Set(key, string(data), ttl)
}

// GetRefreshTokenInfo 获取刷新Token信息
func (m *TokenCacheManager) GetRefreshTokenInfo(ctx context.Context, refreshToken string) (*RefreshTokenInfo, error) {
	key := fmt.Sprintf("%s:refresh_token:%s", constants.KeyPrefixToken, refreshToken)

	value, err := m.storage.Get(key)
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
	key := fmt.Sprintf("%s:access_token:%s", constants.KeyPrefixToken, token)
	return m.storage.Delete(key)
}

// RevokeRefreshToken 撤销刷新Token
func (m *TokenCacheManager) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	key := fmt.Sprintf("%s:refresh_token:%s", constants.KeyPrefixToken, refreshToken)
	return m.storage.Delete(key)
}

// RevokeTokenByID 通过Token ID撤销所有相关Token
func (m *TokenCacheManager) RevokeTokenByID(ctx context.Context, tokenID string) error {
	// 将Token ID加入黑名单
	blacklistKey := fmt.Sprintf("%s:blacklist:%s", constants.KeyPrefixToken, tokenID)

	// 设置黑名单记录，过期时间设置为24小时（防止内存泄漏）
	blacklistInfo := map[string]interface{}{
		"token_id":   tokenID,
		"revoked_at": time.Now().Unix(),
		"reason":     "manual_revoke",
	}

	data, err := json.Marshal(blacklistInfo)
	if err != nil {
		return fmt.Errorf("marshal blacklist info failed: %w", err)
	}

	// 设置24小时过期时间
	return m.storage.Set(blacklistKey, string(data), 24*time.Hour)
}

// IsTokenRevoked 检查Token是否被撤销
func (m *TokenCacheManager) IsTokenRevoked(ctx context.Context, token string) (bool, error) {
	// 首先检查Token是否存在
	key := fmt.Sprintf("%s:access_token:%s", constants.KeyPrefixToken, token)
	exists, err := m.storage.Exists(key)
	if err != nil {
		return false, err
	}

	// 如果Token不存在，说明已被撤销
	if !exists {
		return true, nil
	}

	// 获取Token信息以检查Token ID
	tokenInfo, err := m.GetAccessTokenInfo(ctx, token)
	if err != nil {
		return false, err
	}

	// 检查Token ID是否在黑名单中
	blacklistKey := fmt.Sprintf("%s:blacklist:%s", constants.KeyPrefixToken, tokenInfo.TokenID)
	blacklisted, err := m.storage.Exists(blacklistKey)
	if err != nil {
		return false, err
	}

	return blacklisted, nil
}

// IsRefreshTokenRevoked 检查刷新Token是否被撤销
func (m *TokenCacheManager) IsRefreshTokenRevoked(ctx context.Context, refreshToken string) (bool, error) {
	key := fmt.Sprintf("%s:refresh_token:%s", constants.KeyPrefixToken, refreshToken)
	exists, err := m.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return !exists, nil
}
