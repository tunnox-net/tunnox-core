package dispose

import (
	"log"
	"os"
)

// 简单的日志实现，避免循环引用
var (
	debugLogger = log.New(os.Stdout, "[DEBUG] ", log.LstdFlags)
	infoLogger  = log.New(os.Stdout, "[INFO] ", log.LstdFlags)
	warnLogger  = log.New(os.Stderr, "[WARN] ", log.LstdFlags)
	errorLogger = log.New(os.Stderr, "[ERROR] ", log.LstdFlags)
)

// Debugf 调试日志
func Debugf(format string, args ...interface{}) {
	debugLogger.Printf(format, args...)
}

// Infof 信息日志
func Infof(format string, args ...interface{}) {
	infoLogger.Printf(format, args...)
}

// Warnf 警告日志
func Warnf(format string, args ...interface{}) {
	warnLogger.Printf(format, args...)
}

// Errorf 错误日志
func Errorf(format string, args ...interface{}) {
	errorLogger.Printf(format, args...)
}

// Warn 警告消息
func Warn(msg string) {
	warnLogger.Println(msg)
}
