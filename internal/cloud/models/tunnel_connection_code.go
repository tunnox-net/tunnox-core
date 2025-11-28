package models

import (
	"fmt"
	"time"
)

// TunnelConnectionCode 隧道连接码
//
// 由TargetClient生成，用于授权任意客户端建立隧道映射。
// 核心特点：
//   - 全局唯一（无需预先绑定ListenClient）
//   - 一次性使用（使用后立即失效）
//   - 短期有效（默认10分钟激活期）
//   - 必须包含目标地址
//
// 使用流程：
//  1. TargetClient生成连接码，指定目标地址（如 tcp://192.168.100.10:8888）
//  2. 通过安全渠道分享连接码给需要访问的ListenClient
//  3. ListenClient在激活期内使用连接码创建PortMapping
//  4. 连接码标记为已使用，不能再次使用
type TunnelConnectionCode struct {
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 基础信息
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// ID 连接码ID，格式：conncode_xxx
	ID string `json:"id"`

	// Code 好记的连接码，格式：abc-def-123
	// 用于安全分享，方便口头或文字传递
	Code string `json:"code"`

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 目标信息（必填）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// TargetClientID 生成连接码的客户端（被访问方）
	TargetClientID int64 `json:"target_client_id"`

	// TargetAddress 目标地址（必填）
	// 格式：tcp://192.168.100.10:8888
	// 防止连接码被滥用访问其他服务
	TargetAddress string `json:"target_address"`

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 时限控制
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// ActivationTTL 激活有效期（默认10分钟）
	// 连接码在此期间内可以被激活
	ActivationTTL time.Duration `json:"activation_ttl"`

	// MappingDuration 映射有效期（默认7天）
	// 激活后创建的PortMapping的有效期
	MappingDuration time.Duration `json:"mapping_duration"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`

	// ActivationExpiresAt 激活过期时间
	// = CreatedAt + ActivationTTL
	ActivationExpiresAt time.Time `json:"activation_expires_at"`

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 使用控制（一次性）
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// IsActivated 是否已被激活（一次性使用）
	IsActivated bool `json:"is_activated"`

	// ActivatedAt 激活时间
	ActivatedAt *time.Time `json:"activated_at,omitempty"`

	// ActivatedBy 激活者的ClientID
	// 只有激活后才知道是哪个ListenClient使用了此连接码
	ActivatedBy *int64 `json:"activated_by,omitempty"`

	// MappingID 激活后创建的映射ID
	MappingID *string `json:"mapping_id,omitempty"`

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 管理信息
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// CreatedBy 创建者（UserID或ClientID的字符串表示）
	CreatedBy string `json:"created_by"`

	// IsRevoked 是否已撤销
	// 未使用的连接码可以被主动撤销
	IsRevoked bool `json:"is_revoked"`

	// RevokedAt 撤销时间
	RevokedAt *time.Time `json:"revoked_at,omitempty"`

	// RevokedBy 撤销者
	RevokedBy string `json:"revoked_by,omitempty"`

	// Description 描述（可选）
	// 如："临时数据库访问"、"项目合作"等
	Description string `json:"description,omitempty"`
}

// IsExpired 检查连接码是否已过期（激活期）
func (c *TunnelConnectionCode) IsExpired() bool {
	return time.Now().After(c.ActivationExpiresAt)
}

// IsValidForActivation 检查连接码是否可用于激活
//
// 有效条件：
//   - 未被撤销
//   - 未被激活（一次性使用）
//   - 未过期（在激活期内）
func (c *TunnelConnectionCode) IsValidForActivation() bool {
	if c.IsRevoked {
		return false
	}
	if c.IsActivated {
		return false
	}
	if c.IsExpired() {
		return false
	}
	return true
}

// CanBeActivatedBy 检查是否可被指定客户端激活
//
// 新设计：任何客户端都可以使用连接码（无ClientID绑定）
// 安全性通过以下机制保障：
//   - 连接码的全局唯一性
//   - 一次性使用
//   - 短期有效期
func (c *TunnelConnectionCode) CanBeActivatedBy(listenClientID int64) bool {
	// 基本有效性检查
	if !c.IsValidForActivation() {
		return false
	}

	// ⭐ 关键简化：不再检查ClientID绑定
	// 任何知道连接码的客户端都可以使用

	return true
}

// Validate 验证连接码数据的完整性
//
// 在创建时调用，确保必填字段都已填写
func (c *TunnelConnectionCode) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("connection code ID is required")
	}
	if c.Code == "" {
		return fmt.Errorf("connection code is required")
	}
	if c.TargetClientID == 0 {
		return fmt.Errorf("target client ID is required")
	}
	if c.TargetAddress == "" {
		return fmt.Errorf("target address is required")
	}
	if c.ActivationTTL <= 0 {
		return fmt.Errorf("activation TTL must be positive")
	}
	if c.MappingDuration <= 0 {
		return fmt.Errorf("mapping duration must be positive")
	}
	return nil
}

// Activate 激活连接码
//
// 标记为已激活，记录激活者和激活时间
// 此方法应该在原子性操作中调用，确保一次性使用
func (c *TunnelConnectionCode) Activate(listenClientID int64, mappingID string) error {
	if !c.CanBeActivatedBy(listenClientID) {
		return fmt.Errorf("connection code cannot be activated")
	}

	now := time.Now()
	c.IsActivated = true
	c.ActivatedAt = &now
	c.ActivatedBy = &listenClientID
	c.MappingID = &mappingID

	return nil
}

// Revoke 撤销连接码
//
// 只能撤销未使用的连接码
func (c *TunnelConnectionCode) Revoke(revokedBy string) error {
	if c.IsActivated {
		return fmt.Errorf("cannot revoke activated connection code")
	}
	if c.IsRevoked {
		return fmt.Errorf("connection code already revoked")
	}

	now := time.Now()
	c.IsRevoked = true
	c.RevokedAt = &now
	c.RevokedBy = revokedBy

	return nil
}

// TimeRemaining 返回距离激活期结束还有多长时间
//
// 返回值：
//   - > 0: 剩余时间
//   - <= 0: 已过期
func (c *TunnelConnectionCode) TimeRemaining() time.Duration {
	return time.Until(c.ActivationExpiresAt)
}

// Status 返回连接码的当前状态
//
// 状态优先级：
//   - revoked（已撤销）
//   - activated（已使用）
//   - expired（已过期）
//   - active（活跃）
func (c *TunnelConnectionCode) Status() string {
	if c.IsRevoked {
		return "revoked"
	}
	if c.IsActivated {
		return "activated"
	}
	if c.IsExpired() {
		return "expired"
	}
	return "active"
}
