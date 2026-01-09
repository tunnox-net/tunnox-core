package models

import (
	"time"
)

// Webhook 代表一个 Webhook 注册记录
type Webhook struct {
	ID             string     `json:"id"`               // Webhook ID（UUID）
	UserID         string     `json:"user_id"`          // 所属用户ID（可为空，系统级webhook）
	Name           string     `json:"name"`             // Webhook名称
	URL            string     `json:"url"`              // 回调URL
	Secret         string     `json:"secret,omitempty"` // 签名密钥（HMAC）
	Events         []string   `json:"events"`           // 订阅的事件类型
	Enabled        bool       `json:"enabled"`          // 是否启用
	RetryCount     int        `json:"retry_count"`      // 重试次数
	TimeoutSeconds int        `json:"timeout_seconds"`  // 超时时间（秒）
	CreatedAt      time.Time  `json:"created_at"`       // 创建时间
	UpdatedAt      time.Time  `json:"updated_at"`       // 更新时间
	LastTriggered  *time.Time `json:"last_triggered"`   // 最后触发时间
}

// WebhookLog 记录每次 Webhook 发送的日志
type WebhookLog struct {
	ID             string    `json:"id"`              // 日志ID
	WebhookID      string    `json:"webhook_id"`      // 关联的Webhook ID
	EventType      string    `json:"event_type"`      // 事件类型
	Payload        string    `json:"payload"`         // 发送的数据（JSON）
	ResponseStatus int       `json:"response_status"` // HTTP响应状态码
	ResponseBody   string    `json:"response_body"`   // 响应体（截断）
	Success        bool      `json:"success"`         // 是否成功
	SentAt         time.Time `json:"sent_at"`         // 发送时间
	Duration       int64     `json:"duration_ms"`     // 耗时（毫秒）
}

// WebhookEvent Webhook 事件类型常量
type WebhookEvent string

const (
	// 客户端事件
	WebhookEventClientOnline  WebhookEvent = "client.online"
	WebhookEventClientOffline WebhookEvent = "client.offline"

	// 映射事件
	WebhookEventMappingCreated       WebhookEvent = "mapping.created"
	WebhookEventMappingDeleted       WebhookEvent = "mapping.deleted"
	WebhookEventMappingStatusChanged WebhookEvent = "mapping.status_changed"

	// 隧道事件
	WebhookEventTunnelOpened WebhookEvent = "tunnel.opened"
	WebhookEventTunnelClosed WebhookEvent = "tunnel.closed"

	// 流量事件
	WebhookEventTrafficQuotaWarning WebhookEvent = "traffic.quota_warning"
)

// AllWebhookEvents 所有支持的事件类型
var AllWebhookEvents = []WebhookEvent{
	WebhookEventClientOnline,
	WebhookEventClientOffline,
	WebhookEventMappingCreated,
	WebhookEventMappingDeleted,
	WebhookEventMappingStatusChanged,
	WebhookEventTunnelOpened,
	WebhookEventTunnelClosed,
	WebhookEventTrafficQuotaWarning,
}

// WebhookPayload Webhook 推送的统一数据格式
type WebhookPayload struct {
	ID        string      `json:"id"`        // 事件ID（UUID）
	Event     string      `json:"event"`     // 事件类型
	Timestamp int64       `json:"timestamp"` // 事件时间戳（Unix毫秒）
	Data      interface{} `json:"data"`      // 事件数据
	Signature string      `json:"signature"` // HMAC-SHA256签名（可选）
}

// WebhookClientEventData 客户端事件数据
type WebhookClientEventData struct {
	ClientID  int64  `json:"client_id"`
	UserID    string `json:"user_id,omitempty"`
	Status    string `json:"status"`
	IPAddress string `json:"ip_address,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
}

// WebhookMappingEventData 映射事件数据
type WebhookMappingEventData struct {
	MappingID      string `json:"mapping_id"`
	UserID         string `json:"user_id,omitempty"`
	Protocol       string `json:"protocol"`
	ListenClientID int64  `json:"listen_client_id"`
	TargetClientID int64  `json:"target_client_id"`
	Status         string `json:"status,omitempty"`
}

// WebhookTunnelEventData 隧道事件数据
type WebhookTunnelEventData struct {
	TunnelID   string `json:"tunnel_id"`
	MappingID  string `json:"mapping_id"`
	ClientID   int64  `json:"client_id"`
	TargetHost string `json:"target_host,omitempty"`
	TargetPort int    `json:"target_port,omitempty"`
}

// WebhookTrafficEventData 流量事件数据
type WebhookTrafficEventData struct {
	UserID      string `json:"user_id"`
	UsedBytes   int64  `json:"used_bytes"`
	LimitBytes  int64  `json:"limit_bytes"`
	UsedPercent int    `json:"used_percent"`
}

// GetID 实现接口
func (w *Webhook) GetID() string {
	return w.ID
}

// HasEvent 检查是否订阅了指定事件
func (w *Webhook) HasEvent(event string) bool {
	for _, e := range w.Events {
		if e == event || e == "*" {
			return true
		}
	}
	return false
}

// DefaultWebhook 返回默认配置的 Webhook
func DefaultWebhook() *Webhook {
	return &Webhook{
		Enabled:        true,
		RetryCount:     3,
		TimeoutSeconds: 30,
		Events:         []string{},
	}
}
