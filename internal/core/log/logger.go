// Package log 提供统一的日志接口和实现
// 支持依赖注入，便于测试时替换
package log

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger 日志接口
// 所有组件应通过此接口记录日志，而非直接使用全局函数
type Logger interface {
	// 基础日志方法
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})

	// 格式化日志方法
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	// 带字段的日志
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
	WithError(err error) Logger
	WithContext(ctx context.Context) Logger
}

// LogLevel 日志级别
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Config 日志配置
type Config struct {
	Level  string `json:"level" yaml:"level"`
	Format string `json:"format" yaml:"format"`
	Output string `json:"output" yaml:"output"`
	File   string `json:"file" yaml:"file"`
}

// ============================================================================
// logrusLogger - 基于 logrus 的 Logger 实现
// ============================================================================

type logrusLogger struct {
	entry *logrus.Entry
}

// NewLogrusLogger 创建基于 logrus 的 Logger
func NewLogrusLogger(l *logrus.Logger) Logger {
	return &logrusLogger{entry: logrus.NewEntry(l)}
}

func (l *logrusLogger) Debug(args ...interface{}) {
	l.entry.Debug(args...)
}

func (l *logrusLogger) Info(args ...interface{}) {
	l.entry.Info(args...)
}

func (l *logrusLogger) Warn(args ...interface{}) {
	l.entry.Warn(args...)
}

func (l *logrusLogger) Error(args ...interface{}) {
	l.entry.Error(args...)
}

func (l *logrusLogger) Debugf(format string, args ...interface{}) {
	l.entry.Debugf(format, args...)
}

func (l *logrusLogger) Infof(format string, args ...interface{}) {
	l.entry.Infof(format, args...)
}

func (l *logrusLogger) Warnf(format string, args ...interface{}) {
	l.entry.Warnf(format, args...)
}

func (l *logrusLogger) Errorf(format string, args ...interface{}) {
	l.entry.Errorf(format, args...)
}

func (l *logrusLogger) WithField(key string, value interface{}) Logger {
	return &logrusLogger{entry: l.entry.WithField(key, value)}
}

func (l *logrusLogger) WithFields(fields map[string]interface{}) Logger {
	return &logrusLogger{entry: l.entry.WithFields(logrus.Fields(fields))}
}

func (l *logrusLogger) WithError(err error) Logger {
	return &logrusLogger{entry: l.entry.WithError(err)}
}

func (l *logrusLogger) WithContext(ctx context.Context) Logger {
	return &logrusLogger{entry: l.entry.WithContext(ctx)}
}

// ============================================================================
// NopLogger - 静默日志（用于测试）
// ============================================================================

// NopLogger 静默日志，不输出任何内容
type NopLogger struct{}

func (NopLogger) Debug(args ...interface{})                         {}
func (NopLogger) Info(args ...interface{})                          {}
func (NopLogger) Warn(args ...interface{})                          {}
func (NopLogger) Error(args ...interface{})                         {}
func (NopLogger) Debugf(format string, args ...interface{})         {}
func (NopLogger) Infof(format string, args ...interface{})          {}
func (NopLogger) Warnf(format string, args ...interface{})          {}
func (NopLogger) Errorf(format string, args ...interface{})         {}
func (n NopLogger) WithField(key string, value interface{}) Logger  { return n }
func (n NopLogger) WithFields(fields map[string]interface{}) Logger { return n }
func (n NopLogger) WithError(err error) Logger                      { return n }
func (n NopLogger) WithContext(ctx context.Context) Logger          { return n }

// NewNopLogger 创建静默日志
func NewNopLogger() Logger {
	return NopLogger{}
}

// ============================================================================
// TestLogger - 测试日志（输出到 testing.T）
// ============================================================================

// TestingT 测试接口（兼容 *testing.T）
type TestingT interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
}

// TestLogger 测试日志，输出到 testing.T
type TestLogger struct {
	t      TestingT
	fields map[string]interface{}
}

// NewTestLogger 创建测试日志
func NewTestLogger(t TestingT) Logger {
	return &TestLogger{t: t, fields: make(map[string]interface{})}
}

func (l *TestLogger) Debug(args ...interface{}) {
	l.t.Log(append([]interface{}{"[DEBUG]"}, args...)...)
}

func (l *TestLogger) Info(args ...interface{}) {
	l.t.Log(append([]interface{}{"[INFO]"}, args...)...)
}

func (l *TestLogger) Warn(args ...interface{}) {
	l.t.Log(append([]interface{}{"[WARN]"}, args...)...)
}

func (l *TestLogger) Error(args ...interface{}) {
	l.t.Log(append([]interface{}{"[ERROR]"}, args...)...)
}

func (l *TestLogger) Debugf(format string, args ...interface{}) {
	l.t.Logf("[DEBUG] "+format, args...)
}

func (l *TestLogger) Infof(format string, args ...interface{}) {
	l.t.Logf("[INFO] "+format, args...)
}

func (l *TestLogger) Warnf(format string, args ...interface{}) {
	l.t.Logf("[WARN] "+format, args...)
}

func (l *TestLogger) Errorf(format string, args ...interface{}) {
	l.t.Logf("[ERROR] "+format, args...)
}

func (l *TestLogger) WithField(key string, value interface{}) Logger {
	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value
	return &TestLogger{t: l.t, fields: newFields}
}

func (l *TestLogger) WithFields(fields map[string]interface{}) Logger {
	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}
	return &TestLogger{t: l.t, fields: newFields}
}

func (l *TestLogger) WithError(err error) Logger {
	return l.WithField("error", err)
}

func (l *TestLogger) WithContext(ctx context.Context) Logger {
	return l
}

// ============================================================================
// 默认 Logger 管理
// ============================================================================

var (
	defaultLogger     Logger
	defaultLoggerOnce sync.Once
	defaultLoggerMu   sync.RWMutex
	currentLogFile    *os.File
)

// initDefaultLogger 初始化默认 Logger
func initDefaultLogger() {
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: time.RFC3339,
		FullTimestamp:   true,
	})
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.InfoLevel)
	defaultLogger = NewLogrusLogger(l)
}

// Default 获取默认 Logger
func Default() Logger {
	defaultLoggerOnce.Do(initDefaultLogger)
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// SetDefault 设置默认 Logger
func SetDefault(l Logger) {
	defaultLoggerOnce.Do(initDefaultLogger)
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = l
}

// SetDefaultFromLogrus 从 logrus.Logger 设置默认 Logger
func SetDefaultFromLogrus(l *logrus.Logger) {
	SetDefault(NewLogrusLogger(l))
}
