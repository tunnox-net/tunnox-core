package conncode

import (
	"errors"
	"time"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	cloudutils "tunnox-core/internal/cloud/utils"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils/random"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码激活和撤销
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ActivateRequest 激活连接码请求
type ActivateRequest struct {
	Code           string // 连接码
	ListenClientID int64  // 激活者（ListenClient）
	ListenAddress  string // 监听地址（如 0.0.0.0:9999）
}

// ActivateConnectionCode 激活连接码，创建端口映射
//
// 业务逻辑：
//  1. 验证请求参数
//  2. 获取连接码
//  3. 验证连接码有效性（未使用、未过期、未撤销）
//  4. 检查映射配额
//  5. 解析地址并创建PortMapping（不包含ConnectionCodeID）
//  6. 连接码记录MappingID（反向关系）
//  7. 更新连接码状态为已激活
func (s *Service) ActivateConnectionCode(req *ActivateRequest) (*models.PortMapping, error) {
	// 1. 参数验证
	if req.Code == "" {
		return nil, coreerrors.New(coreerrors.CodeMissingParam, "connection code is required")
	}
	if req.ListenClientID == 0 {
		return nil, coreerrors.New(coreerrors.CodeMissingParam, "listen client ID is required")
	}
	if req.ListenAddress == "" {
		return nil, coreerrors.New(coreerrors.CodeMissingParam, "listen address is required")
	}

	// 2. 获取连接码
	connCode, err := s.connCodeRepo.GetByCode(req.Code)
	if err != nil {
		if errors.Is(err, repos.ErrNotFound) {
			return nil, coreerrors.New(coreerrors.CodeNotFound, "connection code not found or expired")
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get connection code")
	}

	// 3. 验证连接码有效性
	if !connCode.CanBeActivatedBy(req.ListenClientID) {
		if connCode.IsRevoked {
			return nil, coreerrors.New(coreerrors.CodeForbidden, "connection code has been revoked")
		}
		if connCode.IsActivated {
			return nil, coreerrors.New(coreerrors.CodeConflict, "connection code has already been used")
		}
		if connCode.IsExpired() {
			return nil, coreerrors.New(coreerrors.CodeExpired, "connection code has expired")
		}
		return nil, coreerrors.New(coreerrors.CodeForbidden, "connection code cannot be activated")
	}

	// 4. 解析地址
	_, listenPort, err := cloudutils.ParseListenAddress(req.ListenAddress)
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid listen address %q", req.ListenAddress)
	}

	targetHost, targetPort, protocol, err := cloudutils.ParseTargetAddress(connCode.TargetAddress)
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid target address %q", connCode.TargetAddress)
	}

	// 5. 检查映射配额
	clientKey := random.Int64ToString(req.ListenClientID)
	mappings, err := s.portMappingRepo.GetClientPortMappings(clientKey)
	if err != nil {
		corelog.Warnf("ConnectionCodeService: failed to get client mappings for quota check: %v", err)
		// 不因为查询失败而阻止激活，只记录警告
	} else {
		// 统计活跃映射数量
		activeMappings := 0
		for _, m := range mappings {
			if m.Status == models.MappingStatusActive && !m.IsRevoked && !m.IsExpired() {
				activeMappings++
			}
		}

		// 检查是否超过配额
		if activeMappings >= s.maxActiveMappingsPerClient {
			return nil, coreerrors.Newf(coreerrors.CodeQuotaExceeded, "quota exceeded: max %d active mappings allowed, current: %d",
				s.maxActiveMappingsPerClient, activeMappings)
		}

		corelog.Debugf("ConnectionCodeService: quota check passed for client %d: %d/%d active mappings",
			req.ListenClientID, activeMappings, s.maxActiveMappingsPerClient)
	}

	// 6. 创建PortMapping
	now := time.Now()
	expiresAt := now.Add(connCode.MappingDuration)

	mapping := &models.PortMapping{
		// 基础信息
		UserID: "", // 连接码创建的映射是匿名的

		// 映射双方
		ListenClientID: req.ListenClientID,
		TargetClientID: connCode.TargetClientID,

		// 地址信息
		Protocol:      models.Protocol(protocol),
		SourcePort:    listenPort,
		TargetHost:    targetHost,
		TargetPort:    targetPort,
		ListenAddress: req.ListenAddress,
		TargetAddress: connCode.TargetAddress,

		// 认证和配置
		SecretKey: "", // 由 PortMappingService 生成
		Config: configs.MappingConfig{
			EnableCompression: true,
			BandwidthLimit:    0, // 无限制
			MaxConnections:    100,
		},
		Status: models.MappingStatusActive,

		// 时限控制
		ExpiresAt: &expiresAt,
		IsRevoked: false,

		// 时间戳
		CreatedAt: now,
		UpdatedAt: now,

		// 元数据
		Type:        models.MappingTypeAnonymous, // 通过 Type 标识是通过连接码创建的
		Description: connCode.Description,
	}

	// 7. 通过 PortMappingService 创建映射（会自动生成ID和SecretKey，并更新索引）
	createdMapping, err := s.portMappingService.CreatePortMapping(mapping)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to create port mapping")
	}

	// 8. 连接码记录 MappingID（反向关系）
	if err := connCode.Activate(req.ListenClientID, createdMapping.ID); err != nil {
		// 回滚：删除已创建的映射（忽略删除错误，主流程已失败）
		_ = s.portMappingService.DeletePortMapping(createdMapping.ID)
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to activate connection code")
	}

	if err := s.connCodeRepo.Update(connCode); err != nil {
		// 回滚：删除已创建的映射（忽略删除错误，主流程已失败）
		_ = s.portMappingService.DeletePortMapping(createdMapping.ID)
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update connection code")
	}

	corelog.Infof("ConnectionCodeService: activated code %s, created mapping %s (%d -> %d)",
		req.Code, createdMapping.ID, req.ListenClientID, connCode.TargetClientID)

	return createdMapping, nil
}

