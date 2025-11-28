package models

import "time"

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码生成器配置
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ConnectionCodeGenerator 连接码生成器配置
type ConnectionCodeGenerator struct {
	// 格式配置
	SegmentLength int    // 每段长度（默认3）
	SegmentCount  int    // 段数（默认3）
	Separator     string // 分隔符（默认"-"）
	Charset       string // 字符集（默认"0-9a-z"，排除易混淆字符）
}

// DefaultConnectionCodeGenerator 默认生成器配置
func DefaultConnectionCodeGenerator() *ConnectionCodeGenerator {
	return &ConnectionCodeGenerator{
		SegmentLength: 3,
		SegmentCount:  3,
		Separator:     "-",
		Charset:       "0123456789abcdefghjkmnpqrstuvwxyz", // 排除 i, l, o（易混淆）
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 时间周期工具
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Duration 时间周期类型（用于预设的常用时长）
type Duration string

const (
	Duration10Min  Duration = "10m"    // 10分钟（常用于连接码激活期）
	Duration1Hour  Duration = "1h"     // 1小时
	Duration1Day   Duration = "1d"     // 1天
	Duration1Week  Duration = "1w"     // 1周
	Duration10Day  Duration = "10d"    // 10天（示例中的访问期）
	Duration1Month Duration = "1month" // 1月
)

// ToDuration 转换为time.Duration
func (d Duration) ToDuration() time.Duration {
	switch d {
	case Duration10Min:
		return 10 * time.Minute
	case Duration1Hour:
		return 1 * time.Hour
	case Duration1Day:
		return 24 * time.Hour
	case Duration1Week:
		return 7 * 24 * time.Hour
	case Duration10Day:
		return 10 * 24 * time.Hour
	case Duration1Month:
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour // 默认1天
	}
}
