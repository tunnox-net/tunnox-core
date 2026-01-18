package idgen

import (
	"github.com/google/uuid"
)

// UUIDGenerator 基于 UUID v7 的 ID 生成器
// 不需要 Redis 存储来跟踪已使用的 ID，因为 UUID v7 有 122 位随机性，冲突概率可忽略
// 这解决了 Connection ID 永不过期导致的 Redis 内存泄漏问题
type UUIDGenerator struct {
	prefix string
}

// NewUUIDGenerator 创建 UUID 生成器
func NewUUIDGenerator(prefix string) *UUIDGenerator {
	return &UUIDGenerator{
		prefix: prefix,
	}
}

// Generate 生成唯一 ID
// UUID v7 是时间有序的，适合作为数据库主键和排序
func (g *UUIDGenerator) Generate() (string, error) {
	// 使用 UUID v7（时间有序 + 随机）
	id, err := uuid.NewV7()
	if err != nil {
		// 回退到 UUID v4
		id = uuid.New()
	}

	if g.prefix != "" {
		return g.prefix + id.String(), nil
	}
	return id.String(), nil
}

// Release 释放 ID（UUID 不需要释放，此方法为接口兼容）
func (g *UUIDGenerator) Release(id string) error {
	// UUID 不需要释放，因为不跟踪已使用的 ID
	return nil
}

// IsUsed 检查 ID 是否已使用（UUID 总是返回 false）
func (g *UUIDGenerator) IsUsed(id string) (bool, error) {
	// UUID 的冲突概率可忽略，总是返回 false
	return false, nil
}

// GetUsedCount 获取已使用的 ID 数量
func (g *UUIDGenerator) GetUsedCount() int {
	// UUID 不跟踪已使用的 ID
	return 0
}

// Close 关闭生成器
func (g *UUIDGenerator) Close() error {
	return nil
}

// 编译时接口断言
var _ IDGenerator[string] = (*UUIDGenerator)(nil)
