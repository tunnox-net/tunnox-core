package models

import (
	"time"
)

// ModelUtils 模型工具，提供通用的模型操作
type ModelUtils struct{}

// NewModelUtils 创建模型工具实例
func NewModelUtils() *ModelUtils {
	return &ModelUtils{}
}

// SetCreatedTimestamps 设置创建时间戳
func (m *ModelUtils) SetCreatedTimestamps(createdAt, updatedAt *time.Time) {
	now := time.Now()
	if createdAt != nil {
		*createdAt = now
	}
	if updatedAt != nil {
		*updatedAt = now
	}
}

// SetUpdatedTimestamp 设置更新时间戳
func (m *ModelUtils) SetUpdatedTimestamp(updatedAt *time.Time) {
	if updatedAt != nil {
		*updatedAt = time.Now()
	}
}

// SetActivityTimestamps 设置活动时间戳
func (m *ModelUtils) SetActivityTimestamps(lastActivity, updatedAt *time.Time) {
	now := time.Now()
	if lastActivity != nil {
		*lastActivity = now
	}
	if updatedAt != nil {
		*updatedAt = now
	}
}

// SetHeartbeatTimestamp 设置心跳时间戳
func (m *ModelUtils) SetHeartbeatTimestamp(lastHeartbeat, updatedAt *time.Time) {
	now := time.Now()
	if lastHeartbeat != nil {
		*lastHeartbeat = now
	}
	if updatedAt != nil {
		*updatedAt = now
	}
}

// SetConnectionTimestamps 设置连接时间戳
func (m *ModelUtils) SetConnectionTimestamps(establishedAt, lastActivity, updatedAt *time.Time) {
	now := time.Now()
	if establishedAt != nil {
		*establishedAt = now
	}
	if lastActivity != nil {
		*lastActivity = now
	}
	if updatedAt != nil {
		*updatedAt = now
	}
}

// SetTrafficStatsTimestamp 设置流量统计时间戳
func (m *ModelUtils) SetTrafficStatsTimestamp(lastUpdated *time.Time) {
	if lastUpdated != nil {
		*lastUpdated = time.Now()
	}
}
