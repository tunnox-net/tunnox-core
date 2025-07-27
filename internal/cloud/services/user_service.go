package services

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
)

// UserServiceImpl 用户服务实现
type UserServiceImpl struct {
	*dispose.ServiceBase
	baseService *BaseService
	userRepo    *repos.UserRepository
	idManager   *idgen.IDManager
}

// NewUserService 创建用户服务
func NewUserService(userRepo *repos.UserRepository, idManager *idgen.IDManager, parentCtx context.Context) UserService {
	service := &UserServiceImpl{
		ServiceBase: dispose.NewService("UserService", parentCtx),
		baseService: NewBaseService(),
		userRepo:    userRepo,
		idManager:   idManager,
	}
	return service
}

// CreateUser 创建用户
func (s *UserServiceImpl) CreateUser(username, email string) (*models.User, error) {
	// 生成用户ID
	userID, err := s.idManager.GenerateUserID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate user ID")
	}

	// 创建用户
	user := &models.User{
		ID:       userID,
		Username: username,
		Email:    email,
		Type:     models.UserTypeRegistered,
		Status:   models.UserStatusActive,
	}

	// 设置时间戳
	s.baseService.SetTimestamps(&user.CreatedAt, &user.UpdatedAt)

	// 保存到存储
	if err := s.userRepo.CreateUser(user); err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseString(err, userID, s.idManager.ReleaseUserID, "create user")
	}

	s.baseService.LogCreated("user", fmt.Sprintf("%s (ID: %s)", username, userID))
	return user, nil
}

// GetUser 获取用户
func (s *UserServiceImpl) GetUser(userID string) (*models.User, error) {
	user, err := s.userRepo.GetUser(userID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get user", userID)
	}
	return user, nil
}

// UpdateUser 更新用户
func (s *UserServiceImpl) UpdateUser(user *models.User) error {
	s.baseService.SetUpdatedTimestamp(&user.UpdatedAt)
	if err := s.userRepo.UpdateUser(user); err != nil {
		return s.baseService.WrapErrorWithID(err, "update user", user.ID)
	}
	s.baseService.LogUpdated("user", user.ID)
	return nil
}

// DeleteUser 删除用户
func (s *UserServiceImpl) DeleteUser(userID string) error {
	// 删除用户
	if err := s.userRepo.DeleteUser(userID); err != nil {
		return s.baseService.WrapErrorWithID(err, "delete user", userID)
	}

	// 释放用户ID
	if err := s.idManager.ReleaseUserID(userID); err != nil {
		s.baseService.LogWarning("release user ID", err, userID)
	}

	s.baseService.LogDeleted("user", userID)
	return nil
}

// ListUsers 列出用户
func (s *UserServiceImpl) ListUsers(userType models.UserType) ([]*models.User, error) {
	users, err := s.userRepo.ListUsers(userType)
	if err != nil {
		return nil, s.baseService.WrapError(err, fmt.Sprintf("list users by type %v", userType))
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
	// 这里需要根据实际的repository方法来实现
	// 暂时返回nil，实际项目中需要实现具体的统计逻辑
	return nil, fmt.Errorf("user stats not implemented")
}
