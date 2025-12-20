package models

import (
	"fmt"
	"time"
	corelog "tunnox-core/internal/core/log"
)

// IsExpired 检查映射是否已过期
func (m *PortMapping) IsExpired() bool {
	if m.ExpiresAt == nil {
		return false // 没有过期时间，视为永久有效
	}
	return time.Now().After(*m.ExpiresAt)
}

// IsValid 检查映射是否有效
//
// 有效条件：
//   - 未被撤销
//   - 未过期
//   - 状态为活跃
func (m *PortMapping) IsValid() bool {
	if m.IsRevoked {
		return false
	}
	if m.IsExpired() {
		return false
	}
	if m.Status != MappingStatusActive {
		return false
	}
	return true
}

// CanBeAccessedBy 检查是否允许指定客户端访问
//
// 只有 ListenClient 可以使用此映射
// 防止映射被其他客户端劫持
func (m *PortMapping) CanBeAccessedBy(clientID int64) bool {
	if !m.IsValid() {
		corelog.Debugf("PortMapping.CanBeAccessedBy: IsValid() returned false for mappingID=%s, Status=%s, IsRevoked=%v, IsExpired=%v",
			m.ID, m.Status, m.IsRevoked, m.IsExpired())
		return false
	}

	// 只有 ListenClient 可以使用此映射
	result := m.ListenClientID == clientID
	if !result {
		corelog.Debugf("PortMapping.CanBeAccessedBy: clientID mismatch - mappingID=%s, listenClientID=%d, clientID=%d",
			m.ID, m.ListenClientID, clientID)
	}
	return result
}

// CanBeRevokedBy 检查是否允许指定客户端撤销
//
// TargetClient 和 ListenClient 都可以撤销映射
func (m *PortMapping) CanBeRevokedBy(clientID int64) bool {
	return m.ListenClientID == clientID || m.TargetClientID == clientID
}

// Revoke 撤销映射
//
// TargetClient 或 ListenClient 都可以撤销
func (m *PortMapping) Revoke(revokedBy string, clientID int64) error {
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
	m.Status = MappingStatusInactive

	return nil
}

// TimeRemaining 返回距离过期还有多长时间
//
// 返回值：
//   - > 0: 剩余时间
//   - <= 0: 已过期或没有过期时间
func (m *PortMapping) TimeRemaining() time.Duration {
	if m.ExpiresAt == nil {
		return time.Duration(0) // 没有过期时间
	}
	return time.Until(*m.ExpiresAt)
}
