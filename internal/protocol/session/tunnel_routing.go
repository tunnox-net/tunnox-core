package session

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/core/storage"
)

// TunnelWaitingState 隧道等待状态（用于跨服务器隧道建立）
// 当源端客户端发起TunnelOpen到ServerA，ServerA创建Bridge并等待目标端连接
// 如果目标端连接到了ServerB，ServerB需要知道如何将连接路由回ServerA
type TunnelWaitingState struct {
	TunnelID       string    `json:"tunnel_id"`
	MappingID      string    `json:"mapping_id"`
	SecretKey      string    `json:"secret_key"`
	
	// 源端信息（发起TunnelOpen的客户端）
	SourceNodeID   string    `json:"source_node_id"`   // 源端连接所在的Server节点ID
	SourceClientID int64     `json:"source_client_id"` // 源端客户端ID
	
	// 目标端信息（需要建立连接的客户端）
	TargetClientID int64     `json:"target_client_id"` // 目标客户端ID
	TargetHost     string    `json:"target_host"`      // 目标地址
	TargetPort     int       `json:"target_port"`      // 目标端口
	
	// 元数据
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// TunnelRoutingTable 隧道路由表（Redis-based）
// 负责记录和查询跨服务器隧道的等待状态
type TunnelRoutingTable struct {
	storage storage.Storage
	ttl     time.Duration // Tunnel等待状态的TTL，默认30秒
}

// NewTunnelRoutingTable 创建隧道路由表
func NewTunnelRoutingTable(storage storage.Storage, ttl time.Duration) *TunnelRoutingTable {
	if ttl == 0 {
		ttl = 30 * time.Second
	}
	
	return &TunnelRoutingTable{
		storage: storage,
		ttl:     ttl,
	}
}

// RegisterWaitingTunnel 注册等待中的隧道
// 当源端Server创建TunnelBridge后调用，记录路由信息到Redis
func (t *TunnelRoutingTable) RegisterWaitingTunnel(ctx context.Context, state *TunnelWaitingState) error {
	if state.TunnelID == "" {
		return fmt.Errorf("tunnel_id is required")
	}
	
	// 设置时间戳
	now := time.Now()
	state.CreatedAt = now
	state.ExpiresAt = now.Add(t.ttl)
	
	// 序列化为JSON
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel state: %w", err)
	}
	
	// 存储到Redis，带TTL
	key := t.makeKey(state.TunnelID)
	if err := t.storage.Set(key, data, t.ttl); err != nil {
		return fmt.Errorf("failed to store tunnel state: %w", err)
	}
	
	corelog.Infof("TunnelRouting: registered waiting tunnel %s (source_node=%s, target_client=%d, ttl=%v)",
		state.TunnelID, state.SourceNodeID, state.TargetClientID, t.ttl)
	
	return nil
}

// LookupWaitingTunnel 查找等待中的隧道
// 当目标端Server收到TunnelOpen连接时调用，查询源端Server位置
func (t *TunnelRoutingTable) LookupWaitingTunnel(ctx context.Context, tunnelID string) (*TunnelWaitingState, error) {
	if tunnelID == "" {
		return nil, fmt.Errorf("tunnel_id is required")
	}
	
	key := t.makeKey(tunnelID)
	value, err := t.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil, ErrTunnelNotFound
		}
		return nil, fmt.Errorf("failed to get tunnel state: %w", err)
	}
	
	// 类型断言为[]byte
	data, ok := value.([]byte)
	if !ok {
		// 尝试string类型
		if str, ok := value.(string); ok {
			data = []byte(str)
		} else {
			return nil, fmt.Errorf("unexpected value type: %T, expected []byte or string", value)
		}
	}
	
	// 反序列化
	var state TunnelWaitingState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tunnel state: %w", err)
	}
	
	// 检查是否过期
	if time.Now().After(state.ExpiresAt) {
		// 已过期，删除并返回错误
		t.storage.Delete(key)
		return nil, ErrTunnelExpired
	}
	
	corelog.Debugf("TunnelRouting: found waiting tunnel %s (source_node=%s)",
		tunnelID, state.SourceNodeID)
	
	return &state, nil
}

// RemoveWaitingTunnel 移除等待中的隧道
// 当隧道成功建立或超时后调用，清理Redis中的记录
func (t *TunnelRoutingTable) RemoveWaitingTunnel(ctx context.Context, tunnelID string) error {
	if tunnelID == "" {
		return fmt.Errorf("tunnel_id is required")
	}
	
	key := t.makeKey(tunnelID)
	if err := t.storage.Delete(key); err != nil {
		// 删除失败不是致命错误，因为有TTL自动清理
		corelog.Warnf("TunnelRouting: failed to delete tunnel state for %s: %v", tunnelID, err)
		return nil
	}
	
	corelog.Debugf("TunnelRouting: removed waiting tunnel %s", tunnelID)
	return nil
}

// CleanupExpiredTunnels 清理过期的隧道（可选，Redis TTL会自动清理）
// 这个方法主要用于统计和监控
func (t *TunnelRoutingTable) CleanupExpiredTunnels(ctx context.Context) (int, error) {
	// Redis会自动清理过期的key，这里只是为了统计
	// 实际生产环境中可以通过Redis的SCAN命令遍历所有tunnox:tunnel_waiting:*的key
	// 但为了性能考虑，这里暂不实现全量扫描
	corelog.Debugf("TunnelRouting: cleanup triggered (Redis auto-expires keys)")
	return 0, nil
}

// makeKey 生成Redis key
func (t *TunnelRoutingTable) makeKey(tunnelID string) string {
	return fmt.Sprintf("tunnox:tunnel_waiting:%s", tunnelID)
}

// ErrTunnelNotFound Tunnel未找到错误
var ErrTunnelNotFound = fmt.Errorf("tunnel not found in routing table")

// ErrTunnelExpired Tunnel已过期错误
var ErrTunnelExpired = fmt.Errorf("tunnel waiting state expired")

