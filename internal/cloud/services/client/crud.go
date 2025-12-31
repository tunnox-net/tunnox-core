package client

import (
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils/random"
)

// ============================================================================
// 客户端CRUD操作
// ============================================================================

// CreateClient 创建客户端
func (s *Service) CreateClient(userID, clientName string) (*models.Client, error) {
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
			s.baseService.LogWarning("update client stats counter", err, random.Int64ToString(clientID))
		}
	}

	s.baseService.LogCreated("client", fmt.Sprintf("%s (ID: %d) for user: %s", clientName, clientID, userID))

	// 返回完整的Client对象（无状态 = 离线）
	return models.FromConfigAndState(config, nil, nil), nil
}

// GetClient 获取客户端完整信息（聚合配置+状态+Token）
func (s *Service) GetClient(clientID int64) (*models.Client, error) {
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
			corelog.Debugf("Failed to get client %d state: %v", clientID, stateErr)
			stateErr = nil // 状态不存在不算错误
		}
	}()

	// 3. 读取Token（可选）
	go func() {
		defer wg.Done()
		token, tokenErr = s.tokenRepo.GetToken(clientID)
		if tokenErr != nil {
			corelog.Debugf("Failed to get client %d token: %v", clientID, tokenErr)
			tokenErr = nil // Token不存在不算错误
		}
	}()

	wg.Wait()

	// 配置是必需的
	if configErr != nil {
		return nil, coreerrors.Wrap(configErr, coreerrors.CodeStorageError, "failed to get client config")
	}
	if config == nil {
		return nil, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	// 聚合返回
	client := models.FromConfigAndState(config, state, token)
	return client, nil
}

// TouchClient 更新客户端最后活动时间
func (s *Service) TouchClient(clientID int64) {
	if err := s.stateRepo.TouchState(clientID); err != nil {
		corelog.Warnf("Failed to touch client %d state: %v", clientID, err)
	}
}

// UpdateClient 更新客户端配置
//
// 注意：此方法只更新持久化配置，不更新运行时状态
// 如需更新状态，使用UpdateClientStatus或ConnectClient
func (s *Service) UpdateClient(client *models.Client) error {
	if client == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "client is nil")
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
func (s *Service) DeleteClient(clientID int64) error {
	// 获取客户端信息
	client, err := s.GetClient(clientID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get client", clientID)
	}

	// 删除配置
	if err := s.configRepo.DeleteConfig(clientID); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "delete client config", clientID)
	}

	// 删除状态（状态可能不存在，例如客户端从未上线过）
	if err := s.stateRepo.DeleteState(clientID); err != nil {
		// 状态删除失败不影响主流程，记录警告日志
		s.baseService.LogWarning("delete client state", err)
	}

	// 删除Token（Token可能不存在，例如客户端从未认证过）
	if err := s.tokenRepo.DeleteToken(clientID); err != nil {
		// Token删除失败不影响主流程，记录警告日志
		s.baseService.LogWarning("delete client token", err)
	}

	// ✅ 兼容性：从旧Repository删除
	if err := s.clientRepo.DeleteClient(random.Int64ToString(clientID)); err != nil {
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
			s.baseService.LogWarning("update client stats counter", err, random.Int64ToString(clientID))
		}
		// 如果客户端之前在线，减少在线数
		if client.Status == models.ClientStatusOnline {
			if err := s.statsCounter.IncrOnlineClients(-1); err != nil {
				s.baseService.LogWarning("update online clients counter", err, random.Int64ToString(clientID))
			}
		}
	}

	s.baseService.LogDeleted("client", fmt.Sprintf("%d", clientID))
	return nil
}

// ============================================================================
// 辅助方法
// ============================================================================

// getDefaultClientConfig 获取默认客户端配置
func (s *Service) getDefaultClientConfig() configs.ClientConfig {
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
