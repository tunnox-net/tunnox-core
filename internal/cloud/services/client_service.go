package services

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/utils"
)

// clientService 客户端服务实现
//
// 职责：
// - 聚合ClientConfig, ClientRuntimeState, ClientToken
// - 提供完整的客户端业务逻辑
// - 管理客户端连接状态
//
// 数据分离：
// - ClientConfig: 持久化配置（数据库+缓存）
// - ClientRuntimeState: 运行时状态（仅缓存，TTL=90秒）
// - ClientToken: JWT Token（仅缓存，自动过期）
type clientService struct {
	*dispose.ServiceBase
	baseService *BaseService

	// 新的Repository（分离存储）
	configRepo *repos.ClientConfigRepository
	stateRepo  *repos.ClientStateRepository
	tokenRepo  *repos.ClientTokenRepository

	// 保留的Repository（兼容性）
	clientRepo  *repos.ClientRepository // 旧版，逐步迁移
	mappingRepo *repos.PortMappingRepo

	// 其他依赖
	idManager    *idgen.IDManager
	statsMgr     *managers.StatsManager
	statsCounter *stats.StatsCounter
}

// NewClientService 创建客户端服务
//
// 参数：
//   - configRepo: 配置Repository
//   - stateRepo: 状态Repository
//   - tokenRepo: TokenRepository
//   - clientRepo: 旧版Repository（兼容性，逐步迁移）
//   - mappingRepo: 映射Repository
//   - idManager: ID管理器
//   - statsMgr: 统计管理器
//   - parentCtx: 父上下文
//
// 返回：
//   - ClientService: 客户端服务接口
func NewClientService(
	configRepo *repos.ClientConfigRepository,
	stateRepo *repos.ClientStateRepository,
	tokenRepo *repos.ClientTokenRepository,
	clientRepo *repos.ClientRepository,
	mappingRepo *repos.PortMappingRepo,
	idManager *idgen.IDManager,
	statsMgr *managers.StatsManager,
	parentCtx context.Context,
) ClientService {
	service := &clientService{
		ServiceBase:  dispose.NewService("ClientService", parentCtx),
		baseService:  NewBaseService(),
		configRepo:   configRepo,
		stateRepo:    stateRepo,
		tokenRepo:    tokenRepo,
		clientRepo:   clientRepo,
		mappingRepo:  mappingRepo,
		idManager:    idManager,
		statsMgr:     statsMgr,
		statsCounter: statsMgr.GetCounter(),
	}
	return service
}

// ============================================================================
// 客户端CRUD操作
// ============================================================================

// CreateClient 创建客户端
func (s *clientService) CreateClient(userID, clientName string) (*models.Client, error) {
	// 生成客户端ID
	clientID, err := s.idManager.GenerateClientID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate client ID")
	}

	// 生成认证码和密钥
	authCode, err := s.idManager.GenerateAuthCode()
	if err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate auth code")
	}

	secretKey, err := s.idManager.GenerateSecretKey()
	if err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate secret key")
	}

	// 创建客户端配置
	now := time.Now()
	config := &models.ClientConfig{
		ID:        clientID,
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Type:      models.ClientTypeRegistered,
		Config:    s.getDefaultClientConfig(),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 保存配置到持久化存储
	if err := s.configRepo.SaveConfig(config); err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "save client config")
	}

	// 添加到全局列表
	if err := s.configRepo.AddConfigToList(config); err != nil {
		s.baseService.LogWarning("add config to list", err)
	}

	// ✅ 兼容性：同步到旧的ClientRepository
	legacyClient := models.FromConfigAndState(config, nil, nil)
	if err := s.clientRepo.CreateClient(legacyClient); err != nil {
		s.baseService.LogWarning("sync to legacy client repo", err)
	}

	// 添加到用户客户端列表
	if userID != "" && s.clientRepo != nil {
		if err := s.clientRepo.AddClientToUser(userID, legacyClient); err != nil {
			s.baseService.LogWarning("add client to user list", err)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrClient(1); err != nil {
			s.baseService.LogWarning("update client stats counter", err, utils.Int64ToString(clientID))
		}
	}

	s.baseService.LogCreated("client", fmt.Sprintf("%s (ID: %d) for user: %s", clientName, clientID, userID))

	// 返回完整的Client对象（无状态 = 离线）
	return models.FromConfigAndState(config, nil, nil), nil
}

