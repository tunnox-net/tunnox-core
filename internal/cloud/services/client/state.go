package client

import (
	"encoding/json"
	"sync"
	"time"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils/random"
)

// ============================================================================
// 客户端状态管理（运行时）
// ============================================================================

// UpdateClientStatus 更新客户端状态（仅运行时状态）
func (s *Service) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	// 获取当前状态（如果有）
	// 注意：错误被有意忽略，因为状态可能不存在（客户端从未上线过）
	oldState, err := s.stateRepo.GetState(clientID)
	if err != nil {
		corelog.Debugf("Failed to get client %d state (may not exist): %v", clientID, err)
	}
	oldStatus := models.ClientStatusOffline
	if oldState != nil {
		oldStatus = oldState.Status
	}

	// 构建新状态
	newState := &models.ClientRuntimeState{
		ClientID: clientID,
		NodeID:   nodeID,
		Status:   status,
		LastSeen: time.Now(),
	}

	// 保留部分字段（如果之前有状态）
	if oldState != nil {
		newState.ConnID = oldState.ConnID
		newState.IPAddress = oldState.IPAddress
		newState.Protocol = oldState.Protocol
		newState.Version = oldState.Version
	}

	// 保存状态
	if err := s.stateRepo.SetState(newState); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update client state")
	}

	// 更新节点的客户端列表
	if status == models.ClientStatusOnline && nodeID != "" {
		if err := s.stateRepo.AddToNodeClients(nodeID, clientID); err != nil {
			s.baseService.LogWarning("add to node clients", err)
		}
	} else if oldState != nil && oldState.NodeID != "" {
		if err := s.stateRepo.RemoveFromNodeClients(oldState.NodeID, clientID); err != nil {
			s.baseService.LogWarning("remove from node clients", err)
		}
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(random.Int64ToString(clientID), status, nodeID); err != nil {
			s.baseService.LogWarning("sync status to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if oldStatus != models.ClientStatusOnline && status == models.ClientStatusOnline {
			// 从离线变为在线
			if err := s.statsCounter.IncrOnlineClients(1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, random.Int64ToString(clientID))
			}
		} else if oldStatus == models.ClientStatusOnline && status != models.ClientStatusOnline {
			// 从在线变为离线
			if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, random.Int64ToString(clientID))
			}
		}
	}

	corelog.Infof("Updated client %d status to %s on node %s", clientID, status, nodeID)
	return nil
}

// ConnectClient 客户端连接（更新完整运行时状态）
//
// 调用时机：客户端握手成功后
//
// 参数：
//   - clientID: 客户端ID
//   - nodeID: 节点ID
//   - connID: 连接ID
//   - ipAddress: 客户端IP
//   - protocol: 连接协议
//   - version: 客户端版本
//
// 返回：
//   - error: 错误信息
func (s *Service) ConnectClient(clientID int64, nodeID, connID, ipAddress, protocol, version string) error {
	// 获取旧状态（如果有）
	// 注意：错误被有意忽略，因为状态可能不存在（客户端首次连接）
	oldState, err := s.stateRepo.GetState(clientID)
	if err != nil {
		corelog.Debugf("Failed to get client %d state (may not exist): %v", clientID, err)
	}

	// 构建新状态
	state := &models.ClientRuntimeState{
		ClientID:  clientID,
		NodeID:    nodeID,
		ConnID:    connID,
		Status:    models.ClientStatusOnline,
		IPAddress: ipAddress,
		Protocol:  protocol,
		Version:   version,
		LastSeen:  time.Now(),
	}

	// 保存状态
	if err := s.stateRepo.SetState(state); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to set client state")
	}

	// 添加到节点列表
	if err := s.stateRepo.AddToNodeClients(nodeID, clientID); err != nil {
		s.baseService.LogWarning("add to node clients", err)
	}

	// ✅ 首次连接检测：更新 FirstConnectedAt（激活时间）
	if s.configRepo != nil {
		cfg, err := s.configRepo.GetConfig(clientID)
		if err == nil && cfg != nil && cfg.FirstConnectedAt == nil {
			now := time.Now()
			cfg.FirstConnectedAt = &now
			if err := s.configRepo.UpdateConfig(cfg); err != nil {
				s.baseService.LogWarning("update first connected at", err, random.Int64ToString(clientID))
			} else {
				corelog.Infof("Client %d activated (first connected)", clientID)
			}
		}
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(random.Int64ToString(clientID), models.ClientStatusOnline, nodeID); err != nil {
			s.baseService.LogWarning("sync connect to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		// 如果之前是离线，增加在线数
		oldOnline := oldState != nil && oldState.Status == models.ClientStatusOnline
		if !oldOnline {
			if err := s.statsCounter.IncrOnlineClients(1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, random.Int64ToString(clientID))
			}
		}
	}

	corelog.Infof("Client %d connected to node %s (conn=%s, ip=%s, proto=%s)",
		clientID, nodeID, connID, ipAddress, protocol)

	// 发布客户端上线事件
	s.publishClientOnlineEvent(clientID, nodeID, ipAddress)

	return nil
}

