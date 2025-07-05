package cloud

import "time"

// 键值前缀常量，用于标准化Repository的键值命名空间
const (
	// 基础前缀
	KeyPrefixTunnox = "tunnox"

	// 用户相关键值前缀
	KeyPrefixUser        = "tunnox:user"
	KeyPrefixUserList    = "tunnox:users:list"
	KeyPrefixUserClients = "tunnox:user_clients"

	// 客户端相关键值前缀
	KeyPrefixClient = "tunnox:client"

	// 端口映射相关键值前缀
	KeyPrefixPortMapping    = "tunnox:port_mapping"
	KeyPrefixUserMappings   = "tunnox:user_mappings"
	KeyPrefixClientMappings = "tunnox:client_mappings"

	// 节点相关键值前缀
	KeyPrefixNode     = "tunnox:node"
	KeyPrefixNodeList = "tunnox:nodes:list"

	// 统计相关键值前缀
	KeyPrefixStats      = "tunnox:stats"
	KeyPrefixTraffic    = "tunnox:traffic"
	KeyPrefixConnection = "tunnox:connection"

	// 认证相关键值前缀
	KeyPrefixAuth  = "tunnox:auth"
	KeyPrefixToken = "tunnox:token"
)

// 时间相关常量
const (
	DefaultCleanupInterval = 5 * time.Minute
	DefaultDataTTL         = 24 * time.Hour
	DefaultUserDataTTL     = 0 // 用户数据不过期
	DefaultClientDataTTL   = 24 * time.Hour
	DefaultMappingDataTTL  = 24 * time.Hour
	DefaultNodeDataTTL     = 24 * time.Hour
	DefaultConnectionTTL   = 24 * time.Hour
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
	DefaultHeartbeatInterval = 30 // 秒
	DefaultMappingTimeout    = 30 // 秒
	DefaultMappingRetryCount = 3
	DefaultMaxAttempts       = 10
)

// 连接相关常量
const (
	DefaultAutoReconnect     = true
	DefaultEnableCompression = true
)