// GetClient 获取客户端完整信息（聚合配置+状态+Token）
func (s *clientService) GetClient(clientID int64) (*models.Client, error) {
	// 并发读取配置、状态、Token
	var (
		config                        *models.ClientConfig
		state                         *models.ClientRuntimeState
		token                         *models.ClientToken
		configErr, stateErr, tokenErr error
		wg                            sync.WaitGroup
	)

	wg.Add(3)

	// 1. 读取配置（必需）
	go func() {
		defer wg.Done()
		config, configErr = s.configRepo.GetConfig(clientID)
	}()

	// 2. 读取状态（可选）
	go func() {
		defer wg.Done()
		state, stateErr = s.stateRepo.GetState(clientID)
		if stateErr != nil {
			utils.Debugf("Failed to get client %d state: %v", clientID, stateErr)
			stateErr = nil // 状态不存在不算错误
		}
	}()

	// 3. 读取Token（可选）
	go func() {
		defer wg.Done()
		token, tokenErr = s.tokenRepo.GetToken(clientID)
		if tokenErr != nil {
			utils.Debugf("Failed to get client %d token: %v", clientID, tokenErr)
			tokenErr = nil // Token不存在不算错误
		}
	}()

	wg.Wait()

	// 配置是必需的
	if configErr != nil {
		return nil, fmt.Errorf("failed to get client config: %w", configErr)
	}
	if config == nil {
		return nil, fmt.Errorf("client %d not found", clientID)
	}

	// 聚合返回
	client := models.FromConfigAndState(config, state, token)
	return client, nil
}

// TouchClient 更新客户端最后活动时间
func (s *clientService) TouchClient(clientID int64) {
	if err := s.stateRepo.TouchState(clientID); err != nil {
		utils.Warnf("Failed to touch client %d state: %v", clientID, err)
	}
}

// UpdateClient 更新客户端配置
//
// 注意：此方法只更新持久化配置，不更新运行时状态
// 如需更新状态，使用UpdateClientStatus或ConnectClient
func (s *clientService) UpdateClient(client *models.Client) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}

	// 构建配置对象
	config := &models.ClientConfig{
		ID:        client.ID,
		UserID:    client.UserID,
		Name:      client.Name,
		AuthCode:  client.AuthCode,
		SecretKey: client.SecretKey,
		Type:      client.Type,
		Config:    client.Config,
		CreatedAt: client.CreatedAt,
		UpdatedAt: time.Now(),
	}

	// 更新配置
	if err := s.configRepo.UpdateConfig(config); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "update client config", client.ID)
	}

	// ✅ 兼容性：同步到旧Repository
	if err := s.clientRepo.UpdateClient(client); err != nil {
		s.baseService.LogWarning("sync to legacy client repo", err)
	}

	s.baseService.LogUpdated("client", fmt.Sprintf("%d", client.ID))
	return nil
}

