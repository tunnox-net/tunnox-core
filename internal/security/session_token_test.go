package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SessionTokenManager 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestGenerateSessionToken(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "tls-fp-123")
	require.NoError(t, err)
	require.NotNil(t, token)

	// 验证字段
	assert.NotEmpty(t, token.TokenID)
	assert.Equal(t, int64(123), token.ClientID)
	assert.Equal(t, "192.168.1.100", token.IP)
	assert.Equal(t, "tls-fp-123", token.TLSFingerprint)
	assert.NotEmpty(t, token.Signature)
	assert.True(t, token.ExpiresAt.After(token.IssuedAt))
}

func TestValidateSessionToken_Success(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成Token
	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "tls-fp-123")
	require.NoError(t, err)

	// 验证Token（不检查IP）
	err = manager.ValidateSessionToken(token, "", false)
	assert.NoError(t, err)

	// 验证Token（检查IP，IP匹配）
	err = manager.ValidateSessionToken(token, "192.168.1.100", true)
	assert.NoError(t, err)
}

func TestValidateSessionToken_InvalidSignature(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成Token
	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "tls-fp-123")
	require.NoError(t, err)

	// 篡改Token
	token.ClientID = 999

	// 验证失败（签名不匹配）
	err = manager.ValidateSessionToken(token, "", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestValidateSessionToken_Expired(t *testing.T) {
	// 使用1秒TTL
	config := &SessionTokenConfig{
		SecretKey: "test-secret",
		TTL:       1 * time.Second,
	}
	manager := NewSessionTokenManager(config)

	// 生成Token
	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	// 等待过期
	time.Sleep(1100 * time.Millisecond)

	// 验证失败（已过期）
	err = manager.ValidateSessionToken(token, "", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestValidateSessionToken_IPMismatch(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成Token
	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	// 验证失败（IP不匹配）
	err = manager.ValidateSessionToken(token, "192.168.1.200", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IP mismatch")
}

func TestShouldRenew(t *testing.T) {
	// 使用2小时TTL，1小时续期阈值
	config := &SessionTokenConfig{
		SecretKey:        "test-secret",
		TTL:              2 * time.Hour,
		RenewalThreshold: 1 * time.Hour,
	}
	manager := NewSessionTokenManager(config)

	// 生成Token
	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	// 刚生成的Token不应该续期（剩余2小时）
	assert.False(t, manager.ShouldRenew(token))

	// 手动修改过期时间（剩余30分钟）
	token.ExpiresAt = time.Now().Add(30 * time.Minute)

	// 应该续期（剩余时间<1小时）
	assert.True(t, manager.ShouldRenew(token))
}

func TestRenewToken(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成原始Token
	oldToken, err := manager.GenerateSessionToken(123, "192.168.1.100", "tls-fp-123")
	require.NoError(t, err)

	// 等待1毫秒确保时间不同
	time.Sleep(1 * time.Millisecond)

	// 续期Token
	newToken, err := manager.RenewToken(oldToken)
	require.NoError(t, err)

	// 验证新Token
	assert.NotEqual(t, oldToken.TokenID, newToken.TokenID)
	assert.Equal(t, oldToken.ClientID, newToken.ClientID)
	assert.Equal(t, oldToken.IP, newToken.IP)
	assert.Equal(t, oldToken.TLSFingerprint, newToken.TLSFingerprint)
	assert.True(t, newToken.IssuedAt.After(oldToken.IssuedAt))
}

func TestUpdateActivity(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成Token
	token, err := manager.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	originalActivity := token.LastActivity

	// 等待1毫秒
	time.Sleep(1 * time.Millisecond)

	// 更新活动时间
	manager.UpdateActivity(token)

	assert.True(t, token.LastActivity.After(originalActivity))
}

func TestEncodeDecodeSessionToken(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成Token
	originalToken, err := manager.GenerateSessionToken(123, "192.168.1.100", "tls-fp-123")
	require.NoError(t, err)

	// 编码
	encoded, err := manager.EncodeToken(originalToken)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	// 解码
	decodedToken, err := manager.DecodeToken(encoded)
	require.NoError(t, err)

	// 验证解码后的Token
	assert.Equal(t, originalToken.TokenID, decodedToken.TokenID)
	assert.Equal(t, originalToken.ClientID, decodedToken.ClientID)
	assert.Equal(t, originalToken.IP, decodedToken.IP)
	assert.Equal(t, originalToken.TLSFingerprint, decodedToken.TLSFingerprint)
	assert.Equal(t, originalToken.Signature, decodedToken.Signature)
}

func TestSessionToken_DifferentSecretKey(t *testing.T) {
	// 使用不同的密钥
	manager1 := NewSessionTokenManager(&SessionTokenConfig{
		SecretKey: "secret-1",
		TTL:       24 * time.Hour,
	})

	manager2 := NewSessionTokenManager(&SessionTokenConfig{
		SecretKey: "secret-2",
		TTL:       24 * time.Hour,
	})

	// manager1生成Token
	token, err := manager1.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	// manager2验证失败（密钥不同）
	err = manager2.ValidateSessionToken(token, "", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestSessionToken_UniqueID(t *testing.T) {
	manager := NewSessionTokenManager(nil)

	// 生成两个Token
	token1, err := manager.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	token2, err := manager.GenerateSessionToken(123, "192.168.1.100", "")
	require.NoError(t, err)

	// TokenID应该不同
	assert.NotEqual(t, token1.TokenID, token2.TokenID)
}