// DisconnectClient 客户端断开连接
//
// 调用时机：客户端断开连接后
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (s *Service) DisconnectClient(clientID int64) error {
	// 获取当前状态
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client state")
	}

	if state == nil {
		return nil // 已经离线，无需处理
	}

	// ✅ 保存最后的 IP 信息到持久化配置（离线后仍可查看）
	if s.configRepo != nil && state.IPAddress != "" {
		cfg, err := s.configRepo.GetConfig(clientID)
		if err == nil && cfg != nil {
			cfg.LastIPAddress = state.IPAddress
			// IPRegion 需要从其他地方获取，这里暂时不设置
			// 因为 ClientRuntimeState 没有 IPRegion 字段
			if err := s.configRepo.UpdateConfig(cfg); err != nil {
				s.baseService.LogWarning("save last IP to config", err, random.Int64ToString(clientID))
			} else {
				corelog.Debugf("Client %d: saved last IP %s to config", clientID, state.IPAddress)
			}
		}
	}

	// 从节点列表移除
	if state.NodeID != "" {
		if err := s.stateRepo.RemoveFromNodeClients(state.NodeID, clientID); err != nil {
			s.baseService.LogWarning("remove from node clients", err)
		}
	}

	// 删除状态（表示离线）
	if err := s.stateRepo.DeleteState(clientID); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete client state")
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(random.Int64ToString(clientID), models.ClientStatusOffline, ""); err != nil {
			s.baseService.LogWarning("sync disconnect to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil && state.Status == models.ClientStatusOnline {
		if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
			s.baseService.LogWarning("update online clients counter", err, random.Int64ToString(clientID))
		}
	}

	corelog.Infof("Client %d disconnected from node %s", clientID, state.NodeID)

	// 发布客户端下线事件
	s.publishClientOfflineEvent(clientID)

	return nil
}

