package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"tunnox-core/internal/constants"
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

		// 关闭之前的日志文件（如果存在，忽略关闭错误）
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
		// 没有配置文件地址，不输出日志（忽略关闭错误）
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
