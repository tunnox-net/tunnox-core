package packet

import "time"

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 通知类型定义
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// NotificationType 通知类型
type NotificationType uint8

const (
	// 系统通知 (1-19)
	NotifyTypeSystemMessage     NotificationType = 1 // 系统消息
	NotifyTypeSystemMaintain    NotificationType = 2 // 系统维护通知
	NotifyTypeQuotaWarning      NotificationType = 3 // 配额预警
	NotifyTypeQuotaExhausted    NotificationType = 4 // 配额耗尽
	NotifyTypeVersionUpdate     NotificationType = 5 // 版本更新通知
	NotifyTypeSecurityAlert     NotificationType = 6 // 安全告警
	NotifyTypeAnnounceBroadcast NotificationType = 7 // 公告广播

	// 映射通知 (20-39)
	NotifyTypeMappingCreated   NotificationType = 20 // 映射创建
	NotifyTypeMappingDeleted   NotificationType = 21 // 映射删除
	NotifyTypeMappingUpdated   NotificationType = 22 // 映射更新
	NotifyTypeMappingExpired   NotificationType = 23 // 映射过期
	NotifyTypeMappingActivated NotificationType = 24 // 映射激活（连接码被使用）

	// 隧道通知 (40-59)
	NotifyTypeTunnelOpened    NotificationType = 40 // 隧道已打开
	NotifyTypeTunnelClosed    NotificationType = 41 // 隧道已关闭
	NotifyTypeTunnelError     NotificationType = 42 // 隧道错误
	NotifyTypeTunnelMigrated  NotificationType = 43 // 隧道已迁移
	NotifyTypeTunnelSuspended NotificationType = 44 // 隧道暂停

	// 自定义通知 (100+)
	NotifyTypeCustom NotificationType = 100 // 自定义通知（C2C）
)

// NotifyPriority 通知优先级
type NotifyPriority uint8

