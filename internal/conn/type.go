package conn

import "fmt"

type ConnectionType byte

const (
	// ServiceControl 服务端到服务端的指令连接
	// 用于集群中服务端之间的控制指令通信
	ServiceControl ConnectionType = 1

	// ClientControl 客户端到服务端的指令连接
	// 用于客户端向服务端发送控制指令
	ClientControl ConnectionType = 2

	// ServerControlReply 跨服务端指令转发通道
	// 当客户端a连到服务端A，客户端b连到服务端B时
	// a和b之间的通信需要A和B之间的指令转发
	ServerControlReply ConnectionType = 3

	// DataTransfer 客户端间数据传输通道
	// 同一服务端内的客户端可以直接透传数据
	DataTransfer ConnectionType = 4

	// DataTransferReply 跨服务端数据传输通道
	// 类似ServerControlReply，但用于数据而非指令
	DataTransferReply ConnectionType = 5
)

// String 返回连接类型的字符串表示
func (ct ConnectionType) String() string {
	switch ct {
	case ServiceControl:
		return "ServiceControl"
	case ClientControl:
		return "ClientControl"
	case ServerControlReply:
		return "ServerControlReply"
	case DataTransfer:
		return "DataTransfer"
	case DataTransferReply:
		return "DataTransferReply"
	default:
		return fmt.Sprintf("Unknown(%d)", ct)
	}
}

// IsControl 判断是否为控制类连接
func (ct ConnectionType) IsControl() bool {
	return ct == ServiceControl || ct == ClientControl || ct == ServerControlReply
}

// IsData 判断是否为数据类连接
func (ct ConnectionType) IsData() bool {
	return ct == DataTransfer || ct == DataTransferReply
}

// IsReply 判断是否为回复/转发类连接
func (ct ConnectionType) IsReply() bool {
	return ct == ServerControlReply || ct == DataTransferReply
}
