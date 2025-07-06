package cloud

import "time"

// 说明：本文件为云控相关常量，通用常量请放在internal/constants/constants.go，避免重复。

// 仅保留cloud专有常量
// 时间相关常量
const (
	// 清理间隔
	DefaultCleanupInterval = 5 * time.Minute

	// 数据过期时间
	DefaultDataTTL        = 24 * time.Hour
	DefaultUserDataTTL    = 0 // 用户数据不过期
	DefaultClientDataTTL  = 24 * time.Hour
	DefaultMappingDataTTL = 24 * time.Hour
	DefaultNodeDataTTL    = 24 * time.Hour
	DefaultConnectionTTL  = 24 * time.Hour

	// JWT相关时间
	DefaultJWTExpiration     = 24 * time.Hour
	DefaultRefreshExpiration = 7 * 24 * time.Hour

	// 锁超时时间
	DefaultLockTimeout        = 30 * time.Second
	DefaultCleanupLockTimeout = 5 * time.Minute
)

// 大小相关常量
const (
	MB = 1024 * 1024
	GB = 1024 * 1024 * 1024

	// 带宽限制
	DefaultUserBandwidthLimit      = 100 * MB // 100MB/s
	DefaultClientBandwidthLimit    = 10 * MB  // 10MB/s
	DefaultAnonymousBandwidthLimit = 5 * MB   // 5MB/s
	DefaultMappingBandwidthLimit   = 5 * MB   // 5MB/s

	// 存储限制
	DefaultUserStorageLimit = 1 * GB // 1GB

	// 连接数限制
	DefaultUserMaxConnections      = 100
	DefaultClientMaxConnections    = 10
	DefaultAnonymousMaxConnections = 5
	DefaultUserMaxClientIds        = 10
)

// 端口相关常量
var (
	// 允许的端口
	DefaultAllowedPorts          = []int{80, 443, 8080, 3000, 5000}
	DefaultAnonymousAllowedPorts = []int{80, 443, 8080}

	// 禁止的端口
	DefaultBlockedPorts = []int{22, 23, 25}
)

// 配置相关常量
const (
	// 心跳和超时
	DefaultHeartbeatInterval = 30 // 秒
	DefaultMappingTimeout    = 30 // 秒
	DefaultMappingRetryCount = 3

	// 重试次数
	DefaultMaxAttempts = 10

	// 配置监听间隔
	DefaultConfigWatchInterval = 30 * time.Second
)

// 连接相关常量
const (
	DefaultAutoReconnect     = true
	DefaultEnableCompression = true
)

// 错误消息常量
const (
	ErrMsgAuthenticationFailed  = "authentication failed"
	ErrMsgNodeNotFound          = "node not found"
	ErrMsgClientNotFound        = "client not found"
	ErrMsgUserNotFound          = "user not found"
	ErrMsgMappingNotFound       = "port mapping not found"
	ErrMsgConnectionNotFound    = "connection not found"
	ErrMsgInvalidAuthCode       = "invalid auth code"
	ErrMsgInvalidSecretKey      = "invalid secret key"
	ErrMsgClientBlocked         = "client is blocked"
	ErrMsgTokenInvalid          = "invalid token"
	ErrMsgTokenExpired          = "token expired"
	ErrMsgTokenRevoked          = "token has been revoked"
	ErrMsgIDExhausted           = "failed to generate unique ID after maximum attempts"
	ErrMsgEntityAlreadyExists   = "entity already exists"
	ErrMsgEntityDoesNotExist    = "entity does not exist"
	ErrMsgInvalidRequest        = "invalid request"
	ErrMsgInternalError         = "internal server error"
	ErrMsgStorageError          = "storage operation failed"
	ErrMsgLockAcquisitionFailed = "failed to acquire lock"
	ErrMsgCleanupTaskFailed     = "cleanup task failed"
	ErrMsgConfigUpdateFailed    = "configuration update failed"
)

// 成功消息常量（已移除未使用的SuccessMsg*常量）

// 日志相关常量
// const (
// 	LogLevelDebug   = "debug"
// 	LogLevelInfo    = "info"
// 	LogLevelWarning = "warning"
// 	LogLevelError   = "error"
// 	LogLevelFatal   = "fatal"
// )

// 操作类型常量（已移除未使用的Operation*常量）
