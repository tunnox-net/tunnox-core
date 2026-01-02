package repos

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
)

// 编译时接口断言，确保 ClientConfigRepository 实现了 IClientConfigRepository 接口
var _ IClientConfigRepository = (*ClientConfigRepository)(nil)

// ClientConfigRepository 客户端配置数据访问层
//
// 职责：
// - 管理客户端持久化配置的CRUD操作
// - 使用HybridStorage自动处理缓存+数据库
//
// 数据存储：
// - 键前缀：tunnox:persist:client:config:
// - 存储：数据库 + 缓存（HybridStorage）
// - TTL：永久（0 = 不过期）
type ClientConfigRepository struct {
	*GenericRepositoryImpl[*models.ClientConfig]
}

// NewClientConfigRepository 创建Repository
//
// 参数：
//   - repo: 基础Repository（包含Storage）
//
// 返回：
//   - *ClientConfigRepository: 配置Repository实例
func NewClientConfigRepository(repo *Repository) *ClientConfigRepository {
	genericRepo := NewGenericRepository[*models.ClientConfig](
		repo,
		// ID提取函数：从ClientConfig提取ID字符串
		func(config *models.ClientConfig) (string, error) {
			if config == nil {
				return "", coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
			}
			return fmt.Sprintf("%d", config.ID), nil
		},
	)

	return &ClientConfigRepository{
		GenericRepositoryImpl: genericRepo,
	}
}

// GetConfig 获取客户端配置
//
// 流程：
// 1. 先从缓存读取
// 2. 缓存miss → 查数据库
// 3. 回写缓存
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - *models.ClientConfig: 配置对象
//   - error: 错误信息
func (r *ClientConfigRepository) GetConfig(clientID int64) (*models.ClientConfig, error) {
	return r.Get(
		fmt.Sprintf("%d", clientID),
		constants.KeyPrefixPersistClientConfig,
	)
}

// SaveConfig 保存客户端配置（创建或更新）
//
// 流程：
// 1. 更新UpdatedAt时间戳
// 2. 写入数据库
// 3. 写入缓存
//
// 参数：
//   - config: 客户端配置
//
// 返回：
//   - error: 错误信息
func (r *ClientConfigRepository) SaveConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}

	// 验证配置有效性
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 保存（TTL=0表示永久）
	return r.Save(
		config,
		constants.KeyPrefixPersistClientConfig,
		0, // 永久保存
	)
}

// CreateConfig 创建新的客户端配置（仅创建，不允许覆盖）
//
// 参数：
//   - config: 客户端配置
//
// 返回：
//   - error: 错误信息（如果已存在则返回错误）
func (r *ClientConfigRepository) CreateConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	// 设置创建时间
	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	// 创建（不覆盖已存在的）
	return r.Create(
		config,
		constants.KeyPrefixPersistClientConfig,
		0, // 永久保存
	)
}

// UpdateConfig 更新客户端配置（仅更新，不允许创建）
//
// 参数：
//   - config: 客户端配置
//
// 返回：
//   - error: 错误信息（如果不存在则返回错误）
func (r *ClientConfigRepository) UpdateConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	// 获取旧配置（用于从列表中移除）
	oldConfig, err := r.GetConfig(config.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNotFound, "config not found")
	}

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 更新主数据
	if err := r.Update(
		config,
		constants.KeyPrefixPersistClientConfig,
		0, // 永久保存
	); err != nil {
		return err
	}

	// 同步全局列表：先移除旧的，再添加新的
	// 注意：RemoveFromList 使用 JSON 精确匹配，必须使用旧配置对象
	if err := r.RemoveFromList(oldConfig, constants.KeyPrefixPersistClientsList); err != nil {
		// 列表同步失败不影响主流程，记录警告
		// 可能是旧配置不在列表中（匿名客户端等情况）
	}
	if err := r.AddToList(config, constants.KeyPrefixPersistClientsList); err != nil {
		// 列表同步失败不影响主流程，记录警告
	}

	return nil
}

// DeleteConfig 删除客户端配置
//
// 流程：
// 1. 获取配置（用于从列表移除）
// 2. 从数据库删除
// 3. 从缓存删除
// 4. 从全局列表移除
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientConfigRepository) DeleteConfig(clientID int64) error {
	// 先获取配置，用于从列表中移除
	config, err := r.GetConfig(clientID)
	if err != nil {
		// 配置不存在，直接返回（可能已被删除）
		return nil
	}

	// 删除主数据
	if err := r.Delete(
		fmt.Sprintf("%d", clientID),
		constants.KeyPrefixPersistClientConfig,
	); err != nil {
		return err
	}

	// 从全局列表中移除
	// 注意：RemoveFromList 使用 JSON 精确匹配
	if err := r.RemoveFromList(config, constants.KeyPrefixPersistClientsList); err != nil {
		// 列表同步失败不影响主流程
	}

	return nil
}

// ListConfigs 列出所有客户端配置
//
// 返回：
//   - []*models.ClientConfig: 配置列表
//   - error: 错误信息
func (r *ClientConfigRepository) ListConfigs() ([]*models.ClientConfig, error) {
	return r.List(constants.KeyPrefixPersistClientsList)
}

// ListUserConfigs 列出用户的所有客户端配置
//
// 参数：
//   - userID: 用户ID
//
// 返回：
//   - []*models.ClientConfig: 用户的配置列表
//   - error: 错误信息
func (r *ClientConfigRepository) ListUserConfigs(userID string) ([]*models.ClientConfig, error) {
	// 获取所有配置
	allConfigs, err := r.ListConfigs()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list all configs")
	}

	// 过滤出指定用户的配置
	userConfigs := make([]*models.ClientConfig, 0)
	for _, config := range allConfigs {
		if config.UserID == userID {
			userConfigs = append(userConfigs, config)
		}
	}

	return userConfigs, nil
}

// AddConfigToList 将配置添加到全局列表
//
// 参数：
//   - config: 客户端配置
//
// 返回：
//   - error: 错误信息
func (r *ClientConfigRepository) AddConfigToList(config *models.ClientConfig) error {
	return r.AddToList(config, constants.KeyPrefixPersistClientsList)
}

// ExistsConfig 检查配置是否存在
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - bool: 是否存在
//   - error: 错误信息
func (r *ClientConfigRepository) ExistsConfig(clientID int64) (bool, error) {
	key := fmt.Sprintf("%s%d", constants.KeyPrefixPersistClientConfig, clientID)
	return r.storage.Exists(key)
}
