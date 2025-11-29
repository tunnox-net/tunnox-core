package utils

import (
	"context"
	"io"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tunnox-core/internal/constants"

	"github.com/sirupsen/logrus"
)

// Logger 全局日志实例
var Logger *logrus.Logger

// currentLogFile 当前日志文件句柄（用于正确关闭）
var currentLogFile *os.File

// 初始化日志系统
func init() {
	Logger = logrus.New()

	// 设置默认格式为文本格式
	Logger.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: time.RFC3339,
		FullTimestamp:   true,
	})

	// 默认不输出到console，等待InitLogger配置
	// 如果没有配置文件，日志将输出到 io.Discard（不显示）
	Logger.SetOutput(io.Discard)

	// 设置默认级别为info
	Logger.SetLevel(logrus.InfoLevel)
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `json:"level" yaml:"level"`
	Format string `json:"format" yaml:"format"`
	Output string `json:"output" yaml:"output"`
	File   string `json:"file" yaml:"file"`
}

// InitLogger 初始化日志系统
func InitLogger(config *LogConfig) error {
	if config == nil {
		return nil
	}

	// 设置日志级别
	if config.Level != "" {
		level, err := logrus.ParseLevel(config.Level)
		if err != nil {
			return fmt.Errorf("invalid log level: %s", config.Level)
		}
		Logger.SetLevel(level)
	}

	// 设置日志格式
	if config.Format == constants.LogFormatJSON {
		Logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	} else {
		// 默认使用文本格式
		Logger.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: time.RFC3339,
			FullTimestamp:   true,
		})
	}

	// 设置日志输出 - 默认只输出到文件，不输出到console
	// 如果有配置文件地址就写文件，否则不输出（/dev/null）
	if config.File != "" {
		// 展开路径（支持 ~ 和相对路径）
		expandedPath, err := ExpandPath(config.File)
		if err != nil {
			return fmt.Errorf("failed to expand log file path %q: %w", config.File, err)
		}

		// 确保日志目录存在
		logDir := filepath.Dir(expandedPath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory %q: %w", logDir, err)
		}

		// 关闭之前的日志文件（如果存在）
		if currentLogFile != nil {
			_ = currentLogFile.Close()
			currentLogFile = nil
		}

		file, err := os.OpenFile(expandedPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open log file %q: %w", expandedPath, err)
		}
		currentLogFile = file
		Logger.SetOutput(file)
	} else {
		// 没有配置文件地址，不输出日志（输出到 io.Discard）
		// 关闭之前的日志文件（如果存在）
		if currentLogFile != nil {
			_ = currentLogFile.Close()
			currentLogFile = nil
		}
		Logger.SetOutput(io.Discard)
	}

	return nil
}

// LogEntry 日志条目，用于添加上下文信息
type LogEntry struct {
	*logrus.Entry
}

// WithContext 创建带上下文的日志条目
func WithContext(ctx context.Context) *LogEntry {
	entry := Logger.WithContext(ctx)
	return &LogEntry{entry}
}

// WithField 添加字段到日志条目
func (l *LogEntry) WithField(key string, value interface{}) *LogEntry {
	return &LogEntry{l.Entry.WithField(key, value)}
}

// WithFields 添加多个字段到日志条目
func (l *LogEntry) WithFields(fields logrus.Fields) *LogEntry {
	return &LogEntry{l.Entry.WithFields(fields)}
}

// WithRequest 添加请求信息到日志条目
func (l *LogEntry) WithRequest(method, path, ip, userAgent string) *LogEntry {
	return l.WithFields(logrus.Fields{
		constants.LogFieldMethod:    method,
		constants.LogFieldPath:      path,
		constants.LogFieldIPAddress: ip,
		constants.LogFieldUserAgent: userAgent,
	})
}

// WithUser 添加用户信息到日志条目
func (l *LogEntry) WithUser(userID string) *LogEntry {
	return l.WithField(constants.LogFieldUserID, userID)
}

// WithClient 添加客户端信息到日志条目
func (l *LogEntry) WithClient(clientID string) *LogEntry {
	return l.WithField(constants.LogFieldClientID, clientID)
}

// WithNode 添加节点信息到日志条目
func (l *LogEntry) WithNode(nodeID string) *LogEntry {
	return l.WithField(constants.LogFieldNodeID, nodeID)
}

// WithMapping 添加映射信息到日志条目
func (l *LogEntry) WithMapping(mappingID string) *LogEntry {
	return l.WithField(constants.LogFieldMappingID, mappingID)
}

// WithError 添加错误信息到日志条目
func (l *LogEntry) WithError(err error) *LogEntry {
	return l.WithField(constants.LogFieldError, err)
}

// WithDuration 添加耗时信息到日志条目
func (l *LogEntry) WithDuration(duration time.Duration) *LogEntry {
	return l.WithField(constants.LogFieldDuration, duration)
}

// WithSize 添加大小信息到日志条目
func (l *LogEntry) WithSize(size int64) *LogEntry {
	return l.WithField(constants.LogFieldSize, size)
}

// Debug 记录调试日志
func (l *LogEntry) Debug(args ...interface{}) {
	l.Entry.Debug(args...)
}

// Info 记录信息日志
func (l *LogEntry) Info(args ...interface{}) {
	l.Entry.Info(args...)
}

// Warn 记录警告日志
func (l *LogEntry) Warn(args ...interface{}) {
	l.Entry.Warn(args...)
}

// Error 记录错误日志
func (l *LogEntry) Error(args ...interface{}) {
	l.Entry.Error(args...)
}

// Fatal 记录致命错误日志并退出
func (l *LogEntry) Fatal(args ...interface{}) {
	l.Entry.Fatal(args...)
}

