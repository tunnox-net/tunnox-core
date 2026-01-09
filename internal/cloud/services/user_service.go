package services

import (
	"context"
	"fmt"
	"strings"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
)

// userService 用户服务实现
type userService struct {
	*dispose.ServiceBase
	baseService  *BaseService
	userRepo     *repos.UserRepository
	idManager    *idgen.IDManager
	statsCounter *stats.StatsCounter
}

// NewUserService 创建用户服务
func NewUserService(userRepo *repos.UserRepository, idManager *idgen.IDManager, statsCounter *stats.StatsCounter, parentCtx context.Context) UserService {
	service := &userService{
		ServiceBase:  dispose.NewService("UserService", parentCtx),
		baseService:  NewBaseService(),
		userRepo:     userRepo,
		idManager:    idManager,
		statsCounter: statsCounter,
	}
	return service
}

// CreateUser 创建用户
// platformUserID: Platform 用户 ID（BIGINT），用于双向关联，0 表示未关联
func (s *userService) CreateUser(username, email string, platformUserID int64) (*models.User, error) {
	// 生成用户ID
	userID, err := s.idManager.GenerateUserID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate user ID")
	}

	// 创建用户
	user := &models.User{
		ID:             userID,
		PlatformUserID: platformUserID, // 保存 Platform 用户 ID，实现双向关联
		Username:       username,
		Email:          email,
		Type:           models.UserTypeRegistered,
		Status:         models.UserStatusActive,
	}

	// 设置时间戳
	s.baseService.SetTimestamps(&user.CreatedAt, &user.UpdatedAt)

	// 保存到存储
	if err := s.userRepo.CreateUser(user); err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseString(err, userID, s.idManager.ReleaseUserID, "create user")
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrUser(1); err != nil {
			s.baseService.LogWarning("update user stats counter", err, userID)
		}
	}

	s.baseService.LogCreated("user", fmt.Sprintf("%s (ID: %s)", username, userID))
	return user, nil
}

// GetUser 获取用户
func (s *userService) GetUser(userID string) (*models.User, error) {
	user, err := s.userRepo.GetUser(userID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get user", userID)
	}
	return user, nil
}

// UpdateUser 更新用户
func (s *userService) UpdateUser(user *models.User) error {
	s.baseService.SetUpdatedTimestamp(&user.UpdatedAt)
	if err := s.userRepo.UpdateUser(user); err != nil {
		return s.baseService.WrapErrorWithID(err, "update user", user.ID)
	}
	s.baseService.LogUpdated("user", user.ID)
	return nil
}

// DeleteUser 删除用户
func (s *userService) DeleteUser(userID string) error {
	// 删除用户
	if err := s.userRepo.DeleteUser(userID); err != nil {
		return s.baseService.WrapErrorWithID(err, "delete user", userID)
	}

	// 释放用户ID
	if err := s.idManager.ReleaseUserID(userID); err != nil {
		s.baseService.LogWarning("release user ID", err, userID)
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrUser(-1); err != nil {
			s.baseService.LogWarning("update user stats counter", err, userID)
		}
	}

	s.baseService.LogDeleted("user", userID)
	return nil
}

// ListUsers 列出用户
func (s *userService) ListUsers(userType models.UserType) ([]*models.User, error) {
	users, err := s.userRepo.ListUsers(userType)
	if err != nil {
		return nil, s.baseService.WrapError(err, fmt.Sprintf("list users by type %v", userType))
	}
	return users, nil
}

// SearchUsers 搜索用户
func (s *userService) SearchUsers(keyword string) ([]*models.User, error) {
	// 获取所有用户
	allUsers, err := s.userRepo.ListAllUsers()
	if err != nil {
		return nil, s.baseService.WrapError(err, "list all users for search")
	}

	// 如果关键词为空，返回空列表
	if keyword == "" {
		return []*models.User{}, nil
	}

	// 大小写不敏感搜索
	keyword = strings.ToLower(keyword)

	// 过滤匹配的用户
	matchedUsers := make([]*models.User, 0)
	for _, user := range allUsers {
		// 匹配用户名或邮箱（不区分大小写）
		if strings.Contains(strings.ToLower(user.Username), keyword) ||
			strings.Contains(strings.ToLower(user.Email), keyword) ||
			strings.Contains(strings.ToLower(user.ID), keyword) {
			matchedUsers = append(matchedUsers, user)
		}
	}

	return matchedUsers, nil
}

// GetUserStats 获取用户统计信息
func (s *userService) GetUserStats(userID string) (*stats.UserStats, error) {
	// 验证用户存在
	_, err := s.userRepo.GetUser(userID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get user for stats", userID)
	}

	// 返回基本统计信息
	// 注意：完整的统计信息（如 TotalClients、OnlineClients 等）需要通过 StatsManager 获取
	return &stats.UserStats{
		UserID: userID,
	}, nil
}
