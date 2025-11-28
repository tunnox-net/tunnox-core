package packet

type Type byte

const (
	// 控制类数据包（需要解析）
	Handshake     Type = 0x01 // 握手认证
	HandshakeResp Type = 0x02 // 握手响应
	Heartbeat     Type = 0x03 // 心跳
	JsonCommand   Type = 0x10 // JSON 命令
	CommandResp   Type = 0x11 // 命令响应

	// 转发类数据包（透传）
	TunnelOpen    Type = 0x20 // 隧道打开（一次性，携带 MappingID）
	TunnelOpenAck Type = 0x21 // 隧道打开确认
	TunnelData    Type = 0x22 // 隧道数据（纯透传）
	TunnelClose   Type = 0x23 // 隧道关闭

	// 数据包特性标志（可组合）
	Compressed Type = 0x40 // 压缩标志
	Encrypted  Type = 0x80 // 加密标志
)

// IsHeartbeat 判断是否为心跳包
func (t Type) IsHeartbeat() bool {
	return t&0x3F == Heartbeat // 忽略压缩/加密标志
}

// IsJsonCommand 判断是否为JsonCommand包
func (t Type) IsJsonCommand() bool {
	return t&0x3F == JsonCommand
}

// IsCompressed 判断是否压缩
func (t Type) IsCompressed() bool {
	return t&Compressed != 0
}

// IsEncrypted 判断是否加密
func (t Type) IsEncrypted() bool {
	return t&Encrypted != 0
}

// IsTunnelPacket 判断是否为隧道数据包
func (t Type) IsTunnelPacket() bool {
	baseType := t & 0x3F
	return baseType >= TunnelOpen && baseType <= TunnelClose
}

// IsHandshake 判断是否为握手包
func (t Type) IsHandshake() bool {
	return t&0x3F == Handshake
}

type CommandType byte

const (
	// ==================== 连接管理类命令 (10-19) ====================
	Connect      CommandType = 10 // 建立连接
	Disconnect   CommandType = 11 // 连接断开，可以任何方向
	Reconnect    CommandType = 12 // 重新连接
	HeartbeatCmd CommandType = 13 // 心跳保活
	KickClient   CommandType = 14 // 踢下线（服务器通知客户端断开连接）

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

	// ==================== 隧道管理类命令 (35-39) ====================
	TunnelOpenRequestCmd CommandType = 35 // 服务器请求目标客户端打开隧道

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
	TunnelID      string // 隧道ID（用于 TunnelData/TunnelClose）
	Payload       []byte // 原始数据（用于 Tunnel 类型）
}

type CommandPacket struct {
	CommandType CommandType
	CommandId   string // 客户端生成的唯一命令ID
	Token       string
	SenderId    string
	ReceiverId  string
	CommandBody string
}

// HandshakeRequest 握手请求（连接级认证）
type HandshakeRequest struct {
	ClientID int64  `json:"client_id"` // 客户端ID
	Token    string `json:"token"`     // JWT Token
	Version  string `json:"version"`   // 协议版本
	Protocol string `json:"protocol"`  // 连接协议（tcp/websocket/quic）
}

// HandshakeResponse 握手响应
type HandshakeResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// TunnelOpenRequest 隧道打开请求（映射连接认证）
//
// 验证优先级：
//  1. MappingID - 通过连接码创建的隧道映射ID（新设计，推荐）
//  2. SecretKey - 传统的固定密钥（向后兼容，用于API调用）
type TunnelOpenRequest struct {
	MappingID string `json:"mapping_id"` // ⭐ 隧道映射ID（通过ActivateConnectionCode创建）
	TunnelID  string `json:"tunnel_id"`  // 隧道ID（唯一标识本次隧道连接）
	SecretKey string `json:"secret_key"` // ⚠️ 传统密钥（向后兼容，用于旧版API调用）
}

// TunnelOpenAckResponse 隧道打开确认响应
type TunnelOpenAckResponse struct {
	TunnelID string `json:"tunnel_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}
