package constants

// 日志级别常量
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
	LogLevelFatal = "fatal"
	LogLevelPanic = "panic"
)

// 日志消息常量
const (
	// 服务相关
	LogMsgServerStarting     = "Server starting on %s"
	LogMsgServerStarted      = "Server started successfully"
	LogMsgServerShuttingDown = "Server shutting down"
	LogMsgServerShutdown     = "Server shutdown completed"

	// HTTP相关
	LogMsgHTTPRequestReceived  = "HTTP request received: %s %s"
	LogMsgHTTPRequestCompleted = "HTTP request completed: %s %s - %d"
	LogMsgHTTPRequestFailed    = "HTTP request failed: %s %s - %v"

	// 云控相关
	LogMsgCloudControlInitialized = "Cloud control initialized"
	LogMsgNodeRegistered          = "Node registered: %s"
	LogMsgNodeUnregistered        = "Node unregistered: %s"
	LogMsgClientAuthenticated     = "Client authenticated: %s"
	LogMsgUserCreated             = "User created: %s"
	LogMsgClientCreated           = "Client created: %s"
	LogMsgMappingCreated          = "Port mapping created: %s"

	// 错误相关
	LogMsgErrorInternalServer   = "Internal server error: %v"
	LogMsgErrorInvalidRequest   = "Invalid request: %v"
	LogMsgErrorNotFound         = "Resource not found: %s"
	LogMsgErrorUnauthorized     = "Unauthorized access: %s"
	LogMsgErrorForbidden        = "Forbidden access: %s"
	LogMsgErrorValidationFailed = "Validation failed: %v"

	// 数据库/存储相关
	LogMsgStorageOperationFailed = "Storage operation failed: %v"
	LogMsgDatabaseConnectionLost = "Database connection lost"
	LogMsgDatabaseReconnected    = "Database reconnected"

	// 配置相关
	LogMsgConfigLoaded   = "Configuration loaded successfully"
	LogMsgConfigInvalid  = "Invalid configuration: %v"
	LogMsgConfigReloaded = "Configuration reloaded"

	// 性能相关
	LogMsgPerformanceSlow     = "Slow operation detected: %s took %v"
	LogMsgPerformanceHighLoad = "High load detected: %d concurrent requests"
	LogMsgMemoryUsage         = "Memory usage: %d MB"
	LogMsgGoroutineCount      = "Goroutine count: %d"
)

// 日志字段名常量
const (
	LogFieldRequestID    = "request_id"
	LogFieldUserID       = "user_id"
	LogFieldClientID     = "client_id"
	LogFieldNodeID       = "node_id"
	LogFieldMappingID    = "mapping_id"
	LogFieldIPAddress    = "ip_address"
	LogFieldUserAgent    = "user_agent"
	LogFieldStatusCode   = "status_code"
	LogFieldResponseTime = "response_time"
	LogFieldError        = "error"
	LogFieldOperation    = "operation"
	LogFieldDuration     = "duration"
	LogFieldSize         = "size"
	LogFieldMethod       = "method"
	LogFieldPath         = "path"
	LogFieldVersion      = "version"
)

// 日志格式常量
const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

// 日志输出常量
const (
	LogOutputStdout = "stdout"
	LogOutputStderr = "stderr"
	LogOutputFile   = "file"
)
