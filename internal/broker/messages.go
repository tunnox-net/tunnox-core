package broker

// ClientOnlineMessage 客户端上线消息
type ClientOnlineMessage struct {
	ClientID  int64  `json:"client_id"`
	NodeID    string `json:"node_id"`
	IPAddress string `json:"ip_address"`
	Timestamp int64  `json:"timestamp"`
}

// ClientOfflineMessage 客户端下线消息
type ClientOfflineMessage struct {
	ClientID  int64 `json:"client_id"`
	Timestamp int64 `json:"timestamp"`
}

// ConfigUpdateMessage 配置更新消息
type ConfigUpdateMessage struct {
	TargetType string `json:"target_type"` // user/client/mapping
	TargetID   int64  `json:"target_id"`
	ConfigType string `json:"config_type"` // quota/mapping/settings
	ConfigData string `json:"config_data"` // JSON格式配置数据
	Version    int64  `json:"version"`     // 配置版本号
	Timestamp  int64  `json:"timestamp"`
}

// ConfigPushMessage 配置推送消息（定向推送到特定客户端）
type ConfigPushMessage struct {
	ClientID   int64  `json:"client_id"`
	ConfigBody string `json:"config_body"` // JSON格式配置
	Timestamp  int64  `json:"timestamp"`
}

// TunnelOpenMessage 跨节点隧道打开请求消息
type TunnelOpenMessage struct {
	ClientID   int64  `json:"client_id"`
	TunnelID   string `json:"tunnel_id"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	Timestamp  int64  `json:"timestamp"`
}

// MappingCreatedMessage 映射创建消息
type MappingCreatedMessage struct {
	MappingID      int64  `json:"mapping_id"`
	ListenClientID int64  `json:"listen_client_id"` // 监听端客户端ID
	TargetClientID int64  `json:"target_client_id"` // 目标端客户端ID
	Protocol       string `json:"protocol"`
	Timestamp      int64  `json:"timestamp"`
}

// MappingDeletedMessage 映射删除消息
type MappingDeletedMessage struct {
	MappingID int64 `json:"mapping_id"`
	Timestamp int64 `json:"timestamp"`
}

// BridgeRequestMessage 桥接请求消息
type BridgeRequestMessage struct {
	RequestID      string `json:"request_id"`
	SourceNodeID   string `json:"source_node_id"`
	TargetNodeID   string `json:"target_node_id"`
	SourceClientID int64  `json:"source_client_id"`
	TargetClientID int64  `json:"target_client_id"`
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`
	TunnelID       string `json:"tunnel_id"`  // 隧道ID（用于关联）
	MappingID      string `json:"mapping_id"` // 映射ID
}

// BridgeResponseMessage 桥接响应消息
type BridgeResponseMessage struct {
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	StreamID  string `json:"stream_id"` // gRPC 逻辑流ID
}

// NodeHeartbeatMessage 节点心跳消息
type NodeHeartbeatMessage struct {
	NodeID    string `json:"node_id"`
	Address   string `json:"address"`
	Timestamp int64  `json:"timestamp"`
}

// NodeShutdownMessage 节点下线消息
type NodeShutdownMessage struct {
	NodeID    string `json:"node_id"`
	Timestamp int64  `json:"timestamp"`
}
