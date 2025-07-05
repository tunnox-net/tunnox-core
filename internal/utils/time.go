package utils

import (
	"time"
)

// GetCurrentTimestamp 获取当前时间戳（秒）
func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// GetCurrentTimestampMillis 获取当前时间戳（毫秒）
func GetCurrentTimestampMillis() int64 {
	return time.Now().UnixMilli()
}

// GetCurrentTimestampNanos 获取当前时间戳（纳秒）
func GetCurrentTimestampNanos() int64 {
	return time.Now().UnixNano()
}

// FormatTime 格式化时间
func FormatTime(t time.Time, format string) string {
	return t.Format(format)
}

// ParseTime 解析时间字符串
func ParseTime(timeStr, format string) (time.Time, error) {
	return time.Parse(format, timeStr)
}

// GetTimeRange 获取时间范围
func GetTimeRange(timeRange string) (time.Time, time.Time, error) {
	now := time.Now()
	var start time.Time

	switch timeRange {
	case "1h":
		start = now.Add(-1 * time.Hour)
	case "6h":
		start = now.Add(-6 * time.Hour)
	case "12h":
		start = now.Add(-12 * time.Hour)
	case "24h", "1d":
		start = now.Add(-24 * time.Hour)
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
	case "30d":
		start = now.Add(-30 * 24 * time.Hour)
	default:
		// 默认24小时
		start = now.Add(-24 * time.Hour)
	}

	return start, now, nil
}

// IsExpired 检查是否过期
func IsExpired(timestamp int64, duration time.Duration) bool {
	return time.Now().Unix() > timestamp+int64(duration.Seconds())
}

// AddDuration 添加时间间隔
func AddDuration(timestamp int64, duration time.Duration) int64 {
	return timestamp + int64(duration.Seconds())
}

// SubDuration 减去时间间隔
func SubDuration(timestamp int64, duration time.Duration) int64 {
	return timestamp - int64(duration.Seconds())
}
