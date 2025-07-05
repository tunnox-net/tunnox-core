package cloud

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/utils"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager JWT Token管理器
type JWTManager struct {
	config *CloudControlConfig
	cache  *TokenCacheManager
	utils.Dispose
}

// JWTClaims JWT声明
type JWTClaims struct {
	ClientID   string `json:"client_id"`
	UserID     string `json:"user_id,omitempty"`
	ClientType string `json:"client_type"`
	NodeID     string `json:"node_id,omitempty"`
	jwt.RegisteredClaims
}

// RefreshTokenClaims 刷新Token声明
type RefreshTokenClaims struct {
	ClientID string `json:"client_id"`
	TokenID  string `json:"token_id"` // 用于撤销特定Token
	jwt.RegisteredClaims
}

// NewJWTManager 创建JWT管理器
func NewJWTManager(config *CloudControlConfig, repo *Repository) *JWTManager {
	manager := &JWTManager{
		config: config,
		cache:  NewTokenCacheManager(repo.GetStorage()),
	}
	manager.SetCtx(context.Background(), manager.onClose)
	return manager
}

// onClose 资源清理回调
func (m *JWTManager) onClose() {

}

// GenerateTokenPair 生成Token对（访问Token + 刷新Token）
func (m *JWTManager) GenerateTokenPair(ctx context.Context, client *Client) (*JWTTokenInfo, error) {
	now := time.Now()

	// 生成Token ID用于撤销
	tokenID, err := m.generateTokenID()
	if err != nil {
		return nil, fmt.Errorf("generate token ID failed: %w", err)
	}

	// 创建访问Token声明
	accessClaims := &JWTClaims{
		ClientID:   client.ID,
		UserID:     client.UserID,
		ClientType: string(client.Type),
		NodeID:     client.NodeID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.config.JWTIssuer,
			Subject:   client.ID,
			Audience:  []string{"tunnox-client"},
			ExpiresAt: jwt.NewNumericDate(now.Add(m.config.JWTExpiration)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        tokenID,
		},
	}

	// 创建刷新Token声明
	refreshClaims := &RefreshTokenClaims{
		ClientID: client.ID,
		TokenID:  tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.config.JWTIssuer,
			Subject:   client.ID,
			Audience:  []string{"tunnox-refresh"},
			ExpiresAt: jwt.NewNumericDate(now.Add(m.config.RefreshExpiration)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        tokenID,
		},
	}

	// 生成访问Token
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(m.config.JWTSecretKey))
	if err != nil {
		return nil, fmt.Errorf("sign access token failed: %w", err)
	}

	// 生成刷新Token
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(m.config.JWTSecretKey))
	if err != nil {
		return nil, fmt.Errorf("sign refresh token failed: %w", err)
	}

	// 存储Token信息到缓存
	tokenInfo := &TokenInfo{
		ClientID:   client.ID,
		UserID:     client.UserID,
		ClientType: string(client.Type),
		NodeID:     client.NodeID,
		TokenID:    tokenID,
		ExpiresAt:  now.Add(m.config.JWTExpiration),
	}

	refreshTokenInfo := &RefreshTokenInfo{
		ClientID:  client.ID,
		TokenID:   tokenID,
		ExpiresAt: now.Add(m.config.RefreshExpiration),
	}

	if err := m.cache.StoreAccessToken(ctx, accessTokenString, tokenInfo); err != nil {
		return nil, fmt.Errorf("store access token failed: %w", err)
	}

	if err := m.cache.StoreRefreshToken(ctx, refreshTokenString, refreshTokenInfo); err != nil {
		// 如果存储刷新Token失败，需要清理已存储的访问Token
		m.cache.RevokeAccessToken(ctx, accessTokenString)
		return nil, fmt.Errorf("store refresh token failed: %w", err)
	}

	return &JWTTokenInfo{
		Token:        accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    now.Add(m.config.JWTExpiration),
		ClientId:     client.ID,
		TokenID:      tokenID,
	}, nil
}

