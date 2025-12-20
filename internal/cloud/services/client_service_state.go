package services

import (
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

// ============================================================================
// 客户端状态管理（运行时）
// ============================================================================

// UpdateClientStatus 更新客户端状态（仅运行时状态）
func (s *clientService) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	// 获取当前状态（如果有）
	oldState, _ := s.stateRepo.GetState(clientID)
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
		return fmt.Errorf("failed to update client state: %w", err)
	}

	// 更新节点的客户端列表
	if status == models.ClientStatusOnline && nodeID != "" {
		_ = s.stateRepo.AddToNodeClients(nodeID, clientID)
	} else if oldState != nil && oldState.NodeID != "" {
		_ = s.stateRepo.RemoveFromNodeClients(oldState.NodeID, clientID)
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(utils.Int64ToString(clientID), status, nodeID); err != nil {
			s.baseService.LogWarning("sync status to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if oldStatus != models.ClientStatusOnline && status == models.ClientStatusOnline {
			// 从离线变为在线
			if err := s.statsCounter.IncrOnlineClients(1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, utils.Int64ToString(clientID))
			}
		} else if oldStatus == models.ClientStatusOnline && status != models.ClientStatusOnline {
			// 从在线变为离线
			if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, utils.Int64ToString(clientID))
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
func (s *clientService) ConnectClient(clientID int64, nodeID, connID, ipAddress, protocol, version string) error {
	// 获取旧状态（如果有）
	oldState, _ := s.stateRepo.GetState(clientID)

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
		return fmt.Errorf("failed to set client state: %w", err)
	}

	// 添加到节点列表
	if err := s.stateRepo.AddToNodeClients(nodeID, clientID); err != nil {
		s.baseService.LogWarning("add to node clients", err)
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(utils.Int64ToString(clientID), models.ClientStatusOnline, nodeID); err != nil {
			s.baseService.LogWarning("sync connect to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		// 如果之前是离线，增加在线数
		oldOnline := oldState != nil && oldState.Status == models.ClientStatusOnline
		if !oldOnline {
			if err := s.statsCounter.IncrOnlineClients(1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, utils.Int64ToString(clientID))
			}
		}
	}

	corelog.Infof("Client %d connected to node %s (conn=%s, ip=%s, proto=%s)",
		clientID, nodeID, connID, ipAddress, protocol)
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
func (s *clientService) DisconnectClient(clientID int64) error {
	// 获取当前状态
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return fmt.Errorf("failed to get client state: %w", err)
	}

	if state == nil {
		return nil // 已经离线，无需处理
	}

	// 从节点列表移除
	if state.NodeID != "" {
		if err := s.stateRepo.RemoveFromNodeClients(state.NodeID, clientID); err != nil {
			s.baseService.LogWarning("remove from node clients", err)
		}
	}

	// 删除状态（表示离线）
	if err := s.stateRepo.DeleteState(clientID); err != nil {
		return fmt.Errorf("failed to delete client state: %w", err)
	}

	// ✅ 兼容性：同步到旧Repository
	if s.clientRepo != nil {
		if err := s.clientRepo.UpdateClientStatus(utils.Int64ToString(clientID), models.ClientStatusOffline, ""); err != nil {
			s.baseService.LogWarning("sync disconnect to legacy repo", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil && state.Status == models.ClientStatusOnline {
		if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
			s.baseService.LogWarning("update online clients counter", err, utils.Int64ToString(clientID))
		}
	}

	corelog.Infof("Client %d disconnected from node %s", clientID, state.NodeID)
	return nil
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
func (s *clientService) GetClientNodeID(clientID int64) (string, error) {
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return "", fmt.Errorf("failed to get client state: %w", err)
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
func (s *clientService) IsClientOnNode(clientID int64, nodeID string) (bool, error) {
	state, err := s.stateRepo.GetState(clientID)
	if err != nil {
		return false, fmt.Errorf("failed to get client state: %w", err)
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
func (s *clientService) GetNodeClients(nodeID string) ([]*models.Client, error) {
	// 获取节点的客户端ID列表
	clientIDs, err := s.stateRepo.GetNodeClients(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node clients: %w", err)
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
