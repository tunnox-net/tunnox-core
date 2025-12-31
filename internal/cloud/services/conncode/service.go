package conncode

import (
	"context"
	"errors"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// PortMappingService 端口映射服务接口（避免循环依赖）
type PortMappingService interface {
	CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error)
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	UpdatePortMapping(mapping *models.PortMapping) error
	DeletePortMapping(mappingID string) error
	UpdatePortMappingStats(mappingID string, stats interface{}) error
}

// Service 连接码服务
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
type Service struct {
	*dispose.ServiceBase

	// Repositories
	connCodeRepo *repos.ConnectionCodeRepository

	// Services
	portMappingService PortMappingService         // 统一使用 PortMappingService
	portMappingRepo    *repos.PortMappingRepo // 用于查询客户端映射

	// 连接码生成器
	generator *Generator

	// 配额限制
	maxActiveCodesPerClient    int // 每个客户端最多活跃连接码数
	maxActiveMappingsPerClient int // 每个客户端最多活跃映射数
}

// Config 连接码服务配置
type Config struct {
	MaxActiveCodesPerClient    int // 默认: 10
	MaxActiveMappingsPerClient int // 默认: 50
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxActiveCodesPerClient:    10,
		MaxActiveMappingsPerClient: 50,
	}
}

// NewService 创建连接码服务
func NewService(
	connCodeRepo *repos.ConnectionCodeRepository,
	portMappingService PortMappingService,
	portMappingRepo *repos.PortMappingRepo,
	config *Config,
	ctx context.Context,
) *Service {
	if config == nil {
		config = DefaultConfig()
	}

	service := &Service{
		ServiceBase:                dispose.NewService("ConnectionCodeService", ctx),
		connCodeRepo:               connCodeRepo,
		portMappingService:         portMappingService,
		portMappingRepo:            portMappingRepo,
		generator:                  NewGenerator(models.DefaultConnectionCodeGenerator()),
		maxActiveCodesPerClient:    config.MaxActiveCodesPerClient,
		maxActiveMappingsPerClient: config.MaxActiveMappingsPerClient,
	}

	// 启动后台任务：定期清理过期的连接码和映射
	go service.cleanupExpiredEntities(ctx)

	return service
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码创建
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateRequest 创建连接码请求
type CreateRequest struct {
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
func (s *Service) CreateConnectionCode(req *CreateRequest) (*models.TunnelConnectionCode, error) {
	// 1. 参数验证
	if req.TargetClientID == 0 {
		return nil, coreerrors.New(coreerrors.CodeMissingParam, "target client ID is required")
	}
	if req.TargetAddress == "" {
		return nil, coreerrors.New(coreerrors.CodeMissingParam, "target address is required")
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to count active codes")
	}
	if activeCount >= s.maxActiveCodesPerClient {
		return nil, coreerrors.Newf(coreerrors.CodeQuotaExceeded, "max %d active connection codes allowed",
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternalError, "failed to generate unique code")
	}

	// 4. 创建连接码对象
	now := time.Now()
	id, err := s.generateID("conncode")
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternalError, "failed to generate ID")
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
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to create connection code")
	}

	corelog.Infof("ConnectionCodeService: created code %s for target client %d (expires in %v)",
		code, req.TargetClientID, req.ActivationTTL)

	return connCode, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 后台任务
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cleanupExpiredEntities 定期清理过期的连接码和映射
//
// 虽然Redis会自动TTL过期，但索引列表需要手动清理
func (s *Service) cleanupExpiredEntities(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			corelog.Infof("ConnectionCodeService: cleanup task stopped")
			return
		case <-ticker.C:
			corelog.Debugf("ConnectionCodeService: running cleanup task")
			// 清理过期的连接码索引
			// 注意：Redis TTL会自动删除过期的键，但索引列表需要手动清理
			// 这里通过遍历所有客户端来清理索引中的过期引用
			// 由于没有全局客户端列表，清理工作主要在List时进行
		}
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// generateID 生成ID
//
// 格式：prefix_xxxxxxxx（8位16进制随机字符）
func (s *Service) generateID(prefix string) (string, error) {
	randomPart, err := utils.GenerateRandomStringWithCharset(8, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeInternalError, "failed to generate random ID")
	}
	return prefix + "_" + randomPart, nil
}

// GetPortMappingService 获取端口映射服务
func (s *Service) GetPortMappingService() PortMappingService {
	return s.portMappingService
}
