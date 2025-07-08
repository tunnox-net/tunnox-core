package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/constants"

	"tunnox-core/internal/utils"
)

// DistributedIDGenerator 分布式ID生成器
type DistributedIDGenerator struct {
	storage storages.Storage
	lock    DistributedLock
	// 优化的客户端ID生成器
	optimizedClientIDGen *generators.OptimizedClientIDGenerator
}

// NewDistributedIDGenerator 创建分布式ID生成器
func NewDistributedIDGenerator(storage storages.Storage, lock DistributedLock) *DistributedIDGenerator {
	generator := &DistributedIDGenerator{
		storage: storage,
		lock:    lock,
	}

	// 初始化优化的客户端ID生成器
	generator.optimizedClientIDGen = generators.NewOptimizedClientIDGenerator(storage, context.Background())

	return generator
}

// GenerateClientID 生成客户端ID（使用优化的分段位图算法）
func (g *DistributedIDGenerator) GenerateClientID(ctx context.Context) (int64, error) {
	// 直接使用优化的ID生成器，无需分布式锁
	// 因为优化生成器内部已经处理了并发安全
	return g.optimizedClientIDGen.GenerateClientID()
}

// GenerateNodeID 生成节点ID
func (g *DistributedIDGenerator) GenerateNodeID(ctx context.Context) (string, error) {
	lockKey := "lock:generate_node_id"

	// 获取分布式锁
	acquired, err := g.lock.Acquire(lockKey, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("acquire lock failed: %w", err)
	}
	if !acquired {
		return "", fmt.Errorf("failed to acquire lock for ID generation")
	}
	defer g.lock.Release(lockKey)

	for attempts := 0; attempts < generators.MaxAttempts; attempts++ {
		nodeID, err := utils.GenerateRandomString(generators.NodeIDLength)
		if err != nil {
			return "", err
		}

		// 检查ID是否已被使用
		used, err := g.isNodeIDUsed(ctx, nodeID)
		if err != nil {
			return "", err
		}

		if !used {
			// 标记ID为已使用
			if err := g.markNodeIDAsUsed(ctx, nodeID); err != nil {
				return "", err
			}
			return nodeID, nil
		}
	}

	return "", generators.ErrIDExhausted
}

// GenerateUserID 生成用户ID
func (g *DistributedIDGenerator) GenerateUserID(ctx context.Context) (string, error) {
	lockKey := "lock:generate_user_id"

	// 获取分布式锁
	acquired, err := g.lock.Acquire(lockKey, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("acquire lock failed: %w", err)
	}
	if !acquired {
		return "", fmt.Errorf("failed to acquire lock for ID generation")
	}
	defer g.lock.Release(lockKey)

	for attempts := 0; attempts < generators.MaxAttempts; attempts++ {
		userID, err := utils.GenerateRandomString(generators.UserIDLength)
		if err != nil {
			return "", err
		}

		// 检查ID是否已被使用
		used, err := g.isUserIDUsed(ctx, userID)
		if err != nil {
			return "", err
		}

		if !used {
			// 标记ID为已使用
			if err := g.markUserIDAsUsed(ctx, userID); err != nil {
				return "", err
			}
			return userID, nil
		}
	}

	return "", generators.ErrIDExhausted
}

// GenerateMappingID 生成端口映射ID
func (g *DistributedIDGenerator) GenerateMappingID(ctx context.Context) (string, error) {
	lockKey := "lock:generate_mapping_id"

	// 获取分布式锁
	acquired, err := g.lock.Acquire(lockKey, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("acquire lock failed: %w", err)
	}
	if !acquired {
		return "", fmt.Errorf("failed to acquire lock for ID generation")
	}
	defer g.lock.Release(lockKey)

	for attempts := 0; attempts < generators.MaxAttempts; attempts++ {
		mappingID, err := utils.GenerateRandomString(generators.MappingIDLength)
		if err != nil {
			return "", err
		}

		// 检查ID是否已被使用
		used, err := g.isMappingIDUsed(ctx, mappingID)
		if err != nil {
			return "", err
		}

		if !used {
			// 标记ID为已使用
			if err := g.markMappingIDAsUsed(ctx, mappingID); err != nil {
				return "", err
			}
			return mappingID, nil
		}
	}

	return "", generators.ErrIDExhausted
}

// ReleaseClientID 释放客户端ID
func (g *DistributedIDGenerator) ReleaseClientID(ctx context.Context, clientID int64) error {
	// 使用优化的ID生成器释放ID
	return g.optimizedClientIDGen.ReleaseClientID(clientID)
}

