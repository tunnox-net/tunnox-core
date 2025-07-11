package generators

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/utils"
)

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
	PrefixNodeID       = "node_"
	PrefixConnectionID = "conn_"
	PrefixConfigID     = "cfg_"
	PrefixTunnelID     = "tun_"

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
func (g *StorageBasedIDGenerator[T]) onClose() {
	utils.Infof("Storage-based ID generator resources cleaned up")
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
func (g *ClientIDGenerator) onClose() {
	utils.Infof("Client ID generator resources cleaned up")
}

// Generate 生成客户端ID
func (g *ClientIDGenerator) Generate() (int64, error) {
	// 使用优化的分段位图算法
	// 这里简化实现，实际应该使用optimized_idgen.go中的算法
	for attempts := 0; attempts < MaxAttempts; attempts++ {
		randomInt, err := utils.GenerateRandomInt64(ClientIDMin, ClientIDMax)
		if err != nil {
			continue
		}

		// 检查ID是否已被使用
		used, err := g.IsUsed(randomInt)
		if err != nil {
			continue
		}

		if !used {
			// 标记ID为已使用
			if err := g.markAsUsed(randomInt); err != nil {
				continue
			}
			return randomInt, nil
		}
	}

	return 0, ErrIDExhausted
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

// ConnectionIDGenerator 连接ID生成器（基于时间戳和计数器）
type ConnectionIDGenerator struct {
	counter int64
	mu      sync.Mutex
	utils.Dispose
}

// NewConnectionIDGenerator 创建连接ID生成器
func NewConnectionIDGenerator(parentCtx context.Context) *ConnectionIDGenerator {
	generator := &ConnectionIDGenerator{}
	generator.SetCtx(parentCtx, generator.onClose)
	return generator
}

// onClose 资源清理回调
func (g *ConnectionIDGenerator) onClose() {
	utils.Infof("Connection ID generator resources cleaned up")
}

// Generate 生成连接ID
func (g *ConnectionIDGenerator) Generate() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counter++
	return fmt.Sprintf("%s%d_%d", PrefixConnectionID, time.Now().UnixMilli(), g.counter), nil
}

// Release 释放连接ID（连接ID不需要释放，因为基于时间戳）
func (g *ConnectionIDGenerator) Release(id string) error {
	// 连接ID基于时间戳，不需要释放
	return nil
}

// IsUsed 检查连接ID是否已使用（连接ID天然唯一）
func (g *ConnectionIDGenerator) IsUsed(id string) (bool, error) {
	// 连接ID基于时间戳+计数器，天然唯一
	return false, nil
}

// GetUsedCount 获取已使用的连接ID数量
func (g *ConnectionIDGenerator) GetUsedCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return int(g.counter)
}

// Close 关闭生成器
func (g *ConnectionIDGenerator) Close() error {
	g.Dispose.Close()
	return nil
}

// IDManager 统一ID管理器
type IDManager struct {
	storage storages.Storage

	// 不同类型的专门生成器实例
	clientIDGen     IDGenerator[int64]
	nodeIDGen       IDGenerator[string]
	connectionIDGen IDGenerator[string]
	configIDGen     IDGenerator[string]
	tunnelIDGen     IDGenerator[string]

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
	manager.connectionIDGen = NewConnectionIDGenerator(parentCtx)
	manager.configIDGen = NewStorageBasedIDGenerator[string](storage, PrefixConfigID, "tunnox:id:used:config", parentCtx)
	manager.tunnelIDGen = NewStorageBasedIDGenerator[string](storage, PrefixTunnelID, "tunnox:id:used:tunnel", parentCtx)

	manager.SetCtx(parentCtx, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (m *IDManager) onClose() {
	utils.Infof("Cleaning up ID manager resources...")

	// 关闭所有生成器
	if m.clientIDGen != nil {
		m.clientIDGen.Close()
		utils.Infof("Closed client ID generator")
	}

	if m.nodeIDGen != nil {
		m.nodeIDGen.Close()
		utils.Infof("Closed node ID generator")
	}

	if m.connectionIDGen != nil {
		m.connectionIDGen.Close()
		utils.Infof("Closed connection ID generator")
	}

	if m.configIDGen != nil {
		m.configIDGen.Close()
		utils.Infof("Closed config ID generator")
	}

	if m.tunnelIDGen != nil {
		m.tunnelIDGen.Close()
		utils.Infof("Closed tunnel ID generator")
	}

	utils.Infof("ID manager resources cleanup completed")
}

// 便捷方法
func (m *IDManager) GenerateClientID() (int64, error) {
	return m.clientIDGen.Generate()
}

func (m *IDManager) GenerateNodeID() (string, error) {
	return m.nodeIDGen.Generate()
}

func (m *IDManager) GenerateConnectionID() (string, error) {
	return m.connectionIDGen.Generate()
}

func (m *IDManager) GenerateConfigID() (string, error) {
	return m.configIDGen.Generate()
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

func (m *IDManager) ReleaseConnectionID(id string) error {
	return m.connectionIDGen.Release(id)
}

func (m *IDManager) ReleaseConfigID(id string) error {
	return m.configIDGen.Release(id)
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

func (m *IDManager) IsConnectionIDUsed(id string) (bool, error) {
	return m.connectionIDGen.IsUsed(id)
}

func (m *IDManager) IsConfigIDUsed(id string) (bool, error) {
	return m.configIDGen.IsUsed(id)
}

func (m *IDManager) IsTunnelIDUsed(id string) (bool, error) {
	return m.tunnelIDGen.IsUsed(id)
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