const (
	PriorityLow      NotifyPriority = 0 // 低优先级（可丢弃）
	PriorityNormal   NotifyPriority = 1 // 普通优先级
	PriorityHigh     NotifyPriority = 2 // 高优先级
	PriorityCritical NotifyPriority = 3 // 关键（必须送达）
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 核心通知结构
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ClientNotification 客户端通知（服务端 -> 客户端）
type ClientNotification struct {
	NotifyID       string           `json:"notify_id"`                  // 通知唯一ID
	Type           NotificationType `json:"type"`                       // 通知类型
	Timestamp      int64            `json:"timestamp"`                  // 发送时间戳（Unix毫秒）
	Payload        string           `json:"payload"`                    // JSON编码的载荷
	SenderClientID int64            `json:"sender_client_id,omitempty"` // 发送者客户端ID（C2C通知）
	Priority       NotifyPriority   `json:"priority"`                   // 优先级
	ExpireAt       int64            `json:"expire_at,omitempty"`        // 过期时间（Unix毫秒，0表示不过期）
	RequireAck     bool             `json:"require_ack,omitempty"`      // 是否需要确认
}

// NotifyAckRequest 通知确认请求
type NotifyAckRequest struct {
	NotifyID  string `json:"notify_id"`       // 被确认的通知ID
	Received  bool   `json:"received"`        // 是否成功接收
	Processed bool   `json:"processed"`       // 是否已处理
	Error     string `json:"error,omitempty"` // 处理错误（如有）
}

// NotifyAckResponse 通知确认响应
type NotifyAckResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// C2C 通知（客户端到客户端）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// C2CNotifyRequest 客户端到客户端通知请求
type C2CNotifyRequest struct {
	TargetClientID int64            `json:"target_client_id"`      // 目标客户端ID
	Type           NotificationType `json:"type"`                  // 通知类型
	Payload        string           `json:"payload"`               // JSON编码的载荷
	Priority       NotifyPriority   `json:"priority"`              // 优先级
	ExpireAt       int64            `json:"expire_at,omitempty"`   // 过期时间
	RequireAck     bool             `json:"require_ack,omitempty"` // 是否需要确认
}

// C2CNotifyResponse 客户端到客户端通知响应
type C2CNotifyResponse struct {
	Success  bool   `json:"success"`
	NotifyID string `json:"notify_id,omitempty"` // 分配的通知ID
	Error    string `json:"error,omitempty"`
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 通知载荷类型
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SystemMessagePayload 系统消息载荷
type SystemMessagePayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   string `json:"level"` // info, warning, error
}

// QuotaWarningPayload 配额预警载荷
type QuotaWarningPayload struct {
	QuotaType    string  `json:"quota_type"`    // bandwidth, connections, mappings
	CurrentUsage float64 `json:"current_usage"` // 当前使用量
	Limit        float64 `json:"limit"`         // 限制值
	UsagePercent float64 `json:"usage_percent"` // 使用百分比
	Message      string  `json:"message"`
}

// MappingEventPayload 映射事件载荷
type MappingEventPayload struct {
	MappingID   string `json:"mapping_id"`
	MappingName string `json:"mapping_name,omitempty"`
	Protocol    string `json:"protocol"` // tcp, http, socks5
	SourcePort  int    `json:"source_port"`
	TargetHost  string `json:"target_host"`
	TargetPort  int    `json:"target_port"`
	Status      string `json:"status"`                 // active, inactive, expired
	ByClientID  int64  `json:"by_client_id,omitempty"` // 操作者客户端ID
	Message     string `json:"message,omitempty"`
}

// TunnelClosedPayload 隧道关闭载荷（快速通知）
type TunnelClosedPayload struct {
	TunnelID     string `json:"tunnel_id"`
	MappingID    string `json:"mapping_id"`
	Reason       string `json:"reason"` // normal, error, timeout, peer_closed, migration
	ErrorMessage string `json:"error_message,omitempty"`
	BytesSent    int64  `json:"bytes_sent"`
	BytesRecv    int64  `json:"bytes_recv"`
	Duration     int64  `json:"duration_ms"` // 隧道持续时间（毫秒）
	ClosedAt     int64  `json:"closed_at"`   // 关闭时间戳
}

// TunnelOpenedPayload 隧道打开载荷
type TunnelOpenedPayload struct {
	TunnelID      string `json:"tunnel_id"`
	MappingID     string `json:"mapping_id"`
	PeerClientID  int64  `json:"peer_client_id"`
	LocalEndpoint string `json:"local_endpoint"` // 本地端点
	PeerEndpoint  string `json:"peer_endpoint"`  // 对端端点
	OpenedAt      int64  `json:"opened_at"`
}

// TunnelErrorPayload 隧道错误载荷
type TunnelErrorPayload struct {
	TunnelID     string `json:"tunnel_id"`
	MappingID    string `json:"mapping_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Recoverable  bool   `json:"recoverable"`
	OccurredAt   int64  `json:"occurred_at"`
}

// VersionUpdatePayload 版本更新载荷
type VersionUpdatePayload struct {
	CurrentVersion string   `json:"current_version"`
	NewVersion     string   `json:"new_version"`
	ReleaseNotes   string   `json:"release_notes,omitempty"`
	DownloadURL    string   `json:"download_url,omitempty"`
	Mandatory      bool     `json:"mandatory"`
	Features       []string `json:"features,omitempty"`
}

// CustomPayload 自定义载荷（C2C通知）
type CustomPayload struct {
	Action string            `json:"action"`         // 自定义动作
	Data   map[string]string `json:"data,omitempty"` // 键值对数据
	Raw    string            `json:"raw,omitempty"`  // 原始数据
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 工具函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// NewNotification 创建新通知
func NewNotification(notifyType NotificationType, payload string) *ClientNotification {
	return &ClientNotification{
		Type:      notifyType,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
		Priority:  PriorityNormal,
	}
}

// WithPriority 设置优先级
func (n *ClientNotification) WithPriority(priority NotifyPriority) *ClientNotification {
	n.Priority = priority
	return n
}

// WithExpiry 设置过期时间
func (n *ClientNotification) WithExpiry(expireAt time.Time) *ClientNotification {
	n.ExpireAt = expireAt.UnixMilli()
	return n
}

// WithSender 设置发送者（C2C通知）
func (n *ClientNotification) WithSender(clientID int64) *ClientNotification {
	n.SenderClientID = clientID
	return n
}

// WithAckRequired 设置需要确认
func (n *ClientNotification) WithAckRequired() *ClientNotification {
	n.RequireAck = true
	return n
}

// IsExpired 检查通知是否已过期
func (n *ClientNotification) IsExpired() bool {
	if n.ExpireAt == 0 {
		return false
	}
	return time.Now().UnixMilli() > n.ExpireAt
}

// IsSystemNotification 是否为系统通知
func (t NotificationType) IsSystemNotification() bool {
	return t >= 1 && t < 20
}

// IsMappingNotification 是否为映射通知
func (t NotificationType) IsMappingNotification() bool {
	return t >= 20 && t < 40
}

// IsTunnelNotification 是否为隧道通知
func (t NotificationType) IsTunnelNotification() bool {
	return t >= 40 && t < 60
}

// IsCustomNotification 是否为自定义通知
func (t NotificationType) IsCustomNotification() bool {
	return t >= 100
}

// String 返回通知类型的字符串表示
func (t NotificationType) String() string {
	switch t {
	case NotifyTypeSystemMessage:
		return "SystemMessage"
	case NotifyTypeSystemMaintain:
		return "SystemMaintain"
	case NotifyTypeQuotaWarning:
		return "QuotaWarning"
	case NotifyTypeQuotaExhausted:
		return "QuotaExhausted"
	case NotifyTypeVersionUpdate:
		return "VersionUpdate"
	case NotifyTypeSecurityAlert:
		return "SecurityAlert"
	case NotifyTypeAnnounceBroadcast:
		return "AnnounceBroadcast"
	case NotifyTypeMappingCreated:
		return "MappingCreated"
	case NotifyTypeMappingDeleted:
		return "MappingDeleted"
	case NotifyTypeMappingUpdated:
		return "MappingUpdated"
	case NotifyTypeMappingExpired:
		return "MappingExpired"
	case NotifyTypeMappingActivated:
		return "MappingActivated"
	case NotifyTypeTunnelOpened:
		return "TunnelOpened"
	case NotifyTypeTunnelClosed:
		return "TunnelClosed"
	case NotifyTypeTunnelError:
		return "TunnelError"
	case NotifyTypeTunnelMigrated:
		return "TunnelMigrated"
	case NotifyTypeTunnelSuspended:
		return "TunnelSuspended"
	case NotifyTypeCustom:
		return "Custom"
	default:
		return "Unknown"
	}
}
