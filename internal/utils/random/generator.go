package random

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
)

// RandomGenerator 随机数生成器接口
type RandomGenerator interface {
	// GenerateBytes 生成随机字节
	GenerateBytes(length int) ([]byte, error)

	// GenerateInt 生成随机整数
	GenerateInt(min, max int64) (int64, error)

	// GenerateString 生成随机字符串
	GenerateString(length int, charset string) (string, error)

	// GenerateUUID 生成UUID
	GenerateUUID() (string, error)

	// GenerateID 生成ID
	GenerateID(prefix string) (string, error)
}

// DefaultRandomGenerator 默认随机数生成器
type DefaultRandomGenerator struct {
	mutex sync.Mutex
}

// NewDefaultRandomGenerator 创建新的默认随机数生成器
func NewDefaultRandomGenerator() *DefaultRandomGenerator {
	return &DefaultRandomGenerator{}
}

// GenerateBytes 生成随机字节
func (rg *DefaultRandomGenerator) GenerateBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	return bytes, err
}

// GenerateInt 生成随机整数
func (rg *DefaultRandomGenerator) GenerateInt(min, max int64) (int64, error) {
	if min >= max {
		return 0, ErrInvalidRange
	}

	delta := max - min
	bigDelta := big.NewInt(delta)

	randomBigInt, err := rand.Int(rand.Reader, bigDelta)
	if err != nil {
		return 0, err
	}

	return min + randomBigInt.Int64(), nil
}

// GenerateString 生成随机字符串
func (rg *DefaultRandomGenerator) GenerateString(length int, charset string) (string, error) {
	if length <= 0 {
		return "", ErrInvalidLength
	}

	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}

	charsetLen := big.NewInt(int64(len(charset)))
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		result[i] = charset[randomIndex.Int64()]
	}

	return string(result), nil
}

// GenerateUUID 生成UUID
func (rg *DefaultRandomGenerator) GenerateUUID() (string, error) {
	bytes, err := rg.GenerateBytes(16)
	if err != nil {
		return "", err
	}

	// 设置版本和变体位
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // 版本4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // 变体位

	// 格式化为UUID字符串
	return formatUUID(bytes), nil
}

// GenerateID 生成ID
func (rg *DefaultRandomGenerator) GenerateID(prefix string) (string, error) {
	randomPart, err := rg.GenerateString(8, "0123456789abcdef")
	if err != nil {
		return "", err
	}

	if prefix == "" {
		return randomPart, nil
	}

	return prefix + "_" + randomPart, nil
}

// formatUUID 格式化UUID
func formatUUID(bytes []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		bytes[0], bytes[1], bytes[2], bytes[3],
		bytes[4], bytes[5],
		bytes[6], bytes[7],
		bytes[8], bytes[9],
		bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15])
}

// 错误定义
var (
	ErrInvalidRange  = &RandomError{Message: "invalid range"}
	ErrInvalidLength = &RandomError{Message: "invalid length"}
)

// RandomError 随机数错误
type RandomError struct {
	Message string
}

func (e *RandomError) Error() string {
	return e.Message
}