// DisconnectClientIfMatch 断开客户端连接（仅当 nodeID 和 connID 匹配时）
//
// 调用时机：清理过期连接时调用，避免误删其他节点的新连接状态
//
// 参数：
//   - clientID: 客户端ID
//   - nodeID: 期望的节点ID
//   - connID: 期望的连接ID
//
// 返回：
//   - bool: 是否执行了断开操作
//   - error: 错误信息
//
// 说明：
//
//	如果 Redis 中的状态 nodeID/connID 与参数不匹配，说明客户端已在其他节点重连，
//	此时不应删除状态，避免造成状态闪烁（在线→离线→在线）
func (s *Service) DisconnectClientIfMatch(clientID int64, nodeID, connID string) (bool, error) {
	// 获取当前状态
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client state")
	}

	if state == nil {
		return false, nil // 已经离线，无需处理
	}

	// 检查状态是否匹配
	if state.NodeID != nodeID || state.ConnID != connID {
		corelog.Infof("DisconnectClientIfMatch: skipped - client %d has reconnected (state: node=%s/conn=%s, expected: node=%s/conn=%s)",
			clientID, state.NodeID, state.ConnID, nodeID, connID)
		return false, nil // 状态已被新连接覆盖，跳过
	}

	// 状态匹配，执行断开逻辑
	corelog.Infof("DisconnectClientIfMatch: proceeding - client %d state matches (node=%s, conn=%s)", clientID, nodeID, connID)

	// 保存最后的 IP 信息到持久化配置
	if s.configRepo != nil && state.IPAddress != "" {
		cfg, err := s.configRepo.GetConfig(clientID)
		if err == nil && cfg != nil {
			cfg.LastIPAddress = state.IPAddress
			if err := s.configRepo.UpdateConfig(cfg); err != nil {
				s.baseService.LogWarning("save last IP to config", err, random.Int64ToString(clientID))
			}
		}
	}

	// 从节点列表移除
	if state.NodeID != "" {
		if err := s.stateRepo.RemoveFromNodeClients(state.NodeID, clientID); err != nil {
			s.baseService.LogWarning("remove from node clients", err)
		}
	}

	// 删除状态
	if err := s.stateRepo.DeleteState(clientID); err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete client state")
	}

	// 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(random.Int64ToString(clientID), models.ClientStatusOffline, ""); err != nil {
			s.baseService.LogWarning("sync disconnect to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil && state.Status == models.ClientStatusOnline {
		if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
			s.baseService.LogWarning("update online clients counter", err, random.Int64ToString(clientID))
		}
	}

	corelog.Infof("Client %d disconnected from node %s (matched)", clientID, state.NodeID)

	// 发布客户端下线事件
	s.publishClientOfflineEvent(clientID)

	return true, nil
}

// EnsureClientOnline 确保客户端在线状态存在
//
// 调用时机：每次心跳时调用，代替 TouchClient
//
// 与 ConnectClient 的区别：
//   - 如果状态存在，只刷新 TTL（不发布事件）
//   - 如果状态丢失（Redis TTL 过期/重启），重建状态（不发布事件，避免重复通知）
//
// 这解决了以下问题：
//   - Redis 状态因 TTL 过期丢失后，TouchState 无法恢复
//   - 客户端实际在线但显示离线的状态不一致问题
func (s *Service) EnsureClientOnline(clientID int64, nodeID, connID, ipAddress, protocol, version string) error {
	// 获取当前状态
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client state")
	}

	if state != nil {
		// 状态存在，只刷新 TTL
		state.Touch()
		if err := s.stateRepo.SetState(state); err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to touch client state")
		}
		return nil
	}

	// 状态丢失，需要重建
	corelog.Warnf("Client %d state lost, rebuilding (node=%s, conn=%s)", clientID, nodeID, connID)

	// 构建新状态
	newState := &models.ClientRuntimeState{
		ClientID:  clientID,
		NodeID:    nodeID,
		ConnID:    connID,
		Status:    models.ClientStatusOnline,
		IPAddress: ipAddress,
		Protocol:  protocol,
		Version:   version,
		LastSeen:  time.Now(),
	}

	// 保存状态
	if err := s.stateRepo.SetState(newState); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to rebuild client state")
	}

	// 添加到节点列表
	if err := s.stateRepo.AddToNodeClients(nodeID, clientID); err != nil {
		s.baseService.LogWarning("add to node clients", err)
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(random.Int64ToString(clientID), models.ClientStatusOnline, nodeID); err != nil {
			s.baseService.LogWarning("sync status to legacy repo", err)
		}
	}

	corelog.Infof("Client %d state rebuilt on node %s", clientID, nodeID)

	// 注意：不发布 online 事件，因为这是状态恢复，不是真正的上线
	// 如果需要发布事件，可以添加一个 "state_recovered" 事件类型

	return nil
}

func (s *Service) publishClientOnlineEvent(clientID int64, nodeID, ipAddress string) {
	// 获取客户端所属用户（用于 SSE 定向推送和 Webhook）
	userID := ""
	if s.configRepo != nil {
		if cfg, err := s.configRepo.GetConfig(clientID); err == nil && cfg != nil {
			userID = cfg.UserID
		}
	}

	if s.broker != nil {
		msg := broker.ClientOnlineMessage{
			ClientID:  clientID,
			UserID:    userID,
			NodeID:    nodeID,
			IPAddress: ipAddress,
			Timestamp: time.Now().Unix(),
		}

		data, err := json.Marshal(msg)
		if err != nil {
			corelog.Errorf("Failed to marshal client online message: %v", err)
		} else if err := s.broker.Publish(s.Ctx(), broker.TopicClientOnline, data); err != nil {
			corelog.Warnf("Failed to publish client online event for client %d: %v", clientID, err)
		} else {
			corelog.Debugf("Published client online event: client_id=%d, user_id=%s, node_id=%s, ip=%s",
				clientID, userID, nodeID, ipAddress)
		}
	}

	if s.webhookNotifier != nil {
		s.webhookNotifier.DispatchClientOnline(clientID, userID, ipAddress, nodeID)
	}
}

