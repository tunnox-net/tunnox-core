package dispose

// 使用延迟导入避免循环依赖
// dispose包的日志将通过主log系统输出
var (
	logFunc func(level string, format string, args ...interface{})
)

// SetLogger 设置日志函数（由主程序初始化时调用）
func SetLogger(fn func(level string, format string, args ...interface{})) {
	logFunc = fn
}

func log(level string, format string, args ...interface{}) {
	if logFunc != nil {
		logFunc(level, format, args...)
	}
	// 如果logFunc未设置，静默忽略（避免输出到stderr）
}

// Debugf 调试日志
func Debugf(format string, args ...interface{}) {
	log("debug", format, args...)
}

// Infof 信息日志
func Infof(format string, args ...interface{}) {
	log("info", format, args...)
}

// Warnf 警告日志
func Warnf(format string, args ...interface{}) {
	log("warn", format, args...)
}

// Errorf 错误日志
func Errorf(format string, args ...interface{}) {
	log("error", format, args...)
}

// Warn 警告消息
func Warn(msg string) {
	log("warn", "%s", msg)
}

// Error 错误消息
func Error(msg string) {
	log("error", "%s", msg)
}

// Info 信息消息
func Info(msg string) {
	log("info", "%s", msg)
}

// Debug 调试消息
func Debug(msg string) {
	log("debug", "%s", msg)
}
