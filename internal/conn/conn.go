package conn

import "fmt"

type Type byte

const (
	// ServiceControl 服务端到服务端的指令连接
	// 用于集群中服务端之间的控制指令通信
	ServiceControl Type = 1

	// ClientControl 客户端到服务端的指令连接
	// 用于客户端向服务端发送控制指令
	ClientControl Type = 2

	// ServerControlReply 跨服务端指令转发通道
	// 当客户端a连到服务端A，客户端b连到服务端B时
	// a和b之间的通信需要A和B之间的指令转发
	ServerControlReply Type = 3

	// DataTransfer 客户端间数据传输通道
	// 同一服务端内的客户端可以直接透传数据
	DataTransfer Type = 4

	// DataTransferReply 跨服务端数据传输通道
	// 类似ServerControlReply，但用于数据而非指令
	DataTransferReply Type = 5
)

// String 返回连接类型的字符串表示
func (ct Type) String() string {
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
func (ct Type) IsControl() bool {
	return ct == ServiceControl || ct == ClientControl || ct == ServerControlReply
}

// IsData 判断是否为数据类连接
func (ct Type) IsData() bool {
	return ct == DataTransfer || ct == DataTransferReply
}

// IsReply 判断是否为回复/转发类连接
func (ct Type) IsReply() bool {
	return ct == ServerControlReply || ct == DataTransferReply
}

// Info 连接信息结构体
// 用于描述集群内映射系统中各种连接的基本信息
type Info struct {
	Type       Type   // 连接类型
	ConnId     string // 连接ID，每次新连接由服务端分配的临时ID
	NodeId     string // 连接接入的节点ID(服务端ID)
	SourceId   string // 连接的来源ID(可能是ClientId,也可能是ServerId,如果是serverId，说明是转发)
	TargetId   string // 连接的目的ID(应该只会是ClientId）
	PairConnId string // 配对的连接ID，只会是数据通道
}

// String 返回连接信息的字符串表示
func (ci *Info) String() string {
	return fmt.Sprintf("Connection{Type:%s, ConnId:%s, NodeId:%s, SourceId:%s, TargetId:%s, PairConnId:%s}",
		ci.Type.String(), ci.ConnId, ci.NodeId, ci.SourceId, ci.TargetId, ci.PairConnId)
}

// IsControl 判断是否为控制类连接
func (ci *Info) IsControl() bool {
	return ci.Type.IsControl()
}

// IsData 判断是否为数据类连接
func (ci *Info) IsData() bool {
	return ci.Type.IsData()
}

// IsReply 判断是否为回复/转发类连接
func (ci *Info) IsReply() bool {
	return ci.Type.IsReply()
}

// HasPair 判断是否有配对的连接
func (ci *Info) HasPair() bool {
	return ci.PairConnId != ""
}

// SetPair 设置配对连接ID
func (ci *Info) SetPair(pairConnId string) {
	ci.PairConnId = pairConnId
}

// ClearPair 清除配对连接ID
func (ci *Info) ClearPair() {
	ci.PairConnId = ""
}
