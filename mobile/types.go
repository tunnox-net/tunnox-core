package mobile

// 协议类型常量
const (
	ProtocolTCP       = "tcp"
	ProtocolWebSocket = "websocket"
	ProtocolKCP       = "kcp"
	ProtocolQUIC      = "quic"
)

// ConnectionStatus 连接状态
type ConnectionStatus struct {
	Connected    bool   // 是否已连接
	ClientID     int64  // 客户端 ID
	ServerAddr   string // 服务器地址
	Protocol     string // 连接协议
	UptimeMillis int64  // 运行时长（毫秒）
	MappingCount int    // 映射数量
}

// Socks5Mapping SOCKS5 映射信息
type Socks5Mapping struct {
	MappingID      string // 映射 ID
	ListenPort     int64  // 监听端口
	TargetClientID int64  // 目标客户端 ID
	SecretKey      string // 密钥
	Status         string // 状态："active" or "inactive"
}

// ClientConfig 客户端配置（简化版）
type ClientConfig struct {
	ServerAddr string // 服务器地址
	Protocol   string // 连接协议
	ClientID   int64  // 客户端 ID
	SecretKey  string // 密钥
}
