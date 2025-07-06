package constants

import "time"

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

	MB                             = 1024 * 1024
	GB                             = 1024 * 1024 * 1024
	DefaultClientBandwidthLimit    = 10 * MB // 10MB/s
	DefaultAnonymousBandwidthLimit = 5 * MB  // 5MB/s

	DefaultClientMaxConnections    = 10
	DefaultAnonymousMaxConnections = 5
)

var (
	// 允许的端口
	DefaultAllowedPorts = []int{80, 443, 8080, 3000, 5000}

	// 禁止的端口
	DefaultBlockedPorts = []int{22, 23, 25}
)

const (
	// 心跳和超时
	DefaultHeartbeatInterval = 30 // 秒

	// 重试次数
	DefaultMaxAttempts = 10
)

// 连接相关常量
const (
	DefaultAutoReconnect     = true
	DefaultEnableCompression = true
)

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
	ErrMsgEntityAlreadyExists   = "entity already exists"
	ErrMsgEntityDoesNotExist    = "entity does not exist"
	ErrMsgInvalidRequest        = "invalid request"
	ErrMsgInternalError         = "internal server error"
	ErrMsgStorageError          = "storage operation failed"
	ErrMsgLockAcquisitionFailed = "failed to acquire lock"
	ErrMsgCleanupTaskFailed     = "cleanup task failed"
	ErrMsgConfigUpdateFailed    = "configuration update failed"
)
