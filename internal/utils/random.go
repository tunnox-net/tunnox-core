package utils

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
)

// 错误定义
var (
	ErrRandomFailed = errors.New("failed to generate random bytes")
)

// 常量定义
const (
	// 字符集
	Charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	// 数字字符集
	DigitCharset = "0123456789"
)

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
		return 0, errors.New("invalid range: min must be less than max")
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
		return 0, errors.New("invalid range: min must be less than max")
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
		return 0, errors.New("invalid range: min must be less than max")
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
