package conn

import "fmt"

// ConnectionInfo 连接信息结构体
// 用于描述集群内映射系统中各种连接的基本信息
type ConnectionInfo struct {
	Type       ConnectionType // 连接类型
	ConnId     string         // 连接ID，每次新连接由服务端分配的临时ID
	NodeId     string         // 连接接入的节点ID(服务端ID)
	SourceId   string         // 连接的来源ID(可能是ClientId,也可能是ServerId,如果是serverId，说明是转发)
	TargetId   string         // 连接的目的ID(应该只会是ClientId）
	PairConnId string         // 配对的连接ID，只会是数据通道
}

// String 返回连接信息的字符串表示
func (ci *ConnectionInfo) String() string {
	return fmt.Sprintf("Connection{Type:%s, ConnId:%s, NodeId:%s, SourceId:%s, TargetId:%s, PairConnId:%s}",
		ci.Type.String(), ci.ConnId, ci.NodeId, ci.SourceId, ci.TargetId, ci.PairConnId)
}

// IsControl 判断是否为控制类连接
func (ci *ConnectionInfo) IsControl() bool {
	return ci.Type.IsControl()
}

// IsData 判断是否为数据类连接
func (ci *ConnectionInfo) IsData() bool {
	return ci.Type.IsData()
}

// IsReply 判断是否为回复/转发类连接
func (ci *ConnectionInfo) IsReply() bool {
	return ci.Type.IsReply()
}

// HasPair 判断是否有配对的连接
func (ci *ConnectionInfo) HasPair() bool {
	return ci.PairConnId != ""
}

// SetPair 设置配对连接ID
func (ci *ConnectionInfo) SetPair(pairConnId string) {
	ci.PairConnId = pairConnId
}

// ClearPair 清除配对连接ID
func (ci *ConnectionInfo) ClearPair() {
	ci.PairConnId = ""
}
