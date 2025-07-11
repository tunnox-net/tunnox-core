package utils

import (
	"fmt"
	"time"
)

// GenerateOrderedRandomString 生成统一格式的有序随机串
// 格式：prefix_timestamp_randomString
// 例如：node_1701234567890_Ab3x9kLm
func GenerateOrderedRandomString(prefix string, randomLength int) (string, error) {
	if randomLength <= 0 {
		return "", fmt.Errorf("random length must be positive, got %d", randomLength)
	}

	// 获取毫秒级时间戳
	timestamp := time.Now().UnixMilli()

	// 生成随机字符串
	randomPart, err := GenerateRandomString(randomLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate random string: %w", err)
	}

	// 组装ID
	return fmt.Sprintf("%s%d_%s", prefix, timestamp, randomPart), nil
}

// GenerateTimestampOrderedString 生成时间戳有序的随机串（别名，保持兼容性）
func GenerateTimestampOrderedString(prefix string, randomLength int) (string, error) {
	return GenerateOrderedRandomString(prefix, randomLength)
}
