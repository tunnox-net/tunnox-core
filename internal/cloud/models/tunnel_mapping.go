package models

import (
	"fmt"
	"time"
)

// TunnelMapping 隧道映射
//
// 由ListenClient使用连接码激活创建，实现端口映射和流量转发。
// 核心特点：
//   - 从TunnelConnectionCode激活创建
//   - 绑定ListenClient和TargetClient（防止劫持）
//   - 长期有效（默认7天）
//   - 可多次使用（直到过期或撤销）
//   - 记录使用统计
//
// 使用流程：
//   1. ListenClient使用TunnelConnectionCode激活创建TunnelMapping
//   2. ListenClient提供本地监听地址（如 0.0.0.0:9999）
//   3. 映射创建后，ListenClient可以多次连接TargetClient的目标地址
//   4. 映射有效期到期或被撤销后失效
type TunnelMapping struct {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 基础信息
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// ID 映射ID，格式：mapping_xxx
	ID string `json:"id"`
	
	// ConnectionCodeID 关联的连接码ID
	// 用于追溯此映射是通过哪个连接码创建的
	ConnectionCodeID string `json:"connection_code_id"`
	
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 映射双方
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// ListenClientID 监听端客户端ID
	// 使用连接码创建映射的客户端（访问方）
	ListenClientID int64 `json:"listen_client_id"`
	
	// TargetClientID 目标端客户端ID
	// 生成连接码的客户端（被访问方）
	TargetClientID int64 `json:"target_client_id"`
	
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 地址信息
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// ListenAddress ListenClient的监听地址
	// 格式：0.0.0.0:9999 或 127.0.0.1:9999
	// ListenClient在此地址监听，接收外部连接
	ListenAddress string `json:"listen_address"`
	
	// TargetAddress TargetClient的目标地址
	// 格式：tcp://192.168.100.10:8888
	// 从TunnelConnectionCode继承，流量最终转发到此地址
	TargetAddress string `json:"target_address"`
	
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 时限控制
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// CreatedAt 创建时间（激活时间）
	CreatedAt time.Time `json:"created_at"`
	
	// ExpiresAt 过期时间
	// = CreatedAt + Duration
	ExpiresAt time.Time `json:"expires_at"`
	
	// Duration 映射有效期
	// 从TunnelConnectionCode的MappingDuration继承
	Duration time.Duration `json:"duration"`
	
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 管理信息
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// CreatedBy 创建者（通常是ListenClientID的字符串表示）
	CreatedBy string `json:"created_by"`
	
	// IsRevoked 是否已撤销
	// TargetClient或ListenClient都可以撤销映射
	IsRevoked bool `json:"is_revoked"`
	
	// RevokedAt 撤销时间
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	
	// RevokedBy 撤销者
	RevokedBy string `json:"revoked_by,omitempty"`
	
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 使用统计
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// LastUsedAt 最后使用时间
	// 每次建立隧道连接时更新
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	
	// UsageCount 使用次数（连接数）
	// 累计通过此映射建立的隧道连接次数
	UsageCount int64 `json:"usage_count"`
	
	// BytesSent 发送字节数
	// ListenClient → TargetClient 的总字节数
	BytesSent int64 `json:"bytes_sent"`
	
	// BytesReceived 接收字节数
	// TargetClient → ListenClient 的总字节数
	BytesReceived int64 `json:"bytes_received"`
	
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 元数据
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	
	// Description 描述
	// 从TunnelConnectionCode继承
	Description string `json:"description,omitempty"`
}

// IsExpired 检查映射是否已过期
func (m *TunnelMapping) IsExpired() bool {
	return time.Now().After(m.ExpiresAt)
}

// IsValid 检查映射是否有效
//
// 有效条件：
//   - 未被撤销
//   - 未过期
func (m *TunnelMapping) IsValid() bool {
	if m.IsRevoked {
		return false
	}
	if m.IsExpired() {
		return false
	}
	return true
}

// CanBeAccessedBy 检查是否允许指定客户端访问
//
// 只有ListenClient可以使用此映射
// 防止映射被其他客户端劫持
func (m *TunnelMapping) CanBeAccessedBy(clientID int64) bool {
	if !m.IsValid() {
		return false
	}
	
	// 只有ListenClient可以使用此映射
	return m.ListenClientID == clientID
}

// CanBeRevokedBy 检查是否允许指定客户端撤销
//
// TargetClient和ListenClient都可以撤销映射
func (m *TunnelMapping) CanBeRevokedBy(clientID int64) bool {
	return m.ListenClientID == clientID || m.TargetClientID == clientID
}

// Validate 验证映射数据的完整性
//
// 在创建时调用，确保必填字段都已填写
func (m *TunnelMapping) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("mapping ID is required")
	}
	if m.ConnectionCodeID == "" {
		return fmt.Errorf("connection code ID is required")
	}
	if m.ListenClientID == 0 {
		return fmt.Errorf("listen client ID is required")
	}
	if m.TargetClientID == 0 {
		return fmt.Errorf("target client ID is required")
	}
	if m.ListenAddress == "" {
		return fmt.Errorf("listen address is required")
	}
	if m.TargetAddress == "" {
		return fmt.Errorf("target address is required")
	}
	if m.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	
	// 防止自己访问自己
	if m.ListenClientID == m.TargetClientID {
		return fmt.Errorf("listen client and target client cannot be the same")
	}
	
	return nil
}

// Revoke 撤销映射
//
// TargetClient或ListenClient都可以撤销
func (m *TunnelMapping) Revoke(revokedBy string, clientID int64) error {
	if !m.CanBeRevokedBy(clientID) {
		return fmt.Errorf("client %d is not allowed to revoke this mapping", clientID)
	}
	if m.IsRevoked {
		return fmt.Errorf("mapping already revoked")
	}
	
	now := time.Now()
	m.IsRevoked = true
	m.RevokedAt = &now
	m.RevokedBy = revokedBy
	
	return nil
}

// RecordUsage 记录一次使用
//
// 在每次建立隧道连接时调用
// 更新使用次数和最后使用时间
func (m *TunnelMapping) RecordUsage() {
	now := time.Now()
	m.LastUsedAt = &now
	m.UsageCount++
}

// UpdateTraffic 更新流量统计
//
// 在隧道连接关闭时调用
// 累加发送和接收的字节数
func (m *TunnelMapping) UpdateTraffic(bytesSent, bytesReceived int64) {
	m.BytesSent += bytesSent
	m.BytesReceived += bytesReceived
}

// TimeRemaining 返回距离过期还有多长时间
//
// 返回值：
//   - > 0: 剩余时间
//   - <= 0: 已过期
func (m *TunnelMapping) TimeRemaining() time.Duration {
	return time.Until(m.ExpiresAt)
}

// Status 返回映射的当前状态
//
// 状态优先级：
//   - revoked（已撤销）
//   - expired（已过期）
//   - active（活跃）
func (m *TunnelMapping) Status() string {
	if m.IsRevoked {
		return "revoked"
	}
	if m.IsExpired() {
		return "expired"
	}
	return "active"
}

// TotalBytes 返回总流量（发送+接收）
func (m *TunnelMapping) TotalBytes() int64 {
	return m.BytesSent + m.BytesReceived
}

