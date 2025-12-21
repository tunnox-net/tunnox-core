package client

// SaaS公共服务端点配置
const (
	// 公共服务域名
	PublicServiceDomain = "gw.tunnox.net"

	// 公共服务端点（按优先级排序）
	PublicServiceWebSocket = "wss://gw.tunnox.net/_tunnox"
	PublicServiceTCP       = "gw.tunnox.net:8000"
	PublicServiceKCP       = "gw.tunnox.net:8000"
	PublicServiceQUIC      = "gw.tunnox.net:443"
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
	"websocket",
	"tcp",
	"kcp",
	"quic",
	"httppoll",
}
