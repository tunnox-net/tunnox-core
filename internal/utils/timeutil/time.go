// Package timeutil 提供时间相关的工具函数
package timeutil

import (
	"time"
)

// CurrentTimestamp 获取当前时间戳（秒）
func CurrentTimestamp() int64 {
	return time.Now().Unix()
}

// CurrentTimestampMillis 获取当前时间戳（毫秒）
func CurrentTimestampMillis() int64 {
	return time.Now().UnixMilli()
}

// CurrentTimestampNanos 获取当前时间戳（纳秒）
func CurrentTimestampNanos() int64 {
	return time.Now().UnixNano()
}

// Format 格式化时间
func Format(t time.Time, format string) string {
	return t.Format(format)
}

// Parse 解析时间字符串
func Parse(timeStr, format string) (time.Time, error) {
	return time.Parse(format, timeStr)
}

// Range 获取时间范围
func Range(timeRange string) (time.Time, time.Time, error) {
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
