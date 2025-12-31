package managers

import (
	coreerrors "tunnox-core/internal/core/errors"

	"tunnox-core/internal/cloud/models"
)

// CreateUser 创建用户
// 注意：此方法委托给 UserService 处理，遵循 Manager -> Service -> Repository 架构
func (c *CloudControl) CreateUser(username, email string) (*models.User, error) {
	if c.userService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "userService not initialized")
	}
	return c.userService.CreateUser(username, email)
}

// GetUser 获取用户
func (c *CloudControl) GetUser(userID string) (*models.User, error) {
	if c.userService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "userService not initialized")
	}
	return c.userService.GetUser(userID)
}

// UpdateUser 更新用户
func (c *CloudControl) UpdateUser(user *models.User) error {
	if c.userService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "userService not initialized")
	}
	return c.userService.UpdateUser(user)
}

// DeleteUser 删除用户
func (c *CloudControl) DeleteUser(userID string) error {
	if c.userService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "userService not initialized")
	}
	return c.userService.DeleteUser(userID)
}

// ListUsers 列出用户
func (c *CloudControl) ListUsers(userType models.UserType) ([]*models.User, error) {
	if c.userService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "userService not initialized")
	}
	return c.userService.ListUsers(userType)
}
