package client

// SaaS公共服务端点配置
const (
	// 公共服务域名
	PublicServiceDomain = "gw.tunnox.net"

	// 公共服务端点（按优先级排序）
	// WebSocket 优先（穿透性最好，大多数网络环境都能成功）
	PublicServiceWebSocket = "wss://ws.tunnox.net"

	// QUIC 端点
	PublicServiceQUIC = "gw.tunnox.net:8443"

	// TCP 端点（备选）
	PublicServiceTCP = "gw.tunnox.net:8080"

	// KCP 端点（最后备选）
	PublicServiceKCP = "gw.tunnox.net:8000"

	// 向后兼容的别名
	PublicServiceQUIC1      = PublicServiceQUIC
	PublicServiceQUIC2      = PublicServiceQUIC
	PublicServiceTCP1       = PublicServiceTCP
	PublicServiceTCP2       = PublicServiceTCP
	PublicServiceWebSocket1 = PublicServiceWebSocket
	PublicServiceWebSocket2 = PublicServiceWebSocket
)

// 自动连接配置
const (
	// 连接尝试轮数
	AutoConnectMaxRounds = 2

	// 每轮的超时时间（秒）- 缩短超时以加快失败检测
	AutoConnectRound1Timeout = 3
	AutoConnectRound2Timeout = 8

	// 单个连接+握手的超时时间（秒）
	AutoConnectDialTimeout      = 3
	AutoConnectHandshakeTimeout = 5
)

// 协议优先级顺序（WebSocket 优先因为穿透性最好）
var DefaultProtocolPriority = []string{
	"websocket",
	"quic",
	"tcp",
	"kcp",
}
