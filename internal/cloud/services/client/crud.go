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

// RegisteredClientExpirationDays 注册客户端不设置过期时间（与匿名客户端不同）
// 注册客户端绑定到用户，不会自动过期
const RegisteredClientExpirationDays = 0

// ============================================================================
// 客户端CRUD操作
// ============================================================================

// CreateClient 创建客户端
//
// 返回的 Client 对象中，SecretKeyPlaintext 字段仅在首次创建时填充（用于一次性展示给用户）
// 数据库中只存储加密后的 SecretKeyEncrypted
func (s *Service) CreateClient(userID, clientName string) (*models.Client, error) {
	// 生成客户端ID
	clientID, err := s.idManager.GenerateClientID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate client ID")
	}

	// 生成认证码
	authCode, err := s.idManager.GenerateAuthCode()
	if err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate auth code")
	}

	// 生成 SecretKey（加密存储）
	var secretKeyPlaintext, secretKeyEncrypted string
	if s.secretKeyMgr != nil {
		// 新模式：使用 SecretKeyManager 生成加密凭据
		plaintext, encrypted, err := s.secretKeyMgr.GenerateCredentials()
		if err != nil {
			return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate encrypted credentials")
		}
		secretKeyPlaintext = plaintext
		secretKeyEncrypted = encrypted
	} else {
		// 兼容模式：使用旧的明文存储（仅用于迁移期间）
		corelog.Warnf("SecretKeyManager not set, using legacy plaintext storage for client %d", clientID)
		secretKey, err := s.idManager.GenerateSecretKey()
		if err != nil {
			return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate secret key")
		}
		secretKeyPlaintext = secretKey
		// 兼容模式下，SecretKey 存在旧字段中
	}

	// 创建客户端配置
	now := time.Now()
	config := &models.ClientConfig{
		ID:                 clientID,
		UserID:             userID,
		Name:               clientName,
		AuthCode:           authCode,
		SecretKeyEncrypted: secretKeyEncrypted, // 加密后的 SecretKey
		SecretKeyVersion:   1,                  // 初始版本
		Type:               models.ClientTypeRegistered,
		Config:             s.getDefaultClientConfig(),
		CreatedAt:          now,
		UpdatedAt:          now,
		// 注册客户端不设置过期时间（绑定用户后永不过期）
	}

	// 兼容模式：如果没有 SecretKeyManager，存到旧字段
	if s.secretKeyMgr == nil {
		config.SecretKey = secretKeyPlaintext
	}

	// 保存配置到持久化存储
	if err := s.configRepo.SaveConfig(config); err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "save client config")
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

	client := models.FromConfigAndState(config, nil, nil)
	client.SecretKeyPlaintext = secretKeyPlaintext
	client.SecretKey = secretKeyPlaintext
	client.SecretKeyVersion = config.SecretKeyVersion

	return client, nil
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
// SecretKey 相关字段（SecretKeyEncrypted, SecretKeyVersion）不允许通过此方法修改，
// 需要使用 ResetSecretKey 方法
func (s *Service) UpdateClient(client *models.Client) error {
	if client == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "client is nil")
	}

	// 获取旧的客户端配置，用于检测 UserID 变化和保留 SecretKey 相关字段
	oldConfig, err := s.configRepo.GetConfig(client.ID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get old client config", client.ID)
	}

	// 获取旧的客户端信息（包含状态），用于检测 UserID 变化
	oldClient, err := s.GetClient(client.ID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get old client", client.ID)
	}
	oldUserID := oldClient.UserID
	newUserID := client.UserID

	// 构建配置对象
	// 注意：SecretKey 相关字段从原配置保留，不允许通过 UpdateClient 修改
	config := &models.ClientConfig{
		ID:                 client.ID,
		UserID:             client.UserID,
		Name:               client.Name,
		AuthCode:           client.AuthCode,
		SecretKey:          oldConfig.SecretKey,          // 保留旧值（兼容模式）
		SecretKeyEncrypted: oldConfig.SecretKeyEncrypted, // 保留加密值
		SecretKeyVersion:   oldConfig.SecretKeyVersion,   // 保留版本号
		ExpiresAt:          oldConfig.ExpiresAt,          // 保留过期时间
		Type:               client.Type,
		Config:             client.Config,
		CreatedAt:          client.CreatedAt,
		UpdatedAt:          time.Now(),
	}

	// 更新配置
	// 注意：不再维护全局列表，ListConfigs 使用 QueryByPrefix 直接查询数据库
	if err := s.configRepo.UpdateConfig(config); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "update client config", client.ID)
	}

	if oldUserID == "" && newUserID != "" {
		corelog.Infof("Client %d bound to user %s", client.ID, newUserID)
	}

	// ✅ 兼容性：同步到旧Repository
	if err := s.clientRepo.UpdateClient(client); err != nil {
		s.baseService.LogWarning("sync to legacy client repo", err)
	}

	// ✅ 同步用户客户端列表（Redis 兼容层）
	// 注意：RemoveFromList 使用完整 JSON 匹配，必须用旧数据删除
	if s.clientRepo != nil {
		// 从旧用户列表移除（使用旧的 client 对象，JSON 才能匹配）
		if oldUserID != "" {
			if err := s.clientRepo.RemoveClientFromUser(oldUserID, oldClient); err != nil {
				s.baseService.LogWarning("remove client from old user list", err)
			}
		}
		// 添加到新用户列表（使用更新后的 client 对象）
		if newUserID != "" {
			if err := s.clientRepo.AddClientToUser(newUserID, client); err != nil {
				s.baseService.LogWarning("add client to new user list", err)
			}
		}
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
