package repos

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
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

	// 检查配置是否存在
	_, err := r.GetConfig(config.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNotFound, "config not found")
	}

	// 更新时间戳
	config.UpdatedAt = time.Now()

	// 更新主数据
	// 注意：不再需要同步全局列表，ListConfigs 使用 QueryByPrefix 直接从数据库查询
	return r.Update(
		config,
		constants.KeyPrefixPersistClientConfig,
		0, // 永久保存
	)
}

// DeleteConfig 删除客户端配置
//
// 流程：
// 1. 删除主数据（如果存在）
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientConfigRepository) DeleteConfig(clientID int64) error {
	// 删除主数据（忽略不存在的错误）
	// 注意：不再需要同步全局列表，ListConfigs 使用 QueryByPrefix 直接从数据库查询
	_ = r.Delete(
		fmt.Sprintf("%d", clientID),
		constants.KeyPrefixPersistClientConfig,
	)
	return nil
}

// ListConfigs 列出所有客户端配置
//
// 实现说明：
// 使用 QueryByPrefix 直接从数据库扫描所有配置，不依赖全局列表
// 这样可以避免数据一致性问题和重复/遗漏问题
//
// 返回：
//   - []*models.ClientConfig: 配置列表
//   - error: 错误信息
func (r *ClientConfigRepository) ListConfigs() ([]*models.ClientConfig, error) {
	// 使用 QueryByPrefix 直接从数据库查询，不再依赖全局列表
	queryStore, ok := r.storage.(interface {
		QueryByPrefix(prefix string, limit int) (map[string]string, error)
	})
	if !ok {
		return nil, coreerrors.New(coreerrors.CodeStorageError, "storage does not support QueryByPrefix")
	}

	// 查询所有客户端配置
	items, err := queryStore.QueryByPrefix(constants.KeyPrefixPersistClientConfig, 0)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query configs by prefix")
	}

	// 反序列化结果
	// 注意：由于历史原因，数据库中的值可能是双重 JSON 编码的
	// 即：外层是 JSON 字符串，内层才是实际的 JSON 对象
	configs := make([]*models.ClientConfig, 0, len(items))
	parseErrors := 0
	for key, jsonValue := range items {
		var config models.ClientConfig

		// 尝试直接解析
		if err := json.Unmarshal([]byte(jsonValue), &config); err != nil {
			// 如果失败，尝试先解析外层字符串（处理双重编码）
			var innerJSON string
			if err2 := json.Unmarshal([]byte(jsonValue), &innerJSON); err2 == nil {
				// 成功解析为字符串，再解析内层 JSON
				if err3 := json.Unmarshal([]byte(innerJSON), &config); err3 == nil {
					// 双重编码解析成功
					configs = append(configs, &config)
					continue
				}
			}

			// 两种方式都失败了，记录错误
			parseErrors++
			if parseErrors <= 3 {
				maxLen := len(jsonValue)
				if maxLen > 100 {
					maxLen = 100
				}
				dispose.Errorf("ListConfigs: failed to parse key=%s, err=%v, value=%s", key, err, jsonValue[:maxLen])
			}
			continue
		}
		configs = append(configs, &config)
	}

	dispose.Infof("ListConfigs: queried %d items, parsed %d configs, errors=%d", len(items), len(configs), parseErrors)

	// 按创建时间降序排序（最新的在前面）
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].CreatedAt.After(configs[j].CreatedAt)
	})

	return configs, nil
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
// Deprecated: 不再需要手动维护全局列表，ListConfigs 使用 QueryByPrefix 直接查询数据库
// 此方法保留仅为向后兼容，实际为空操作
func (r *ClientConfigRepository) AddConfigToList(config *models.ClientConfig) error {
	// 空操作：不再维护全局列表
	return nil
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
