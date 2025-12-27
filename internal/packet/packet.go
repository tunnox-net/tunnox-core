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

// IsCommandResp 判断是否为CommandResp包
func (t Type) IsCommandResp() bool {
	return t&0x3F == CommandResp
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
	Connect        CommandType = 10 // 建立连接
	Disconnect     CommandType = 11 // 连接断开，可以任何方向
	Reconnect      CommandType = 12 // 重新连接
	HeartbeatCmd   CommandType = 13 // 心跳保活
	KickClient     CommandType = 14 // 踢下线（服务器通知客户端断开连接）
	ServerShutdown CommandType = 15 // 服务器优雅关闭通知

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
	TunnelMigrate        CommandType = 36 // 隧道迁移命令
	TunnelMigrateAck     CommandType = 37 // 隧道迁移确认
	TunnelStateSync      CommandType = 38 // 隧道状态同步

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

	// ==================== 连接码管理类命令 (70-79) ====================
	ConnectionCodeGenerate CommandType = 70 // 生成连接码
	ConnectionCodeList     CommandType = 71 // 列出连接码
	ConnectionCodeActivate CommandType = 72 // 激活连接码
	ConnectionCodeRevoke   CommandType = 73 // 撤销连接码
	MappingList            CommandType = 74 // 列出映射列表
	MappingGet             CommandType = 75 // 获取映射详情
	MappingDelete          CommandType = 76 // 删除映射

	// ==================== HTTP 代理类命令 (80-89) ====================
	HTTPProxyRequest  CommandType = 80 // HTTP 代理请求
	HTTPProxyResponse CommandType = 81 // HTTP 代理响应

	// HTTP 域名映射管理命令
	HTTPDomainGetBaseDomains CommandType = 82 // 获取可用的基础域名列表
	HTTPDomainCheckSubdomain CommandType = 83 // 检查子域名可用性
	HTTPDomainGenSubdomain   CommandType = 84 // 生成随机子域名
	HTTPDomainCreate         CommandType = 85 // 创建 HTTP 域名映射
	HTTPDomainDelete         CommandType = 86 // 删除 HTTP 域名映射
	HTTPDomainList           CommandType = 87 // 列出 HTTP 域名映射

	// ==================== SOCKS5 代理类命令 (90-99) ====================
	SOCKS5TunnelRequestCmd CommandType = 90 // SOCKS5 隧道请求（ClientA -> Server）

	// ==================== 通知类命令 (100-109) ====================
	NotifyClient       CommandType = 100 // 服务端 -> 客户端 推送通知
	NotifyClientAck    CommandType = 101 // 客户端 -> 服务端 通知确认（可选）
	SendNotifyToClient CommandType = 102 // 客户端 -> 服务端 -> 目标客户端（C2C通知）
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

// TransferPacket 数据传输包
//
// 版本兼容性：
// - V1 (旧版本): 仅包含基础字段 (PacketType, CommandPacket, TunnelID, Payload)
// - V2 (新版本): 添加序列号支持 (SeqNum, AckNum, Flags)
//   - Flags = 0: V1格式（向后兼容）
//   - Flags != 0: V2格式（启用序列号）
type TransferPacket struct {
	PacketType    Type
	CommandPacket *CommandPacket
	TunnelID      string // 隧道ID（用于 TunnelData/TunnelClose）
	Payload       []byte // 原始数据（用于 Tunnel 类型）

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// V2扩展字段（用于可靠传输和隧道迁移）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	SeqNum uint64      // 序列号（发送端递增）
	AckNum uint64      // 确认号（接收端期望的下一个序列号）
	Flags  PacketFlags // 标志位（SYN, FIN, ACK, RST等）
}

// PacketFlags 数据包标志位
type PacketFlags uint8

const (
	// 基础标志位
	FlagNone PacketFlags = 0      // 无标志（V1兼容模式）
	FlagSYN  PacketFlags = 1      // 建立连接
	FlagFIN  PacketFlags = 1 << 1 // 结束连接
	FlagACK  PacketFlags = 1 << 2 // 确认
	FlagRST  PacketFlags = 1 << 3 // 重置连接

	// 扩展标志位
	FlagMigrate PacketFlags = 1 << 4 // 隧道迁移中
	FlagBuffer  PacketFlags = 1 << 5 // 数据已缓冲
)

// IsV2 判断是否为V2格式（启用序列号）
func (p *TransferPacket) IsV2() bool {
	return p.Flags != FlagNone
}

// HasFlag 检查是否包含指定标志
func (p *TransferPacket) HasFlag(flag PacketFlags) bool {
	return p.Flags&flag != 0
}

// SetFlag 设置标志位
func (p *TransferPacket) SetFlag(flag PacketFlags) {
	p.Flags |= flag
}

// ClearFlag 清除标志位
func (p *TransferPacket) ClearFlag(flag PacketFlags) {
	p.Flags &^= flag
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
	ClientID       int64  `json:"client_id"`                 // 客户端ID
	Token          string `json:"token"`                     // JWT Token
	Version        string `json:"version"`                   // 协议版本
	Protocol       string `json:"protocol"`                  // 连接协议（tcp/websocket/quic）
	ConnectionType string `json:"connection_type,omitempty"` // 连接类型：control（控制连接）或 tunnel（隧道连接）
}

// HandshakeResponse 握手响应
type HandshakeResponse struct {
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	Message      string `json:"message,omitempty"`
	SessionToken string `json:"session_token,omitempty"` // 会话Token（认证成功后返回）
	ClientID     int64  `json:"client_id,omitempty"`     // 分配的ClientID（匿名客户端首次握手）
	SecretKey    string `json:"secret_key,omitempty"`    // 分配的SecretKey（匿名客户端首次握手）
	ConnectionID string `json:"connection_id,omitempty"` // 服务端分配的ConnectionID（HTTP长轮询专用）
}

// TunnelOpenRequest 隧道打开请求（映射连接认证）
//
// 验证优先级：
//  1. MappingID - 通过连接码创建的隧道映射ID（新设计，推荐）
//  2. SecretKey - 传统的固定密钥（向后兼容，用于API调用）
//  3. ResumeToken - 用于恢复中断的隧道（Phase 2 迁移支持）
type TunnelOpenRequest struct {
	MappingID   string `json:"mapping_id"`             // ⭐ 隧道映射ID（通过ActivateConnectionCode创建）
	TunnelID    string `json:"tunnel_id"`              // 隧道ID（唯一标识本次隧道连接）
	SecretKey   string `json:"secret_key"`             // ⚠️ 传统密钥（向后兼容，用于旧版API调用）
	ResumeToken string `json:"resume_token,omitempty"` // ✨ Phase 2: 恢复Token（断线重连，包含TunnelID+签名）

	// SOCKS5 动态目标地址（仅 SOCKS5 协议使用）
	TargetHost string `json:"target_host,omitempty"` // 动态目标主机（由 SOCKS5 协议指定）
	TargetPort int    `json:"target_port,omitempty"` // 动态目标端口（由 SOCKS5 协议指定）
}

// TunnelOpenAckResponse 隧道打开确认响应
type TunnelOpenAckResponse struct {
	TunnelID string `json:"tunnel_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 服务器优雅关闭相关
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ServerShutdownCommand 服务器优雅关闭通知
//
// 当服务器准备关闭（如滚动更新）时，通过指令通道向所有客户端广播此消息，
// 告知客户端服务器即将关闭，请求客户端完成当前工作并准备重连。
type ServerShutdownCommand struct {
	Reason             string `json:"reason"`                    // 关闭原因（rolling_update, maintenance, shutdown）
	GracePeriodSeconds int    `json:"grace_period_seconds"`      // 优雅期（秒），在此期间服务器将等待活跃隧道完成
	RecommendReconnect bool   `json:"recommend_reconnect"`       // 是否建议客户端重连
	Message            string `json:"message,omitempty"`         // 可选的人类可读消息
	ReconnectToken     string `json:"reconnect_token,omitempty"` // 重连Token（JSON编码，客户端可用于快速重连）
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道迁移相关命令
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// TunnelMigrateCommand 隧道迁移命令
//
// 从源节点发送到目标节点，请求迁移隧道。
type TunnelMigrateCommand struct {
	TunnelID         string `json:"tunnel_id"`
	MappingID        string `json:"mapping_id"`
	SourceNodeID     string `json:"source_node_id"`
	TargetNodeID     string `json:"target_node_id"`
	LastSeqNum       uint64 `json:"last_seq_num"`
	LastAckNum       uint64 `json:"last_ack_num"`
	NextExpectedSeq  uint64 `json:"next_expected_seq"`
	StateSignature   string `json:"state_signature"`    // 状态签名
	BufferedDataSize int    `json:"buffered_data_size"` // 缓冲数据大小
}

// TunnelMigrateAckCommand 隧道迁移确认
//
// 目标节点确认接收迁移请求。
type TunnelMigrateAckCommand struct {
	TunnelID  string `json:"tunnel_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	NewNodeID string `json:"new_node_id,omitempty"`
}

// TunnelStateSyncCommand 隧道状态同步命令
//
// 用于同步缓冲的数据包。
type TunnelStateSyncCommand struct {
	TunnelID        string                 `json:"tunnel_id"`
	BufferedPackets []TunnelBufferedPacket `json:"buffered_packets"`
}

// TunnelBufferedPacket 缓冲包（用于状态同步）
type TunnelBufferedPacket struct {
	SeqNum uint64 `json:"seq_num"`
	Data   []byte `json:"data"`
	SentAt int64  `json:"sent_at"` // Unix timestamp
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射相关命令
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainBaseDomainInfo 基础域名信息
type HTTPDomainBaseDomainInfo struct {
	Domain      string `json:"domain"`      // 基础域名，如 "tunnox.net"
	Description string `json:"description"` // 描述
	IsDefault   bool   `json:"is_default"`  // 是否为默认域名
}

// HTTPDomainGetBaseDomainsRequest 获取可用基础域名请求
type HTTPDomainGetBaseDomainsRequest struct{}

// HTTPDomainGetBaseDomainsResponse 获取可用基础域名响应
type HTTPDomainGetBaseDomainsResponse struct {
	Success     bool                       `json:"success"`
	BaseDomains []HTTPDomainBaseDomainInfo `json:"base_domains"`
	Error       string                     `json:"error,omitempty"`
}

// HTTPDomainCheckSubdomainRequest 检查子域名可用性请求
type HTTPDomainCheckSubdomainRequest struct {
	Subdomain  string `json:"subdomain"`   // 子域名，如 "myapp"
	BaseDomain string `json:"base_domain"` // 基础域名，如 "tunnox.net"
}

// HTTPDomainCheckSubdomainResponse 检查子域名可用性响应
type HTTPDomainCheckSubdomainResponse struct {
	Success    bool   `json:"success"`
	Available  bool   `json:"available"`
	FullDomain string `json:"full_domain"` // 完整域名，如 "myapp.tunnox.net"
	Error      string `json:"error,omitempty"`
}

// HTTPDomainGenSubdomainRequest 生成随机子域名请求
type HTTPDomainGenSubdomainRequest struct {
	BaseDomain string `json:"base_domain"` // 基础域名
}

// HTTPDomainGenSubdomainResponse 生成随机子域名响应
type HTTPDomainGenSubdomainResponse struct {
	Success    bool   `json:"success"`
	Subdomain  string `json:"subdomain"`   // 生成的子域名
	FullDomain string `json:"full_domain"` // 完整域名
	Error      string `json:"error,omitempty"`
}

// HTTPDomainCreateRequest 创建 HTTP 域名映射请求
type HTTPDomainCreateRequest struct {
	TargetURL   string `json:"target_url"`            // 目标 URL，如 "http://localhost:3000"
	Subdomain   string `json:"subdomain"`             // 子域名
	BaseDomain  string `json:"base_domain"`           // 基础域名
	MappingTTL  int    `json:"mapping_ttl,omitempty"` // 映射有效期（秒），0表示使用默认值
	Description string `json:"description,omitempty"` // 描述
}

// HTTPDomainCreateResponse 创建 HTTP 域名映射响应
type HTTPDomainCreateResponse struct {
	Success    bool   `json:"success"`
	MappingID  string `json:"mapping_id"`
	FullDomain string `json:"full_domain"` // 完整域名
	TargetURL  string `json:"target_url"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// HTTPDomainDeleteRequest 删除 HTTP 域名映射请求
type HTTPDomainDeleteRequest struct {
	MappingID string `json:"mapping_id"` // 映射ID
}

// HTTPDomainDeleteResponse 删除 HTTP 域名映射响应
type HTTPDomainDeleteResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// HTTPDomainListRequest 列出 HTTP 域名映射请求
type HTTPDomainListRequest struct{}

// HTTPDomainMappingInfo HTTP 域名映射信息
type HTTPDomainMappingInfo struct {
	MappingID  string `json:"mapping_id"`
	FullDomain string `json:"full_domain"` // 完整域名
	TargetURL  string `json:"target_url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// HTTPDomainListResponse 列出 HTTP 域名映射响应
type HTTPDomainListResponse struct {
	Success  bool                    `json:"success"`
	Mappings []HTTPDomainMappingInfo `json:"mappings"`
	Total    int                     `json:"total"`
	Error    string                  `json:"error,omitempty"`
}
