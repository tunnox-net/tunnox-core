package log

import (
	"os"
)

// ============================================================================
// 全局便捷函数（向后兼容，内部调用 Default()）
// ============================================================================

// Debug 记录调试日志
func Debug(args ...interface{}) {
	Default().Debug(args...)
}

// Info 记录信息日志
func Info(args ...interface{}) {
	Default().Info(args...)
}

// Warn 记录警告日志
func Warn(args ...interface{}) {
	Default().Warn(args...)
}

// Error 记录错误日志
func Error(args ...interface{}) {
	Default().Error(args...)
}

// Debugf 记录格式化调试日志
func Debugf(format string, args ...interface{}) {
	Default().Debugf(format, args...)
}

// Infof 记录格式化信息日志
func Infof(format string, args ...interface{}) {
	Default().Infof(format, args...)
}

// Warnf 记录格式化警告日志
func Warnf(format string, args ...interface{}) {
	Default().Warnf(format, args...)
}

// Errorf 记录格式化错误日志
func Errorf(format string, args ...interface{}) {
	Default().Errorf(format, args...)
}

// Fatal 记录致命错误日志并退出
func Fatal(args ...interface{}) {
	Default().Error(args...)
	os.Exit(1)
}

// Fatalf 记录格式化致命错误日志并退出
func Fatalf(format string, args ...interface{}) {
	Default().Errorf(format, args...)
	os.Exit(1)
}

// WithField 创建带字段的日志
func WithField(key string, value interface{}) Logger {
	return Default().WithField(key, value)
}

// WithFields 创建带多个字段的日志
func WithFields(fields map[string]interface{}) Logger {
	return Default().WithFields(fields)
}

// WithError 创建带错误的日志
func WithError(err error) Logger {
	return Default().WithError(err)
}
