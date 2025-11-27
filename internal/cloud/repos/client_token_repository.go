package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils"
)

// ClientTokenRepository 客户端Token数据访问层
//
// 职责：
// - 管理客户端JWT Token（仅缓存，不持久化到数据库）
// - Token过期自动删除
//
// 数据存储：
// - 键前缀：tunnox:runtime:client:token:
// - 存储：仅缓存（Redis/Memory）
// - TTL：Token过期时间（自动过期）
type ClientTokenRepository struct {
	*dispose.ManagerBase
	storage storage.Storage
}

// NewClientTokenRepository 创建Repository
//
// 参数：
//   - ctx: 上下文（用于Dispose）
//   - storage: 存储接口
//
// 返回：
//   - *ClientTokenRepository: TokenRepository实例
func NewClientTokenRepository(ctx context.Context, storage storage.Storage) *ClientTokenRepository {
	repo := &ClientTokenRepository{
		ManagerBase: dispose.NewManager("ClientTokenRepository", ctx),
		storage:     storage,
	}
	
	// 设置清理回调
	repo.SetCtx(ctx, repo.onClose)
	
	return repo
}

// onClose 资源清理回调
func (r *ClientTokenRepository) onClose() error {
	utils.Infof("ClientTokenRepository: closing")
	// Token数据存储在缓存中，会自动过期，无需手动清理
	return nil
}

// GetToken 获取Token
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - *models.ClientToken: Token对象（如果不存在或已过期返回nil）
//   - error: 错误信息
func (r *ClientTokenRepository) GetToken(clientID int64) (*models.ClientToken, error) {
	key := fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientToken, clientID)
	
	value, err := r.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil, nil // Token不存在
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	
	// 反序列化
	var token models.ClientToken
	if jsonStr, ok := value.(string); ok {
		if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
			return nil, fmt.Errorf("failed to unmarshal token: %w", err)
		}
	} else {
		return nil, fmt.Errorf("invalid token type: %T", value)
	}
	
	// 检查是否过期
	if token.IsExpired() {
		// 已过期，删除并返回nil
		_ = r.DeleteToken(clientID)
		return nil, nil
	}
	
	return &token, nil
}

// SetToken 设置Token
//
// 参数：
//   - token: Token对象
//
// 返回：
//   - error: 错误信息
func (r *ClientTokenRepository) SetToken(token *models.ClientToken) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	
	// 验证Token有效性
	if err := token.Validate(); err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}
	
	// 如果已过期，不保存
	if token.IsExpired() {
		return fmt.Errorf("token already expired")
	}
	
	key := fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientToken, token.ClientID)
	
	// 序列化
	jsonBytes, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}
	
	// 计算TTL（Token过期时间）
	ttl := token.TTL()
	if ttl <= 0 {
		return fmt.Errorf("token TTL is zero or negative")
	}
	
	// 写入缓存
	if err := r.storage.Set(key, string(jsonBytes), ttl); err != nil {
		return fmt.Errorf("failed to set token: %w", err)
	}
	
	utils.Debugf("ClientTokenRepository: set token for client %d (expires_in=%s)", 
		token.ClientID, ttl)
	
	return nil
}

// DeleteToken 删除Token
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - error: 错误信息
func (r *ClientTokenRepository) DeleteToken(clientID int64) error {
	key := fmt.Sprintf("%s%d", constants.KeyPrefixRuntimeClientToken, clientID)
	
	if err := r.storage.Delete(key); err != nil && err != storage.ErrKeyNotFound {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	
	utils.Debugf("ClientTokenRepository: deleted token for client %d", clientID)
	return nil
}

// TokenExists 检查Token是否存在且有效
//
// 参数：
//   - clientID: 客户端ID
//
// 返回：
//   - bool: 是否存在且有效
//   - error: 错误信息
func (r *ClientTokenRepository) TokenExists(clientID int64) (bool, error) {
	token, err := r.GetToken(clientID)
	if err != nil {
		return false, err
	}
	
	return token != nil && token.IsValid(), nil
}

// RefreshToken 刷新Token（延长TTL）
//
// 参数：
//   - token: 新的Token对象
//
// 返回：
//   - error: 错误信息
func (r *ClientTokenRepository) RefreshToken(token *models.ClientToken) error {
	// 先删除旧Token
	_ = r.DeleteToken(token.ClientID)
	
	// 设置新Token
	return r.SetToken(token)
}

