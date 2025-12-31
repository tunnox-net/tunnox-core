package services

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupStatsServiceTest 创建统计服务测试所需的依赖
func setupStatsServiceTest(t *testing.T) (StatsService, *repos.UserRepository, *repos.ClientRepository, *repos.PortMappingRepo, *repos.NodeRepository, context.Context) {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := repos.NewRepository(stor)

	userRepo := repos.NewUserRepository(repo)
	clientRepo := repos.NewClientRepository(repo)
	mappingRepo := repos.NewPortMappingRepo(repo)
	nodeRepo := repos.NewNodeRepository(repo)

	service := NewstatsService(userRepo, clientRepo, mappingRepo, nodeRepo, ctx)
	return service, userRepo, clientRepo, mappingRepo, nodeRepo, ctx
}

func TestStatsService_GetSystemStats(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository)
		expectError bool
		validate    func(t *testing.T, stats interface{})
	}{
		{
			name:        "空系统统计",
			setupFunc:   nil,
			expectError: false,
			validate: func(t *testing.T, s interface{}) {
				ss := s.(*struct {
					TotalUsers    int
					TotalClients  int
					TotalMappings int
					TotalNodes    int
				})
				assert.Equal(t, 0, ss.TotalUsers)
				assert.Equal(t, 0, ss.TotalClients)
				assert.Equal(t, 0, ss.TotalMappings)
				assert.Equal(t, 0, ss.TotalNodes)
			},
		},
		{
			name: "有数据的系统统计",
			setupFunc: func(t *testing.T, clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository) {
				// 创建客户端（CreateClient 内部已调用 AddClientToList）
				for i := 0; i < 3; i++ {
					client := &models.Client{
						ID:        int64(12345678 + i),
						Name:      "TestClient",
						UserID:    "user-001",
						Type:      models.ClientTypeRegistered,
						Status:    models.ClientStatusOnline,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}
					err := clientRepo.CreateClient(client)
					require.NoError(t, err)
					// CreateClient 内部已调用 AddClientToList，无需重复调用
				}

				// 创建节点（CreateNode 内部不调用 AddNodeToList，需手动调用）
				for i := 0; i < 2; i++ {
					node := &models.Node{
						ID:        "node-" + string(rune('A'+i)),
						Name:      "TestNode",
						Address:   "192.168.1.100:8000",
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}
					err := nodeRepo.CreateNode(node)
					require.NoError(t, err)
					err = nodeRepo.AddNodeToList(node)
					require.NoError(t, err)
				}
			},
			expectError: false,
			validate: func(t *testing.T, s interface{}) {
				ss := s.(*struct {
					TotalUsers    int
					TotalClients  int
					TotalMappings int
					TotalNodes    int
				})
				assert.Equal(t, 0, ss.TotalUsers) // 用户数暂时为0
				assert.GreaterOrEqual(t, ss.TotalClients, 3) // 至少3个客户端
				assert.Equal(t, 0, ss.TotalMappings)         // 映射数暂时为0
				assert.Equal(t, 2, ss.TotalNodes)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, clientRepo, _, nodeRepo, _ := setupStatsServiceTest(t)

			if tc.setupFunc != nil {
				tc.setupFunc(t, clientRepo, nodeRepo)
			}

			stats, err := service.GetSystemStats()

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, stats)

			if tc.validate != nil {
				// 使用包装结构来验证
				wrapper := &struct {
					TotalUsers    int
					TotalClients  int
					TotalMappings int
					TotalNodes    int
				}{
					TotalUsers:    stats.TotalUsers,
					TotalClients:  stats.TotalClients,
					TotalMappings: stats.TotalMappings,
					TotalNodes:    stats.TotalNodes,
				}
				tc.validate(t, wrapper)
			}
		})
	}
}

func TestStatsService_GetTrafficStats(t *testing.T) {
	tests := []struct {
		name        string
		timeRange   string
		expectError bool
	}{
		{
			name:        "获取小时流量统计",
			timeRange:   "hour",
			expectError: false,
		},
		{
			name:        "获取天流量统计",
			timeRange:   "day",
			expectError: false,
		},
		{
			name:        "获取周流量统计",
			timeRange:   "week",
			expectError: false,
		},
		{
			name:        "获取月流量统计",
			timeRange:   "month",
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, _, _, _ := setupStatsServiceTest(t)

			trafficStats, err := service.GetTrafficStats(tc.timeRange)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			// 当前实现返回空数组
			assert.Empty(t, trafficStats)
		})
	}
}

func TestStatsService_GetConnectionStats(t *testing.T) {
	tests := []struct {
		name        string
		timeRange   string
		expectError bool
	}{
		{
			name:        "获取小时连接统计",
			timeRange:   "hour",
			expectError: false,
		},
		{
			name:        "获取天连接统计",
			timeRange:   "day",
			expectError: false,
		},
		{
			name:        "获取周连接统计",
			timeRange:   "week",
			expectError: false,
		},
		{
			name:        "获取月连接统计",
			timeRange:   "month",
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, _, _, _ := setupStatsServiceTest(t)

			connectionStats, err := service.GetConnectionStats(tc.timeRange)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			// 当前实现返回空数组
			assert.Empty(t, connectionStats)
		})
	}
}