// Debugf 记录格式化调试日志
func (l *LogEntry) Debugf(format string, args ...interface{}) {
	l.Entry.Debugf(format, args...)
}

// Infof 记录格式化信息日志
func (l *LogEntry) Infof(format string, args ...interface{}) {
	l.Entry.Infof(format, args...)
}

// Warnf 记录格式化警告日志
func (l *LogEntry) Warnf(format string, args ...interface{}) {
	l.Entry.Warnf(format, args...)
}

// Errorf 记录格式化错误日志
func (l *LogEntry) Errorf(format string, args ...interface{}) {
	l.Entry.Errorf(format, args...)
}

// Fatalf 记录格式化致命错误日志并退出
func (l *LogEntry) Fatalf(format string, args ...interface{}) {
	l.Entry.Fatalf(format, args...)
}

// 便捷的全局日志方法
func Debug(args ...interface{}) {
	Logger.Debug(args...)
}

func Info(args ...interface{}) {
	Logger.Info(args...)
}

func Warn(args ...interface{}) {
	Logger.Warn(args...)
}

func Error(args ...interface{}) {
	Logger.Error(args...)
}

func Fatal(args ...interface{}) {
	Logger.Fatal(args...)
}

func Debugf(format string, args ...interface{}) {
	Logger.Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	Logger.Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	Logger.Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	Logger.Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	Logger.Fatalf(format, args...)
}

// 结构化日志记录函数
// LogOperation 记录操作日志
func LogOperation(operation, entityType, entityID string, success bool, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation":  operation,
		"entityType": entityType,
		"entityID":   entityID,
		"success":    success,
	})

	if err != nil {
		entry.WithError(err).Errorf("Operation %s failed for %s %s", operation, entityType, entityID)
	} else {
		entry.Infof("Operation %s completed successfully for %s %s", operation, entityType, entityID)
	}
}

// LogAuthentication 记录认证日志
func LogAuthentication(userID, clientID string, success bool, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation": "authentication",
		"userID":    userID,
		"clientID":  clientID,
		"success":   success,
	})

	if err != nil {
		entry.WithError(err).Errorf("Authentication failed for user %s, client %s", userID, clientID)
	} else {
		entry.Infof("Authentication successful for user %s, client %s", userID, clientID)
	}
}

// LogStorageOperation 记录存储操作日志
func LogStorageOperation(operation, key string, success bool, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation": "storage",
		"storageOp": operation,
		"key":       key,
		"success":   success,
	})

	if err != nil {
		entry.WithError(err).Errorf("Storage operation %s failed for key %s", operation, key)
	} else {
		entry.Debugf("Storage operation %s completed for key %s", operation, key)
	}
}

// LogSystemEvent 记录系统事件日志
func LogSystemEvent(event, component string, details map[string]interface{}) {
	fields := logrus.Fields{
		"event":     event,
		"component": component,
	}

	for k, v := range details {
		fields[k] = v
	}

	Logger.WithFields(fields).Infof("System event: %s in component %s", event, component)
}

// LogErrorWithContext 记录带上下文的错误日志
func LogErrorWithContext(err error, context string, fields map[string]interface{}) {
	entry := Logger.WithError(err).WithField("context", context)

	if fields != nil {
		entry = entry.WithFields(logrus.Fields(fields))
	}

	entry.Error("Error occurred")
}

// LogPanic 记录panic日志
func LogPanic(recover interface{}, stack []byte) {
	Logger.WithFields(logrus.Fields{
		"panic": recover,
		"stack": string(stack),
	}).Fatal("Panic occurred")
}

// LogRequest 记录HTTP请求日志
func LogRequest(method, path, ip, userAgent string, statusCode int, duration time.Duration) {
	level := logrus.InfoLevel
	if statusCode >= 400 {
		level = logrus.WarnLevel
	}
	if statusCode >= 500 {
		level = logrus.ErrorLevel
	}

	entry := Logger.WithFields(logrus.Fields{
		"method":     method,
		"path":       path,
		"ip":         ip,
		"userAgent":  userAgent,
		"statusCode": statusCode,
		"duration":   duration,
	})

	switch level {
	case logrus.InfoLevel:
		entry.Info("HTTP request completed")
	case logrus.WarnLevel:
		entry.Warn("HTTP request completed with client error")
	case logrus.ErrorLevel:
		entry.Error("HTTP request completed with server error")
	}
}

// LogHeartbeat 记录心跳日志
func LogHeartbeat(nodeID string, success bool, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation": "heartbeat",
		"nodeID":    nodeID,
		"success":   success,
	})

	if err != nil {
		entry.WithError(err).Errorf("Heartbeat failed for node %s", nodeID)
	} else {
		entry.Debugf("Heartbeat received from node %s", nodeID)
	}
}

// LogConnection 记录连接日志
func LogConnection(connectionType, entityID string, connected bool, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation":      "connection",
		"connectionType": connectionType,
		"entityID":       entityID,
		"connected":      connected,
	})

	if err != nil {
		entry.WithError(err).Errorf("Connection %s failed for %s %s",
			map[bool]string{true: "establishment", false: "termination"}[connected],
			connectionType, entityID)
	} else {
		entry.Infof("Connection %s for %s %s",
			map[bool]string{true: "established", false: "terminated"}[connected],
			connectionType, entityID)
	}
}

// LogCleanup 记录清理操作日志
func LogCleanup(component string, itemsCleaned int, duration time.Duration, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation":    "cleanup",
		"component":    component,
		"itemsCleaned": itemsCleaned,
		"duration":     duration,
	})

	if err != nil {
		entry.WithError(err).Errorf("Cleanup failed for component %s", component)
	} else {
		entry.Infof("Cleanup completed for component %s, cleaned %d items in %v",
			component, itemsCleaned, duration)
	}
}
