package idgen

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
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
			return zero, fmt.Errorf("unsupported ID type: %T", zero)
		}

		// 检查ID是否已被使用
		used, err := g.IsUsed(candidate)
		if err != nil {
			continue
		}

		if !used {
			// 标记ID为已使用
			if err := g.markAsUsed(candidate); err != nil {
				continue
			}
			return candidate, nil
		}
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

// markAsUsed 标记ID为已使用
func (g *StorageIDGenerator[T]) markAsUsed(id T) error {
	key := g.getKey(id)
	info := &IDUsageInfo{
		ID:        fmt.Sprintf("%v", id),
		Type:      g.keyPrefix,
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return g.storage.Set(key, string(data), 0) // 永久存储
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
		"",                         // 无前缀，直接生成数字
		"tunnox:id:used:client",    // 存储键前缀
		parentCtx,
	)
}

// IDManager 统一ID管理器
type IDManager struct {
	storage storage.Storage

	// 不同类型的专门生成器实例
	clientIDGen              IDGenerator[int64]
	nodeIDGen                IDGenerator[string]
	connectionIDGen          IDGenerator[string]
	portMappingIDGen         IDGenerator[string]
	portMappingInstanceIDGen IDGenerator[string]
	userIDGen                IDGenerator[string]
	tunnelIDGen              IDGenerator[string]

	dispose.Dispose
}

