package services

import (
	"context"
	"testing"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupUserServiceTest 创建用户服务测试所需的依赖
func setupUserServiceTest(t *testing.T) (UserService, *repos.UserRepository, context.Context) {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := repos.NewRepository(stor)
	userRepo := repos.NewUserRepository(repo)
	idManager := idgen.NewIDManager(stor, ctx)
	statsCounter, err := stats.NewStatsCounter(stor, ctx)
	require.NoError(t, err)

	service := NewUserService(userRepo, idManager, statsCounter, ctx)
	return service, userRepo, ctx
}

func TestUserService_CreateUser(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		email       string
		expectError bool
		validate    func(t *testing.T, user *models.User)
	}{
		{
			name:        "创建普通用户",
			username:    "testuser",
			email:       "test@example.com",
			expectError: false,
			validate: func(t *testing.T, user *models.User) {
				assert.Equal(t, "testuser", user.Username)
				assert.Equal(t, "test@example.com", user.Email)
				assert.Equal(t, models.UserTypeRegistered, user.Type)
				assert.Equal(t, models.UserStatusActive, user.Status)
				assert.NotEmpty(t, user.ID)
				assert.NotZero(t, user.CreatedAt)
				assert.NotZero(t, user.UpdatedAt)
			},
		},
		{
			name:        "创建用户名为空",
			username:    "",
			email:       "empty@example.com",
			expectError: false, // 服务层不验证用户名
			validate: func(t *testing.T, user *models.User) {
				assert.Empty(t, user.Username)
				assert.Equal(t, "empty@example.com", user.Email)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupUserServiceTest(t)

			user, err := service.CreateUser(tc.username, tc.email)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, user)

			if tc.validate != nil {
				tc.validate(t, user)
			}
		})
	}
}

func TestUserService_GetUser(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, service UserService) string // 返回 userID
		expectError bool
	}{
		{
			name: "获取存在的用户",
			setupFunc: func(t *testing.T, service UserService) string {
				user, err := service.CreateUser("getuser", "get@example.com")
				require.NoError(t, err)
				return user.ID
			},
			expectError: false,
		},
		{
			name: "获取不存在的用户",
			setupFunc: func(t *testing.T, service UserService) string {
				return "non-existent-user"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupUserServiceTest(t)

			userID := tc.setupFunc(t, service)
			user, err := service.GetUser(userID)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, user)
			assert.Equal(t, userID, user.ID)
		})
	}
}

func TestUserService_UpdateUser(t *testing.T) {
	t.Run("更新用户信息", func(t *testing.T) {
		service, _, _ := setupUserServiceTest(t)

		// 创建用户
		user, err := service.CreateUser("updateuser", "update@example.com")
		require.NoError(t, err)

		// 更新用户
		user.Email = "updated@example.com"
		user.Status = models.UserStatusSuspended
		err = service.UpdateUser(user)
		require.NoError(t, err)

		// 验证更新
		updatedUser, err := service.GetUser(user.ID)
		require.NoError(t, err)
		assert.Equal(t, "updated@example.com", updatedUser.Email)
		assert.Equal(t, models.UserStatusSuspended, updatedUser.Status)
	})
}

func TestUserService_DeleteUser(t *testing.T) {
	t.Run("删除存在的用户", func(t *testing.T) {
		service, _, _ := setupUserServiceTest(t)

		user, err := service.CreateUser("deleteuser", "delete@example.com")
		require.NoError(t, err)

		err = service.DeleteUser(user.ID)
		require.NoError(t, err)

		// 验证用户已被删除
		_, err = service.GetUser(user.ID)
		assert.Error(t, err)
	})
}

func TestUserService_ListUsers(t *testing.T) {
	t.Run("按类型列出用户", func(t *testing.T) {
		service, _, _ := setupUserServiceTest(t)

		// 创建多个不同类型的用户
		// CreateUser 内部已调用 AddUserToList，无需重复调用
		_, err := service.CreateUser("user1", "user1@example.com")
		require.NoError(t, err)

		_, err = service.CreateUser("user2", "user2@example.com")
		require.NoError(t, err)

		// 列出所有注册用户
		users, err := service.ListUsers(models.UserTypeRegistered)
		require.NoError(t, err)
		assert.Len(t, users, 2)
	})

	t.Run("列出空类型用户", func(t *testing.T) {
		service, _, _ := setupUserServiceTest(t)

		users, err := service.ListUsers(models.UserTypeAnonymous)
		require.NoError(t, err)
		assert.Empty(t, users)
	})
}

func TestUserService_SearchUsers(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T, service UserService)
		keyword       string
		expectedCount int
	}{
		{
			name: "按用户名搜索",
			setupFunc: func(t *testing.T, service UserService) {
				// CreateUser 内部已调用 AddUserToList
				_, err := service.CreateUser("searchable", "search@example.com")
				require.NoError(t, err)
			},
			keyword:       "search",
			expectedCount: 1,
		},
		{
			name: "按邮箱搜索",
			setupFunc: func(t *testing.T, service UserService) {
				_, err := service.CreateUser("emailuser", "findme@domain.com")
				require.NoError(t, err)
			},
			keyword:       "findme",
			expectedCount: 1,
		},
		{
			name: "大小写不敏感搜索",
			setupFunc: func(t *testing.T, service UserService) {
				_, err := service.CreateUser("CamelCase", "camel@case.com")
				require.NoError(t, err)
			},
			keyword:       "camelcase",
			expectedCount: 1,
		},
		{
			name: "空关键词搜索",
			setupFunc: func(t *testing.T, service UserService) {
				_, err := service.CreateUser("anyuser", "any@example.com")
				require.NoError(t, err)
			},
			keyword:       "",
			expectedCount: 0,
		},
		{
			name: "无匹配结果",
			setupFunc: func(t *testing.T, service UserService) {
				_, err := service.CreateUser("nouser", "no@example.com")
				require.NoError(t, err)
			},
			keyword:       "nonexistent",
			expectedCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupUserServiceTest(t)

			tc.setupFunc(t, service)

			users, err := service.SearchUsers(tc.keyword)
			require.NoError(t, err)
			assert.Len(t, users, tc.expectedCount)
		})
	}
}

func TestUserService_GetUserStats(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, service UserService) string
		expectError bool
	}{
		{
			name: "获取存在用户的统计",
			setupFunc: func(t *testing.T, service UserService) string {
				user, err := service.CreateUser("statsuser", "stats@example.com")
				require.NoError(t, err)
				return user.ID
			},
			expectError: false,
		},
		{
			name: "获取不存在用户的统计",
			setupFunc: func(t *testing.T, service UserService) string {
				return "non-existent-user"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupUserServiceTest(t)

			userID := tc.setupFunc(t, service)
			userStats, err := service.GetUserStats(userID)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, userStats)
			assert.Equal(t, userID, userStats.UserID)
		})
	}
}