// RevokeConnectionCode 撤销连接码
//
// 只能撤销未使用的连接码
func (s *Service) RevokeConnectionCode(code string, revokedBy string) error {
	// 1. 获取连接码
	connCode, err := s.connCodeRepo.GetByCode(code)
	if err != nil {
		if errors.Is(err, repos.ErrNotFound) {
			return coreerrors.New(coreerrors.CodeNotFound, "connection code not found or expired")
		}
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get connection code")
	}

	// 2. 撤销
	if err := connCode.Revoke(revokedBy); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to revoke connection code")
	}

	// 3. 更新
	if err := s.connCodeRepo.Update(connCode); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update connection code")
	}

	corelog.Infof("ConnectionCodeService: revoked code %s by %s", code, revokedBy)

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道映射管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ValidateMapping 验证端口映射权限
//
// 用于HandleTunnelOpen时验证ListenClient是否有权限使用此映射
func (s *Service) ValidateMapping(mappingID string, clientID int64) (*models.PortMapping, error) {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found or expired")
	}

	// 添加详细日志
	corelog.Debugf("ConnectionCodeService.ValidateMapping: mappingID=%s, clientID=%d, ListenClientID=%d, TargetClientID=%d, Status=%s, IsRevoked=%v, IsExpired=%v, IsValid=%v",
		mappingID, clientID, mapping.ListenClientID, mapping.TargetClientID, mapping.Status, mapping.IsRevoked, mapping.IsExpired(), mapping.IsValid())

	// 验证权限
	if !mapping.CanBeAccessedBy(clientID) {
		corelog.Warnf("ConnectionCodeService.ValidateMapping: CanBeAccessedBy returned false for mappingID=%s, clientID=%d", mappingID, clientID)
		if mapping.IsRevoked {
			return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has been revoked")
		}
		if mapping.IsExpired() {
			return nil, coreerrors.New(coreerrors.CodeExpired, "mapping has expired")
		}
		if mapping.ListenClientID != clientID {
			corelog.Warnf("ConnectionCodeService.ValidateMapping: clientID mismatch - expected ListenClientID=%d, got clientID=%d", mapping.ListenClientID, clientID)
			return nil, coreerrors.Newf(coreerrors.CodeForbidden, "client %d is not authorized to use this mapping", clientID)
		}
		// 如果到这里，说明 IsValid() 返回了 false，但具体原因未知
		corelog.Errorf("ConnectionCodeService.ValidateMapping: mapping cannot be accessed - Status=%s, IsRevoked=%v, IsExpired=%v",
			mapping.Status, mapping.IsRevoked, mapping.IsExpired())
		return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping cannot be accessed")
	}

	corelog.Debugf("ConnectionCodeService.ValidateMapping: validation passed for mappingID=%s, clientID=%d", mappingID, clientID)
	return mapping, nil
}

// RevokeMapping 撤销映射
//
// TargetClient 或 ListenClient 都可以撤销
func (s *Service) RevokeMapping(mappingID string, clientID int64, revokedBy string) error {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found or expired")
	}

	// 撤销
	if err := mapping.Revoke(revokedBy, clientID); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to revoke mapping")
	}

	// 更新
	if err := s.portMappingService.UpdatePortMapping(mapping); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update mapping")
	}

	corelog.Infof("ConnectionCodeService: revoked mapping %s by %s (client %d)",
		mappingID, revokedBy, clientID)
	return nil
}

// RecordMappingUsage 记录映射使用
//
// 在每次建立隧道连接时调用
func (s *Service) RecordMappingUsage(mappingID string) error {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found")
	}

	// 更新最后活跃时间
	now := time.Now()
	mapping.LastActive = &now
	if err := s.portMappingService.UpdatePortMapping(mapping); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update mapping usage")
	}

	return nil
}

// RecordMappingTraffic 记录映射流量
//
// 在隧道连接关闭时调用
func (s *Service) RecordMappingTraffic(mappingID string, bytesSent, bytesReceived int64) error {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeMappingNotFound, "mapping not found")
	}

	// 更新流量统计
	mapping.TrafficStats.BytesSent += bytesSent
	mapping.TrafficStats.BytesReceived += bytesReceived
	mapping.TrafficStats.LastUpdated = time.Now()

	if err := s.portMappingService.UpdatePortMappingStats(mappingID, &mapping.TrafficStats); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update mapping traffic")
	}

	return nil
}
