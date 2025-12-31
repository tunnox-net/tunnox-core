package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 会话Token机制
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SessionToken 会话Token
//
// 用于认证后的会话验证，避免每次请求都重新认证。
type SessionToken struct {
	TokenID        string    `json:"token_id"`        // Token唯一ID
	ClientID       int64     `json:"client_id"`       // 客户端ID
	IP             string    `json:"ip"`              // 客户端IP
	TLSFingerprint string    `json:"tls_fingerprint"` // TLS指纹（可选）
	IssuedAt       time.Time `json:"issued_at"`       // 签发时间
	ExpiresAt      time.Time `json:"expires_at"`      // 过期时间
	LastActivity   time.Time `json:"last_activity"`   // 最后活动时间
	Signature      string    `json:"signature"`       // HMAC-SHA256签名
}

// SessionTokenConfig 会话Token配置
type SessionTokenConfig struct {
	SecretKey        string        // HMAC密钥
	TTL              time.Duration // Token有效期（默认24小时）
	RenewalThreshold time.Duration // 续期阈值（默认剩余时间<30分钟时自动续期）
}

// DefaultSessionTokenConfig 默认配置
func DefaultSessionTokenConfig() *SessionTokenConfig {
	return &SessionTokenConfig{
		SecretKey:        "tunnox-session-secret-change-me", // 应该从配置读取
		TTL:              24 * time.Hour,
		RenewalThreshold: 30 * time.Minute,
	}
}

// SessionTokenManager 会话Token管理器
type SessionTokenManager struct {
	config *SessionTokenConfig
}

// NewSessionTokenManager 创建会话Token管理器
func NewSessionTokenManager(config *SessionTokenConfig) *SessionTokenManager {
	if config == nil {
		config = DefaultSessionTokenConfig()
	}

	return &SessionTokenManager{
		config: config,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Token生成
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateSessionToken 生成会话Token
func (m *SessionTokenManager) GenerateSessionToken(clientID int64, ip string, tlsFingerprint string) (*SessionToken, error) {
	now := time.Now()
	tokenID, err := generateSessionTokenID()
	if err != nil {
		return nil, err
	}

	token := &SessionToken{
		TokenID:        tokenID,
		ClientID:       clientID,
		IP:             ip,
		TLSFingerprint: tlsFingerprint,
		IssuedAt:       now,
		ExpiresAt:      now.Add(m.config.TTL),
		LastActivity:   now,
	}

	// 计算签名
	signature, err := m.computeSignature(token)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "failed to compute signature")
	}
	token.Signature = signature

	return token, nil
}

// computeSignature 计算Token签名
func (m *SessionTokenManager) computeSignature(token *SessionToken) (string, error) {
	// 构造签名数据（不包含Signature和LastActivity字段）
	// 使用 fmt.Sprintf 格式化，保持与原有逻辑一致
	data := fmt.Sprintf("%s|%d|%s|%s|%d|%d",
		token.TokenID,
		token.ClientID,
		token.IP,
		token.TLSFingerprint,
		token.IssuedAt.Unix(),
		token.ExpiresAt.Unix(),
	)

	// 使用HMAC-SHA256签名
	h := hmac.New(sha256.New, []byte(m.config.SecretKey))
	h.Write([]byte(data))
	signature := hex.EncodeToString(h.Sum(nil))

	return signature, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Token验证
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ValidateSessionToken 验证会话Token
//
// 验证包括：
// 1. 签名验证（防篡改）
// 2. 过期检查
// 3. IP验证（可选）
func (m *SessionTokenManager) ValidateSessionToken(token *SessionToken, currentIP string, checkIP bool) error {
	// 1. 签名验证
	expectedSignature, err := m.computeSignature(token)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "failed to compute signature")
	}
	if token.Signature != expectedSignature {
		return errors.New("invalid signature")
	}

	// 2. 过期检查
	if time.Now().After(token.ExpiresAt) {
		return errors.New("token expired")
	}

	// 3. IP验证（可选）
	if checkIP && currentIP != "" && token.IP != currentIP {
		return coreerrors.Newf(coreerrors.CodeAuthFailed, "IP mismatch: expected %s, got %s", token.IP, currentIP)
	}

	return nil
}

// ShouldRenew 检查是否应该续期
//
// 如果Token剩余时间少于配置的阈值，返回true。
func (m *SessionTokenManager) ShouldRenew(token *SessionToken) bool {
	remaining := time.Until(token.ExpiresAt)
	return remaining < m.config.RenewalThreshold
}

// RenewToken 续期Token
//
// 生成一个新的Token，保留相同的ClientID和IP，但更新时间。
func (m *SessionTokenManager) RenewToken(oldToken *SessionToken) (*SessionToken, error) {
	return m.GenerateSessionToken(oldToken.ClientID, oldToken.IP, oldToken.TLSFingerprint)
}

// UpdateActivity 更新最后活动时间
func (m *SessionTokenManager) UpdateActivity(token *SessionToken) {
	token.LastActivity = time.Now()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Token序列化
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// EncodeToken 将Token编码为字符串（JSON）
func (m *SessionTokenManager) EncodeToken(token *SessionToken) (string, error) {
	data, err := json.Marshal(token)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "failed to marshal token")
	}
	return string(data), nil
}

// DecodeToken 从字符串解码Token
func (m *SessionTokenManager) DecodeToken(tokenStr string) (*SessionToken, error) {
	var token SessionToken
	if err := json.Unmarshal([]byte(tokenStr), &token); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "failed to unmarshal token")
	}
	return &token, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// generateSessionTokenID 生成Session Token ID
func generateSessionTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "crypto/rand failed in generateSessionTokenID")
	}
	return hex.EncodeToString(b), nil
}