// ValidateAccessToken 验证访问Token
func (m *JWTManager) ValidateAccessToken(ctx context.Context, tokenString string) (*JWTClaims, error) {
	// 首先检查Token是否被撤销
	revoked, err := m.cache.IsTokenRevoked(ctx, tokenString)
	if err == nil && revoked {
		return nil, fmt.Errorf("token has been revoked")
	}

	// 从缓存获取Token信息（快速验证）
	cachedInfo, err := m.cache.GetAccessTokenInfo(ctx, tokenString)
	if err == nil {
		// 缓存中有信息，验证过期时间
		if time.Now().After(cachedInfo.ExpiresAt) {
			// Token已过期，从缓存中删除
			m.cache.RevokeAccessToken(ctx, tokenString)
			return nil, fmt.Errorf("token expired")
		}

		// 返回缓存的声明信息
		return &JWTClaims{
			ClientID:   cachedInfo.ClientID,
			UserID:     cachedInfo.UserID,
			ClientType: cachedInfo.ClientType,
			NodeID:     cachedInfo.NodeID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    m.config.JWTIssuer,
				Subject:   cachedInfo.ClientID,
				Audience:  []string{"tunnox-client"},
				ExpiresAt: jwt.NewNumericDate(cachedInfo.ExpiresAt),
				ID:        cachedInfo.TokenID,
			},
		}, nil
	}

	// 缓存中没有信息，进行完整的JWT验证
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.config.JWTSecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("parse token failed: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// 验证受众
	if !utils.ContainsString(claims.Audience, "tunnox-client") {
		return nil, fmt.Errorf("invalid audience")
	}

	// 验证签发者
	if claims.Issuer != m.config.JWTIssuer {
		return nil, fmt.Errorf("invalid issuer")
	}

	// 验证通过后，将Token信息存储到缓存中（如果缓存中没有的话）
	tokenInfo := &TokenInfo{
		ClientID:   claims.ClientID,
		UserID:     claims.UserID,
		ClientType: claims.ClientType,
		NodeID:     claims.NodeID,
		TokenID:    claims.ID,
		ExpiresAt:  claims.ExpiresAt.Time,
	}

	// 异步存储到缓存，不阻塞验证流程
	go func() {
		m.cache.StoreAccessToken(context.Background(), tokenString, tokenInfo)
	}()

	return claims, nil
}

// ValidateRefreshToken 验证刷新Token
func (m *JWTManager) ValidateRefreshToken(ctx context.Context, refreshTokenString string) (*RefreshTokenClaims, error) {
	// 首先检查刷新Token是否被撤销
	revoked, err := m.cache.IsRefreshTokenRevoked(ctx, refreshTokenString)
	if err == nil && revoked {
		return nil, fmt.Errorf("refresh token has been revoked")
	}

	// 从缓存获取刷新Token信息（快速验证）
	cachedInfo, err := m.cache.GetRefreshTokenInfo(ctx, refreshTokenString)
	if err == nil {
		// 缓存中有信息，验证过期时间
		if time.Now().After(cachedInfo.ExpiresAt) {
			// 刷新Token已过期，从缓存中删除
			m.cache.RevokeRefreshToken(ctx, refreshTokenString)
			return nil, fmt.Errorf("refresh token expired")
		}

		// 返回缓存的声明信息
		return &RefreshTokenClaims{
			ClientID: cachedInfo.ClientID,
			TokenID:  cachedInfo.TokenID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    m.config.JWTIssuer,
				Subject:   cachedInfo.ClientID,
				Audience:  []string{"tunnox-refresh"},
				ExpiresAt: jwt.NewNumericDate(cachedInfo.ExpiresAt),
				ID:        cachedInfo.TokenID,
			},
		}, nil
	}

	// 缓存中没有信息，进行完整的JWT验证
	token, err := jwt.ParseWithClaims(refreshTokenString, &RefreshTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.config.JWTSecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("parse refresh token failed: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	claims, ok := token.Claims.(*RefreshTokenClaims)
	if !ok {
		return nil, fmt.Errorf("invalid refresh token claims")
	}

	// 验证受众
	if !utils.ContainsString(claims.Audience, "tunnox-refresh") {
		return nil, fmt.Errorf("invalid audience")
	}

	// 验证签发者
	if claims.Issuer != m.config.JWTIssuer {
		return nil, fmt.Errorf("invalid issuer")
	}

	// 验证通过后，将刷新Token信息存储到缓存中（如果缓存中没有的话）
	refreshTokenInfo := &RefreshTokenInfo{
		ClientID:  claims.ClientID,
		TokenID:   claims.TokenID,
		ExpiresAt: claims.ExpiresAt.Time,
	}

	// 异步存储到缓存，不阻塞验证流程
	go func() {
		m.cache.StoreRefreshToken(context.Background(), refreshTokenString, refreshTokenInfo)
	}()

	return claims, nil
}

// RefreshAccessToken 使用刷新Token生成新的访问Token
func (m *JWTManager) RefreshAccessToken(ctx context.Context, refreshTokenString string, client *Client) (*JWTTokenInfo, error) {
	// 验证刷新Token
	refreshClaims, err := m.ValidateRefreshToken(ctx, refreshTokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 校验ClientID是否匹配
	if refreshClaims.ClientID != client.ID {
		return nil, fmt.Errorf("client ID mismatch")
	}

	// 生成新的Token对
	return m.GenerateTokenPair(ctx, client)
}

// RevokeToken 撤销Token
func (m *JWTManager) RevokeToken(ctx context.Context, tokenID string) error {
	// 将Token ID加入黑名单
	return m.cache.RevokeTokenByID(ctx, tokenID)
}

// generateTokenID 生成Token ID
func (m *JWTManager) generateTokenID() (string, error) {
	return utils.GenerateRandomString(32)
}

// contains 检查字符串切片是否包含指定字符串
// 已移到utils包

func (m *JWTManager) Cache() *TokenCacheManager {
	return m.cache
}
