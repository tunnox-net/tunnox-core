package time

import (
	"fmt"
	"time"
)

// TimeUtils 时间工具接口
type TimeUtils interface {
	// Now 获取当前时间
	Now() time.Time

	// NowUnix 获取当前Unix时间戳
	NowUnix() int64

	// NowUnixNano 获取当前Unix纳秒时间戳
	NowUnixNano() int64

	// FormatTime 格式化时间
	FormatTime(t time.Time, format string) string

	// ParseTime 解析时间字符串
	ParseTime(timeStr, format string) (time.Time, error)

	// DurationToString 将持续时间转换为字符串
	DurationToString(d time.Duration) string

	// StringToDuration 将字符串转换为持续时间
	StringToDuration(durationStr string) (time.Duration, error)

	// IsExpired 检查时间是否过期
	IsExpired(t time.Time) bool

	// GetTimeUntil 获取距离指定时间的剩余时间
	GetTimeUntil(t time.Time) time.Duration
}

// DefaultTimeUtils 默认时间工具实现
type DefaultTimeUtils struct{}

// NewDefaultTimeUtils 创建新的默认时间工具
func NewDefaultTimeUtils() *DefaultTimeUtils {
	return &DefaultTimeUtils{}
}

// Now 获取当前时间
func (tu *DefaultTimeUtils) Now() time.Time {
	return time.Now()
}

// NowUnix 获取当前Unix时间戳
func (tu *DefaultTimeUtils) NowUnix() int64 {
	return time.Now().Unix()
}

// NowUnixNano 获取当前Unix纳秒时间戳
func (tu *DefaultTimeUtils) NowUnixNano() int64 {
	return time.Now().UnixNano()
}

// FormatTime 格式化时间
func (tu *DefaultTimeUtils) FormatTime(t time.Time, format string) string {
	if format == "" {
		format = "2006-01-02 15:04:05"
	}
	return t.Format(format)
}

// ParseTime 解析时间字符串
func (tu *DefaultTimeUtils) ParseTime(timeStr, format string) (time.Time, error) {
	if format == "" {
		format = "2006-01-02 15:04:05"
	}
	return time.Parse(format, timeStr)
}

// DurationToString 将持续时间转换为字符串
func (tu *DefaultTimeUtils) DurationToString(d time.Duration) string {
	if d < time.Second {
		return d.String()
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

// StringToDuration 将字符串转换为持续时间
func (tu *DefaultTimeUtils) StringToDuration(durationStr string) (time.Duration, error) {
	return time.ParseDuration(durationStr)
}

// IsExpired 检查时间是否过期
func (tu *DefaultTimeUtils) IsExpired(t time.Time) bool {
	return time.Now().After(t)
}

// GetTimeUntil 获取距离指定时间的剩余时间
func (tu *DefaultTimeUtils) GetTimeUntil(t time.Time) time.Duration {
	return t.Sub(time.Now())
}

// TimeConstants 时间常量
const (
	Second = time.Second
	Minute = time.Minute
	Hour   = time.Hour
	Day    = 24 * time.Hour
	Week   = 7 * Day
	Month  = 30 * Day
	Year   = 365 * Day
)

// CommonTimeFormats 常用时间格式
var CommonTimeFormats = map[string]string{
	"datetime": "2006-01-02 15:04:05",
	"date":     "2006-01-02",
	"time":     "15:04:05",
	"iso":      "2006-01-02T15:04:05Z07:00",
	"rfc3339":  time.RFC3339,
	"rfc822":   time.RFC822,
	"unix":     "unix",
}

// ============================================================================
// SafeTimer - 避免 time.After 内存泄漏的工具
// ============================================================================

// SafeTimer 安全的定时器，用于替代循环中的 time.After
// 使用方法:
//
//	timer := NewSafeTimer(5 * time.Second)
//	defer timer.Stop()
//	for {
//	    timer.Reset(5 * time.Second)
//	    select {
//	    case <-ctx.Done():
//	        return
//	    case <-timer.C():
//	        // 超时处理
//	    case msg := <-msgChan:
//	        // 消息处理
//	    }
//	}
type SafeTimer struct {
	timer *time.Timer
}

// NewSafeTimer 创建新的安全定时器
func NewSafeTimer(d time.Duration) *SafeTimer {
	return &SafeTimer{
		timer: time.NewTimer(d),
	}
}

// C 返回定时器通道
func (t *SafeTimer) C() <-chan time.Time {
	return t.timer.C
}

// Reset 重置定时器
// 注意：在 select 中使用时，应该在循环开始时调用 Reset
func (t *SafeTimer) Reset(d time.Duration) {
	// 先停止并清空通道
	if !t.timer.Stop() {
		// 如果定时器已经触发，需要排空通道
		select {
		case <-t.timer.C:
		default:
		}
	}
	t.timer.Reset(d)
}

// Stop 停止定时器
func (t *SafeTimer) Stop() {
	if !t.timer.Stop() {
		select {
		case <-t.timer.C:
		default:
		}
	}
}

// SafeAfter 安全的 time.After 替代品，用于非循环场景
// 返回定时器和通道，调用者必须在不再需要时调用 Stop
func SafeAfter(d time.Duration) (*time.Timer, <-chan time.Time) {
	timer := time.NewTimer(d)
	return timer, timer.C
}

// StopTimer 安全停止定时器
func StopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