// ReleaseNodeID 释放节点ID
func (g *DistributedIDGenerator) ReleaseNodeID(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("%s:used_node_id:%s", constants.KeyPrefixID, nodeID)
	return g.storage.Delete(key)
}

// ReleaseUserID 释放用户ID
func (g *DistributedIDGenerator) ReleaseUserID(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s:used_user_id:%s", constants.KeyPrefixID, userID)
	return g.storage.Delete(key)
}

// ReleaseMappingID 释放端口映射ID
func (g *DistributedIDGenerator) ReleaseMappingID(ctx context.Context, mappingID string) error {
	key := fmt.Sprintf("%s:used_mapping_id:%s", constants.KeyPrefixID, mappingID)
	return g.storage.Delete(key)
}

// 辅助方法：检查客户端ID是否已使用
func (g *DistributedIDGenerator) isClientIDUsed(ctx context.Context, clientID int64) (bool, error) {
	// 使用优化的ID生成器检查ID使用状态
	return g.optimizedClientIDGen.IsClientIDUsed(clientID)
}

// 辅助方法：标记客户端ID为已使用（已由优化生成器内部处理）
func (g *DistributedIDGenerator) markClientIDAsUsed(ctx context.Context, clientID int64) error {
	// 此方法已不再需要，因为优化的生成器在GenerateClientID时自动标记ID为已使用
	return nil
}

// 辅助方法：检查节点ID是否已使用
func (g *DistributedIDGenerator) isNodeIDUsed(ctx context.Context, nodeID string) (bool, error) {
	key := fmt.Sprintf("%s:used_node_id:%s", constants.KeyPrefixID, nodeID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记节点ID为已使用
func (g *DistributedIDGenerator) markNodeIDAsUsed(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("%s:used_node_id:%s", constants.KeyPrefixID, nodeID)
	info := &IDUsageInfo{
		ID:        nodeID,
		Type:      "node",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return g.storage.Set(key, string(data), 0) // 永久存储
}

// 辅助方法：检查用户ID是否已使用
func (g *DistributedIDGenerator) isUserIDUsed(ctx context.Context, userID string) (bool, error) {
	key := fmt.Sprintf("%s:used_user_id:%s", constants.KeyPrefixID, userID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记用户ID为已使用
func (g *DistributedIDGenerator) markUserIDAsUsed(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s:used_user_id:%s", constants.KeyPrefixID, userID)
	info := &IDUsageInfo{
		ID:        userID,
		Type:      "user",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return g.storage.Set(key, string(data), 0) // 永久存储
}

// 辅助方法：检查端口映射ID是否已使用
func (g *DistributedIDGenerator) isMappingIDUsed(ctx context.Context, mappingID string) (bool, error) {
	key := fmt.Sprintf("%s:used_mapping_id:%s", constants.KeyPrefixID, mappingID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记端口映射ID为已使用
func (g *DistributedIDGenerator) markMappingIDAsUsed(ctx context.Context, mappingID string) error {
	key := fmt.Sprintf("%s:used_mapping_id:%s", constants.KeyPrefixID, mappingID)
	info := &IDUsageInfo{
		ID:        mappingID,
		Type:      "mapping",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return g.storage.Set(key, string(data), 0) // 永久存储
}

// IDUsageInfo ID使用信息
type IDUsageInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// 保持向后兼容的方法（用于生成认证码和密钥）
func (g *DistributedIDGenerator) GenerateAuthCode() (string, error) {
	return utils.GenerateRandomDigits(generators.AuthCodeLength)
}

func (g *DistributedIDGenerator) GenerateSecretKey() (string, error) {
	return utils.GenerateRandomString(generators.SecretKeyLength)
}

// GetClientIDUsedCount 获取已使用的客户端ID数量
func (g *DistributedIDGenerator) GetClientIDUsedCount() int {
	return g.optimizedClientIDGen.GetUsedCount()
}

// GetClientIDSegmentStats 获取客户端ID段统计信息
func (g *DistributedIDGenerator) GetClientIDSegmentStats() map[int]float64 {
	return g.optimizedClientIDGen.GetSegmentStats()
}

// Close 关闭分布式ID生成器
func (g *DistributedIDGenerator) Close() {
	if g.optimizedClientIDGen != nil {
		g.optimizedClientIDGen.Close()
	}
}
