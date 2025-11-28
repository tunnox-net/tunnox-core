package constants

// Storage Key Prefixes
// 存储键前缀定义，统一管理所有存储键的命名规范

// ============================================================================
// 持久化数据键前缀（Persistent Data Keys）
// 存储：数据库 + 缓存（HybridStorage自动处理）
// 特点：慢变化，需要持久保存
// ============================================================================

const (
	// KeyPrefixPersist 持久化数据根前缀
	KeyPrefixPersist = "tunnox:persist:"

	// KeyPrefixPersistUser 用户信息
	KeyPrefixPersistUser = "tunnox:persist:user:"

	// KeyPrefixPersistClientConfig 客户端持久化配置
	// 格式：tunnox:persist:client:config:{client_id}
	KeyPrefixPersistClientConfig = "tunnox:persist:client:config:"

	// KeyPrefixPersistMapping 端口映射配置
	KeyPrefixPersistMapping = "tunnox:persist:mapping:"

	// KeyPrefixPersistNode 节点信息
	KeyPrefixPersistNode = "tunnox:persist:node:"

	// KeyPrefixPersistClientsList 所有客户端配置列表
	KeyPrefixPersistClientsList = "tunnox:persist:clients:list"

	// KeyPrefixPersistUsersList 所有用户列表
	KeyPrefixPersistUsersList = "tunnox:persist:users:list"
)

// ============================================================================
// 运行时数据键前缀（Runtime State Keys）
// 存储：仅缓存（Redis/Memory），带TTL自动过期
// 特点：快变化，服务重启丢失
// ============================================================================

const (
	// KeyPrefixRuntime 运行时数据根前缀
	KeyPrefixRuntime = "tunnox:runtime:"

	// KeyPrefixRuntimeClientState 客户端运行时状态
	// 格式：tunnox:runtime:client:state:{client_id}
	// TTL：90秒（心跳间隔30秒 * 3）
	KeyPrefixRuntimeClientState = "tunnox:runtime:client:state:"

	// KeyPrefixRuntimeClientToken 客户端JWT Token
	// 格式：tunnox:runtime:client:token:{client_id}
	// TTL：Token过期时间
	KeyPrefixRuntimeClientToken = "tunnox:runtime:client:token:"

	// KeyPrefixRuntimeNodeClients 节点的在线客户端列表
	// 格式：tunnox:runtime:node:clients:{node_id}
	// TTL：300秒
	KeyPrefixRuntimeNodeClients = "tunnox:runtime:node:clients:"

	// KeyPrefixRuntimeSession 会话数据
	// 格式：tunnox:runtime:session:{session_id}
	KeyPrefixRuntimeSession = "tunnox:runtime:session:"

	// ============ 连接码相关（运行时数据，短期有效） ============

	// KeyPrefixRuntimeConnectionCodeByCode 连接码（按Code查询）
	// 格式：tunnox:runtime:conncode:code:{code}
	// TTL：ActivationTTL（如10分钟）
	KeyPrefixRuntimeConnectionCodeByCode = "tunnox:runtime:conncode:code:"

	// KeyPrefixRuntimeConnectionCodeByID 连接码（按ID查询）
	// 格式：tunnox:runtime:conncode:id:{id}
	// TTL：ActivationTTL（如10分钟）
	KeyPrefixRuntimeConnectionCodeByID = "tunnox:runtime:conncode:id:"
)

// ============================================================================
// 临时数据键前缀（Temporary Data Keys）
// 存储：仅缓存，短期有效
// 特点：验证码、限流等临时数据
// ============================================================================

const (
	// KeyPrefixTemp 临时数据根前缀
	KeyPrefixTemp = "tunnox:temp:"

	// KeyPrefixTempVerifyCode 验证码
	// 格式：tunnox:temp:verify_code:{phone}
	// TTL：300秒
	KeyPrefixTempVerifyCode = "tunnox:temp:verify_code:"

	// KeyPrefixTempRateLimit 限流计数
	// 格式：tunnox:temp:rate_limit:{ip}:{endpoint}
	// TTL：60秒
	KeyPrefixTempRateLimit = "tunnox:temp:rate_limit:"

	// ============ 授权码相关（临时数据，用于激活） ============

	// KeyPrefixAuthCode 授权码（按Code查询）
	// 格式：tunnox:temp:authcode:code:{code}
	// TTL：ActivationTTL（如10分钟）
	KeyPrefixAuthCode = "tunnox:temp:authcode:code:"

	// KeyPrefixAuthCodeID 授权码（按ID查询）
	// 格式：tunnox:temp:authcode:id:{authcode_id}
	// TTL：ActivationTTL
	KeyPrefixAuthCodeID = "tunnox:temp:authcode:id:"

	// KeyPrefixAuthCodeTarget TargetClient的授权码列表
	// 格式：tunnox:temp:authcode:target:{target_client_id}
	// TTL：永久（成员自动过期清理）
	KeyPrefixAuthCodeTarget = "tunnox:temp:authcode:target:"
)

// ============================================================================
// 索引数据键前缀（Index Keys）
// 存储：缓存，用于加速查询
// 特点：可重建的索引数据
// ============================================================================

const (
	// KeyPrefixIndex 索引数据根前缀
	KeyPrefixIndex = "tunnox:index:"

	// KeyPrefixIndexUserClients 用户的客户端列表
	// 格式：tunnox:index:user:clients:{user_id}
	KeyPrefixIndexUserClients = "tunnox:index:user:clients:"

	// KeyPrefixIndexClientMappings 客户端的端口映射列表
	// 格式：tunnox:index:client:mappings:{client_id}
	KeyPrefixIndexClientMappings = "tunnox:index:client:mappings:"

	// ============ 连接码索引 ============

	// KeyPrefixIndexConnectionCodeByTarget TargetClient的连接码列表
	// 格式：tunnox:index:conncode:target:{target_client_id}
	// 用途：查询某个TargetClient生成的所有连接码
	KeyPrefixIndexConnectionCodeByTarget = "tunnox:index:conncode:target:"
)

// ============================================================================
// TTL常量定义
// ============================================================================

const (
	// TTLClientState 客户端状态TTL（90秒 = 心跳间隔30秒 * 3）
	TTLClientState = 90

	// TTLNodeClients 节点客户端列表TTL（5分钟）
	TTLNodeClients = 300

	// TTLVerifyCode 验证码TTL（5分钟）
	TTLVerifyCode = 300

	// TTLRateLimit 限流TTL（1分钟）
	TTLRateLimit = 60
)
