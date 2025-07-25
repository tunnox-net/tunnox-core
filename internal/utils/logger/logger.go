package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String 返回日志级别的字符串表示
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger 日志接口
type Logger interface {
	// Debug 调试日志
	Debug(format string, args ...interface{})

	// Info 信息日志
	Info(format string, args ...interface{})

	// Warn 警告日志
	Warn(format string, args ...interface{})

	// Error 错误日志
	Error(format string, args ...interface{})

	// Fatal 致命错误日志
	Fatal(format string, args ...interface{})

	// SetLevel 设置日志级别
	SetLevel(level LogLevel)

	// GetLevel 获取日志级别
	GetLevel() LogLevel
}

// DefaultLogger 默认日志实现
type DefaultLogger struct {
	level LogLevel
	file  *os.File
}

// NewDefaultLogger 创建新的默认日志器
func NewDefaultLogger(logFile string) (*DefaultLogger, error) {
	// 创建日志目录
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// 打开日志文件
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &DefaultLogger{
		level: INFO,
		file:  file,
	}, nil
}

// Debug 调试日志
func (l *DefaultLogger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.log(DEBUG, format, args...)
	}
}

// Info 信息日志
func (l *DefaultLogger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.log(INFO, format, args...)
	}
}

// Warn 警告日志
func (l *DefaultLogger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.log(WARN, format, args...)
	}
}

// Error 错误日志
func (l *DefaultLogger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.log(ERROR, format, args...)
	}
}

// Fatal 致命错误日志
func (l *DefaultLogger) Fatal(format string, args ...interface{}) {
	if l.level <= FATAL {
		l.log(FATAL, format, args...)
	}
	os.Exit(1)
}

// SetLevel 设置日志级别
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel 获取日志级别
func (l *DefaultLogger) GetLevel() LogLevel {
	return l.level
}

// log 内部日志方法
func (l *DefaultLogger) log(level LogLevel, format string, args ...interface{}) {
	// 获取调用者信息
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	}

	// 格式化日志消息
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("[%s] [%s] [%s:%d] %s\n", timestamp, level.String(), filepath.Base(file), line, message)

	// 写入文件
	if l.file != nil {
		l.file.WriteString(logLine)
	}

	// 同时输出到控制台
	fmt.Print(logLine)
}

// Close 关闭日志文件
func (l *DefaultLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
