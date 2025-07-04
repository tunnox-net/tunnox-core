package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"
)

var ErrIDExhausted = errors.New("ID exhausted")

// IDGenerator ID生成器
type IDGenerator struct {
	usedIDs map[int64]bool
	mu      sync.RWMutex
}

// NewIDGenerator 创建新的ID生成器
func NewIDGenerator() *IDGenerator {
	return &IDGenerator{
		usedIDs: make(map[int64]bool),
	}
}

// GenerateClientID 生成客户端ID（8位大于10000000的随机整数）
func (g *IDGenerator) GenerateClientID() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 生成范围：10000000 - 99999999
	min := int64(10000000)
	max := int64(99999999)

	for attempts := 0; attempts < 100; attempts++ {
		// 生成随机数
		randomBytes := make([]byte, 8)
		_, err := rand.Read(randomBytes)
		if err != nil {
			return 0, err
		}

		// 转换为int64
		randomInt := int64(binary.BigEndian.Uint64(randomBytes))

		// 确保在范围内
		rangeSize := max - min + 1
		randomInt = min + (randomInt % rangeSize)
		if randomInt < 0 {
			randomInt = -randomInt
			randomInt = min + (randomInt % rangeSize)
		}

		// 检查是否已使用
		if !g.usedIDs[randomInt] {
			g.usedIDs[randomInt] = true
			return randomInt, nil
		}
	}

	return 0, ErrIDExhausted
}

// GenerateAuthCode 生成认证码（类似TeamViewer的6位数字）
func (g *IDGenerator) GenerateAuthCode() (string, error) {
	// 生成6位随机数字
	code := ""
	for i := 0; i < 6; i++ {
		randomByte := make([]byte, 1)
		_, err := rand.Read(randomByte)
		if err != nil {
			return "", err
		}
		digit := int(randomByte[0]) % 10
		code += string(rune('0' + digit))
	}
	return code, nil
}

// GenerateSecretKey 生成密钥（32位随机字符串）
func (g *IDGenerator) GenerateSecretKey() (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	key := make([]byte, 32)

	for i := range key {
		randomByte := make([]byte, 1)
		_, err := rand.Read(randomByte)
		if err != nil {
			return "", err
		}
		key[i] = charset[randomByte[0]%byte(len(charset))]
	}

	return string(key), nil
}

// GenerateUserID 生成用户ID
func (g *IDGenerator) GenerateUserID() (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	userID := make([]byte, 16)

	for i := range userID {
		randomByte := make([]byte, 1)
		_, err := rand.Read(randomByte)
		if err != nil {
			return "", err
		}
		userID[i] = charset[randomByte[0]%byte(len(charset))]
	}

	return string(userID), nil
}

// GenerateMappingID 生成端口映射ID
func (g *IDGenerator) GenerateMappingID() (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	mappingID := make([]byte, 12)

	for i := range mappingID {
		randomByte := make([]byte, 1)
		_, err := rand.Read(randomByte)
		if err != nil {
			return "", err
		}
		mappingID[i] = charset[randomByte[0]%byte(len(charset))]
	}

	return string(mappingID), nil
}

// ReleaseClientID 释放客户端ID
func (g *IDGenerator) ReleaseClientID(clientID int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.usedIDs, clientID)
}

// IsClientIDUsed 检查客户端ID是否已使用
func (g *IDGenerator) IsClientIDUsed(clientID int64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.usedIDs[clientID]
}

// GetUsedCount 获取已使用的ID数量
func (g *IDGenerator) GetUsedCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.usedIDs)
}