// DeleteClient 删除客户端
func (s *clientService) DeleteClient(clientID int64) error {
	// 获取客户端信息
	client, err := s.GetClient(clientID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get client", clientID)
	}

	// 删除配置
	if err := s.configRepo.DeleteConfig(clientID); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "delete client config", clientID)
	}

	// 删除状态
	_ = s.stateRepo.DeleteState(clientID)

	// 删除Token
	_ = s.tokenRepo.DeleteToken(clientID)

	// ✅ 兼容性：从旧Repository删除
	if err := s.clientRepo.DeleteClient(utils.Int64ToString(clientID)); err != nil {
		s.baseService.LogWarning("delete from legacy client repo", err)
	}

	// 从用户客户端列表中移除
	if client.UserID != "" && s.clientRepo != nil {
		if err := s.clientRepo.RemoveClientFromUser(client.UserID, client); err != nil {
			s.baseService.LogWarning("remove client from user list", err)
		}
	}

	// 释放客户端ID
	if err := s.idManager.ReleaseClientID(clientID); err != nil {
		s.baseService.LogWarning("release client ID", err, clientID)
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrClient(-1); err != nil {
			s.baseService.LogWarning("update client stats counter", err, utils.Int64ToString(clientID))
		}
		// 如果客户端之前在线，减少在线数
		if client.Status == models.ClientStatusOnline {
			if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, utils.Int64ToString(clientID))
			}
		}
	}

	s.baseService.LogDeleted("client", fmt.Sprintf("%d", clientID))
	return nil
}

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

	utils.Infof("Updated client %d status to %s on node %s", clientID, status, nodeID)
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

	utils.Infof("Client %d connected to node %s (conn=%s, ip=%s, proto=%s)",
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

	utils.Infof("Client %d disconnected from node %s", clientID, state.NodeID)
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

// ============================================================================
// 兼容性方法（保持接口一致）
// ============================================================================

// ListClients 列出客户端
func (s *clientService) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	var configs []*models.ClientConfig
	var err error

	if userID != "" {
		// 获取用户的客户端配置
		// TODO: 需要实现ConfigRepo的ListUserConfigs方法
		// 暂时fallback到旧逻辑
		return s.ListUserClients(userID)
	}

	// 获取所有客户端配置
	configs, err = s.configRepo.ListConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to list client configs: %w", err)
	}

	// 并发聚合每个客户端的完整信息
	clients := make([]*models.Client, 0, len(configs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cfg := range configs {
		// 类型过滤
		if clientType != "" && cfg.Type != clientType {
			continue
		}

		wg.Add(1)
		go func(config *models.ClientConfig) {
			defer wg.Done()

			// 获取状态和Token
			var state *models.ClientRuntimeState
			var token *models.ClientToken

			state, _ = s.stateRepo.GetState(config.ID)
			token, _ = s.tokenRepo.GetToken(config.ID)

			// 聚合
			client := models.FromConfigAndState(config, state, token)

			mu.Lock()
			clients = append(clients, client)
			mu.Unlock()
		}(cfg)
	}

	wg.Wait()
	return clients, nil
}

// ListUserClients 列出用户的所有客户端
func (s *clientService) ListUserClients(userID string) ([]*models.Client, error) {
	// TODO: 实现基于ConfigRepo的查询
	// 暂时使用旧Repository
	clients, err := s.clientRepo.ListUserClients(userID)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

// GetClientPortMappings 获取客户端的端口映射
func (s *clientService) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	mappings, err := s.mappingRepo.GetClientPortMappings(utils.Int64ToString(clientID))
	if err != nil {
		return nil, fmt.Errorf("failed to get client port mappings for %d: %w", clientID, err)
	}
	return mappings, nil
}

// SearchClients 搜索客户端
func (s *clientService) SearchClients(keyword string) ([]*models.Client, error) {
	// 暂时返回空列表
	utils.Warnf("SearchClients not implemented yet")
	return []*models.Client{}, nil
}

// GetClientStats 获取客户端统计信息
func (s *clientService) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	clientStats, err := s.statsMgr.GetClientStats(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client stats for %d: %w", clientID, err)
	}
	return clientStats, nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// getDefaultClientConfig 获取默认客户端配置
func (s *clientService) getDefaultClientConfig() configs.ClientConfig {
	return configs.ClientConfig{
		EnableCompression: constants.DefaultEnableCompression,
		BandwidthLimit:    constants.DefaultClientBandwidthLimit,
		MaxConnections:    constants.DefaultClientMaxConnections,
		AllowedPorts:      constants.DefaultAllowedPorts,
		BlockedPorts:      constants.DefaultBlockedPorts,
		AutoReconnect:     constants.DefaultAutoReconnect,
		HeartbeatInterval: constants.DefaultHeartbeatInterval,
	}
}
