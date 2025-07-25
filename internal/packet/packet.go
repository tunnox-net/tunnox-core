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
	// ==================== 连接管理类命令 ====================
	Connect      CommandType = 1  // 建立连接
	Disconnect   CommandType = 8  // 连接断开，可以任何方向
	Reconnect    CommandType = 10 // 重新连接
	HeartbeatCmd CommandType = 11 // 心跳保活

	// ==================== 端口映射类命令 ====================
	TcpMapCreate CommandType = 2  // 创建TCP端口映射
	TcpMapDelete CommandType = 12 // 删除TCP端口映射
	TcpMapUpdate CommandType = 13 // 更新TCP端口映射
	TcpMapList   CommandType = 14 // 列出TCP端口映射
	TcpMapStatus CommandType = 15 // 获取TCP端口映射状态

	HttpMapCreate CommandType = 3  // 创建HTTP端口映射
	HttpMapDelete CommandType = 16 // 删除HTTP端口映射
	HttpMapUpdate CommandType = 17 // 更新HTTP端口映射
	HttpMapList   CommandType = 18 // 列出HTTP端口映射
	HttpMapStatus CommandType = 19 // 获取HTTP端口映射状态

	SocksMapCreate CommandType = 4  // 创建SOCKS代理映射
	SocksMapDelete CommandType = 20 // 删除SOCKS代理映射
	SocksMapUpdate CommandType = 21 // 更新SOCKS代理映射
	SocksMapList   CommandType = 22 // 列出SOCKS代理映射
	SocksMapStatus CommandType = 23 // 获取SOCKS代理映射状态

	// ==================== 数据传输类命令 ====================
	DataTransferStart  CommandType = 5  // 开始数据传输
	DataTransferStop   CommandType = 24 // 停止数据传输
	DataTransferStatus CommandType = 25 // 获取数据传输状态
	ProxyForward       CommandType = 6  // 代理转发数据

	// ==================== 系统管理类命令 ====================
	ConfigGet   CommandType = 26 // 获取配置信息
	ConfigSet   CommandType = 27 // 设置配置信息
	StatsGet    CommandType = 28 // 获取统计信息
	LogGet      CommandType = 29 // 获取日志信息
	HealthCheck CommandType = 30 // 健康检查

	// ==================== RPC类命令 ====================
	RpcInvoke     CommandType = 9  // RPC调用
	RpcRegister   CommandType = 31 // 注册RPC服务
	RpcUnregister CommandType = 32 // 注销RPC服务
	RpcList       CommandType = 33 // 列出RPC服务

	// ==================== 兼容性命令（保留原有ID） ====================
	TcpMap   CommandType = 2 // 兼容性：TCP端口映射
	HttpMap  CommandType = 3 // 兼容性：HTTP端口映射
	SocksMap CommandType = 4 // 兼容性：SOCKS代理映射
	DataIn   CommandType = 5 // 兼容性：数据输入通知
	Forward  CommandType = 6 // 兼容性：服务端间转发
	DataOut  CommandType = 7 // 兼容性：数据输出通知
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
