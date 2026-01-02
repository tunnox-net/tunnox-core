package repository

import (
	"context"
)

// =============================================================================
// Repository 接口定义
// =============================================================================

// Repository 基础 Repository 接口
type Repository[E Entity] interface {
	// Get 根据 ID 获取实体
	Get(ctx context.Context, id string) (E, error)

	// Create 创建实体
	Create(ctx context.Context, entity E) error

	// Update 更新实体
	Update(ctx context.Context, entity E) error

	// Delete 删除实体
	Delete(ctx context.Context, id string) error

	// Exists 检查实体是否存在
	Exists(ctx context.Context, id string) (bool, error)
}

// BatchRepository 支持批量操作的 Repository
type BatchRepository[E Entity] interface {
	Repository[E]

	// BatchGet 批量获取实体
	BatchGet(ctx context.Context, ids []string) (map[string]E, error)

	// BatchCreate 批量创建实体
	BatchCreate(ctx context.Context, entities []E) error

	// BatchDelete 批量删除实体
	BatchDelete(ctx context.Context, ids []string) error
}

// ListableRepository 支持列表查询的 Repository
type ListableRepository[E Entity] interface {
	Repository[E]

	// List 列出所有实体
	List(ctx context.Context) ([]E, error)

	// Count 统计实体数量
	Count(ctx context.Context) (int64, error)
}

// UserIndexedRepository 支持用户索引的 Repository
type UserIndexedRepository[E Entity] interface {
	Repository[E]
	BatchRepository[E]

	// ListByUser 根据用户 ID 列出实体
	ListByUser(ctx context.Context, userID string) ([]E, error)

	// CountByUser 统计用户拥有的实体数量
	CountByUser(ctx context.Context, userID string) (int64, error)
}

// =============================================================================
// Repository 错误定义
// =============================================================================

// RepositoryError Repository 操作错误
type RepositoryError struct {
	Op         string // 操作名称
	EntityType string // 实体类型
	EntityID   string // 实体 ID
	Err        error  // 原始错误
}

func (e *RepositoryError) Error() string {
	if e.EntityID != "" {
		return e.EntityType + "." + e.Op + " id=" + e.EntityID + ": " + e.Err.Error()
	}
	return e.EntityType + "." + e.Op + ": " + e.Err.Error()
}

func (e *RepositoryError) Unwrap() error {
	return e.Err
}

// NewRepositoryError 创建 Repository 错误
func NewRepositoryError(entityType, op, entityID string, err error) *RepositoryError {
	return &RepositoryError{
		Op:         op,
		EntityType: entityType,
		EntityID:   entityID,
		Err:        err,
	}
}
