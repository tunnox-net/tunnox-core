package constants

// 网络相关常量
const (
	// DefaultChunkSize 默认分块大小，用于限速和读写操作
	DefaultChunkSize = 1024

	// DefaultBurstRatio 默认突发比例，用于限速器配置
	DefaultBurstRatio = 2

	// MinBurstSize 最小突发大小
	MinBurstSize = 1024
)

// 包相关常量
const (
	// PacketTypeSize 包类型字节大小
	PacketTypeSize = 1

	// PacketBodySizeBytes 包体大小字段字节数
	PacketBodySizeBytes = 4
)

// 键值前缀常量，用于标准化Repository的键值命名空间
const (

	// 用户相关键值前缀
	KeyPrefixUser        = "tunnox:user"
	KeyPrefixUserList    = "tunnox:users:list"
	KeyPrefixUserClients = "tunnox:user_clients"

	// 客户端相关键值前缀
	KeyPrefixClient     = "tunnox:client"
	KeyPrefixClientList = "tunnox:clients:list"

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

	// 连接管理相关键值前缀
	KeyPrefixMappingConnections = "tunnox:mapping_connections"
	KeyPrefixClientConnections  = "tunnox:client_connections"

	// 认证相关键值前缀
	KeyPrefixAuth  = "tunnox:auth"
	KeyPrefixToken = "tunnox:token"

	// ID管理相关键值前缀
	KeyPrefixID = "tunnox:id"

	// 配置管理相关键值前缀
	KeyPrefixConfig = "tunnox:config"

	// 清理管理相关键值前缀
	KeyPrefixCleanup = "tunnox:cleanup"
)
