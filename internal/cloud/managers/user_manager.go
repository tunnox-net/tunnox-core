package managers

import (
	"time"

	"tunnox-core/internal/cloud/models"
)

// CreateUser 创建用户
func (c *CloudControl) CreateUser(username, email string) (*models.User, error) {
	userID, _ := c.idManager.GenerateUserID()
	now := time.Now()
	user := &models.User{
		ID:        userID,
		Username:  username,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := c.userRepo.CreateUser(user); err != nil {
		return nil, err
	}

	// 更新统计计数器
	if c.statsManager != nil && c.statsManager.GetCounter() != nil {
		_ = c.statsManager.GetCounter().IncrUser(1)
	}

	return user, nil
}

// GetUser 获取用户
func (c *CloudControl) GetUser(userID string) (*models.User, error) {
	return c.userRepo.GetUser(userID)
}

// UpdateUser 更新用户
func (c *CloudControl) UpdateUser(user *models.User) error {
	user.UpdatedAt = time.Now()
	return c.userRepo.UpdateUser(user)
}

// DeleteUser 删除用户
func (c *CloudControl) DeleteUser(userID string) error {
	return c.userRepo.DeleteUser(userID)
}

// ListUsers 列出用户
func (c *CloudControl) ListUsers(userType models.UserType) ([]*models.User, error) {
	return c.userRepo.ListUsers(userType)
}
