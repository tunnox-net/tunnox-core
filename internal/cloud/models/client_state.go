package models

import (
	"fmt"
	"time"
)

// ClientRuntimeState 客户端运行时状态
//
// 存储：仅缓存（Redis/Memory），不持久化到数据库
// 键：tunnox:runtime:client:state:{client_id}
// TTL：90秒（心跳间隔30秒 * 3）
// 特点：快变化，服务重启后丢失
//
// 包含字段：
// - 连接信息：NodeID, ConnID, IPAddress, Protocol
// - 状态信息：Status, LastSeen
// - 版本信息：Version
type ClientRuntimeState struct {
	ClientID  int64        `json:"client_id"`  // 客户端ID
	NodeID    string       `json:"node_id"`    // 当前连接的节点ID
	ConnID    string       `json:"conn_id"`    // 当前连接ID
	Status    ClientStatus `json:"status"`     // 客户端状态（online/offline/blocked）
	IPAddress string       `json:"ip_address"` // 客户端IP地址
	Protocol  string       `json:"protocol"`   // 连接协议（tcp/websocket/quic/udp）
	Version   string       `json:"version"`    // 客户端版本
	LastSeen  time.Time    `json:"last_seen"`  // 最后心跳时间
}

// IsOnline 判断客户端是否在线
//
// 条件：
// 1. Status为online
// 2. 距离LastSeen不超过90秒
func (s *ClientRuntimeState) IsOnline() bool {
	if s.Status != ClientStatusOnline {
		return false
	}
	
	// 如果超过90秒没有心跳，认为已离线
	return time.Since(s.LastSeen) < 90*time.Second
}

// IsOnNode 判断客户端是否在指定节点上
func (s *ClientRuntimeState) IsOnNode(nodeID string) bool {
	return s.IsOnline() && s.NodeID == nodeID
}

// IsBlocked 判断客户端是否被封禁
func (s *ClientRuntimeState) IsBlocked() bool {
	return s.Status == ClientStatusBlocked
}

// Validate 验证状态有效性
func (s *ClientRuntimeState) Validate() error {
	if s.ClientID <= 0 {
		return fmt.Errorf("invalid client ID: %d", s.ClientID)
	}
	
	if s.Status != ClientStatusOnline && s.Status != ClientStatusOffline && s.Status != ClientStatusBlocked {
		return fmt.Errorf("invalid status: %s", s.Status)
	}
	
	if s.Status == ClientStatusOnline {
		if s.NodeID == "" {
			return fmt.Errorf("online client must have node_id")
		}
		if s.ConnID == "" {
			return fmt.Errorf("online client must have conn_id")
		}
	}
	
	return nil
}

// Touch 更新最后心跳时间
func (s *ClientRuntimeState) Touch() {
	s.LastSeen = time.Now()
}

// GetConnectionInfo 获取连接信息摘要
func (s *ClientRuntimeState) GetConnectionInfo() string {
	if !s.IsOnline() {
		return "offline"
	}
	return fmt.Sprintf("node=%s, conn=%s, ip=%s, proto=%s", 
		s.NodeID, s.ConnID, s.IPAddress, s.Protocol)
}

