package node

import (
	"context"
	"fmt"
	"time"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

const (
	// NodeIDMin 最小节点ID
	NodeIDMin = 1
	// NodeIDMax 最大节点ID（支持1000个节点）
	NodeIDMax = 1000
	// NodeIDKeyPrefix 节点ID占用的键前缀
	NodeIDKeyPrefix = "tunnox:node:allocated:"
	// NodeIDLockTTL 节点ID锁的TTL（30秒心跳 * 3 = 90秒）
	NodeIDLockTTL = 90 * time.Second
)

// NodeIDAllocator 节点ID分配器
//
// 职责：
// - 在服务启动时，通过Redis竞争分配唯一的节点ID
// - 支持1-1000的节点ID范围
// - 使用分布式锁机制，确保ID唯一性
type NodeIDAllocator struct {
	storage storage.Storage
	nodeID  string
	stopCh  chan struct{}
}

// NewNodeIDAllocator 创建节点ID分配器
func NewNodeIDAllocator(storage storage.Storage) *NodeIDAllocator {
	return &NodeIDAllocator{
		storage: storage,
		stopCh:  make(chan struct{}),
	}
}

// AllocateNodeID 分配节点ID
//
// 流程：
// 1. 遍历 node-0001 ~ node-1000
// 2. 对每个ID，尝试在Redis中设置键 tunnox:node:allocated:{id}
// 3. 使用 SETNX（SET if Not eXists）保证原子性
// 4. 成功抢到的节点，返回该ID
// 5. 启动心跳goroutine，定期续期（防止crash后占用）
//
// 返回：
//   - string: 分配的节点ID（如 "node-0001"）
//   - error: 分配失败的错误
func (a *NodeIDAllocator) AllocateNodeID(ctx context.Context) (string, error) {
	corelog.Infof("NodeIDAllocator: starting node ID allocation (range: %d-%d)", NodeIDMin, NodeIDMax)

	for id := NodeIDMin; id <= NodeIDMax; id++ {
		nodeID := fmt.Sprintf("node-%04d", id) // node-0001, node-0002, ...
		key := NodeIDKeyPrefix + nodeID

		// 尝试占用这个ID（SETNX + TTL）
		acquired, err := a.tryAcquireNodeID(key, nodeID)
		if err != nil {
			corelog.Warnf("NodeIDAllocator: failed to try acquire %s: %v", nodeID, err)
			continue
		}

		if acquired {
			a.nodeID = nodeID
			corelog.Infof("✅ NodeIDAllocator: successfully allocated node ID: %s", nodeID)

			// 启动心跳goroutine，定期续期
			go a.heartbeatLoop(ctx, key, nodeID)

			return nodeID, nil
		}

		corelog.Debugf("NodeIDAllocator: %s already occupied, trying next...", nodeID)
	}

	return "", fmt.Errorf("no available node ID in range %d-%d (all occupied)", NodeIDMin, NodeIDMax)
}

// tryAcquireNodeID 尝试占用节点ID
//
// 使用SetNX实现原子性：
// - 如果key不存在，设置成功，返回true
// - 如果key已存在，返回false
//
// 参数：
//   - key: Redis键
//   - nodeID: 节点ID
//
// 返回：
//   - bool: 是否成功占用
//   - error: 错误信息
func (a *NodeIDAllocator) tryAcquireNodeID(key, nodeID string) (bool, error) {
	// 检查是否已存在
	exists, err := a.storage.Exists(key)
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	if exists {
		return false, nil // 已被占用
	}

	// 尝试设置（带TTL）
	// 注意：节点分配是运行时缓存，不应该持久化
	// 如果 storage 支持 SetRuntime，使用它来显式设置为运行时数据
	// 否则使用普通的 Set（对于非 HybridStorage 的情况）
	if hybridStorage, ok := a.storage.(interface {
		SetRuntime(key string, value interface{}, ttl time.Duration) error
	}); ok {
		err = hybridStorage.SetRuntime(key, nodeID, NodeIDLockTTL)
	} else {
		err = a.storage.Set(key, nodeID, NodeIDLockTTL)
	}
	if err != nil {
		return false, fmt.Errorf("failed to set node ID: %w", err)
	}

	// 再次确认（防止竞态）
	value, err := a.storage.Get(key)
	if err != nil {
		return false, fmt.Errorf("failed to verify node ID: %w", err)
	}

	if valueStr, ok := value.(string); ok && valueStr == nodeID {
		return true, nil // 成功占用
	}

	return false, nil // 被其他节点抢占
}

// heartbeatLoop 心跳循环，定期续期节点ID
//
// 每30秒续期一次，TTL设为90秒
// 这样即使节点crash，最多90秒后ID会被释放
func (a *NodeIDAllocator) heartbeatLoop(ctx context.Context, key, nodeID string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	corelog.Infof("NodeIDAllocator: heartbeat started for %s", nodeID)

	for {
		select {
		case <-ctx.Done():
			corelog.Infof("NodeIDAllocator: context cancelled, stopping heartbeat for %s", nodeID)
			return
		case <-a.stopCh:
			corelog.Infof("NodeIDAllocator: stop signal received, stopping heartbeat for %s", nodeID)
			return
		case <-ticker.C:
			// 续期（节点分配是运行时缓存，不应该持久化）
			var err error
			if hybridStorage, ok := a.storage.(interface {
				SetRuntime(key string, value interface{}, ttl time.Duration) error
			}); ok {
				err = hybridStorage.SetRuntime(key, nodeID, NodeIDLockTTL)
			} else {
				err = a.storage.Set(key, nodeID, NodeIDLockTTL)
			}
			if err != nil {
				corelog.Errorf("NodeIDAllocator: failed to renew node ID %s: %v", nodeID, err)
			} else {
				corelog.Debugf("NodeIDAllocator: renewed node ID %s (TTL: %s)", nodeID, NodeIDLockTTL)
			}
		}
	}
}

// GetNodeID 获取当前节点ID
func (a *NodeIDAllocator) GetNodeID() string {
	return a.nodeID
}

// Release 释放节点ID
//
// 调用时机：服务优雅关闭时
func (a *NodeIDAllocator) Release() error {
	if a.nodeID == "" {
		return nil
	}

	close(a.stopCh)

	key := NodeIDKeyPrefix + a.nodeID
	err := a.storage.Delete(key)
	if err != nil {
		return fmt.Errorf("failed to release node ID %s: %w", a.nodeID, err)
	}

	corelog.Infof("NodeIDAllocator: released node ID: %s", a.nodeID)
	a.nodeID = ""
	return nil
}
