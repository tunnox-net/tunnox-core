package utils

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"github.com/sirupsen/logrus"
)

// Logger 全局日志实例（用于初始化配置）
// 日志记录请使用 corelog 包：corelog.Infof(), corelog.Errorf() 等
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
	Logger.SetOutput(io.Discard)

	// 设置默认级别为info
	Logger.SetLevel(logrus.InfoLevel)

	// 同步设置 core/log 包的默认 Logger
	corelog.SetDefaultFromLogrus(Logger)
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

	// 设置日志输出
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

		// 根据 output 配置决定输出目标
		if config.Output == "both" {
			// 同时输出到文件和控制台
			Logger.SetOutput(io.MultiWriter(file, os.Stderr))
		} else {
			// 只输出到文件（CLI模式）
			Logger.SetOutput(file)
		}
	} else {
		// 没有配置文件地址，不输出日志
		if currentLogFile != nil {
			_ = currentLogFile.Close()
			currentLogFile = nil
		}
		Logger.SetOutput(io.Discard)
	}

	// 同步更新 core/log 包的默认 Logger
	corelog.SetDefaultFromLogrus(Logger)

	return nil
}

// ============================================================================
// LogEntry - 结构化日志条目（保留用于特殊场景）
// ============================================================================

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
	entry := l.WithField(constants.LogFieldError, err)
	if err != nil {
		errorCode := coreerrors.GetCode(err)
		entry = entry.WithField(constants.LogFieldErrorType, string(errorCode))
		entry = entry.WithField(constants.LogFieldRetryable, coreerrors.IsRetryable(err))
		entry = entry.WithField(constants.LogFieldAlertable, coreerrors.IsSystemError(err))
	}
	return entry
}

// WithDuration 添加耗时信息到日志条目
func (l *LogEntry) WithDuration(duration time.Duration) *LogEntry {
	return l.WithField(constants.LogFieldDuration, duration)
}

// WithSize 添加大小信息到日志条目
func (l *LogEntry) WithSize(size int64) *LogEntry {
	return l.WithField(constants.LogFieldSize, size)
}

// LogEntry 的日志方法
func (l *LogEntry) Debug(args ...interface{})                 { l.Entry.Debug(args...) }
func (l *LogEntry) Info(args ...interface{})                  { l.Entry.Info(args...) }
func (l *LogEntry) Warn(args ...interface{})                  { l.Entry.Warn(args...) }
func (l *LogEntry) Error(args ...interface{})                 { l.Entry.Error(args...) }
func (l *LogEntry) Fatal(args ...interface{})                 { l.Entry.Fatal(args...) }
func (l *LogEntry) Debugf(format string, args ...interface{}) { l.Entry.Debugf(format, args...) }
func (l *LogEntry) Infof(format string, args ...interface{})  { l.Entry.Infof(format, args...) }
func (l *LogEntry) Warnf(format string, args ...interface{})  { l.Entry.Warnf(format, args...) }
func (l *LogEntry) Errorf(format string, args ...interface{}) { l.Entry.Errorf(format, args...) }
func (l *LogEntry) Fatalf(format string, args ...interface{}) { l.Entry.Fatalf(format, args...) }

// ============================================================================
// 结构化日志记录函数（保留用于特殊场景）
// ============================================================================

// logErrorWithLevel 根据错误类型选择日志级别
func logErrorWithLevel(entry *logrus.Entry, err error, messages ...string) {
	if err == nil {
		return
	}

	errorCode := coreerrors.GetCode(err)
	isSystemError := coreerrors.IsSystemError(err)
	isRetryable := coreerrors.IsRetryable(err)

	message := ""
	if len(messages) > 0 {
		message = messages[0]
	} else {
		message = err.Error()
	}

	// 根据错误码选择日志级别
	switch errorCode {
	case coreerrors.CodeInternal, coreerrors.CodeStorageError:
		// 系统级错误，使用 Error 级别
		entry.Error(message)
	case coreerrors.CodeAuthFailed, coreerrors.CodeInvalidToken, coreerrors.CodeTokenExpired,
		coreerrors.CodeForbidden, coreerrors.CodeClientBlocked:
		// 认证/权限错误，使用 Error 级别
		entry.Error(message)
	case coreerrors.CodeNetworkError, coreerrors.CodeTimeout, coreerrors.CodeUnavailable,
		coreerrors.CodeRateLimited:
		// 网络/临时错误，使用 Warn 级别（可重试）
		entry.Warn(message)
	case coreerrors.CodeStreamClosed, coreerrors.CodeConnectionError:
		// 连接错误，使用 Warn 级别
		entry.Warn(message)
	default:
		if isSystemError {
			entry.Error(message)
		} else if isRetryable {
			entry.Warn(message)
		} else {
			entry.Error(message)
		}
	}
}

// LogOperation 记录操作日志
func LogOperation(operation, entityType, entityID string, success bool, err error) {
	entry := Logger.WithFields(logrus.Fields{
		"operation":  operation,
		"entityType": entityType,
		"entityID":   entityID,
		"success":    success,
	})

	if err != nil {
		logErrorWithLevel(entry.WithError(err), err, fmt.Sprintf("Operation %s failed for %s %s", operation, entityType, entityID))
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
		logErrorWithLevel(entry.WithError(err), err, fmt.Sprintf("Authentication failed for user %s, client %s", userID, clientID))
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
		logErrorWithLevel(entry.WithError(err), err, fmt.Sprintf("Storage operation %s failed for key %s", operation, key))
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

// LogError 记录错误日志
func LogError(err error, message string, fields map[string]interface{}) {
	entry := Logger.WithError(err)
	if message != "" {
		entry = entry.WithField("message", message)
	}
	if fields != nil {
		entry = entry.WithFields(logrus.Fields(fields))
	}
	logErrorWithLevel(entry, err)
}

// LogErrorf 格式化记录错误日志
func LogErrorf(err error, format string, args ...interface{}) {
	entry := Logger.WithError(err)
	logErrorWithLevel(entry, err, fmt.Sprintf(format, args...))
}

// LogErrorWithContext 记录带上下文的错误日志
func LogErrorWithContext(err error, context string, fields map[string]interface{}) {
	entry := Logger.WithError(err).WithField("context", context)
	if fields != nil {
		entry = entry.WithFields(logrus.Fields(fields))
	}
	logErrorWithLevel(entry, err)
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