// NewIDManager 创建ID管理器
func NewIDManager(storage storage.Storage, parentCtx context.Context) *IDManager {
	manager := &IDManager{
		storage: storage,
	}

	// 初始化各种ID生成器
	// ClientID 使用 int64 类型，生成完全随机的 8 位数字
	manager.clientIDGen = NewStorageIDGenerator[int64](storage, "", "tunnox:id:used:client", parentCtx)
	
	// 其他 ID 使用 string 类型，生成带前缀的随机字符串
	manager.nodeIDGen = NewStorageIDGenerator[string](storage, PrefixNodeID, "tunnox:id:used:node", parentCtx)
	manager.connectionIDGen = NewStorageIDGenerator[string](storage, PrefixConnectionID, "tunnox:id:used:conn", parentCtx)
	manager.portMappingIDGen = NewStorageIDGenerator[string](storage, PrefixPortMappingID, "tunnox:id:used:pmap", parentCtx)
	manager.portMappingInstanceIDGen = NewStorageIDGenerator[string](storage, PrefixPortMappingInstanceID, "tunnox:id:used:pmi", parentCtx)
	manager.userIDGen = NewStorageIDGenerator[string](storage, PrefixUserID, "tunnox:id:used:user", parentCtx)
	manager.tunnelIDGen = NewStorageIDGenerator[string](storage, PrefixTunnelID, "tunnox:id:used:tunnel", parentCtx)

	manager.SetCtx(parentCtx, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (m *IDManager) onClose() error {
	corelog.Infof("Cleaning up ID manager resources...")

	// 关闭所有生成器
	if m.clientIDGen != nil {
		err := m.clientIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed client ID generator")
	}

	if m.nodeIDGen != nil {
		err := m.nodeIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed node ID generator")
	}

	if m.connectionIDGen != nil {
		err := m.connectionIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed connection ID generator")
	}

	if m.portMappingIDGen != nil {

		err := m.portMappingIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed port mapping ID generator")
	}

	if m.portMappingInstanceIDGen != nil {
		err := m.portMappingInstanceIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed port mapping instance ID generator")
	}

	if m.userIDGen != nil {
		err := m.userIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed user ID generator")
	}

	if m.tunnelIDGen != nil {
		err := m.tunnelIDGen.Close()
		if err != nil {
			return err
		}
		corelog.Infof("Closed tunnel ID generator")
	}

	corelog.Infof("ID manager resources cleanup completed")
	return nil
}

// 便捷方法
func (m *IDManager) GenerateClientID() (int64, error) {
	return m.clientIDGen.Generate()
}

func (m *IDManager) GenerateNodeID() (string, error) {
	return m.nodeIDGen.Generate()
}

func (m *IDManager) GenerateUserID() (string, error) {
	return m.userIDGen.Generate()
}

func (m *IDManager) GeneratePortMappingID() (string, error) {
	return m.portMappingIDGen.Generate()
}

func (m *IDManager) GeneratePortMappingInstanceID() (string, error) {
	return m.portMappingInstanceIDGen.Generate()
}

func (m *IDManager) GenerateConnectionID() (string, error) {
	return m.connectionIDGen.Generate()
}

func (m *IDManager) GenerateTunnelID() (string, error) {
	return m.tunnelIDGen.Generate()
}

func (m *IDManager) ReleaseClientID(id int64) error {
	return m.clientIDGen.Release(id)
}

func (m *IDManager) ReleaseNodeID(id string) error {
	return m.nodeIDGen.Release(id)
}

func (m *IDManager) ReleaseUserID(id string) error {
	return m.userIDGen.Release(id)
}

func (m *IDManager) ReleasePortMappingID(id string) error {
	return m.portMappingIDGen.Release(id)
}

func (m *IDManager) ReleasePortMappingInstanceID(id string) error {
	return m.portMappingInstanceIDGen.Release(id)
}

func (m *IDManager) ReleaseConnectionID(id string) error {
	return m.connectionIDGen.Release(id)
}

func (m *IDManager) ReleaseTunnelID(id string) error {
	return m.tunnelIDGen.Release(id)
}

func (m *IDManager) IsClientIDUsed(id int64) (bool, error) {
	return m.clientIDGen.IsUsed(id)
}

func (m *IDManager) IsNodeIDUsed(id string) (bool, error) {
	return m.nodeIDGen.IsUsed(id)
}

func (m *IDManager) IsUserIDUsed(id string) (bool, error) {
	return m.userIDGen.IsUsed(id)
}

func (m *IDManager) IsPortMappingIDUsed(id string) (bool, error) {
	return m.portMappingIDGen.IsUsed(id)
}

func (m *IDManager) IsPortMappingInstanceIDUsed(id string) (bool, error) {
	return m.portMappingInstanceIDGen.IsUsed(id)
}

func (m *IDManager) IsConnectionIDUsed(id string) (bool, error) {
	return m.connectionIDGen.IsUsed(id)
}

func (m *IDManager) IsTunnelIDUsed(id string) (bool, error) {
	return m.tunnelIDGen.IsUsed(id)
}

// GenerateAuthCode 生成认证码
func (m *IDManager) GenerateAuthCode() (string, error) {
	return utils.GenerateRandomDigits(6)
}

// GenerateSecretKey 生成密钥
func (m *IDManager) GenerateSecretKey() (string, error) {
	return utils.GenerateRandomString(32)
}

// IDUsageInfo ID使用信息
type IDUsageInfo struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// Close 关闭ID管理器
func (m *IDManager) Close() error {
	m.Dispose.Close()
	return nil
}

// GenerateUniqueID 通用ID生成重试函数
// 用于生成唯一ID，自动处理重试和冲突检查
func (m *IDManager) GenerateUniqueID(
	generateFunc func() (int64, error),
	checkFunc func(int64) (bool, error),
	releaseFunc func(int64) error,
	idType string,
) (int64, error) {
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		generatedID, err := generateFunc()
		if err != nil {
			return 0, fmt.Errorf("generate %s ID failed: %w", idType, err)
		}

		// 检查是否已存在
		exists, err := checkFunc(generatedID)
		if err != nil {
			// 如果检查失败，假设不存在，使用这个ID
			return generatedID, nil
		}

		if !exists {
			// ID不存在，可以使用
			return generatedID, nil
		}

		// ID已存在，释放并重试
		_ = releaseFunc(generatedID)
		continue
	}

	return 0, fmt.Errorf("failed to generate unique %s ID after %d attempts", idType, MaxAttempts)
}

// GenerateUniqueClientID 生成唯一客户端ID
func (m *IDManager) GenerateUniqueClientID(checkFunc func(int64) (bool, error)) (int64, error) {
	return m.GenerateUniqueID(
		m.GenerateClientID,
		checkFunc,
		m.ReleaseClientID,
		"client",
	)
}

// GenerateUniquePortMappingID 生成唯一端口映射ID
func (m *IDManager) GenerateUniquePortMappingID(checkFunc func(string) (bool, error)) (string, error) {
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		generatedID, err := m.GeneratePortMappingID()
		if err != nil {
			return "", fmt.Errorf("generate port mapping ID failed: %w", err)
		}

		// 检查是否已存在
		exists, err := checkFunc(generatedID)
		if err != nil {
			// 如果检查失败，假设不存在，使用这个ID
			return generatedID, nil
		}

		if !exists {
			// ID不存在，可以使用
			return generatedID, nil
		}

		// ID已存在，释放并重试
		_ = m.ReleasePortMappingID(generatedID)
		continue
	}

	return "", fmt.Errorf("failed to generate unique port mapping ID after %d attempts", MaxAttempts)
}

// GenerateUniqueNodeID 生成唯一节点ID
func (m *IDManager) GenerateUniqueNodeID(checkFunc func(string) (bool, error)) (string, error) {
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		generatedID, err := m.GenerateNodeID()
		if err != nil {
			return "", fmt.Errorf("generate node ID failed: %w", err)
		}

		// 检查是否已存在
		exists, err := checkFunc(generatedID)
		if err != nil {
			// 如果检查失败，假设不存在，使用这个ID
			return generatedID, nil
		}

		if !exists {
			// ID不存在，可以使用
			return generatedID, nil
		}

		// ID已存在，释放并重试
		_ = m.ReleaseNodeID(generatedID)
		continue
	}

	return "", fmt.Errorf("failed to generate unique node ID after %d attempts", MaxAttempts)
}
