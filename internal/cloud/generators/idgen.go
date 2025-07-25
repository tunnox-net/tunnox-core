package generators

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/utils"
)

//TODO: mapping连接实例的ID实现有问题

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

// 使用core/idgen包中的IDGenerator接口
type IDGenerator[T any] = idgen.IDGenerator[T]

// StorageBasedIDGenerator 基于Storage的泛型ID生成器
type StorageBasedIDGenerator[T any] struct {
	storage   storages.Storage
	prefix    string
	keyPrefix string
	mu        sync.RWMutex
	utils.Dispose
}

// NewStorageBasedIDGenerator 创建基于Storage的ID生成器
func NewStorageBasedIDGenerator[T any](storage storages.Storage, prefix, keyPrefix string, parentCtx context.Context) *StorageBasedIDGenerator[T] {
	generator := &StorageBasedIDGenerator[T]{
		storage:   storage,
		prefix:    prefix,
		keyPrefix: keyPrefix,
	}
	generator.SetCtx(parentCtx, generator.onClose)
	return generator
}

// onClose 资源清理回调
func (g *StorageBasedIDGenerator[T]) onClose() error {
	utils.Infof("Storage-based ID generator resources cleaned up")
	return nil
}

// getKey 生成存储键
func (g *StorageBasedIDGenerator[T]) getKey(id T) string {
	return fmt.Sprintf("%s:%v", g.keyPrefix, id)
}

