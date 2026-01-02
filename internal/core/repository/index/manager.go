// Package index 提供索引管理功能
package index

import (
	"context"

	"tunnox-core/internal/core/repository"
)

// =============================================================================
// IndexManager 接口定义
// =============================================================================

// IndexManager 索引管理器接口
// 负责维护实体的二级索引，如 用户ID -> 实体ID 列表
type IndexManager[E repository.Entity] interface {
	// AddIndex 为实体添加索引
	AddIndex(ctx context.Context, entity E) error

	// RemoveIndex 移除实体的索引
	RemoveIndex(ctx context.Context, entity E) error

	// UpdateIndex 更新实体的索引（处理索引键变化的情况）
	// oldEntity 是更新前的实体，newEntity 是更新后的实体
	UpdateIndex(ctx context.Context, oldEntity, newEntity E) error

	// GetEntityIDs 根据索引类型和值获取实体 ID 列表
	// indexType 是索引类型（如 "user"）
	// indexValue 是索引值（如 用户ID）
	GetEntityIDs(ctx context.Context, indexType string, indexValue string) ([]string, error)

	// RebuildIndex 重建所有索引
	// 接收所有实体列表，清除现有索引后重新构建
	RebuildIndex(ctx context.Context, entities []E) error

	// VerifyIndex 校验索引一致性
	// 返回发现的不一致记录
	VerifyIndex(ctx context.Context) ([]IndexInconsistency, error)
}

// IndexInconsistency 索引不一致记录
type IndexInconsistency struct {
	// IndexKey 索引键
	IndexKey string

	// EntityID 相关实体 ID
	EntityID string

	// Type 不一致类型
	Type InconsistencyType

	// Description 描述信息
	Description string
}

// InconsistencyType 不一致类型
type InconsistencyType string

const (
	// InconsistencyMissingIndex 实体存在但索引缺失
	InconsistencyMissingIndex InconsistencyType = "missing_index"

	// InconsistencyOrphanIndex 索引存在但实体不存在
	InconsistencyOrphanIndex InconsistencyType = "orphan_index"

	// InconsistencyStaleIndex 索引指向的实体数据已变更
	InconsistencyStaleIndex InconsistencyType = "stale_index"
)

// =============================================================================
// 索引键生成器
// =============================================================================

// IndexKeyBuilder 索引键构建器
type IndexKeyBuilder interface {
	// BuildIndexKey 构建索引键
	// indexType 是索引类型，indexValue 是索引值
	BuildIndexKey(indexType string, indexValue string) string
}

// DefaultIndexKeyBuilder 默认索引键构建器
type DefaultIndexKeyBuilder struct {
	Prefix string
}

// BuildIndexKey 构建索引键
func (b *DefaultIndexKeyBuilder) BuildIndexKey(indexType string, indexValue string) string {
	return b.Prefix + indexType + ":" + indexValue
}

// =============================================================================
// NullIndexManager 空实现（用于不需要索引的场景）
// =============================================================================

// NullIndexManager 空索引管理器
// 所有操作都是空操作，用于不需要索引的 Repository
type NullIndexManager[E repository.Entity] struct{}

// AddIndex 空操作
func (m *NullIndexManager[E]) AddIndex(ctx context.Context, entity E) error {
	return nil
}

// RemoveIndex 空操作
func (m *NullIndexManager[E]) RemoveIndex(ctx context.Context, entity E) error {
	return nil
}

// UpdateIndex 空操作
func (m *NullIndexManager[E]) UpdateIndex(ctx context.Context, oldEntity, newEntity E) error {
	return nil
}

// GetEntityIDs 返回空列表
func (m *NullIndexManager[E]) GetEntityIDs(ctx context.Context, indexType string, indexValue string) ([]string, error) {
	return []string{}, nil
}

// RebuildIndex 空操作
func (m *NullIndexManager[E]) RebuildIndex(ctx context.Context, entities []E) error {
	return nil
}

// VerifyIndex 返回空列表
func (m *NullIndexManager[E]) VerifyIndex(ctx context.Context) ([]IndexInconsistency, error) {
	return []IndexInconsistency{}, nil
}
