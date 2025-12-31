package base

import (
	"context"
	"time"
)

// JWTTokenResult JWT令牌生成结果接口
type JWTTokenResult interface {
	GetToken() string
	GetRefreshToken() string
	GetExpiresAt() time.Time
	GetClientId() int64
	GetTokenID() string
}

// JWTClaimsResult JWT声明结果接口
type JWTClaimsResult interface {
	GetClientID() int64
	GetUserID() string
	GetClientType() string
	GetNodeID() string
}

// RefreshTokenClaimsResult 刷新Token声明结果接口
type RefreshTokenClaimsResult interface {
	GetClientID() int64
	GetTokenID() string
}

// JWTProvider JWT令牌提供者接口
// 由 managers.JWTManager 实现，供 Service 层使用
// 注意：返回类型使用 interface{} 以避免循环依赖，实际实现会返回具体类型
type JWTProvider interface {
	// GenerateTokenPair 生成Token对（访问Token + 刷新Token）
	// 返回 *managers.JWTTokenInfo
	GenerateTokenPair(ctx context.Context, client interface{}) (JWTTokenResult, error)
	// ValidateAccessToken 验证访问Token
	// 返回 *managers.JWTClaims
	ValidateAccessToken(ctx context.Context, tokenString string) (JWTClaimsResult, error)
	// ValidateRefreshToken 验证刷新Token
	// 返回 *managers.RefreshTokenClaims
	ValidateRefreshToken(ctx context.Context, refreshTokenString string) (RefreshTokenClaimsResult, error)
	// RefreshAccessToken 使用刷新Token生成新的访问Token
	// 返回 *managers.JWTTokenInfo
	RefreshAccessToken(ctx context.Context, refreshTokenString string, client interface{}) (JWTTokenResult, error)
	// RevokeToken 撤销Token
	RevokeToken(ctx context.Context, tokenID string) error
}
