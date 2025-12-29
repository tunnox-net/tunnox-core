package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// ============================================================================
// 连接状态分布式存储（跨节点查询）
// ============================================================================

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
func (s *ConnectionStateStore) RegisterConnection(ctx context.Context, state *ConnectionStateInfo) error {
	if state.ConnectionID == "" {
		return fmt.Errorf("connection_id is required")
	}

	state.NodeID = s.nodeID
	now := time.Now()
	state.CreatedAt = now
	state.ExpiresAt = now.Add(s.ttl)

	key := s.makeConnectionKey(state.ConnectionID)
	if err := s.storage.Set(key, state, s.ttl); err != nil {
		return fmt.Errorf("failed to store connection state: %w", err)
	}

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
func (s *ConnectionStateStore) UnregisterConnection(ctx context.Context, connectionID string) error {
	if connectionID == "" {
		return fmt.Errorf("connection_id is required")
	}

	state, err := s.GetConnectionState(ctx, connectionID)
	if err == nil && state != nil {
		if state.ConnType == "control" && state.ClientID > 0 {
			clientKey := s.makeClientKey(state.ClientID)
			if delErr := s.storage.Delete(clientKey); delErr != nil {
				corelog.Warnf("ConnectionStateStore: failed to delete client index: %v", delErr)
			}
		}
	}

	key := s.makeConnectionKey(connectionID)
	if err := s.storage.Delete(key); err != nil {
		corelog.Warnf("ConnectionStateStore: failed to delete connection state for %s: %v", connectionID, err)
		return nil
	}

	corelog.Debugf("ConnectionStateStore: unregistered connection %s", connectionID)
	return nil
}

// GetConnectionState 获取连接状态
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

	var state ConnectionStateInfo

	switch v := value.(type) {
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to re-marshal connection state: %w", err)
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection state: %w", err)
		}
	case []byte:
		if err := json.Unmarshal(v, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection state: %w", err)
		}
	case string:
		if err := json.Unmarshal([]byte(v), &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection state: %w", err)
		}
	default:
		return nil, fmt.Errorf("unexpected value type: %T", value)
	}

	if time.Now().After(state.ExpiresAt) {
		s.storage.Delete(key)
		return nil, ErrConnectionExpired
	}

	return &state, nil
}

// FindConnectionNode 查找连接所在的节点
func (s *ConnectionStateStore) FindConnectionNode(ctx context.Context, connectionID string) (string, error) {
	state, err := s.GetConnectionState(ctx, connectionID)
	if err != nil {
		return "", err
	}
	return state.NodeID, nil
}

// FindClientNode 查找客户端所在的节点
func (s *ConnectionStateStore) FindClientNode(ctx context.Context, clientID int64) (string, string, error) {
	if clientID <= 0 {
		return "", "", fmt.Errorf("invalid client_id")
	}

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

	state, err := s.GetConnectionState(ctx, connectionID)
	if err != nil {
		return "", "", err
	}

	return state.NodeID, connectionID, nil
}

// RefreshConnection 刷新连接状态的 TTL
func (s *ConnectionStateStore) RefreshConnection(ctx context.Context, connectionID string) error {
	state, err := s.GetConnectionState(ctx, connectionID)
	if err != nil {
		return err
	}

	state.ExpiresAt = time.Now().Add(s.ttl)

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

func (s *ConnectionStateStore) makeConnectionKey(connectionID string) string {
	return fmt.Sprintf("tunnox:conn_state:%s", connectionID)
}

func (s *ConnectionStateStore) makeClientKey(clientID int64) string {
	return fmt.Sprintf("tunnox:client_conn:%d", clientID)
}

// ErrConnectionNotFound 连接未找到错误
var ErrConnectionNotFound = fmt.Errorf("connection not found in state store")

// ErrConnectionExpired 连接状态已过期错误
var ErrConnectionExpired = fmt.Errorf("connection state expired")
