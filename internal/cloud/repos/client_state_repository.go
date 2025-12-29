package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// 编译时接口断言，确保 ClientStateRepository 实现了 IClientStateRepository 接口
var _ IClientStateRepository = (*ClientStateRepository)(nil)

// ClientStateRepository 客户端状态数据访问层
//
// 职责：
// - 管理客户端运行时状态（仅缓存，不持久化到数据库）
// - 提供快速的状态查询接口
// - 管理节点的客户端列表
//
// 数据存储：
// - 键前缀：tunnox:runtime:client:state:
// - 存储：仅缓存（Redis/Memory）
// - TTL：90秒（心跳间隔30秒 * 3）
type ClientStateRepository struct {
	*dispose.ManagerBase
	storage storage.Storage
}

// NewClientStateRepository 创建Repository
//
// 参数：
//   - ctx: 上下文（用于Dispose）
//   - storage: 存储接口
//
// 返回：
//   - *ClientStateRepository: 状态Repository实例
func NewClientStateRepository(ctx context.Context, storage storage.Storage) *ClientStateRepository {
	repo := &ClientStateRepository{
		ManagerBase: dispose.NewManager("ClientStateRepository", ctx),
		storage:     storage,
	}

	// 设置清理回调
	repo.SetCtx(ctx, repo.onClose)

	return repo
}

// onClose 资源清理回调
func (r *ClientStateRepository) onClose() error {
	corelog.Infof("ClientStateRepository: closing")
	// 状态数据存储在缓存中，会自动过期，无需手动清理
	return nil
}

// GetState 获取客户端状态
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - *models.ClientRuntimeState: 状态对象（如果不存在返回nil）
//   - error: 错误信息
func (r *ClientStateRepository) GetState(clientID int64) (*models.ClientRuntimeState, error) {
	key := fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientState, clientID)

	value, err := r.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil, nil // 状态不存在 = 离线
		}
		return nil, fmt.Errorf("failed to get client state: %w", err)
	}

	// 反序列化
	var state models.ClientRuntimeState
	if jsonStr, ok := value.(string); ok {
		if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}
	} else {
		return nil, fmt.Errorf("invalid state type: %T", value)
	}

	return &state, nil
}

// SetState 设置客户端状态
//
// 参数：
//   - state: 客户端状态
//
// 返回：
//   - error: 错误信息
func (r *ClientStateRepository) SetState(state *models.ClientRuntimeState) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}

	// 验证状态有效性
	if err := state.Validate(); err != nil {
		return fmt.Errorf("invalid state: %w", err)
	}

	key := fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientState, state.ClientID)

	// 序列化
	jsonBytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// 写入缓存，TTL = 90秒
	ttl := time.Duration(constants.TTLClientState) * time.Second
	if err := r.storage.Set(key, string(jsonBytes), ttl); err != nil {
		return fmt.Errorf("failed to set state: %w", err)
	}

	corelog.Debugf("ClientStateRepository: set state for client %d (node=%s, status=%s)",
		state.ClientID, state.NodeID, state.Status)

	return nil
}

// DeleteState 删除客户端状态
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientStateRepository) DeleteState(clientID int64) error {
	key := fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientState, clientID)

	if err := r.storage.Delete(key); err != nil && err != storage.ErrKeyNotFound {
		return fmt.Errorf("failed to delete state: %w", err)
	}

	corelog.Debugf("ClientStateRepository: deleted state for client %d", clientID)
	return nil
}

// TouchState 更新客户端心跳时间
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientStateRepository) TouchState(clientID int64) error {
	state, err := r.GetState(clientID)
	if err != nil || state == nil {
		return err
	}

	state.Touch()
	return r.SetState(state)
}

// ============================================================================
// 节点客户端列表管理
// ============================================================================

// GetNodeClients 获取指定节点的所有在线客户端ID列表
//
// 参数：
//   - nodeID: 节点ID
//
// 返回：
//   - []int64: 客户端ID列表
//   - error: 错误信息
func (r *ClientStateRepository) GetNodeClients(nodeID string) ([]int64, error) {
	key := fmt.Sprintf("%s%s", constants.KeyPrefixRuntimeNodeClients, nodeID)

	value, err := r.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return []int64{}, nil // 空列表
		}
		return nil, fmt.Errorf("failed to get node clients: %w", err)
	}

	var clientIDs []int64
	if jsonStr, ok := value.(string); ok {
		if err := json.Unmarshal([]byte(jsonStr), &clientIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal client IDs: %w", err)
		}
	}

	return clientIDs, nil
}

// AddToNodeClients 将客户端添加到节点的客户端列表
//
// 参数：
//   - nodeID: 节点ID
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientStateRepository) AddToNodeClients(nodeID string, clientID int64) error {
	if nodeID == "" {
		return fmt.Errorf("node_id is empty")
	}

	key := fmt.Sprintf("%s%s", constants.KeyPrefixRuntimeNodeClients, nodeID)

	// 获取当前列表
	clientIDs, err := r.GetNodeClients(nodeID)
	if err != nil {
		return err
	}

	// 检查是否已存在
	for _, id := range clientIDs {
		if id == clientID {
			return nil // 已存在，无需重复添加
		}
	}

	// 添加到列表
	clientIDs = append(clientIDs, clientID)

	// 序列化并保存
	jsonBytes, _ := json.Marshal(clientIDs)
	ttl := time.Duration(constants.TTLNodeClients) * time.Second

	if err := r.storage.Set(key, string(jsonBytes), ttl); err != nil {
		return fmt.Errorf("failed to save node clients: %w", err)
	}

	corelog.Debugf("ClientStateRepository: added client %d to node %s", clientID, nodeID)
	return nil
}

// RemoveFromNodeClients 从节点的客户端列表中移除客户端
//
// 参数：
//   - nodeID: 节点ID
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientStateRepository) RemoveFromNodeClients(nodeID string, clientID int64) error {
	if nodeID == "" {
		return nil // 空nodeID，忽略
	}

	key := fmt.Sprintf("%s%s", constants.KeyPrefixRuntimeNodeClients, nodeID)

	// 获取当前列表
	clientIDs, err := r.GetNodeClients(nodeID)
	if err != nil {
		return err
	}

	// 过滤掉指定clientID
	newIDs := make([]int64, 0, len(clientIDs))
	for _, id := range clientIDs {
		if id != clientID {
			newIDs = append(newIDs, id)
		}
	}

	// 如果列表为空，删除key
	if len(newIDs) == 0 {
		return r.storage.Delete(key)
	}

	// 保存新列表
	jsonBytes, _ := json.Marshal(newIDs)
	ttl := time.Duration(constants.TTLNodeClients) * time.Second

	if err := r.storage.Set(key, string(jsonBytes), ttl); err != nil {
		return fmt.Errorf("failed to save node clients: %w", err)
	}

	corelog.Debugf("ClientStateRepository: removed client %d from node %s", clientID, nodeID)
	return nil
}
