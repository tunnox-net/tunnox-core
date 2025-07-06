package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/utils"
)

// DistributedIDGenerator 分布式ID生成器
type DistributedIDGenerator struct {
	storage Storage
	lock    DistributedLock
}

// NewDistributedIDGenerator 创建分布式ID生成器
func NewDistributedIDGenerator(storage Storage, lock DistributedLock) *DistributedIDGenerator {
	return &DistributedIDGenerator{
		storage: storage,
		lock:    lock,
	}
}

// GenerateClientID 生成客户端ID（8位大于10000000的随机整数）
func (g *DistributedIDGenerator) GenerateClientID(ctx context.Context) (int64, error) {
	lockKey := "lock:generate_client_id"

	// 获取分布式锁，确保ID生成的原子性
	acquired, err := g.lock.Acquire(lockKey, 10*time.Second)
	if err != nil {
		return 0, fmt.Errorf("acquire lock failed: %w", err)
	}
	if !acquired {
		return 0, fmt.Errorf("failed to acquire lock for ID generation")
	}
	defer g.lock.Release(lockKey)

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		randomInt, err := utils.GenerateRandomInt64(ClientIDMin, ClientIDMax)
		if err != nil {
			return 0, err
		}

		// 检查ID是否已被使用
		used, err := g.isClientIDUsed(ctx, randomInt)
		if err != nil {
			return 0, err
		}

		if !used {
			// 标记ID为已使用
			if err := g.markClientIDAsUsed(ctx, randomInt); err != nil {
				return 0, err
			}
			return randomInt, nil
		}
	}

	return 0, ErrIDExhausted
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

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		nodeID, err := utils.GenerateRandomString(NodeIDLength)
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

	return "", ErrIDExhausted
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

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		userID, err := utils.GenerateRandomString(UserIDLength)
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

	return "", ErrIDExhausted
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

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		mappingID, err := utils.GenerateRandomString(MappingIDLength)
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

	return "", ErrIDExhausted
}

// ReleaseClientID 释放客户端ID
func (g *DistributedIDGenerator) ReleaseClientID(ctx context.Context, clientID int64) error {
	key := fmt.Sprintf("%s:used_client_id:%d", KeyPrefixID, clientID)
	return g.storage.Delete(key)
}

// ReleaseNodeID 释放节点ID
func (g *DistributedIDGenerator) ReleaseNodeID(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("%s:used_node_id:%s", KeyPrefixID, nodeID)
	return g.storage.Delete(key)
}

// ReleaseUserID 释放用户ID
func (g *DistributedIDGenerator) ReleaseUserID(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s:used_user_id:%s", KeyPrefixID, userID)
	return g.storage.Delete(key)
}

// ReleaseMappingID 释放端口映射ID
func (g *DistributedIDGenerator) ReleaseMappingID(ctx context.Context, mappingID string) error {
	key := fmt.Sprintf("%s:used_mapping_id:%s", KeyPrefixID, mappingID)
	return g.storage.Delete(key)
}

// 辅助方法：检查客户端ID是否已使用
func (g *DistributedIDGenerator) isClientIDUsed(ctx context.Context, clientID int64) (bool, error) {
	key := fmt.Sprintf("%s:used_client_id:%d", KeyPrefixID, clientID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记客户端ID为已使用
func (g *DistributedIDGenerator) markClientIDAsUsed(ctx context.Context, clientID int64) error {
	key := fmt.Sprintf("%s:used_client_id:%d", KeyPrefixID, clientID)
	info := &IDUsageInfo{
		ID:        fmt.Sprintf("%d", clientID),
		Type:      "client",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return g.storage.Set(key, string(data), 0) // 永久存储
}

// 辅助方法：检查节点ID是否已使用
func (g *DistributedIDGenerator) isNodeIDUsed(ctx context.Context, nodeID string) (bool, error) {
	key := fmt.Sprintf("%s:used_node_id:%s", KeyPrefixID, nodeID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记节点ID为已使用
func (g *DistributedIDGenerator) markNodeIDAsUsed(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("%s:used_node_id:%s", KeyPrefixID, nodeID)
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
	key := fmt.Sprintf("%s:used_user_id:%s", KeyPrefixID, userID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记用户ID为已使用
func (g *DistributedIDGenerator) markUserIDAsUsed(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s:used_user_id:%s", KeyPrefixID, userID)
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
	key := fmt.Sprintf("%s:used_mapping_id:%s", KeyPrefixID, mappingID)
	exists, err := g.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// 辅助方法：标记端口映射ID为已使用
func (g *DistributedIDGenerator) markMappingIDAsUsed(ctx context.Context, mappingID string) error {
	key := fmt.Sprintf("%s:used_mapping_id:%s", KeyPrefixID, mappingID)
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
	return utils.GenerateRandomDigits(AuthCodeLength)
}

func (g *DistributedIDGenerator) GenerateSecretKey() (string, error) {
	return utils.GenerateRandomString(SecretKeyLength)
}
