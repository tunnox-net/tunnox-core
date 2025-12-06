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
	"tunnox-core/internal/core/storage"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 重连Token机制
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ReconnectToken 重连Token
//
// 用于服务器优雅关闭时，客户端快速重连到新节点。
// Token包含签名，防止伪造和篡改。
type ReconnectToken struct {
	TokenID   string    `json:"token_id"`   // Token唯一ID
	ClientID  int64     `json:"client_id"`  // 客户端ID
	NodeID    string    `json:"node_id"`    // 当前节点ID
	IssuedAt  time.Time `json:"issued_at"`  // 签发时间
	ExpiresAt time.Time `json:"expires_at"` // 过期时间（30秒）
	Nonce     string    `json:"nonce"`      // 随机数（防重放）
	Signature string    `json:"signature"`  // HMAC-SHA256签名
}

// ReconnectTokenConfig 重连Token配置
type ReconnectTokenConfig struct {
	SecretKey string        // HMAC密钥
	TTL       time.Duration // Token有效期（默认30秒）
}

// DefaultReconnectTokenConfig 默认配置
func DefaultReconnectTokenConfig() *ReconnectTokenConfig {
	return &ReconnectTokenConfig{
		SecretKey: "tunnox-reconnect-secret-change-me", // 应该从配置读取
		TTL:       30 * time.Second,
	}
}

// ReconnectTokenManager 重连Token管理器
type ReconnectTokenManager struct {
	config  *ReconnectTokenConfig
	storage storage.Storage // 用于存储已使用的Token（防重放）
}

// NewReconnectTokenManager 创建重连Token管理器
func NewReconnectTokenManager(config *ReconnectTokenConfig, storage storage.Storage) *ReconnectTokenManager {
	if config == nil {
		config = DefaultReconnectTokenConfig()
	}

	return &ReconnectTokenManager{
		config:  config,
		storage: storage,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Token生成
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateReconnectToken 生成重连Token
func (m *ReconnectTokenManager) GenerateReconnectToken(clientID int64, nodeID string) (*ReconnectToken, error) {
	now := time.Now()
	tokenID := generateTokenID()
	nonce := generateNonce()

	token := &ReconnectToken{
		TokenID:   tokenID,
		ClientID:  clientID,
		NodeID:    nodeID,
		IssuedAt:  now,
		ExpiresAt: now.Add(m.config.TTL),
		Nonce:     nonce,
	}

	// 计算签名
	signature, err := m.computeSignature(token)
	if err != nil {
		return nil, fmt.Errorf("failed to compute signature: %w", err)
	}
	token.Signature = signature

	return token, nil
}

// computeSignature 计算Token签名
func (m *ReconnectTokenManager) computeSignature(token *ReconnectToken) (string, error) {
	// 构造签名数据（不包含Signature字段）
	data := fmt.Sprintf("%s|%d|%s|%d|%d|%s",
		token.TokenID,
		token.ClientID,
		token.NodeID,
		token.IssuedAt.Unix(),
		token.ExpiresAt.Unix(),
		token.Nonce,
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

// ValidateReconnectToken 验证重连Token
//
// 多重验证：
// 1. 签名验证（防篡改）
// 2. 过期检查
// 3. Nonce防重放（一次性使用）
func (m *ReconnectTokenManager) ValidateReconnectToken(token *ReconnectToken) error {
	// 1. 签名验证
	expectedSignature, err := m.computeSignature(token)
	if err != nil {
		return fmt.Errorf("failed to compute signature: %w", err)
	}
	if token.Signature != expectedSignature {
		return errors.New("invalid signature")
	}

	// 2. 过期检查
	if time.Now().After(token.ExpiresAt) {
		return errors.New("token expired")
	}

	// 3. Nonce防重放检查
	usedKey := fmt.Sprintf("reconnect:token:used:%s", token.TokenID)
	exists, err := m.storage.Exists(usedKey)
	if err != nil {
		return fmt.Errorf("failed to check token usage: %w", err)
	}
	if exists {
		return errors.New("token already used")
	}

	return nil
}

// MarkTokenAsUsed 标记Token为已使用
//
// Token验证成功后，必须立即调用此方法标记为已使用，防止重放攻击。
func (m *ReconnectTokenManager) MarkTokenAsUsed(token *ReconnectToken) error {
	usedKey := fmt.Sprintf("reconnect:token:used:%s", token.TokenID)
	
	// 存储到Redis，TTL为Token的剩余有效期
	ttl := time.Until(token.ExpiresAt)
	if ttl <= 0 {
		return errors.New("token already expired")
	}

	// 存储一个标记（值不重要，只要存在即可）
	if err := m.storage.Set(usedKey, "1", ttl); err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Token序列化
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// EncodeToken 将Token编码为字符串（JSON）
func (m *ReconnectTokenManager) EncodeToken(token *ReconnectToken) (string, error) {
	data, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}
	return string(data), nil
}

// DecodeToken 从字符串解码Token
func (m *ReconnectTokenManager) DecodeToken(tokenStr string) (*ReconnectToken, error) {
	var token ReconnectToken
	if err := json.Unmarshal([]byte(tokenStr), &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}
	return &token, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// generateTokenID 生成Token ID
func generateTokenID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed in generateTokenID: %v", err))
	}
	return hex.EncodeToString(b)
}

// generateNonce 生成随机Nonce
func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed in generateNonce: %v", err))
	}
	return hex.EncodeToString(b)
}

