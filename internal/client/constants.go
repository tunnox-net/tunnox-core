package client

// SaaS公共服务端点配置
const (
	// 公共服务域名
	PublicServiceDomain = "gw.tunnox.net"

	// 公共服务端点（按优先级排序）
	// QUIC 端点
	PublicServiceQUIC1 = "tunnox.mydtc.net:8443"
	PublicServiceQUIC2 = "gw.tunnox.net:8443"

	// TCP 端点
	PublicServiceTCP1 = "tunnox.mydtc.net:8080"
	PublicServiceTCP2 = "gw.tunnox.net:8080"

	// WebSocket 端点
	PublicServiceWebSocket1 = "ws://tunnox.mydtc.net"
	PublicServiceWebSocket2 = "wss://ws.tunnox.net"

	// 保留旧的常量名以保持向后兼容
	PublicServiceQUIC      = PublicServiceQUIC2
	PublicServiceTCP       = PublicServiceTCP2
	PublicServiceWebSocket = PublicServiceWebSocket2
	PublicServiceKCP       = "gw.tunnox.net:8000"
	PublicServiceHTTPPoll  = "https://gw.tunnox.net/_tunnox"
)

// 自动连接配置
const (
	// 连接尝试轮数
	AutoConnectMaxRounds = 3

	// 每轮的超时时间（秒）
	AutoConnectRound1Timeout = 5
	AutoConnectRound2Timeout = 10
	AutoConnectRound3Timeout = 15
)

// 协议优先级顺序
var DefaultProtocolPriority = []string{
	"quic",
	"tcp",
	"websocket",
	"kcp",
	"httppoll",
}
