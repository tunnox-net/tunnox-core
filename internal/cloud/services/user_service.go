package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
)

// UserServiceImpl 用户服务实现
type UserServiceImpl struct {
	*dispose.ResourceBase
	userRepo  *repos.UserRepository
	idManager *generators.IDManager
	statsMgr  *managers.StatsManager
}

// NewUserService 创建用户服务
func NewUserService(userRepo *repos.UserRepository, idManager *generators.IDManager, statsMgr *managers.StatsManager, parentCtx context.Context) UserService {
	service := &UserServiceImpl{
		ResourceBase: dispose.NewResourceBase("UserService"),
		userRepo:     userRepo,
		idManager:    idManager,
		statsMgr:     statsMgr,
	}
	service.Initialize(parentCtx)
	return service
}

// CreateUser 创建用户
func (s *UserServiceImpl) CreateUser(username, email string) (*models.User, error) {
	// 生成用户ID
	userID, err := s.idManager.GenerateUserID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
	}

	// 创建用户
	user := &models.User{
		ID:        userID,
		Username:  username,
		Email:     email,
		Type:      models.UserTypeRegistered,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 保存到存储
	if err := s.userRepo.CreateUser(user); err != nil {
		// 释放已生成的ID
		_ = s.idManager.ReleaseUserID(userID)
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetUser 获取用户
func (s *UserServiceImpl) GetUser(userID string) (*models.User, error) {
	user, err := s.userRepo.GetUser(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user %s: %w", userID, err)
	}
	return user, nil
}

// UpdateUser 更新用户
func (s *UserServiceImpl) UpdateUser(user *models.User) error {
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(user); err != nil {
		return fmt.Errorf("failed to update user %s: %w", user.ID, err)
	}
	return nil
}

// DeleteUser 删除用户
func (s *UserServiceImpl) DeleteUser(userID string) error {
	if err := s.userRepo.DeleteUser(userID); err != nil {
		return fmt.Errorf("failed to delete user %s: %w", userID, err)
	}

	// 释放用户ID
	if err := s.idManager.ReleaseUserID(userID); err != nil {
		return fmt.Errorf("failed to release user ID %s: %w", userID, err)
	}

	return nil
}

// ListUsers 列出用户
func (s *UserServiceImpl) ListUsers(userType models.UserType) ([]*models.User, error) {
	users, err := s.userRepo.ListUsers(userType)
	if err != nil {
		return nil, fmt.Errorf("failed to list users by type %v: %w", userType, err)
	}
	return users, nil
}

// SearchUsers 搜索用户
func (s *UserServiceImpl) SearchUsers(keyword string) ([]*models.User, error) {
	// 暂时返回空列表，因为UserRepository没有Search方法
	// TODO: 实现搜索功能
	return []*models.User{}, nil
}

// GetUserStats 获取用户统计信息
func (s *UserServiceImpl) GetUserStats(userID string) (*stats.UserStats, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	userStats, err := s.statsMgr.GetUserStats(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats for %s: %w", userID, err)
	}
	return userStats, nil
}
