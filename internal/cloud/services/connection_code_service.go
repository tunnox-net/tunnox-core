package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	cloudutils "tunnox-core/internal/cloud/utils"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// ConnectionCodeService 连接码服务
//
// 职责：
//   - 管理连接码的完整生命周期（创建、激活、撤销）
//   - 管理端口映射的创建和管理（统一使用 PortMapping）
//   - 提供权限验证
//   - 配额检查和使用统计
//
// 业务流程：
//  1. TargetClient创建连接码（CreateConnectionCode）
//  2. ListenClient激活连接码（ActivateConnectionCode）→ 创建PortMapping
//  3. ListenClient使用PortMapping建立隧道连接
//  4. 验证隧道连接权限（ValidateMapping）
//  5. 撤销连接码或映射（RevokeConnectionCode/RevokeMapping）
type ConnectionCodeService struct {
	*dispose.ServiceBase

	// Repositories
	connCodeRepo *repos.ConnectionCodeRepository

	// Services
	portMappingService PortMappingService     // ✅ 统一使用 PortMappingService
	portMappingRepo    *repos.PortMappingRepo // 用于查询客户端映射

	// 连接码生成器
	generator *ConnectionCodeGenerator

	// 配额限制
	maxActiveCodesPerClient    int // 每个客户端最多活跃连接码数
	maxActiveMappingsPerClient int // 每个客户端最多活跃映射数
}

// ConnectionCodeServiceConfig 连接码服务配置
type ConnectionCodeServiceConfig struct {
	MaxActiveCodesPerClient    int // 默认: 10
	MaxActiveMappingsPerClient int // 默认: 50
}

// DefaultConnectionCodeServiceConfig 默认配置
func DefaultConnectionCodeServiceConfig() *ConnectionCodeServiceConfig {
	return &ConnectionCodeServiceConfig{
		MaxActiveCodesPerClient:    10,
		MaxActiveMappingsPerClient: 50,
	}
}

