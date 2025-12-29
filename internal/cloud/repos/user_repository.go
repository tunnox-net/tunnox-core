package repos

import (
	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
)

// 编译时接口断言，确保 UserRepository 实现了 IUserRepository 接口
var _ IUserRepository = (*UserRepository)(nil)

// UserRepository 用户数据访问
type UserRepository struct {
	*GenericRepositoryImpl[*models.User]
}

// NewUserRepository 创建用户数据访问层
func NewUserRepository(repo *Repository) *UserRepository {
	genericRepo := NewGenericRepository[*models.User](repo, func(user *models.User) (string, error) {
		return user.ID, nil
	})
	return &UserRepository{GenericRepositoryImpl: genericRepo}
}

// SaveUser 保存用户（创建或更新）
func (r *UserRepository) SaveUser(user *models.User) error {
	if err := r.Save(user, constants.KeyPrefixUser, constants2.DefaultUserDataTTL); err != nil {
		return err
	}
	// 将用户添加到全局用户列表中
	return r.AddUserToList(user)
}

// CreateUser 创建新用户（仅创建，不允许覆盖）
func (r *UserRepository) CreateUser(user *models.User) error {
	if err := r.Create(user, constants.KeyPrefixUser, constants2.DefaultUserDataTTL); err != nil {
		return err
	}
	// 将用户添加到全局用户列表中
	return r.AddUserToList(user)
}

// UpdateUser 更新用户（仅更新，不允许创建）
func (r *UserRepository) UpdateUser(user *models.User) error {
	return r.Update(user, constants.KeyPrefixUser, constants2.DefaultUserDataTTL)
}

// GetUser 获取用户
func (r *UserRepository) GetUser(userID string) (*models.User, error) {
	return r.Get(userID, constants.KeyPrefixUser)
}

// DeleteUser 删除用户
func (r *UserRepository) DeleteUser(userID string) error {
	return r.Delete(userID, constants.KeyPrefixUser)
}

// ListUsers 列出用户
func (r *UserRepository) ListUsers(userType models.UserType) ([]*models.User, error) {
	users, err := r.List(constants.KeyPrefixUserList)
	if err != nil {
		return []*models.User{}, nil
	}

	// 过滤用户类型
	if userType != "" {
		var filteredUsers []*models.User
		for _, user := range users {
			if user.Type == userType {
				filteredUsers = append(filteredUsers, user)
			}
		}
		return filteredUsers, nil
	}

	return users, nil
}

// AddUserToList 添加用户到列表
func (r *UserRepository) AddUserToList(user *models.User) error {
	return r.AddToList(user, constants.KeyPrefixUserList)
}

// ListAllUsers 列出所有用户（不过滤类型）
func (r *UserRepository) ListAllUsers() ([]*models.User, error) {
	return r.List(constants.KeyPrefixUserList)
}