// Generate 生成ID
func (g *StorageBasedIDGenerator[T]) Generate() (T, error) {
	var zero T

	for attempts := 0; attempts < MaxAttempts; attempts++ {
		// 生成候选ID
		var candidate T
		var err error

		switch any(zero).(type) {
		case string:
			// 使用统一格式生成有序随机串
			orderedStr, err := utils.GenerateOrderedRandomString(g.prefix, RandomPartLength)
			if err != nil {
				continue
			}
			candidate = any(orderedStr).(T)
		default:
			return zero, fmt.Errorf("unsupported ID type")
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
func (g *StorageBasedIDGenerator[T]) Release(id T) error {
	key := g.getKey(id)
	return g.storage.Delete(key)
}

// IsUsed 检查ID是否已使用
func (g *StorageBasedIDGenerator[T]) IsUsed(id T) (bool, error) {
	key := g.getKey(id)
	return g.storage.Exists(key)
}

// markAsUsed 标记ID为已使用
func (g *StorageBasedIDGenerator[T]) markAsUsed(id T) error {
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
func (g *StorageBasedIDGenerator[T]) GetUsedCount() int {
	// 这里可以实现更复杂的统计逻辑
	// 目前返回-1表示不支持此操作
	return -1
}

// Close 关闭生成器
func (g *StorageBasedIDGenerator[T]) Close() error {
	g.Dispose.Close()
	return nil
}

// ClientIDGenerator 客户端ID生成器（int64类型，使用分段位图算法）
type ClientIDGenerator struct {
	storage storages.Storage
	utils.Dispose
}

// NewClientIDGenerator 创建客户端ID生成器
func NewClientIDGenerator(storage storages.Storage, parentCtx context.Context) *ClientIDGenerator {
	generator := &ClientIDGenerator{
		storage: storage,
	}
	generator.SetCtx(parentCtx, generator.onClose)
	return generator
}

// onClose 资源清理回调
func (g *ClientIDGenerator) onClose() error {
	utils.Infof("Client ID generator resources cleaned up")
	return nil
}

// Generate 生成客户端ID
func (g *ClientIDGenerator) Generate() (int64, error) {
	// 使用存储层的原子操作生成ID
	counterKey := "tunnox:id:client_counter"

	// 使用原子递增操作
	counter, err := g.storage.Incr(counterKey)
	if err != nil {
		return 0, fmt.Errorf("increment counter failed: %w", err)
	}

	// 转换为客户端ID格式
	clientID := ClientIDMin + counter
	if clientID > ClientIDMax {
		return 0, ErrIDExhausted
	}

	// 标记ID为已使用
	if err := g.markAsUsed(clientID); err != nil {
		// 如果标记失败，返回错误而不是继续
		return 0, fmt.Errorf("failed to mark client ID %d as used: %w", clientID, err)
	}

	return clientID, nil
}

// Release 释放客户端ID
func (g *ClientIDGenerator) Release(clientID int64) error {
	key := fmt.Sprintf("tunnox:id:used:client:%d", clientID)
	return g.storage.Delete(key)
}

// IsUsed 检查客户端ID是否已使用
func (g *ClientIDGenerator) IsUsed(clientID int64) (bool, error) {
	key := fmt.Sprintf("tunnox:id:used:client:%d", clientID)
	return g.storage.Exists(key)
}

// markAsUsed 标记客户端ID为已使用
func (g *ClientIDGenerator) markAsUsed(clientID int64) error {
	key := fmt.Sprintf("tunnox:id:used:client:%d", clientID)
	info := &IDUsageInfo{
		ID:        fmt.Sprintf("%d", clientID),
		Type:      "client",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return g.storage.Set(key, string(data), 0)
}

// GetUsedCount 获取已使用的客户端ID数量
func (g *ClientIDGenerator) GetUsedCount() int {
	// 这里可以实现更复杂的统计逻辑
	return -1
}

// Close 关闭生成器
func (g *ClientIDGenerator) Close() error {
	g.Dispose.Close()
	return nil
}

// IDManager 统一ID管理器
type IDManager struct {
	storage storages.Storage

	// 不同类型的专门生成器实例
	clientIDGen              IDGenerator[int64]
	nodeIDGen                IDGenerator[string]
	connectionIDGen          IDGenerator[string]
	portMappingIDGen         IDGenerator[string]
	portMappingInstanceIDGen IDGenerator[string]
	userIDGen                IDGenerator[string]
	tunnelIDGen              IDGenerator[string]

	utils.Dispose
}

// NewIDManager 创建ID管理器
func NewIDManager(storage storages.Storage, parentCtx context.Context) *IDManager {
	manager := &IDManager{
		storage: storage,
	}

	// 初始化各种ID生成器
	manager.clientIDGen = NewClientIDGenerator(storage, parentCtx)
	manager.nodeIDGen = NewStorageBasedIDGenerator[string](storage, PrefixNodeID, "tunnox:id:used:node", parentCtx)
	manager.connectionIDGen = NewStorageBasedIDGenerator[string](storage, PrefixConnectionID, "tunnox:id:used:conn", parentCtx)
	manager.portMappingIDGen = NewStorageBasedIDGenerator[string](storage, PrefixPortMappingID, "tunnox:id:used:pmap", parentCtx)
	manager.portMappingInstanceIDGen = NewStorageBasedIDGenerator[string](storage, PrefixPortMappingInstanceID, "tunnox:id:used:pmi", parentCtx)
	manager.userIDGen = NewStorageBasedIDGenerator[string](storage, PrefixUserID, "tunnox:id:used:user", parentCtx)
	manager.tunnelIDGen = NewStorageBasedIDGenerator[string](storage, PrefixTunnelID, "tunnox:id:used:tunnel", parentCtx)

	manager.SetCtx(parentCtx, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (m *IDManager) onClose() error {
	utils.Infof("Cleaning up ID manager resources...")

	// 关闭所有生成器
	if m.clientIDGen != nil {
		err := m.clientIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed client ID generator")
	}

	if m.nodeIDGen != nil {
		err := m.nodeIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed node ID generator")
	}

	if m.connectionIDGen != nil {
		err := m.connectionIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed connection ID generator")
	}

	if m.portMappingIDGen != nil {

		err := m.portMappingIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed port mapping ID generator")
	}

	if m.portMappingInstanceIDGen != nil {
		err := m.portMappingInstanceIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed port mapping instance ID generator")
	}

	if m.userIDGen != nil {
		err := m.userIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed user ID generator")
	}

	if m.tunnelIDGen != nil {
		err := m.tunnelIDGen.Close()
		if err != nil {
			return err
		}
		utils.Infof("Closed tunnel ID generator")
	}

	utils.Infof("ID manager resources cleanup completed")
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