// NewConnectionCodeService 创建连接码服务
func NewConnectionCodeService(
	connCodeRepo *repos.ConnectionCodeRepository,
	portMappingService PortMappingService,
	portMappingRepo *repos.PortMappingRepo,
	config *ConnectionCodeServiceConfig,
	ctx context.Context,
) *ConnectionCodeService {
	if config == nil {
		config = DefaultConnectionCodeServiceConfig()
	}

	service := &ConnectionCodeService{
		ServiceBase:                dispose.NewService("ConnectionCodeService", ctx),
		connCodeRepo:               connCodeRepo,
		portMappingService:         portMappingService,
		portMappingRepo:            portMappingRepo,
		generator:                  NewConnectionCodeGenerator(models.DefaultConnectionCodeGenerator()),
		maxActiveCodesPerClient:    config.MaxActiveCodesPerClient,
		maxActiveMappingsPerClient: config.MaxActiveMappingsPerClient,
	}

	// 启动后台任务：定期清理过期的连接码和映射
	go service.cleanupExpiredEntities(ctx)

	return service
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateConnectionCodeRequest 创建连接码请求
type CreateConnectionCodeRequest struct {
	TargetClientID  int64         // 生成连接码的客户端
	TargetAddress   string        // 目标地址（必填）
	ActivationTTL   time.Duration // 激活有效期（默认10分钟）
	MappingDuration time.Duration // 映射有效期（默认7天）
	Description     string        // 描述（可选）
	CreatedBy       string        // 创建者
}

// CreateConnectionCode 创建连接码
//
// 业务逻辑：
//  1. 验证请求参数
//  2. 检查配额（防止滥用）
//  3. 生成唯一的连接码
//  4. 保存到Repository
func (s *ConnectionCodeService) CreateConnectionCode(req *CreateConnectionCodeRequest) (*models.TunnelConnectionCode, error) {
	// 1. 参数验证
	if req.TargetClientID == 0 {
		return nil, fmt.Errorf("target client ID is required")
	}
	if req.TargetAddress == "" {
		return nil, fmt.Errorf("target address is required")
	}

	// 设置默认值
	if req.ActivationTTL <= 0 {
		req.ActivationTTL = 10 * time.Minute
	}
	if req.MappingDuration <= 0 {
		req.MappingDuration = 7 * 24 * time.Hour
	}

	// 2. 配额检查
	activeCount, err := s.connCodeRepo.CountActiveByTargetClient(req.TargetClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to count active codes: %w", err)
	}
	if activeCount >= s.maxActiveCodesPerClient {
		return nil, fmt.Errorf("quota exceeded: max %d active connection codes allowed",
			s.maxActiveCodesPerClient)
	}

	// 3. 生成唯一的连接码
	code, err := s.generator.GenerateUnique(func(c string) (bool, error) {
		_, err := s.connCodeRepo.GetByCode(c)
		if errors.Is(err, repos.ErrNotFound) {
			return false, nil // 不存在，可用
		}
		if err != nil {
			return false, err // 其他错误
		}
		return true, nil // 已存在
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate unique code: %w", err)
	}

	// 4. 创建连接码对象
	now := time.Now()
	id, err := s.generateID("conncode")
	if err != nil {
		return nil, fmt.Errorf("failed to generate ID: %w", err)
	}

	connCode := &models.TunnelConnectionCode{
		ID:                  id,
		Code:                code,
		TargetClientID:      req.TargetClientID,
		TargetAddress:       req.TargetAddress,
		ActivationTTL:       req.ActivationTTL,
		MappingDuration:     req.MappingDuration,
		CreatedAt:           now,
		ActivationExpiresAt: now.Add(req.ActivationTTL),
		IsActivated:         false,
		CreatedBy:           req.CreatedBy,
		IsRevoked:           false,
		Description:         req.Description,
	}

	// 5. 保存到Repository
	if err := s.connCodeRepo.Create(connCode); err != nil {
		return nil, fmt.Errorf("failed to create connection code: %w", err)
	}

	utils.Infof("ConnectionCodeService: created code %s for target client %d (expires in %v)",
		code, req.TargetClientID, req.ActivationTTL)

	return connCode, nil
}

// ActivateConnectionCodeRequest 激活连接码请求
type ActivateConnectionCodeRequest struct {
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
func (s *ConnectionCodeService) ActivateConnectionCode(req *ActivateConnectionCodeRequest) (*models.PortMapping, error) {
	// 1. 参数验证
	if req.Code == "" {
		return nil, fmt.Errorf("connection code is required")
	}
	if req.ListenClientID == 0 {
		return nil, fmt.Errorf("listen client ID is required")
	}
	if req.ListenAddress == "" {
		return nil, fmt.Errorf("listen address is required")
	}

	// 2. 获取连接码
	connCode, err := s.connCodeRepo.GetByCode(req.Code)
	if err != nil {
		if errors.Is(err, repos.ErrNotFound) {
			return nil, fmt.Errorf("connection code not found or expired")
		}
		return nil, fmt.Errorf("failed to get connection code: %w", err)
	}

	// 3. 验证连接码有效性
	if !connCode.CanBeActivatedBy(req.ListenClientID) {
		if connCode.IsRevoked {
			return nil, fmt.Errorf("connection code has been revoked")
		}
		if connCode.IsActivated {
			return nil, fmt.Errorf("connection code has already been used")
		}
		if connCode.IsExpired() {
			return nil, fmt.Errorf("connection code has expired")
		}
		return nil, fmt.Errorf("connection code cannot be activated")
	}

	// 4. 解析地址
	_, listenPort, err := cloudutils.ParseListenAddress(req.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address %q: %w", req.ListenAddress, err)
	}

	targetHost, targetPort, protocol, err := cloudutils.ParseTargetAddress(connCode.TargetAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid target address %q: %w", connCode.TargetAddress, err)
	}

	// 5. 检查映射配额（TODO: 需要实现 GetClientPortMappings 并统计）
	// 暂时跳过配额检查，后续实现

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
		Type:        models.MappingTypeAnonymous, // ✅ 通过 Type 标识是通过连接码创建的
		Description: connCode.Description,
	}

	// 7. 通过 PortMappingService 创建映射（会自动生成ID和SecretKey，并更新索引）
	createdMapping, err := s.portMappingService.CreatePortMapping(mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create port mapping: %w", err)
	}

	// 8. ✅ 连接码记录 MappingID（反向关系）
	if err := connCode.Activate(req.ListenClientID, createdMapping.ID); err != nil {
		// 回滚：删除已创建的映射
		_ = s.portMappingService.DeletePortMapping(createdMapping.ID)
		return nil, fmt.Errorf("failed to activate connection code: %w", err)
	}

	if err := s.connCodeRepo.Update(connCode); err != nil {
		// 回滚：删除已创建的映射
		_ = s.portMappingService.DeletePortMapping(createdMapping.ID)
		return nil, fmt.Errorf("failed to update connection code: %w", err)
	}

	utils.Infof("ConnectionCodeService: activated code %s, created mapping %s (%d → %d)",
		req.Code, createdMapping.ID, req.ListenClientID, connCode.TargetClientID)

	return createdMapping, nil
}

// RevokeConnectionCode 撤销连接码
//
// 只能撤销未使用的连接码
func (s *ConnectionCodeService) RevokeConnectionCode(code string, revokedBy string) error {
	// 1. 获取连接码
	connCode, err := s.connCodeRepo.GetByCode(code)
	if err != nil {
		if errors.Is(err, repos.ErrNotFound) {
			return fmt.Errorf("connection code not found or expired")
		}
		return fmt.Errorf("failed to get connection code: %w", err)
	}

	// 2. 撤销
	if err := connCode.Revoke(revokedBy); err != nil {
		return fmt.Errorf("failed to revoke connection code: %w", err)
	}

	// 3. 更新
	if err := s.connCodeRepo.Update(connCode); err != nil {
		return fmt.Errorf("failed to update connection code: %w", err)
	}

	utils.Infof("ConnectionCodeService: revoked code %s by %s", code, revokedBy)

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道映射管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ValidateMapping 验证端口映射权限
//
// 用于HandleTunnelOpen时验证ListenClient是否有权限使用此映射
func (s *ConnectionCodeService) ValidateMapping(mappingID string, clientID int64) (*models.PortMapping, error) {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return nil, fmt.Errorf("mapping not found or expired: %w", err)
	}

	// 添加详细日志
	utils.Debugf("ConnectionCodeService.ValidateMapping: mappingID=%s, clientID=%d, ListenClientID=%d, TargetClientID=%d, Status=%s, IsRevoked=%v, IsExpired=%v, IsValid=%v",
		mappingID, clientID, mapping.ListenClientID, mapping.TargetClientID, mapping.Status, mapping.IsRevoked, mapping.IsExpired(), mapping.IsValid())

	// 验证权限
	if !mapping.CanBeAccessedBy(clientID) {
		utils.Warnf("ConnectionCodeService.ValidateMapping: CanBeAccessedBy returned false for mappingID=%s, clientID=%d", mappingID, clientID)
		if mapping.IsRevoked {
			return nil, fmt.Errorf("mapping has been revoked")
		}
		if mapping.IsExpired() {
			return nil, fmt.Errorf("mapping has expired")
		}
		listenClientID := mapping.ListenClientID
		if listenClientID == 0 {
			listenClientID = mapping.SourceClientID
		}
		if listenClientID != clientID {
			utils.Warnf("ConnectionCodeService.ValidateMapping: clientID mismatch - expected ListenClientID=%d, got clientID=%d", listenClientID, clientID)
			return nil, fmt.Errorf("client %d is not authorized to use this mapping", clientID)
		}
		// 如果到这里，说明 IsValid() 返回了 false，但具体原因未知
		utils.Errorf("ConnectionCodeService.ValidateMapping: mapping cannot be accessed - Status=%s, IsRevoked=%v, IsExpired=%v",
			mapping.Status, mapping.IsRevoked, mapping.IsExpired())
		return nil, fmt.Errorf("mapping cannot be accessed")
	}

	utils.Debugf("ConnectionCodeService.ValidateMapping: validation passed for mappingID=%s, clientID=%d", mappingID, clientID)
	return mapping, nil
}

// RevokeMapping 撤销映射
//
// TargetClient 或 ListenClient 都可以撤销
func (s *ConnectionCodeService) RevokeMapping(mappingID string, clientID int64, revokedBy string) error {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return fmt.Errorf("mapping not found or expired: %w", err)
	}

	// 撤销
	if err := mapping.Revoke(revokedBy, clientID); err != nil {
		return fmt.Errorf("failed to revoke mapping: %w", err)
	}

	// 更新
	if err := s.portMappingService.UpdatePortMapping(mapping); err != nil {
		return fmt.Errorf("failed to update mapping: %w", err)
	}

	utils.Infof("ConnectionCodeService: revoked mapping %s by %s (client %d)",
		mappingID, revokedBy, clientID)
	return nil
}

// RecordMappingUsage 记录映射使用
//
// 在每次建立隧道连接时调用
func (s *ConnectionCodeService) RecordMappingUsage(mappingID string) error {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return fmt.Errorf("mapping not found: %w", err)
	}

	// 更新最后活跃时间
	now := time.Now()
	mapping.LastActive = &now
	if err := s.portMappingService.UpdatePortMapping(mapping); err != nil {
		return fmt.Errorf("failed to update mapping usage: %w", err)
	}

	return nil
}

