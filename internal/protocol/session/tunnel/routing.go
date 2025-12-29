// Package tunnel 提供隧道桥接和路由功能
package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// WaitingState 隧道等待状态（用于跨服务器隧道建立）
// 当源端客户端发起TunnelOpen到ServerA，ServerA创建Bridge并等待目标端连接
// 如果目标端连接到了ServerB，ServerB需要知道如何将连接路由回ServerA
type WaitingState struct {
	TunnelID  string `json:"tunnel_id"`
	MappingID string `json:"mapping_id"`
	SecretKey string `json:"secret_key"`

	// 源端信息（发起TunnelOpen的客户端）
	SourceNodeID   string `json:"source_node_id"`   // 源端连接所在的Server节点ID
	SourceClientID int64  `json:"source_client_id"` // 源端客户端ID

	// 目标端信息（需要建立连接的客户端）
	TargetClientID int64  `json:"target_client_id"` // 目标客户端ID
	TargetHost     string `json:"target_host"`      // 目标地址
	TargetPort     int    `json:"target_port"`      // 目标端口

	// 元数据
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RoutingTable 隧道路由表（Redis-based）
// 负责记录和查询跨服务器隧道的等待状态
type RoutingTable struct {
	storage storage.Storage
	ttl     time.Duration // Tunnel等待状态的TTL，默认30秒
}

// NewRoutingTable 创建隧道路由表
func NewRoutingTable(storage storage.Storage, ttl time.Duration) *RoutingTable {
	if ttl == 0 {
		ttl = 30 * time.Second
	}

	return &RoutingTable{
		storage: storage,
		ttl:     ttl,
	}
}

// RegisterWaitingTunnel 注册等待中的隧道
// 当源端Server创建TunnelBridge后调用，记录路由信息到Redis
func (t *RoutingTable) RegisterWaitingTunnel(ctx context.Context, state *WaitingState) error {
	if state.TunnelID == "" {
		return fmt.Errorf("tunnel_id is required")
	}

	// 设置时间戳
	now := time.Now()
	state.CreatedAt = now
	state.ExpiresAt = now.Add(t.ttl)

	// 直接存储结构体，让 Storage 层处理序列化
	// 注意：Storage.Set 会自动进行 JSON 序列化
	key := t.makeKey(state.TunnelID)
	if err := t.storage.Set(key, state, t.ttl); err != nil {
		return fmt.Errorf("failed to store tunnel state: %w", err)
	}

	corelog.Infof("TunnelRouting: registered waiting tunnel %s (source_node=%s, target_client=%d, ttl=%v)",
		state.TunnelID, state.SourceNodeID, state.TargetClientID, t.ttl)

	return nil
}

// LookupWaitingTunnel 查找等待中的隧道
// 当目标端Server收到TunnelOpen连接时调用，查询源端Server位置
func (t *RoutingTable) LookupWaitingTunnel(ctx context.Context, tunnelID string) (*WaitingState, error) {
	if tunnelID == "" {
		return nil, fmt.Errorf("tunnel_id is required")
	}

	key := t.makeKey(tunnelID)
	value, err := t.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tunnel state: %w", err)
	}

	// Storage.Get 返回的是反序列化后的 map[string]interface{}
	// 需要重新序列化再反序列化为具体类型
	var state WaitingState

	switch v := value.(type) {
	case *WaitingState:
		// 直接返回（内存存储可能直接返回原始类型）
		state = *v
	case WaitingState:
		// 值类型
		state = v
	case map[string]interface{}:
		// 重新序列化为 JSON，再反序列化为具体类型
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to re-marshal tunnel state: %w", err)
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tunnel state: %w", err)
		}
	case []byte:
		// 直接反序列化
		if err := json.Unmarshal(v, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tunnel state: %w", err)
		}
	case string:
		// 字符串类型，尝试反序列化
		if err := json.Unmarshal([]byte(v), &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tunnel state: %w", err)
		}
	default:
		return nil, fmt.Errorf("unexpected value type: %T, expected map[string]interface{}, []byte or string", value)
	}

	// 检查是否过期
	if time.Now().After(state.ExpiresAt) {
		// 已过期，删除并返回错误
		t.storage.Delete(key)
		return nil, ErrExpired
	}

	return &state, nil
}

// RemoveWaitingTunnel 移除等待中的隧道
// 当隧道成功建立或超时后调用，清理Redis中的记录
func (t *RoutingTable) RemoveWaitingTunnel(ctx context.Context, tunnelID string) error {
	if tunnelID == "" {
		return fmt.Errorf("tunnel_id is required")
	}

	key := t.makeKey(tunnelID)
	if err := t.storage.Delete(key); err != nil {
		// 删除失败不是致命错误，因为有TTL自动清理
		corelog.Warnf("TunnelRouting: failed to delete tunnel state for %s: %v", tunnelID, err)
		return nil
	}

	return nil
}

// CleanupExpiredTunnels 清理过期的隧道（可选，Redis TTL会自动清理）
// 这个方法主要用于统计和监控
func (t *RoutingTable) CleanupExpiredTunnels(ctx context.Context) (int, error) {
	// Redis会自动清理过期的key，这里只是为了统计
	// 实际生产环境中可以通过Redis的SCAN命令遍历所有tunnox:tunnel_waiting:*的key
	// 但为了性能考虑，这里暂不实现全量扫描
	return 0, nil
}

// makeKey 生成Redis key
func (t *RoutingTable) makeKey(tunnelID string) string {
	return fmt.Sprintf("tunnox:tunnel_waiting:%s", tunnelID)
}

// GetNodeAddress 获取节点地址
func (t *RoutingTable) GetNodeAddress(nodeID string) (string, error) {
	if t.storage == nil {
		return "", fmt.Errorf("storage not configured")
	}

	key := fmt.Sprintf("tunnox:node:%s:addr", nodeID)
	value, err := t.storage.Get(key)
	if err != nil {
		return "", err
	}

	if addr, ok := value.(string); ok && addr != "" {
		return addr, nil
	}
	if addrBytes, ok := value.([]byte); ok && len(addrBytes) > 0 {
		return string(addrBytes), nil
	}

	return "", fmt.Errorf("invalid address format for node %s", nodeID)
}

// RegisterNodeAddress 注册节点地址
func (t *RoutingTable) RegisterNodeAddress(nodeID, addr string) error {
	if t.storage == nil {
		return fmt.Errorf("storage not configured")
	}

	key := fmt.Sprintf("tunnox:node:%s:addr", nodeID)
	// 节点地址使用较长的 TTL（1小时）
	return t.storage.Set(key, addr, time.Hour)
}

// ============================================================================
// 访问器方法
// ============================================================================

// GetStorage 获取底层存储（用于跨包访问）
func (t *RoutingTable) GetStorage() storage.Storage {
	return t.storage
}

// ============================================================================
// 错误定义
// ============================================================================

// ErrNotFound Tunnel未找到错误
var ErrNotFound = fmt.Errorf("tunnel not found in routing table")

// ErrExpired Tunnel已过期错误
var ErrExpired = fmt.Errorf("tunnel waiting state expired")
