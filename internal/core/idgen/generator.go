package idgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils"
)

// NOTE: mapping 连接实例的 ID 实现仍需完善

// 错误定义
var (
	ErrIDExhausted = errors.New("ID exhausted")
	ErrInvalidID   = errors.New("invalid ID")
)

// 常量定义
const (
	// ID生成相关常量
	ClientIDMin    = int64(10000000)
	ClientIDMax    = int64(99999999)
	ClientIDLength = 8

	// 统一格式的随机部分长度
	RandomPartLength = 8

	// ID类型前缀
	PrefixNodeID                = "node_"
	PrefixConnectionID          = "conn_"
	PrefixPortMappingID         = "pmap_"
	PrefixPortMappingInstanceID = "pmi_"
	PrefixUserID                = "user_"
	PrefixTunnelID              = "tun_"

	MaxAttempts = 100
)

// IDGenerator 泛型ID生成器接口
type IDGenerator[T any] interface {
	Generate() (T, error)
	Release(id T) error
	IsUsed(id T) (bool, error)
	GetUsedCount() int
	Close() error
}

// StorageIDGenerator 基于Storage的泛型ID生成器 (Renamed from StorageBasedIDGenerator)
type StorageIDGenerator[T any] struct {
	storage   storage.Storage
	prefix    string
	keyPrefix string
	mu        sync.RWMutex
	dispose.Dispose
}

// NewStorageIDGenerator 创建基于Storage的ID生成器 (Renamed from NewStorageBasedIDGenerator)
func NewStorageIDGenerator[T any](storage storage.Storage, prefix, keyPrefix string, parentCtx context.Context) *StorageIDGenerator[T] {
	generator := &StorageIDGenerator[T]{
		storage:   storage,
		prefix:    prefix,
		keyPrefix: keyPrefix,
	}
	generator.SetCtxWithNoOpOnClose(parentCtx)
	return generator
}

// getKey 生成存储键
func (g *StorageIDGenerator[T]) getKey(id T) string {
	return fmt.Sprintf("%s:%v", g.keyPrefix, id)
}

// Generate 生成ID
// 使用 SetNX 原子操作避免竞态条件：高并发时 IsUsed 检查和 markAsUsed 标记之间可能有其他协程抢占
func (g *StorageIDGenerator[T]) Generate() (T, error) {
	var zero T

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		// 生成候选ID
		var candidate T

		switch any(zero).(type) {
		case string:
			// 生成随机字符串
			orderedStr, err := utils.GenerateRandomString(RandomPartLength)
			if err != nil {
				continue
			}
			// 添加前缀
			if g.prefix != "" {
				orderedStr = g.prefix + orderedStr
			}
			candidate = any(orderedStr).(T)

		case int64:
			// 生成随机 int64（用于 ClientID）
			// 在指定范围内生成完全随机的 ID
			randomID, err := utils.GenerateRandomInt64(ClientIDMin, ClientIDMax)
			if err != nil {
				continue
			}
			candidate = any(randomID).(T)

		default:
			return zero, coreerrors.Newf(coreerrors.CodeInvalidParam, "unsupported ID type: %T", zero)
		}

		// 使用原子操作标记ID为已使用
		// SetNX 保证 check 和 set 是原子的，避免竞态条件
		success, err := g.tryMarkAsUsed(candidate)
		if err != nil {
			corelog.Warnf("IDGenerator: tryMarkAsUsed failed for %v: %v", candidate, err)
			continue
		}

		if success {
			return candidate, nil
		}
		// ID 已被其他协程占用，重试生成新 ID
	}

	return zero, ErrIDExhausted
}

// Release 释放ID
func (g *StorageIDGenerator[T]) Release(id T) error {
	key := g.getKey(id)
	return g.storage.Delete(key)
}

// IsUsed 检查ID是否已使用
func (g *StorageIDGenerator[T]) IsUsed(id T) (bool, error) {
	key := g.getKey(id)
	return g.storage.Exists(key)
}

// tryMarkAsUsed 尝试原子标记ID为已使用
// 返回 (true, nil) 表示成功标记，(false, nil) 表示 ID 已被占用，(false, err) 表示操作失败
func (g *StorageIDGenerator[T]) tryMarkAsUsed(id T) (bool, error) {
	key := g.getKey(id)
	info := &IDUsageInfo{
		ID:        fmt.Sprintf("%v", id),
		Type:      g.keyPrefix,
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return false, err
	}

	// 检查 storage 是否支持 CASStore 接口（SetNX 原子操作）
	if casStore, ok := g.storage.(storage.CASStore); ok {
		// 使用 SetNX 原子操作：仅当 key 不存在时设置成功
		return casStore.SetNX(key, string(data), 0)
	}

	// 回退到非原子操作（单节点内存存储场景，有锁保护）
	// 注意：这种方式在分布式场景下仍有竞态风险，但内存存储通常是单节点的
	g.mu.Lock()
	defer g.mu.Unlock()

	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	if err := g.storage.Set(key, string(data), 0); err != nil {
		return false, err
	}
	return true, nil
}

// markAsUsed 标记ID为已使用（保留用于兼容性）
// 注意：此方法不是原子操作，建议使用 tryMarkAsUsed
func (g *StorageIDGenerator[T]) markAsUsed(id T) error {
	success, err := g.tryMarkAsUsed(id)
	if err != nil {
		return err
	}
	if !success {
		return coreerrors.Newf(coreerrors.CodeConflict, "ID %v already in use", id)
	}
	return nil
}

// GetUsedCount 获取已使用的ID数量（简化实现）
func (g *StorageIDGenerator[T]) GetUsedCount() int {
	// 这里可以实现更复杂的统计逻辑
	// 目前返回-1表示不支持此操作
	return -1
}

// Close 关闭生成器
func (g *StorageIDGenerator[T]) Close() error {
	g.Dispose.Close()
	return nil
}

// ClientIDGenerator 已废弃，现在使用 StorageIDGenerator[int64]
// 为保持兼容性，这里保留类型别名
type ClientIDGenerator = StorageIDGenerator[int64]

// NewClientIDGenerator 创建客户端ID生成器
// 现在统一使用 StorageIDGenerator[int64]，生成完全随机的 ClientID
func NewClientIDGenerator(storage storage.Storage, parentCtx context.Context) *ClientIDGenerator {
	return NewStorageIDGenerator[int64](
		storage,
		"",                      // 无前缀，直接生成数字
		"tunnox:id:used:client", // 存储键前缀
		parentCtx,
	)
}

// IDUsageInfo ID使用信息
type IDUsageInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}