// RecordMappingTraffic 记录映射流量
//
// 在隧道连接关闭时调用
func (s *ConnectionCodeService) RecordMappingTraffic(mappingID string, bytesSent, bytesReceived int64) error {
	mapping, err := s.portMappingService.GetPortMapping(mappingID)
	if err != nil {
		return fmt.Errorf("mapping not found: %w", err)
	}

	// 更新流量统计
	mapping.TrafficStats.BytesSent += bytesSent
	mapping.TrafficStats.BytesReceived += bytesReceived
	mapping.TrafficStats.LastUpdated = time.Now()

	if err := s.portMappingService.UpdatePortMappingStats(mappingID, &mapping.TrafficStats); err != nil {
		return fmt.Errorf("failed to update mapping traffic: %w", err)
	}

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 查询方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ListConnectionCodesByTargetClient 列出TargetClient的连接码
//
// 返回指定TargetClient生成的所有连接码
func (s *ConnectionCodeService) ListConnectionCodesByTargetClient(targetClientID int64) ([]*models.TunnelConnectionCode, error) {
	return s.connCodeRepo.ListByTargetClient(targetClientID)
}

// GetConnectionCode 获取连接码详情
func (s *ConnectionCodeService) GetConnectionCode(code string) (*models.TunnelConnectionCode, error) {
	connCode, err := s.connCodeRepo.GetByCode(code)
	if err != nil {
		if errors.Is(err, repos.ErrNotFound) {
			return nil, fmt.Errorf("connection code not found or expired")
		}
		return nil, fmt.Errorf("failed to get connection code: %w", err)
	}
	return connCode, nil
}

// ListOutboundMappings 列出出站映射（ListenClient创建的映射）
//
// 返回指定ListenClient创建的所有映射（我在访问谁）
func (s *ConnectionCodeService) ListOutboundMappings(listenClientID int64) ([]*models.PortMapping, error) {
	clientKey := utils.Int64ToString(listenClientID)
	utils.Infof("ConnectionCodeService.ListOutboundMappings: querying mappings for client %d (key=%s)", listenClientID, clientKey)

	allMappings, err := s.portMappingRepo.GetClientPortMappings(clientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get client port mappings: %w", err)
	}

	utils.Infof("ConnectionCodeService.ListOutboundMappings: found %d mappings from index for client %d", len(allMappings), listenClientID)

	// 过滤出 ListenClientID 匹配的映射
	result := make([]*models.PortMapping, 0)
	for _, m := range allMappings {
		if m.ListenClientID == listenClientID || (m.ListenClientID == 0 && m.SourceClientID == listenClientID) {
			utils.Debugf("ConnectionCodeService.ListOutboundMappings: adding mapping %s (ListenClientID=%d, SourceClientID=%d)", m.ID, m.ListenClientID, m.SourceClientID)
			result = append(result, m)
		} else {
			utils.Debugf("ConnectionCodeService.ListOutboundMappings: skipping mapping %s (ListenClientID=%d != %d, SourceClientID=%d)", m.ID, m.ListenClientID, listenClientID, m.SourceClientID)
		}
	}

	utils.Infof("ConnectionCodeService.ListOutboundMappings: returning %d outbound mappings for client %d", len(result), listenClientID)
	return result, nil
}

// ListInboundMappings 列出入站映射（通过TargetClient的连接码创建的映射）
//
// 返回访问指定TargetClient的所有映射（谁在访问我）
func (s *ConnectionCodeService) ListInboundMappings(targetClientID int64) ([]*models.PortMapping, error) {
	clientKey := utils.Int64ToString(targetClientID)
	utils.Infof("ConnectionCodeService.ListInboundMappings: querying mappings for client %d (key=%s)", targetClientID, clientKey)

	allMappings, err := s.portMappingRepo.GetClientPortMappings(clientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get client port mappings: %w", err)
	}

	utils.Infof("ConnectionCodeService.ListInboundMappings: found %d mappings from index for client %d", len(allMappings), targetClientID)

	// 过滤出 TargetClientID 匹配的映射
	result := make([]*models.PortMapping, 0)
	for _, m := range allMappings {
		if m.TargetClientID == targetClientID {
			utils.Debugf("ConnectionCodeService.ListInboundMappings: adding mapping %s (TargetClientID=%d)", m.ID, m.TargetClientID)
			result = append(result, m)
		} else {
			utils.Debugf("ConnectionCodeService.ListInboundMappings: skipping mapping %s (TargetClientID=%d != %d)", m.ID, m.TargetClientID, targetClientID)
		}
	}

	utils.Infof("ConnectionCodeService.ListInboundMappings: returning %d inbound mappings for client %d", len(result), targetClientID)
	return result, nil
}

// GetMapping 获取映射详情
func (s *ConnectionCodeService) GetMapping(mappingID string) (*models.PortMapping, error) {
	return s.portMappingService.GetPortMapping(mappingID)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 后台任务
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cleanupExpiredEntities 定期清理过期的连接码和映射
//
// 虽然Redis会自动TTL过期，但索引列表需要手动清理
func (s *ConnectionCodeService) cleanupExpiredEntities(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			utils.Infof("ConnectionCodeService: cleanup task stopped")
			return
		case <-ticker.C:
			utils.Debugf("ConnectionCodeService: running cleanup task")
			// 注意：由于使用Redis TTL，过期的键会自动删除
			// Repository的List方法会自动清理失效的索引引用
			// 所以这里不需要做额外的清理工作
		}
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// generateID 生成ID
//
// 格式：prefix_xxxxxxxx（8位16进制随机字符）
func (s *ConnectionCodeService) generateID(prefix string) (string, error) {
	randomPart, err := utils.GenerateRandomStringWithCharset(8, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return prefix + "_" + randomPart, nil
}
