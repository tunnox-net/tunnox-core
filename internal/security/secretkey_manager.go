package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"

	coreerrors "tunnox-core/internal/core/errors"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SecretKey 管理器
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SecretKeyManager SecretKey 管理器
//
// 功能：
// - 生成 SecretKey 凭据
// - AES-256-GCM 加密/解密存储
// - 挑战-响应验证
type SecretKeyManager struct {
	masterKey []byte // AES-256 主密钥（32字节）
}

// SecretKeyConfig SecretKey 管理器配置
type SecretKeyConfig struct {
	MasterKey string // Base64 编码的主密钥（32字节）
}

// DefaultSecretKeyConfig 默认配置
// 注意：MasterKey 必须从应用配置中注入
func DefaultSecretKeyConfig() *SecretKeyConfig {
	return &SecretKeyConfig{
		MasterKey: "", // 必须从配置注入
	}
}

// NewSecretKeyManager 创建 SecretKey 管理器
func NewSecretKeyManager(config *SecretKeyConfig) (*SecretKeyManager, error) {
	if config == nil {
		config = DefaultSecretKeyConfig()
	}

	if config.MasterKey == "" {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "master key is required")
	}

	// 解码 Base64 主密钥
	masterKey, err := base64.StdEncoding.DecodeString(config.MasterKey)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "invalid master key format")
	}

	// AES-256 需要 32 字节密钥
	if len(masterKey) != 32 {
		return nil, coreerrors.Newf(coreerrors.CodeInvalidParam, "master key must be 32 bytes, got %d", len(masterKey))
	}

	return &SecretKeyManager{
		masterKey: masterKey,
	}, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 凭据生成
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateCredentials 生成 SecretKey 凭据
//
// 返回：
// - plaintext: SecretKey 明文（32字符，仅返回一次给客户端）
// - encrypted: 加密后的 SecretKey（Base64 编码，存储到数据库）
func (m *SecretKeyManager) GenerateCredentials() (plaintext, encrypted string, err error) {
	// 生成 32 字节随机 SecretKey
	secretKeyBytes := make([]byte, 32)
	if _, err := rand.Read(secretKeyBytes); err != nil {
		return "", "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate secret key")
	}
	plaintext = hex.EncodeToString(secretKeyBytes)

	// 加密
	encrypted, err = m.Encrypt(plaintext)
	if err != nil {
		return "", "", err
	}

	return plaintext, encrypted, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 加密/解密
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Encrypt 使用 AES-256-GCM 加密 SecretKey
//
// 格式：Base64(nonce + ciphertext)
// - nonce: 12 字节随机数
// - ciphertext: 加密后的数据 + GCM tag
func (m *SecretKeyManager) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create AES cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create GCM")
	}

	// 生成随机 nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate nonce")
	}

	// 加密
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 SecretKey
func (m *SecretKeyManager) Decrypt(encrypted string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "invalid encrypted data format")
	}

	block, err := aes.NewCipher(m.masterKey)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create AES cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create GCM")
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", coreerrors.New(coreerrors.CodeInvalidParam, "ciphertext too short")
	}

	nonce, ciphertextData := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeAuthFailed, "failed to decrypt")
	}

	return string(plaintext), nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 挑战-响应验证
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateChallenge 生成随机挑战
//
// 返回 32 字节的十六进制字符串
func (m *SecretKeyManager) GenerateChallenge() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate challenge")
	}
	return hex.EncodeToString(b), nil
}

// ComputeResponse 计算挑战响应
//
// 使用 HMAC-SHA256(secretKey, challenge) 计算响应
func (m *SecretKeyManager) ComputeResponse(secretKey, challenge string) string {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(challenge))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyResponse 验证挑战响应
//
// 1. 解密存储的 SecretKey
// 2. 计算期望的响应
// 3. 安全比较
func (m *SecretKeyManager) VerifyResponse(encryptedKey, challenge, response string) bool {
	// 解密获取 SecretKey
	secretKey, err := m.Decrypt(encryptedKey)
	if err != nil {
		return false
	}

	// 计算期望的响应
	expectedResponse := m.ComputeResponse(secretKey, challenge)

	// 安全比较（防止时序攻击）
	return hmac.Equal([]byte(expectedResponse), []byte(response))
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateMasterKey 生成新的主密钥（用于首次配置）
//
// 返回 Base64 编码的 32 字节密钥
func GenerateMasterKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate master key")
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
