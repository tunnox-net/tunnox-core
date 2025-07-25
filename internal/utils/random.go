package utils

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"
)

// 错误定义
var (
	ErrRandomFailed  = errors.New("failed to generate random bytes")
	ErrInvalidRange  = errors.New("invalid range: min must be less than max")
	ErrInvalidLength = errors.New("invalid length")
)

// 常量定义
const (
	// 字符集
	Charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	// 数字字符集
	DigitCharset = "0123456789"
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
	rg.mutex.Lock()
	defer rg.mutex.Unlock()

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

// 全局默认随机数生成器实例
var defaultGenerator = NewDefaultRandomGenerator()

// GenerateRandomBytes 生成指定长度的随机字节
func GenerateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, ErrRandomFailed
	}
	return bytes, nil
}

// GenerateRandomString 生成指定长度的随机字符串
func GenerateRandomString(length int) (string, error) {
	return GenerateRandomStringWithCharset(length, Charset)
}

// GenerateRandomStringWithCharset 使用指定字符集生成随机字符串
func GenerateRandomStringWithCharset(length int, charset string) (string, error) {
	bytes, err := GenerateRandomBytes(length)
	if err != nil {
		return "", err
	}

	result := make([]byte, length)
	for i := range result {
		result[i] = charset[bytes[i]%byte(len(charset))]
	}
	return string(result), nil
}

// GenerateRandomDigits 生成指定长度的随机数字字符串
func GenerateRandomDigits(length int) (string, error) {
	return GenerateRandomStringWithCharset(length, DigitCharset)
}

// GenerateRandomInt64 生成指定范围内的随机int64
func GenerateRandomInt64(min, max int64) (int64, error) {
	if min >= max {
		return 0, ErrInvalidRange
	}

	bytes, err := GenerateRandomBytes(8)
	if err != nil {
		return 0, err
	}

	// 转换为uint64
	randomUint := binary.BigEndian.Uint64(bytes)

	// 确保在范围内
	rangeSize := max - min + 1
	randomInt := min + int64(randomUint%uint64(rangeSize))

	return randomInt, nil
}

// GenerateRandomInt 生成指定范围内的随机int
func GenerateRandomInt(min, max int) (int, error) {
	if min >= max {
		return 0, ErrInvalidRange
	}

	bytes, err := GenerateRandomBytes(4)
	if err != nil {
		return 0, err
	}

	// 转换为int
	randomInt := int(binary.BigEndian.Uint32(bytes))

	// 确保在范围内
	rangeSize := max - min + 1
	randomInt = min + (randomInt % rangeSize)
	if randomInt < 0 {
		randomInt = -randomInt
		randomInt = min + (randomInt % rangeSize)
	}

	return randomInt, nil
}

// GenerateRandomFloat64 生成0到1之间的随机浮点数
func GenerateRandomFloat64() (float64, error) {
	bytes, err := GenerateRandomBytes(8)
	if err != nil {
		return 0, err
	}

	// 转换为uint64
	randomUint := binary.BigEndian.Uint64(bytes)

	// 转换为0-1之间的浮点数
	return float64(randomUint) / float64(^uint64(0)), nil
}

// GenerateRandomFloat64Range 生成指定范围内的随机浮点数
func GenerateRandomFloat64Range(min, max float64) (float64, error) {
	if min >= max {
		return 0, ErrInvalidRange
	}

	randomFloat, err := GenerateRandomFloat64()
	if err != nil {
		return 0, err
	}

	return min + randomFloat*(max-min), nil
}

// GenerateUUID 生成UUID v4格式的字符串
func GenerateUUID() (string, error) {
	bytes, err := GenerateRandomBytes(16)
	if err != nil {
		return "", err
	}

	// 设置版本位 (version 4)
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	// 设置变体位
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	// 格式化为UUID字符串
	return formatUUID(bytes), nil
}

// formatUUID 将字节数组格式化为UUID字符串
func formatUUID(bytes []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		bytes[0], bytes[1], bytes[2], bytes[3],
		bytes[4], bytes[5],
		bytes[6], bytes[7],
		bytes[8], bytes[9],
		bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15])
}

// ContainsString 判断字符串切片中是否包含指定字符串
func ContainsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Int64ToString 将int64转换为字符串
func Int64ToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

// StringToInt64 将字符串转换为int64
func StringToInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
