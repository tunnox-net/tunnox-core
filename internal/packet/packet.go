package packet

type Type byte

const (
	JsonCommand Type = 1
	Compressed  Type = 2
	Encrypted   Type = 4
	Heartbeat   Type = 8
)

// IsHeartbeat 判断是否为心跳包
func (t Type) IsHeartbeat() bool {
	return t&Heartbeat != 0
}

// IsJsonCommand 判断是否为JsonCommand包
func (t Type) IsJsonCommand() bool {
	return t&JsonCommand != 0
}

// IsCompressed 判断是否压缩
func (t Type) IsCompressed() bool {
	return t&Compressed != 0
}

// IsEncrypted 判断是否加密
func (t Type) IsEncrypted() bool {
	return t&Encrypted != 0
}

type CommandType byte

const (
	// ==================== 连接管理类命令 (10-19) ====================
	Connect      CommandType = 10 // 建立连接
	Disconnect   CommandType = 11 // 连接断开，可以任何方向
	Reconnect    CommandType = 12 // 重新连接
	HeartbeatCmd CommandType = 13 // 心跳保活

	// ==================== 端口映射类命令 (20-39) ====================
	TcpMapCreate CommandType = 20 // 创建TCP端口映射
	TcpMapDelete CommandType = 21 // 删除TCP端口映射
	TcpMapUpdate CommandType = 22 // 更新TCP端口映射
	TcpMapList   CommandType = 23 // 列出TCP端口映射
	TcpMapStatus CommandType = 24 // 获取TCP端口映射状态

	HttpMapCreate CommandType = 25 // 创建HTTP端口映射
	HttpMapDelete CommandType = 26 // 删除HTTP端口映射
	HttpMapUpdate CommandType = 27 // 更新HTTP端口映射
	HttpMapList   CommandType = 28 // 列出HTTP端口映射
	HttpMapStatus CommandType = 29 // 获取HTTP端口映射状态

	SocksMapCreate CommandType = 30 // 创建SOCKS代理映射
	SocksMapDelete CommandType = 31 // 删除SOCKS代理映射
	SocksMapUpdate CommandType = 32 // 更新SOCKS代理映射
	SocksMapList   CommandType = 33 // 列出SOCKS代理映射
	SocksMapStatus CommandType = 34 // 获取SOCKS代理映射状态

	// ==================== 数据传输类命令 (40-49) ====================
	DataTransferStart  CommandType = 40 // 开始数据传输
	DataTransferStop   CommandType = 41 // 停止数据传输
	DataTransferStatus CommandType = 42 // 获取数据传输状态
	ProxyForward       CommandType = 43 // 代理转发数据
	DataTransferOut    CommandType = 44 // 数据传输输出通知

	// ==================== 系统管理类命令 (50-59) ====================
	ConfigGet   CommandType = 50 // 获取配置信息
	ConfigSet   CommandType = 51 // 设置配置信息
	StatsGet    CommandType = 52 // 获取统计信息
	LogGet      CommandType = 53 // 获取日志信息
	HealthCheck CommandType = 54 // 健康检查

	// ==================== RPC类命令 (60-69) ====================
	RpcInvoke     CommandType = 60 // RPC调用
	RpcRegister   CommandType = 61 // 注册RPC服务
	RpcUnregister CommandType = 62 // 注销RPC服务
	RpcList       CommandType = 63 // 列出RPC服务
)

// InitPacket 初始化数据包
type InitPacket struct {
	Version   string `json:"version"`
	ClientID  string `json:"client_id"`
	AuthCode  string `json:"auth_code"`
	SecretKey string `json:"secret_key"`
	NodeID    string `json:"node_id"`
	IPAddress string `json:"ip_address"`
	Type      string `json:"type"`
}

// AcceptPacket 接受数据包
type AcceptPacket struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ClientID  string `json:"client_id"`
	Token     string `json:"token,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type TransferPacket struct {
	PacketType    Type
	CommandPacket *CommandPacket
}

type CommandPacket struct {
	CommandType CommandType
	CommandId   string // 客户端生成的唯一命令ID
	Token       string
	SenderId    string
	ReceiverId  string
	CommandBody string
}
