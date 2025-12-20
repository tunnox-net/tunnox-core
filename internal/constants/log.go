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

// 日志字段名常量
const (
	LogFieldUserID    = "user_id"
	LogFieldClientID  = "client_id"
	LogFieldNodeID    = "node_id"
	LogFieldMappingID = "mapping_id"
	LogFieldIPAddress = "ip_address"
	LogFieldUserAgent = "user_agent"
	LogFieldError     = "error"
	LogFieldErrorType = "error_type" // 错误类型（temporary/permanent/protocol/network/storage/auth/fatal）
	LogFieldRetryable = "retryable"  // 是否可重试
	LogFieldAlertable = "alertable"  // 是否需要告警
	LogFieldDuration  = "duration"
	LogFieldSize      = "size"
	LogFieldMethod    = "method"
	LogFieldPath      = "path"
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

// 通用提示信息常量
const (
	MsgNoProtocolsEnabled             = "No protocols enabled in configuration"
	MsgFailedToUpgradeConnection      = "Failed to upgrade connection: %v"
	MsgWebSocketConnectionEstablished = "WebSocket connection established from %s"
	MsgWebSocketHandlerExited         = "WebSocket handler goroutine exited for %s (context done)"
	MsgWebSocketDefaultHandlerExited  = "WebSocket default handler goroutine exited for %s (context done)"
	MsgAdapterConfigured              = "%s adapter configured on %s"
	MsgRegisteredAdapters             = "Successfully registered %d protocol adapters: %v"
	MsgStartingServer                 = "Starting tunnox-core server..."
	MsgAllAdaptersStarted             = "All protocol adapters started successfully"
	MsgServerStarted                  = "Tunnox-core server started successfully"
	MsgShuttingDownServer             = "Shutting down tunnox-core server..."
	MsgAllProtocolManagerClosed       = "All protocol manager is closed"
	MsgClosingCloudControl            = "Closing cloud control..."
	MsgCloudControlClosed             = "Cloud control closed successfully"
	MsgServerShutdownCompleted        = "Tunnox-core server shutdown completed"
	MsgReceivedShutdownSignal         = "Received shutdown signal"
	MsgCleaningUpServerResources      = "Cleaning up server resources..."
	MsgServerShutdownMainExited       = "Server shutdown completed, main goroutine exiting"
	MsgConfigFileNotFound             = "Config file %s not found, using default configuration"
	MsgConfigLoadedFrom               = "Configuration loaded from %s"
	MsgFailedToReadConfigFile         = "failed to read config file %s: %v"
	MsgFailedToParseConfigFile        = "failed to parse config file %s: %v"
	MsgInvalidConfiguration           = "invalid configuration: %v"
)