func (s *Service) publishClientOfflineEvent(clientID int64) {
	// 获取客户端所属用户（用于 SSE 定向推送和 Webhook）
	userID := ""
	if s.configRepo != nil {
		if cfg, err := s.configRepo.GetConfig(clientID); err == nil && cfg != nil {
			userID = cfg.UserID
		}
	}

	if s.broker != nil {
		ts := time.Now()
		msg := broker.ClientOfflineMessage{
			ClientID:  clientID,
			UserID:    userID,
			Timestamp: ts.Unix(),
		}

		data, err := json.Marshal(msg)
		if err != nil {
			corelog.Errorf("Failed to marshal client offline message: %v", err)
		} else {
			corelog.Infof("[SSE-TRACE] publishClientOfflineEvent: client=%d, user=%s, ts=%d, publishing...",
				clientID, userID, ts.UnixMilli())

			if err := s.broker.Publish(s.Ctx(), broker.TopicClientOffline, data); err != nil {
				corelog.Warnf("[SSE-TRACE] publishClientOfflineEvent: FAILED client=%d, err=%v", clientID, err)
			} else {
				corelog.Infof("[SSE-TRACE] publishClientOfflineEvent: DONE client=%d, elapsed=%dms",
					clientID, time.Since(ts).Milliseconds())
			}
		}
	} else {
		corelog.Warnf("[SSE-TRACE] publishClientOfflineEvent: broker is nil, client=%d", clientID)
	}

	if s.webhookNotifier != nil {
		s.webhookNotifier.DispatchClientOffline(clientID, userID)
	}
}

// ============================================================================
// 客户端状态查询（快速接口，仅查State）
// ============================================================================

// GetClientNodeID 获取客户端所在节点（快速查询）
//
// 用途：API推送配置前，快速确定客户端在哪个节点
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - string: 节点ID（空字符串表示离线）
//   - error: 错误信息
func (s *Service) GetClientNodeID(clientID int64) (string, error) {
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client state")
	}

	if state == nil || !state.IsOnline() {
		return "", nil // 离线或不存在
	}

	return state.NodeID, nil
}

// IsClientOnNode 检查客户端是否在指定节点
//
// 参数：
//   - clientID: 客户端ID
//   - nodeID: 节点ID
//
// 返回：
//   - bool: 是否在指定节点
//   - error: 错误信息
func (s *Service) IsClientOnNode(clientID int64, nodeID string) (bool, error) {
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get client state")
	}

	if state == nil {
		return false, nil
	}

	return state.IsOnNode(nodeID), nil
}

// GetNodeClients 获取节点的所有在线客户端
//
// 参数：
//   - nodeID: 节点ID
//
// 返回：
//   - []*models.Client: 客户端列表
//   - error: 错误信息
func (s *Service) GetNodeClients(nodeID string) ([]*models.Client, error) {
	// 获取节点的客户端ID列表
	clientIDs, err := s.stateRepo.GetNodeClients(nodeID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get node clients")
	}

	// 并发获取每个客户端的完整信息
	clients := make([]*models.Client, 0, len(clientIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, clientID := range clientIDs {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			client, err := s.GetClient(id)
			if err == nil && client != nil && client.IsOnline() {
				mu.Lock()
				clients = append(clients, client)
				mu.Unlock()
			}
		}(clientID)
	}

	wg.Wait()
	return clients, nil
}

func (s *Service) CleanupStaleNodeClients(nodeID string) (int64, error) {
	return s.stateRepo.CleanupStaleClients(nodeID)
}
