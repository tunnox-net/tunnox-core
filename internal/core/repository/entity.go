// Package repository 提供 Repository 抽象层
//
// Repository 层位于 Store 层之上，提供业务实体的持久化操作。
// 主要职责：
//   - 实体的 CRUD 操作
//   - 索引管理（如用户→客户端映射）
//   - 缓存策略
//   - 批量查询优化
package repository

import (
	"strconv"
)

// =============================================================================
// Entity 接口定义
// =============================================================================

// Entity 实体接口
// 所有需要持久化的业务实体必须实现此接口
// 设计决策：统一使用 string 类型的 ID，简化泛型约束
type Entity interface {
	// GetID 返回实体的唯一标识符
	// ID 必须在整个存储范围内唯一
	GetID() string
}

// =============================================================================
// ID 转换器
// =============================================================================

// IDConverter ID 转换器接口
// 用于在业务层使用的 ID 类型（如 int64）和存储层使用的 string 之间转换
type IDConverter[ID any] interface {
	// ToString 将 ID 转换为 string
	ToString(id ID) string

	// FromString 将 string 转换为 ID
	// 转换失败时返回错误
	FromString(s string) (ID, error)
}

// Int64IDConverter int64 ID 转换器
type Int64IDConverter struct{}

// ToString 将 int64 转换为 string
func (c Int64IDConverter) ToString(id int64) string {
	return strconv.FormatInt(id, 10)
}

// FromString 将 string 转换为 int64
func (c Int64IDConverter) FromString(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// Uint64IDConverter uint64 ID 转换器
type Uint64IDConverter struct{}

// ToString 将 uint64 转换为 string
func (c Uint64IDConverter) ToString(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// FromString 将 string 转换为 uint64
func (c Uint64IDConverter) FromString(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

// StringIDConverter string ID 转换器（用于已经是 string 的 ID）
type StringIDConverter struct{}

// ToString 返回原始 string
func (c StringIDConverter) ToString(id string) string {
	return id
}

// FromString 返回原始 string
func (c StringIDConverter) FromString(s string) (string, error) {
	return s, nil
}

// =============================================================================
// 用户关联实体接口
// =============================================================================

// UserOwnedEntity 用户拥有的实体接口
// 用于支持 "按用户查询" 的实体，如 ClientConfig、PortMapping 等
type UserOwnedEntity interface {
	Entity

	// GetUserID 返回实体所属的用户 ID
	// 如果实体不属于任何用户（如匿名客户端），返回空字符串
	GetUserID() string
}

// =============================================================================
// 辅助函数
// =============================================================================

// GetUserIDFunc 获取用户 ID 的函数类型
// 用于 IndexManager 配置
type GetUserIDFunc[E Entity] func(entity E) string

// NewGetUserIDFunc 创建获取用户 ID 的函数
// 如果实体实现了 UserOwnedEntity，则返回其 GetUserID 方法
// 否则返回 nil（需要手动配置）
func NewGetUserIDFunc[E Entity]() GetUserIDFunc[E] {
	return func(entity E) string {
		if uo, ok := any(entity).(UserOwnedEntity); ok {
			return uo.GetUserID()
		}
		return ""
	}
}
