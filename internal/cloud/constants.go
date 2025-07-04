package cloud

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
