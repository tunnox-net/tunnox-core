// Package session 提供会话管理功能
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// ConnectionStateInfo 连接状态信息（用于跨节点查询）
type ConnectionStateInfo struct {
	ConnectionID string    `json:"connection_id"`
	ClientID     int64     `json:"client_id"`
	NodeID       string    `json:"node_id"`
	Protocol     string    `json:"protocol"`
	ConnType     string    `json:"conn_type"` // "control" 或 "tunnel"
	MappingID    string    `json:"mapping_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ConnectionStateStore 连接状态分布式存储
// 负责在 Redis 中存储和查询连接状态，支持跨节点查询
type ConnectionStateStore struct {
	storage storage.Storage
	nodeID  string
	ttl     time.Duration // 连接状态的 TTL，默认 5 分钟
}

// NewConnectionStateStore 创建连接状态存储
func NewConnectionStateStore(storage storage.Storage, nodeID string, ttl time.Duration) *ConnectionStateStore {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return &ConnectionStateStore{
		storage: storage,
		nodeID:  nodeID,
		ttl:     ttl,
	}
}

// RegisterConnection 注册连接状态
// 当连接建立时调用，记录连接所在的节点
func (s *ConnectionStateStore) RegisterConnection(ctx context.Context, state *ConnectionStateInfo) error {
	if state.ConnectionID == "" {
		return fmt.Errorf("connection_id is required")
	}

	// 设置节点ID和时间戳
	state.NodeID = s.nodeID
	now := time.Now()
	state.CreatedAt = now
	state.ExpiresAt = now.Add(s.ttl)

	// 直接存储结构体，让 Storage 层处理序列化
	key := s.makeConnectionKey(state.ConnectionID)
	if err := s.storage.Set(key, state, s.ttl); err != nil {
		return fmt.Errorf("failed to store connection state: %w", err)
	}

	// 如果是控制连接，还需要建立 ClientID -> ConnectionID 的索引
	if state.ConnType == "control" && state.ClientID > 0 {
		clientKey := s.makeClientKey(state.ClientID)
		if err := s.storage.Set(clientKey, state.ConnectionID, s.ttl); err != nil {
			corelog.Warnf("ConnectionStateStore: failed to create client index: %v", err)
		}
	}

	corelog.Debugf("ConnectionStateStore: registered connection %s (node=%s, type=%s, client=%d)",
		state.ConnectionID, s.nodeID, state.ConnType, state.ClientID)

	return nil
}

// UnregisterConnection 注销连接状态
// 当连接断开时调用，清理 Redis 中的记录
func (s *ConnectionStateStore) UnregisterConnection(ctx context.Context, connectionID string) error {
	if connectionID == "" {
		return fmt.Errorf("connection_id is required")
	}

	// 先获取连接状态，以便清理索引
	state, err := s.GetConnectionState(ctx, connectionID)
	if err == nil && state != nil {
		// 清理 ClientID 索引
		if state.ConnType == "control" && state.ClientID > 0 {
			clientKey := s.makeClientKey(state.ClientID)
			if delErr := s.storage.Delete(clientKey); delErr != nil {
				corelog.Warnf("ConnectionStateStore: failed to delete client index: %v", delErr)
			}
		}
	}

	// 删除连接状态
	key := s.makeConnectionKey(connectionID)
	if err := s.storage.Delete(key); err != nil {
		corelog.Warnf("ConnectionStateStore: failed to delete connection state for %s: %v", connectionID, err)
		return nil // 删除失败不是致命错误，因为有 TTL 自动清理
	}

	corelog.Debugf("ConnectionStateStore: unregistered connection %s", connectionID)
	return nil
}

// GetConnectionState 获取连接状态
// 查询连接所在的节点
func (s *ConnectionStateStore) GetConnectionState(ctx context.Context, connectionID string) (*ConnectionStateInfo, error) {
	if connectionID == "" {
		return nil, fmt.Errorf("connection_id is required")
	}

	key := s.makeConnectionKey(connectionID)
	value, err := s.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil, ErrConnectionNotFound
		}
		return nil, fmt.Errorf("failed to get connection state: %w", err)
	}

	// Storage.Get 返回的是反序列化后的 map[string]interface{}
	// 需要重新序列化再反序列化为具体类型
	var state ConnectionStateInfo

	switch v := value.(type) {
	case map[string]interface{}:
		// 重新序列化为 JSON，再反序列化为具体类型
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to re-marshal connection state: %w", err)
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection state: %w", err)
		}
	case []byte:
		// 直接反序列化
		if err := json.Unmarshal(v, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection state: %w", err)
		}
	case string:
		// 字符串类型，尝试反序列化
		if err := json.Unmarshal([]byte(v), &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection state: %w", err)
		}
	default:
		return nil, fmt.Errorf("unexpected value type: %T, expected map[string]interface{}, []byte or string", value)
	}

	// 检查是否过期
	if time.Now().After(state.ExpiresAt) {
		s.storage.Delete(key)
		return nil, ErrConnectionExpired
	}

	return &state, nil
}

// FindConnectionNode 查找连接所在的节点
// 返回节点ID，如果连接不存在返回错误
func (s *ConnectionStateStore) FindConnectionNode(ctx context.Context, connectionID string) (string, error) {
	state, err := s.GetConnectionState(ctx, connectionID)
	if err != nil {
		return "", err
	}
	return state.NodeID, nil
}

// FindClientNode 查找客户端所在的节点
// 通过 ClientID 查找控制连接所在的节点
func (s *ConnectionStateStore) FindClientNode(ctx context.Context, clientID int64) (string, string, error) {
	if clientID <= 0 {
		return "", "", fmt.Errorf("invalid client_id")
	}

	// 先从 ClientID 索引获取 ConnectionID
	clientKey := s.makeClientKey(clientID)
	value, err := s.storage.Get(clientKey)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return "", "", ErrConnectionNotFound
		}
		return "", "", fmt.Errorf("failed to get client index: %w", err)
	}

	var connectionID string
	switch v := value.(type) {
	case string:
		connectionID = v
	case []byte:
		connectionID = string(v)
	default:
		return "", "", fmt.Errorf("unexpected value type: %T", value)
	}

	// 获取连接状态
	state, err := s.GetConnectionState(ctx, connectionID)
	if err != nil {
		return "", "", err
	}

	return state.NodeID, connectionID, nil
}

// RefreshConnection 刷新连接状态的 TTL
// 用于心跳时延长连接状态的有效期
func (s *ConnectionStateStore) RefreshConnection(ctx context.Context, connectionID string) error {
	state, err := s.GetConnectionState(ctx, connectionID)
	if err != nil {
		return err
	}

	// 更新过期时间
	state.ExpiresAt = time.Now().Add(s.ttl)

	// 直接存储结构体，让 Storage 层处理序列化
	key := s.makeConnectionKey(connectionID)
	if err := s.storage.Set(key, state, s.ttl); err != nil {
		return fmt.Errorf("failed to refresh connection state: %w", err)
	}

	return nil
}

// IsConnectionLocal 检查连接是否在本地节点
func (s *ConnectionStateStore) IsConnectionLocal(ctx context.Context, connectionID string) (bool, error) {
	nodeID, err := s.FindConnectionNode(ctx, connectionID)
	if err != nil {
		return false, err
	}
	return nodeID == s.nodeID, nil
}

// makeConnectionKey 生成连接状态的 Redis key
func (s *ConnectionStateStore) makeConnectionKey(connectionID string) string {
	return fmt.Sprintf("tunnox:conn_state:%s", connectionID)
}

// makeClientKey 生成客户端索引的 Redis key
func (s *ConnectionStateStore) makeClientKey(clientID int64) string {
	return fmt.Sprintf("tunnox:client_conn:%d", clientID)
}

// ErrConnectionNotFound 连接未找到错误
var ErrConnectionNotFound = fmt.Errorf("connection not found in state store")

// ErrConnectionExpired 连接状态已过期错误
var ErrConnectionExpired = fmt.Errorf("connection state expired")
