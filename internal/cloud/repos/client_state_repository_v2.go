// Package repos 提供数据访问层实现
package repos

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// ClientStateRepositoryV2 使用新存储架构的客户端状态 Repository
// =============================================================================

// 编译时接口验证
var _ IClientStateRepository = (*ClientStateRepositoryV2)(nil)

// ClientStateRepositoryV2 客户端状态 Repository（新架构版本）
//
// 特点：
//   - 仅使用共享存储（Redis），无持久化
//   - 自动 TTL 过期
//   - 节点→客户端索引（SET 实现）
type ClientStateRepositoryV2 struct {
	*dispose.ManagerBase

	// stateStore 状态存储（支持 TTL）
	stateStore store.TTLStore[string, *models.ClientRuntimeState]

	// nodeClientIndex 节点→客户端索引存储
	nodeClientIndex store.SetStore[string, string]

	// stateTTL 状态过期时间
	stateTTL time.Duration

	// nodeClientTTL 节点客户端列表过期时间
	nodeClientTTL time.Duration

	// ctx 操作上下文
	ctx context.Context
}

// ClientStateRepoV2Config 创建 ClientStateRepositoryV2 的配置
type ClientStateRepoV2Config struct {
	// StateStore 状态存储（支持 TTL）
	StateStore store.TTLStore[string, *models.ClientRuntimeState]

	// NodeClientIndex 节点→客户端索引存储
	NodeClientIndex store.SetStore[string, string]

	// Ctx 父上下文
	Ctx context.Context
}

// NewClientStateRepositoryV2 创建新版本的 ClientState Repository
func NewClientStateRepositoryV2(cfg ClientStateRepoV2Config) *ClientStateRepositoryV2 {
	repo := &ClientStateRepositoryV2{
		ManagerBase:     dispose.NewManager("ClientStateRepositoryV2", cfg.Ctx),
		stateStore:      cfg.StateStore,
		nodeClientIndex: cfg.NodeClientIndex,
		stateTTL:        time.Duration(constants.TTLClientState) * time.Second,
		nodeClientTTL:   time.Duration(constants.TTLNodeClients) * time.Second,
		ctx:             cfg.Ctx,
	}

	repo.SetCtx(cfg.Ctx, repo.onClose)
	return repo
}

// onClose 资源清理回调
func (r *ClientStateRepositoryV2) onClose() error {
	corelog.Infof("ClientStateRepositoryV2: closing")
	return nil
}

// =============================================================================
// IClientStateRepository 接口实现
// =============================================================================

// GetState 获取客户端状态
func (r *ClientStateRepositoryV2) GetState(clientID int64) (*models.ClientRuntimeState, error) {
	key := r.buildStateKey(clientID)

	state, err := r.stateStore.Get(r.ctx, key)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, nil // 状态不存在 = 离线
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client state")
	}

	return state, nil
}

// SetState 设置客户端状态
func (r *ClientStateRepositoryV2) SetState(state *models.ClientRuntimeState) error {
	if state == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "state is nil")
	}

	// 验证状态
	if err := state.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid state")
	}

	key := r.buildStateKey(state.ClientID)

	// 写入状态（带 TTL）
	if err := r.stateStore.SetWithTTL(r.ctx, key, state, r.stateTTL); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to set state")
	}

	corelog.Debugf("ClientStateRepositoryV2: set state for client %d (node=%s, status=%s)",
		state.ClientID, state.NodeID, state.Status)

	return nil
}

// DeleteState 删除客户端状态
func (r *ClientStateRepositoryV2) DeleteState(clientID int64) error {
	key := r.buildStateKey(clientID)

	if err := r.stateStore.Delete(r.ctx, key); err != nil {
		if !store.IsNotFound(err) {
			return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete state")
		}
	}

	corelog.Debugf("ClientStateRepositoryV2: deleted state for client %d", clientID)
	return nil
}

// TouchState 更新客户端心跳时间
func (r *ClientStateRepositoryV2) TouchState(clientID int64) error {
	state, err := r.GetState(clientID)
	if err != nil || state == nil {
		return err
	}

	state.Touch()
	return r.SetState(state)
}

// =============================================================================
// 节点客户端列表管理
// =============================================================================

