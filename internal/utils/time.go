package utils

import (
	"time"

	"tunnox-core/internal/utils/timeutil"
)

// GetCurrentTimestamp 获取当前时间戳（秒）
func GetCurrentTimestamp() int64 {
	return timeutil.CurrentTimestamp()
}

// GetCurrentTimestampMillis 获取当前时间戳（毫秒）
func GetCurrentTimestampMillis() int64 {
	return timeutil.CurrentTimestampMillis()
}

// GetCurrentTimestampNanos 获取当前时间戳（纳秒）
func GetCurrentTimestampNanos() int64 {
	return timeutil.CurrentTimestampNanos()
}

// FormatTime 格式化时间
func FormatTime(t time.Time, format string) string {
	return timeutil.Format(t, format)
}

// ParseTime 解析时间字符串
func ParseTime(timeStr, format string) (time.Time, error) {
	return timeutil.Parse(timeStr, format)
}

// GetTimeRange 获取时间范围
func GetTimeRange(timeRange string) (time.Time, time.Time, error) {
	return timeutil.Range(timeRange)
}

// IsExpired 检查是否过期
func IsExpired(timestamp int64, duration time.Duration) bool {
	return timeutil.IsExpired(timestamp, duration)
}

// AddDuration 添加时间间隔
func AddDuration(timestamp int64, duration time.Duration) int64 {
	return timeutil.AddDuration(timestamp, duration)
}

// SubDuration 减去时间间隔
func SubDuration(timestamp int64, duration time.Duration) int64 {
	return timeutil.SubDuration(timestamp, duration)
}
