package client

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// ============================================================================
// 凭据管理操作
// ============================================================================

// Kicker 内联接口类型定义，用于凭据重置后踢掉当前连接
// 使用内联接口避免循环依赖
type Kicker = interface {
	KickClient(int64, string, string) error
}

// ResetSecretKey 重置客户端 SecretKey
//
// 功能：
// - 生成新的加密凭据
// - 版本号 +1
// - 清除旧 Token
// - 踢掉当前连接（如果有 kicker）
//
// 返回：
// - newSecretKey: 新的明文 SecretKey（仅此一次返回，用于展示给用户）
// - error: 错误信息
//
// 注意：
// - 此操作不可逆，旧的 SecretKey 将无法再使用
// - 客户端需要使用新的 SecretKey 重新连接
func (s *Service) ResetSecretKey(clientID int64, kicker interface{ KickClient(int64, string, string) error }) (string, error) {
	// 检查 SecretKeyManager 是否已设置
	if s.secretKeyMgr == nil {
		return "", coreerrors.New(coreerrors.CodeInternal, "SecretKeyManager not configured")
	}

	// 获取当前配置
	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return "", s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return "", coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	// 生成新的加密凭据
	plaintext, encrypted, err := s.secretKeyMgr.GenerateCredentials()
	if err != nil {
		return "", s.baseService.WrapErrorWithInt64ID(err, "generate new credentials", clientID)
	}

	// 更新配置
	config.SecretKeyEncrypted = encrypted
	config.SecretKeyVersion++
	config.SecretKey = "" // 清除旧的明文字段（如果有）
	config.UpdatedAt = time.Now()

	// 保存配置
	if err := s.configRepo.UpdateConfig(config); err != nil {
		return "", s.baseService.WrapErrorWithInt64ID(err, "update client config", clientID)
	}

	// 清除旧 Token（强制重新认证）
	if err := s.tokenRepo.DeleteToken(clientID); err != nil {
		// Token 删除失败不影响主流程，记录警告日志
		s.baseService.LogWarning("delete old token", err)
	}

	// 踢掉当前连接（如果有 kicker）
	if kicker != nil {
		if err := kicker.KickClient(clientID, "credentials_reset", "Credentials have been reset, please reconnect"); err != nil {
			// 踢出失败不影响主流程，记录警告日志
			s.baseService.LogWarning("kick client after credentials reset", err)
		}
	}

	corelog.Infof("Client %d SecretKey reset, version: %d -> %d", clientID, config.SecretKeyVersion-1, config.SecretKeyVersion)

	return plaintext, nil
}

// ExtendExpiration 延长客户端过期时间
//
// 用途：
// - 匿名客户端绑定用户后，清除过期时间
// - 手动延长客户端有效期
//
// 参数：
// - clientID: 客户端ID
// - days: 延长天数，0 表示清除过期时间（永不过期）
func (s *Service) ExtendExpiration(clientID int64, days int) error {
	// 获取当前配置
	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	// 更新过期时间
	if days == 0 {
		config.ExpiresAt = nil // 清除过期时间
		corelog.Infof("Client %d expiration cleared (never expires)", clientID)
	} else {
		expiresAt := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		config.ExpiresAt = &expiresAt
		corelog.Infof("Client %d expiration extended to %s", clientID, expiresAt.Format(time.RFC3339))
	}
	config.UpdatedAt = time.Now()

	// 保存配置
	if err := s.configRepo.UpdateConfig(config); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "update client config", clientID)
	}

	return nil
}

// BindToUser 将客户端绑定到用户
//
// 功能：
// - 设置 UserID
// - 清除过期时间（绑定用户后永不过期）
// - 更新客户端类型为 Registered（如果原来是 Anonymous）
//
// 参数：
// - clientID: 客户端ID
// - userID: 用户ID
func (s *Service) BindToUser(clientID int64, userID string) error {
	if userID == "" {
		return coreerrors.New(coreerrors.CodeInvalidParam, "userID is required")
	}

	// 获取当前配置
	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	// 检查是否已绑定
	if config.UserID != "" && config.UserID != userID {
		return coreerrors.Newf(coreerrors.CodeConflict, "client %d already bound to user %s", clientID, config.UserID)
	}

	oldType := config.Type
	oldUserID := config.UserID

	// 更新配置
	config.UserID = userID
	config.ExpiresAt = nil // 绑定用户后永不过期
	if config.Type == models.ClientTypeAnonymous {
		config.Type = models.ClientTypeRegistered // 升级为注册客户端
	}
	config.UpdatedAt = time.Now()

	// 保存配置
	if err := s.configRepo.UpdateConfig(config); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "update client config", clientID)
	}

	// 同步用户客户端列表
	if s.clientRepo != nil {
		// 获取完整的 Client 对象用于同步
		client := models.FromConfigAndState(config, nil, nil)

		// 从旧用户列表移除（如果有）
		if oldUserID != "" {
			oldClient := &models.Client{ID: clientID, UserID: oldUserID, Type: oldType}
			if err := s.clientRepo.RemoveClientFromUser(oldUserID, oldClient); err != nil {
				s.baseService.LogWarning("remove client from old user list", err)
			}
		}

		// 添加到新用户列表
		if err := s.clientRepo.AddClientToUser(userID, client); err != nil {
			s.baseService.LogWarning("add client to user list", err)
		}
	}

	corelog.Infof("Client %d bound to user %s (type: %s -> %s)", clientID, userID, oldType, config.Type)
	return nil
}

