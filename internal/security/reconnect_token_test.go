package security

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// ReconnectTokenManager 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestGenerateReconnectToken(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)
	require.NotNil(t, token)

	// 验证字段
	assert.NotEmpty(t, token.TokenID)
	assert.Equal(t, int64(123), token.ClientID)
	assert.Equal(t, "node-abc", token.NodeID)
	assert.NotEmpty(t, token.Nonce)
	assert.NotEmpty(t, token.Signature)
	assert.True(t, token.ExpiresAt.After(token.IssuedAt))
}

func TestValidateReconnectToken_Success(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	// 生成Token
	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// 验证Token
	err = manager.ValidateReconnectToken(token)
	assert.NoError(t, err)
}

func TestValidateReconnectToken_InvalidSignature(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	// 生成Token
	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// 篡改Token
	token.ClientID = 999

	// 验证失败（签名不匹配）
	err = manager.ValidateReconnectToken(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestValidateReconnectToken_Expired(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	
	// 使用1秒TTL
	config := &ReconnectTokenConfig{
		SecretKey: "test-secret",
		TTL:       1 * time.Second,
	}
	manager := NewReconnectTokenManager(config, memStorage)

	// 生成Token
	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// 等待过期
	time.Sleep(1100 * time.Millisecond)

	// 验证失败（已过期）
	err = manager.ValidateReconnectToken(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestValidateReconnectToken_AlreadyUsed(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	// 生成Token
	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// 第一次验证成功
	err = manager.ValidateReconnectToken(token)
	require.NoError(t, err)

	// 标记为已使用
	err = manager.MarkTokenAsUsed(token)
	require.NoError(t, err)

	// 第二次验证失败（已使用）
	err = manager.ValidateReconnectToken(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token already used")
}

func TestMarkTokenAsUsed(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	// 生成Token
	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// 标记为已使用
	err = manager.MarkTokenAsUsed(token)
	assert.NoError(t, err)

	// 检查存储
	usedKey := "reconnect:token:used:" + token.TokenID
	exists, err := memStorage.Exists(usedKey)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEncodeDecodeToken(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	// 生成Token
	originalToken, err := manager.GenerateReconnectToken(123, "node-abc")
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
	assert.Equal(t, originalToken.NodeID, decodedToken.NodeID)
	assert.Equal(t, originalToken.Nonce, decodedToken.Nonce)
	assert.Equal(t, originalToken.Signature, decodedToken.Signature)
}

func TestReconnectToken_DifferentSecretKey(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	// 使用不同的密钥
	manager1 := NewReconnectTokenManager(&ReconnectTokenConfig{
		SecretKey: "secret-1",
		TTL:       30 * time.Second,
	}, memStorage)

	manager2 := NewReconnectTokenManager(&ReconnectTokenConfig{
		SecretKey: "secret-2",
		TTL:       30 * time.Second,
	}, memStorage)

	// manager1生成Token
	token, err := manager1.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// manager2验证失败（密钥不同）
	err = manager2.ValidateReconnectToken(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestReconnectToken_UniqueNonce(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewReconnectTokenManager(nil, memStorage)

	// 生成两个Token
	token1, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	token2, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// Nonce应该不同
	assert.NotEqual(t, token1.Nonce, token2.Nonce)
	assert.NotEqual(t, token1.TokenID, token2.TokenID)
}

func TestReconnectToken_TTLExpiration(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	// 使用2秒TTL
	config := &ReconnectTokenConfig{
		SecretKey: "test-secret",
		TTL:       2 * time.Second,
	}
	manager := NewReconnectTokenManager(config, memStorage)

	// 生成Token
	token, err := manager.GenerateReconnectToken(123, "node-abc")
	require.NoError(t, err)

	// 标记为已使用
	err = manager.MarkTokenAsUsed(token)
	require.NoError(t, err)

	// 等待TTL过期
	time.Sleep(2100 * time.Millisecond)

	// 存储中的标记应该已过期
	usedKey := "reconnect:token:used:" + token.TokenID
	exists, err := memStorage.Exists(usedKey)
	require.NoError(t, err)
	assert.False(t, exists, "Used token marker should expire after TTL")
}

