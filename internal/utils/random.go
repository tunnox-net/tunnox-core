package utils

import (
	"tunnox-core/internal/utils/random"
)

// 类型别名，保持向后兼容
type RandomGenerator = random.Generator
type DefaultRandomGenerator = random.DefaultGenerator

// 错误别名
var (
	ErrRandomFailed  = random.ErrRandomFailed
	ErrInvalidRange  = random.ErrInvalidRange
	ErrInvalidLength = random.ErrInvalidLength
)

// 常量别名
const (
	Charset      = random.Charset
	DigitCharset = random.DigitCharset
)

// 函数别名，保持向后兼容
var (
	NewDefaultRandomGenerator = random.NewDefaultGenerator
)

// GenerateRandomBytes 生成指定长度的随机字节
func GenerateRandomBytes(length int) ([]byte, error) {
	return random.Bytes(length)
}

// GenerateRandomString 生成指定长度的随机字符串
func GenerateRandomString(length int) (string, error) {
	return random.String(length)
}

// GenerateRandomStringWithCharset 使用指定字符集生成随机字符串
func GenerateRandomStringWithCharset(length int, charset string) (string, error) {
	return random.StringWithCharset(length, charset)
}

// GenerateRandomDigits 生成指定长度的随机数字字符串
func GenerateRandomDigits(length int) (string, error) {
	return random.Digits(length)
}

// GenerateRandomInt64 生成指定范围内的随机int64
func GenerateRandomInt64(min, max int64) (int64, error) {
	return random.Int64(min, max)
}

// GenerateRandomInt 生成指定范围内的随机int
func GenerateRandomInt(min, max int) (int, error) {
	return random.Int(min, max)
}

// GenerateRandomFloat64 生成0到1之间的随机浮点数
func GenerateRandomFloat64() (float64, error) {
	return random.Float64()
}

// GenerateRandomFloat64Range 生成指定范围内的随机浮点数
func GenerateRandomFloat64Range(min, max float64) (float64, error) {
	return random.Float64Range(min, max)
}

// GenerateUUID 生成UUID v4格式的字符串
func GenerateUUID() (string, error) {
	return random.UUID()
}

// ContainsString 判断字符串切片中是否包含指定字符串
func ContainsString(slice []string, item string) bool {
	return random.ContainsString(slice, item)
}

// Int64ToString 将int64转换为字符串
func Int64ToString(n int64) string {
	return random.Int64ToString(n)
}

// StringToInt64 将字符串转换为int64
func StringToInt64(s string) (int64, error) {
	return random.StringToInt64(s)
}