// IsExpired 检查客户端是否已过期
func (s *Service) IsExpired(clientID int64) (bool, error) {
	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return false, s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return false, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	if config.ExpiresAt == nil {
		return false, nil // 未设置过期时间，永不过期
	}

	return time.Now().After(*config.ExpiresAt), nil
}

// GetCredentialInfo 获取凭据信息（不含明文）
//
// 返回：
// - version: 当前版本号
// - expiresAt: 过期时间（nil 表示永不过期）
// - isExpired: 是否已过期
func (s *Service) GetCredentialInfo(clientID int64) (version int, expiresAt *time.Time, isExpired bool, err error) {
	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return 0, nil, false, s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return 0, nil, false, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	version = config.SecretKeyVersion
	expiresAt = config.ExpiresAt

	if expiresAt != nil && time.Now().After(*expiresAt) {
		isExpired = true
	}

	return version, expiresAt, isExpired, nil
}

// MigrateToEncrypted 迁移明文凭据到加密存储
//
// 用于数据迁移：将旧的明文 SecretKey 加密存储
// 注意：迁移后旧的明文字段会被清空
func (s *Service) MigrateToEncrypted(clientID int64) error {
	if s.secretKeyMgr == nil {
		return coreerrors.New(coreerrors.CodeInternal, "SecretKeyManager not configured")
	}

	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	// 检查是否需要迁移
	if config.SecretKeyEncrypted != "" {
		// 已经是加密存储，无需迁移
		return nil
	}

	if config.SecretKey == "" {
		return coreerrors.Newf(coreerrors.CodeInvalidParam, "client %d has no SecretKey to migrate", clientID)
	}

	// 加密现有的明文 SecretKey
	encrypted, err := s.secretKeyMgr.Encrypt(config.SecretKey)
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "encrypt secret key", clientID)
	}

	// 更新配置
	config.SecretKeyEncrypted = encrypted
	if config.SecretKeyVersion == 0 {
		config.SecretKeyVersion = 1
	}
	config.SecretKey = "" // 清除明文
	config.UpdatedAt = time.Now()

	// 保存配置
	if err := s.configRepo.UpdateConfig(config); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "update client config", clientID)
	}

	corelog.Infof("Client %d SecretKey migrated to encrypted storage", clientID)
	return nil
}

// BatchMigrateToEncrypted 批量迁移明文凭据到加密存储
//
// 返回：
// - migrated: 成功迁移的数量
// - skipped: 跳过的数量（已经是加密存储）
// - errors: 迁移失败的 clientID 和错误信息
func (s *Service) BatchMigrateToEncrypted() (migrated, skipped int, errors map[int64]error) {
	errors = make(map[int64]error)

	// 获取所有客户端配置
	configs, err := s.configRepo.ListConfigs()
	if err != nil {
		errors[0] = fmt.Errorf("failed to list configs: %w", err)
		return 0, 0, errors
	}

	for _, config := range configs {
		if config.SecretKeyEncrypted != "" {
			// 已经是加密存储，跳过
			skipped++
			continue
		}

		if config.SecretKey == "" {
			// 没有 SecretKey，跳过
			skipped++
			continue
		}

		// 执行迁移
		if err := s.MigrateToEncrypted(config.ID); err != nil {
			errors[config.ID] = err
		} else {
			migrated++
		}
	}

	corelog.Infof("Batch migration completed: migrated=%d, skipped=%d, errors=%d", migrated, skipped, len(errors))
	return migrated, skipped, errors
}

// VerifySecretKey 验证客户端 SecretKey
//
// 支持两种存储模式：
// - 加密存储（SecretKeyEncrypted）：解密后比较
// - 明文存储（SecretKey，兼容旧数据）：直接比较
func (s *Service) VerifySecretKey(clientID int64, secretKey string) (bool, error) {
	// 获取配置
	config, err := s.configRepo.GetConfig(clientID)
	if err != nil {
		return false, s.baseService.WrapErrorWithInt64ID(err, "get client config", clientID)
	}
	if config == nil {
		return false, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not found", clientID)
	}

	// 优先验证加密存储
	if config.SecretKeyEncrypted != "" {
		if s.secretKeyMgr == nil {
			return false, coreerrors.New(coreerrors.CodeInternal, "SecretKeyManager not configured")
		}
		// 解密后比较
		decrypted, err := s.secretKeyMgr.Decrypt(config.SecretKeyEncrypted)
		if err != nil {
			corelog.Warnf("Client %d SecretKey decrypt failed: %v", clientID, err)
			return false, nil // 解密失败返回验证失败，不返回错误
		}
		return decrypted == secretKey, nil
	}

	// 兼容旧数据：明文存储
	if config.SecretKey != "" {
		return config.SecretKey == secretKey, nil
	}

	// 没有任何 SecretKey
	return false, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d has no SecretKey", clientID)
}
