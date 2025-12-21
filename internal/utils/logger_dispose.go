package utils

import (
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

func init() {
	// 在包初始化时设置dispose的日志函数
	dispose.SetLogger(func(level string, format string, args ...interface{}) {
		switch level {
		case "debug":
			corelog.Debugf(format, args...)
		case "info":
			corelog.Infof(format, args...)
		case "warn":
			corelog.Warnf(format, args...)
		case "error":
			corelog.Errorf(format, args...)
		default:
			corelog.Infof(format, args...)
		}
	})
}
