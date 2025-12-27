package stats

import (
	"time"
	"tunnox-core/internal/cloud/models"
)

// TrafficStats 流量统计类型别名
// 为了向后兼容，使用类型别名指向 models.TrafficStats
type TrafficStats = models.TrafficStats

// UserStats 用户统计信息
type UserStats struct {
	UserID           string    `json:"user_id"`
	TotalClients     int       `json:"total_clients"`
	OnlineClients    int       `json:"online_clients"`
	TotalMappings    int       `json:"total_mappings"`
	ActiveMappings   int       `json:"active_mappings"`
	TotalTraffic     int64     `json:"total_traffic"`     // 总流量(字节)
	TotalConnections int64     `json:"total_connections"` // 总连接数
	LastActive       time.Time `json:"last_active"`
}

// ClientStats 客户端统计信息
type ClientStats struct {
	ClientID         int64     `json:"client_id"`
	UserID           string    `json:"user_id"`
	TotalMappings    int       `json:"total_mappings"`
	ActiveMappings   int       `json:"active_mappings"`
	TotalTraffic     int64     `json:"total_traffic"`     // 总流量(字节)
	TotalConnections int64     `json:"total_connections"` // 总连接数
	Uptime           int64     `json:"uptime"`            // 在线时长(秒)
	LastSeen         time.Time `json:"last_seen"`
}

// SystemStats 系统统计信息
type SystemStats struct {
	TotalUsers       int   `json:"total_users"`
	TotalClients     int   `json:"total_clients"`
	OnlineClients    int   `json:"online_clients"`
	TotalMappings    int   `json:"total_mappings"`
	ActiveMappings   int   `json:"active_mappings"`
	TotalNodes       int   `json:"total_nodes"`
	OnlineNodes      int   `json:"online_nodes"`
	TotalTraffic     int64 `json:"total_traffic"`     // 总流量(字节)
	TotalConnections int64 `json:"total_connections"` // 总连接数
	AnonymousUsers   int   `json:"anonymous_users"`   // 匿名用户数
}

// TrafficDataPoint 流量数据点
type TrafficDataPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	BytesSent     int64     `json:"bytes_sent"`
	BytesReceived int64     `json:"bytes_received"`
	UserID        string    `json:"user_id,omitempty"`
	ClientID      int64     `json:"client_id,omitempty"`
}

// ConnectionDataPoint 连接数据点
type ConnectionDataPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Connections int       `json:"connections"`
	UserID      string    `json:"user_id,omitempty"`
	ClientID    int64     `json:"client_id,omitempty"`
}