// GetNodeClients 获取指定节点的所有在线客户端ID列表
func (r *ClientStateRepositoryV2) GetNodeClients(nodeID string) ([]int64, error) {
	key := r.buildNodeClientsKey(nodeID)

	// 从 SET 获取所有成员
	members, err := r.nodeClientIndex.Members(r.ctx, key)
	if err != nil {
		if store.IsNotFound(err) {
			return []int64{}, nil
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get node clients")
	}

	// 转换为 int64
	clientIDs := make([]int64, 0, len(members))
	for _, member := range members {
		var clientID int64
		if _, err := fmt.Sscanf(member, "%d", &clientID); err == nil {
			clientIDs = append(clientIDs, clientID)
		}
	}

	return clientIDs, nil
}

// AddToNodeClients 将客户端添加到节点的客户端列表
func (r *ClientStateRepositoryV2) AddToNodeClients(nodeID string, clientID int64) error {
	if nodeID == "" {
		return coreerrors.New(coreerrors.CodeInvalidParam, "node_id is empty")
	}

	key := r.buildNodeClientsKey(nodeID)
	member := fmt.Sprintf("%d", clientID)

	if err := r.nodeClientIndex.Add(r.ctx, key, member); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to add client to node")
	}

	// 刷新 TTL（如果 SetStore 支持）
	if ttlStore, ok := r.nodeClientIndex.(store.TTLStore[string, string]); ok {
		_ = ttlStore.Refresh(r.ctx, key, r.nodeClientTTL)
	}

	corelog.Debugf("ClientStateRepositoryV2: added client %d to node %s", clientID, nodeID)
	return nil
}

// RemoveFromNodeClients 从节点的客户端列表中移除客户端
func (r *ClientStateRepositoryV2) RemoveFromNodeClients(nodeID string, clientID int64) error {
	if nodeID == "" {
		return nil
	}

	key := r.buildNodeClientsKey(nodeID)
	member := fmt.Sprintf("%d", clientID)

	if err := r.nodeClientIndex.Remove(r.ctx, key, member); err != nil {
		if !store.IsNotFound(err) {
			return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to remove client from node")
		}
	}

	corelog.Debugf("ClientStateRepositoryV2: removed client %d from node %s", clientID, nodeID)
	return nil
}

// =============================================================================
// 辅助方法
// =============================================================================

// buildStateKey 构建状态存储键
func (r *ClientStateRepositoryV2) buildStateKey(clientID int64) string {
	return fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientState, clientID)
}

// buildNodeClientsKey 构建节点客户端列表键
func (r *ClientStateRepositoryV2) buildNodeClientsKey(nodeID string) string {
	return fmt.Sprintf("%s%s", constants.KeyPrefixRuntimeNodeClients, nodeID)
}

// =============================================================================
// 扩展方法
// =============================================================================

// BatchGetStates 批量获取客户端状态
func (r *ClientStateRepositoryV2) BatchGetStates(clientIDs []int64) (map[int64]*models.ClientRuntimeState, error) {
	if len(clientIDs) == 0 {
		return map[int64]*models.ClientRuntimeState{}, nil
	}

	// 检查是否支持批量操作
	batchStore, ok := r.stateStore.(store.BatchStore[string, *models.ClientRuntimeState])
	if !ok {
		// 降级到单个查询
		result := make(map[int64]*models.ClientRuntimeState)
		for _, clientID := range clientIDs {
			state, err := r.GetState(clientID)
			if err == nil && state != nil {
				result[clientID] = state
			}
		}
		return result, nil
	}

	// 构建 keys
	keys := make([]string, len(clientIDs))
	for i, clientID := range clientIDs {
		keys[i] = r.buildStateKey(clientID)
	}

	// 批量获取
	stateMap, err := batchStore.BatchGet(r.ctx, keys)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "batch get states failed")
	}

	// 转换结果
	result := make(map[int64]*models.ClientRuntimeState, len(stateMap))
	for _, state := range stateMap {
		if state != nil {
			result[state.ClientID] = state
		}
	}

	return result, nil
}

// GetOnlineClientsForNode 获取节点上所有在线客户端的完整状态
func (r *ClientStateRepositoryV2) GetOnlineClientsForNode(nodeID string) ([]*models.ClientRuntimeState, error) {
	// 获取节点的客户端 ID 列表
	clientIDs, err := r.GetNodeClients(nodeID)
	if err != nil {
		return nil, err
	}

	if len(clientIDs) == 0 {
		return []*models.ClientRuntimeState{}, nil
	}

	// 批量获取状态
	stateMap, err := r.BatchGetStates(clientIDs)
	if err != nil {
		return nil, err
	}

	// 过滤出真正在线的客户端
	states := make([]*models.ClientRuntimeState, 0, len(stateMap))
	for _, state := range stateMap {
		if state != nil && state.IsOnline() {
			states = append(states, state)
		}
	}

	return states, nil
}

// CountNodeClients 统计节点上的客户端数量
func (r *ClientStateRepositoryV2) CountNodeClients(nodeID string) (int64, error) {
	key := r.buildNodeClientsKey(nodeID)
	count, err := r.nodeClientIndex.Size(r.ctx, key)
	if err != nil {
		if store.IsNotFound(err) {
			return 0, nil
		}
		return 0, coreerrors.Wrap(err, coreerrors.CodeStorageError, "count node clients failed")
	}
	return count, nil
}
