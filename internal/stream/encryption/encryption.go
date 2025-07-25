package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

// Encryption 加密接口
type Encryption interface {
	// Encrypt 加密数据
	Encrypt(data []byte) ([]byte, error)

	// Decrypt 解密数据
	Decrypt(data []byte) ([]byte, error)

	// GetKey 获取密钥
	GetKey() []byte

	// SetKey 设置密钥
	SetKey(key []byte) error
}

// AESEncryption AES加密实现
type AESEncryption struct {
	key []byte
}

// NewAESEncryption 创建新的AES加密实例
func NewAESEncryption(key []byte) (*AESEncryption, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeyLength
	}
	return &AESEncryption{key: key}, nil
}

// Encrypt 加密数据
func (ae *AESEncryption) Encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(ae.key)
	if err != nil {
		return nil, err
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 创建随机数
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// 加密
	return gcm.Seal(nonce, nonce, data, nil), nil
}

// Decrypt 解密数据
func (ae *AESEncryption) Decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(ae.key)
	if err != nil {
		return nil, err
	}

	// 创建GCM模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 检查数据长度
	if len(data) < gcm.NonceSize() {
		return nil, ErrInvalidDataLength
	}

	// 分离nonce和密文
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]

	// 解密
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// GetKey 获取密钥
func (ae *AESEncryption) GetKey() []byte {
	return ae.key
}

// SetKey 设置密钥
func (ae *AESEncryption) SetKey(key []byte) error {
	if len(key) != 32 {
		return ErrInvalidKeyLength
	}
	ae.key = key
	return nil
}

// NoEncryption 无加密实现
type NoEncryption struct{}

// NewNoEncryption 创建新的无加密实例
func NewNoEncryption() *NoEncryption {
	return &NoEncryption{}
}

// Encrypt 加密数据（无加密）
func (ne *NoEncryption) Encrypt(data []byte) ([]byte, error) {
	return data, nil
}

// Decrypt 解密数据（无加密）
func (ne *NoEncryption) Decrypt(data []byte) ([]byte, error) {
	return data, nil
}

// GetKey 获取密钥
func (ne *NoEncryption) GetKey() []byte {
	return nil
}

// SetKey 设置密钥
func (ne *NoEncryption) SetKey(key []byte) error {
	return nil
}

// 错误定义
var (
	ErrInvalidKeyLength  = &EncryptionError{Message: "invalid key length"}
	ErrInvalidDataLength = &EncryptionError{Message: "invalid data length"}
)

// EncryptionError 加密错误
type EncryptionError struct {
	Message string
}

func (e *EncryptionError) Error() string {
	return e.Message
}
