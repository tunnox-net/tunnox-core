package client

// SaaS公共服务端点配置
const (
	// 公共服务端点（按优先级排序）
	// WebSocket 优先（穿透性最好，大多数网络环境都能成功）
	PublicServiceWebSocket = "wss://ws.tunnox.net"

	// QUIC 端点
	PublicServiceQUIC = "gw.tunnox.net:8443"

	// TCP 端点（备选）
	PublicServiceTCP = "gw.tunnox.net:8080"

	// KCP 端点（最后备选）
	PublicServiceKCP = "gw.tunnox.net:8000"
)

// 自动连接配置
const (
	// 连接尝试轮数
	AutoConnectMaxRounds = 2

	// 每轮的超时时间（秒）
	// 必须大于 (AutoConnectDialTimeout + AutoConnectHandshakeTimeout)
	// 以确保至少有一个协议能完成完整的连接+握手流程
	AutoConnectRound1Timeout = 15
	AutoConnectRound2Timeout = 25

	// 单个连接+握手的超时时间（秒）
	// 注意：通过代理连接时需要更长时间
	AutoConnectDialTimeout      = 10
	AutoConnectHandshakeTimeout = 10
)

// 协议优先级顺序（WebSocket 优先因为穿透性最好）
var DefaultProtocolPriority = []string{
	"websocket",
	"quic",
	"tcp",
	"kcp",
}
