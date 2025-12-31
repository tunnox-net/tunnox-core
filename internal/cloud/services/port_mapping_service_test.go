package services

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupPortMappingServiceTest 创建端口映射服务测试所需的依赖
func setupPortMappingServiceTest(t *testing.T) (PortMappingService, *repos.PortMappingRepo, context.Context) {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := repos.NewRepository(stor)
	mappingRepo := repos.NewPortMappingRepo(repo)
	idManager := idgen.NewIDManager(stor, ctx)
	statsCounter, err := stats.NewStatsCounter(stor, ctx)
	require.NoError(t, err)

	service := NewPortMappingService(mappingRepo, idManager, statsCounter, ctx)
	return service, mappingRepo, ctx
}

func TestPortMappingService_CreatePortMapping(t *testing.T) {
	tests := []struct {
		name        string
		mapping     *models.PortMapping
		expectError bool
		validate    func(t *testing.T, created *models.PortMapping)
	}{
		{
			name: "创建TCP端口映射",
			mapping: &models.PortMapping{
				ListenClientID: 12345678,
				TargetClientID: 87654321,
				Protocol:       models.ProtocolTCP,
				SourcePort:     8080,
				TargetHost:     "127.0.0.1",
				TargetPort:     9090,
				UserID:         "user-001",
			},
			expectError: false,
			validate: func(t *testing.T, created *models.PortMapping) {
				assert.NotEmpty(t, created.ID)
				assert.Equal(t, models.ProtocolTCP, created.Protocol)
				assert.Equal(t, 8080, created.SourcePort)
				assert.Equal(t, 9090, created.TargetPort)
				assert.Equal(t, models.MappingStatusInactive, created.Status) // 默认状态
				assert.NotZero(t, created.CreatedAt)
				assert.NotZero(t, created.UpdatedAt)
			},
		},
		{
			name: "创建HTTP端口映射",
			mapping: &models.PortMapping{
				ListenClientID: 12345678,
				TargetClientID: 87654321,
				Protocol:       models.ProtocolHTTP,
				SourcePort:     80,
				TargetHost:     "localhost",
				TargetPort:     8080,
				HTTPSubdomain:  "myapp",
				HTTPBaseDomain: "tunnel.example.com",
			},
			expectError: false,
			validate: func(t *testing.T, created *models.PortMapping) {
				assert.Equal(t, models.ProtocolHTTP, created.Protocol)
				assert.Equal(t, "myapp", created.HTTPSubdomain)
				assert.Equal(t, "tunnel.example.com", created.HTTPBaseDomain)
			},
		},
		{
			name: "创建带自定义配置的映射",
			mapping: &models.PortMapping{
				ListenClientID: 12345678,
				TargetClientID: 87654321,
				Protocol:       models.ProtocolTCP,
				SourcePort:     3306,
				TargetHost:     "db.local",
				TargetPort:     3306,
				Config: configs.MappingConfig{
					EnableCompression: true,
					BandwidthLimit:    1024 * 1024,
					MaxConnections:    50,
					Timeout:           60,
					RetryCount:        5,
					EnableLogging:     true,
				},
			},
			expectError: false,
			validate: func(t *testing.T, created *models.PortMapping) {
				assert.True(t, created.Config.EnableCompression)
				assert.Equal(t, int64(1024*1024), created.Config.BandwidthLimit)
				assert.Equal(t, 50, created.Config.MaxConnections)
			},
		},
		{
			name: "创建带初始状态的映射",
			mapping: &models.PortMapping{
				ListenClientID: 12345678,
				TargetClientID: 87654321,
				Protocol:       models.ProtocolTCP,
				SourcePort:     22,
				TargetHost:     "ssh.local",
				TargetPort:     22,
				Status:         models.MappingStatusActive,
			},
			expectError: false,
			validate: func(t *testing.T, created *models.PortMapping) {
				assert.Equal(t, models.MappingStatusActive, created.Status)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupPortMappingServiceTest(t)

			created, err := service.CreatePortMapping(tc.mapping)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, created)

			if tc.validate != nil {
				tc.validate(t, created)
			}
		})
	}
}

func TestPortMappingService_GetPortMapping(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, service PortMappingService) string
		expectError bool
	}{
		{
			name: "获取存在的映射",
			setupFunc: func(t *testing.T, service PortMappingService) string {
				mapping := &models.PortMapping{
					ListenClientID: 12345678,
					TargetClientID: 87654321,
					Protocol:       models.ProtocolTCP,
					SourcePort:     8080,
					TargetHost:     "127.0.0.1",
					TargetPort:     9090,
				}
				created, err := service.CreatePortMapping(mapping)
				require.NoError(t, err)
				return created.ID
			},
			expectError: false,
		},
		{
			name: "获取不存在的映射",
			setupFunc: func(t *testing.T, service PortMappingService) string {
				return "non-existent-mapping"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupPortMappingServiceTest(t)

			mappingID := tc.setupFunc(t, service)
			mapping, err := service.GetPortMapping(mappingID)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, mapping)
			assert.Equal(t, mappingID, mapping.ID)
		})
	}
}

func TestPortMappingService_UpdatePortMapping(t *testing.T) {
	t.Run("更新端口映射", func(t *testing.T) {
		service, _, _ := setupPortMappingServiceTest(t)

		// 创建映射
		mapping := &models.PortMapping{
			ListenClientID: 12345678,
			TargetClientID: 87654321,
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080,
			TargetHost:     "127.0.0.1",
			TargetPort:     9090,
		}
		created, err := service.CreatePortMapping(mapping)
		require.NoError(t, err)

		// 更新映射
		created.SourcePort = 8081
		created.TargetPort = 9091
		created.Description = "Updated description"
		err = service.UpdatePortMapping(created)
		require.NoError(t, err)

		// 验证更新
		updated, err := service.GetPortMapping(created.ID)
		require.NoError(t, err)
		assert.Equal(t, 8081, updated.SourcePort)
		assert.Equal(t, 9091, updated.TargetPort)
		assert.Equal(t, "Updated description", updated.Description)
	})
}

func TestPortMappingService_DeletePortMapping(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, service PortMappingService) string
		expectError bool
	}{
		{
			name: "删除存在的映射",
			setupFunc: func(t *testing.T, service PortMappingService) string {
				mapping := &models.PortMapping{
					ListenClientID: 12345678,
					TargetClientID: 87654321,
					Protocol:       models.ProtocolTCP,
					SourcePort:     8080,
					TargetHost:     "127.0.0.1",
					TargetPort:     9090,
				}
				created, err := service.CreatePortMapping(mapping)
				require.NoError(t, err)
				return created.ID
			},
			expectError: false,
		},
		{
			name: "删除不存在的映射",
			setupFunc: func(t *testing.T, service PortMappingService) string {
				return "non-existent-mapping"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupPortMappingServiceTest(t)

			mappingID := tc.setupFunc(t, service)
			err := service.DeletePortMapping(mappingID)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// 验证映射已被删除
			_, err = service.GetPortMapping(mappingID)
			assert.Error(t, err)
		})
	}
}

func TestPortMappingService_UpdatePortMappingStatus(t *testing.T) {
	tests := []struct {
		name        string
		newStatus   models.MappingStatus
		expectError bool
	}{
		{
			name:        "设置为活跃状态",
			newStatus:   models.MappingStatusActive,
			expectError: false,
		},
		{
			name:        "设置为非活跃状态",
			newStatus:   models.MappingStatusInactive,
			expectError: false,
		},
		{
			name:        "设置为错误状态",
			newStatus:   models.MappingStatusError,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := setupPortMappingServiceTest(t)

			// 创建映射
			mapping := &models.PortMapping{
				ListenClientID: 12345678,
				TargetClientID: 87654321,
				Protocol:       models.ProtocolTCP,
				SourcePort:     8080,
				TargetHost:     "127.0.0.1",
				TargetPort:     9090,
			}
			created, err := service.CreatePortMapping(mapping)
			require.NoError(t, err)

			// 更新状态
			err = service.UpdatePortMappingStatus(created.ID, tc.newStatus)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// 验证状态更新
			updated, err := service.GetPortMapping(created.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.newStatus, updated.Status)
		})
	}
}

func TestPortMappingService_UpdatePortMappingStats(t *testing.T) {
	t.Run("更新流量统计", func(t *testing.T) {
		service, _, _ := setupPortMappingServiceTest(t)

		// 创建映射
		mapping := &models.PortMapping{
			ListenClientID: 12345678,
			TargetClientID: 87654321,
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080,
			TargetHost:     "127.0.0.1",
			TargetPort:     9090,
		}
		created, err := service.CreatePortMapping(mapping)
		require.NoError(t, err)

		// 更新统计
		trafficStats := &stats.TrafficStats{
			BytesSent:     1024 * 1024,
			BytesReceived: 2048 * 1024,
			Connections:   10,
			LastUpdated:   time.Now(),
		}
		err = service.UpdatePortMappingStats(created.ID, trafficStats)
		require.NoError(t, err)

		// 验证统计更新
		updated, err := service.GetPortMapping(created.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1024*1024), updated.TrafficStats.BytesSent)
		assert.Equal(t, int64(2048*1024), updated.TrafficStats.BytesReceived)
	})
}

func TestPortMappingService_GetUserPortMappings(t *testing.T) {
	t.Run("获取用户的端口映射", func(t *testing.T) {
		service, _, _ := setupPortMappingServiceTest(t)

		userID := "test-user"

		// 创建多个映射（CreatePortMapping 内部已调用 AddMappingToUser）
		for i := 0; i < 3; i++ {
			mapping := &models.PortMapping{
				ListenClientID: int64(12345678 + i),
				TargetClientID: int64(87654321 + i),
				Protocol:       models.ProtocolTCP,
				SourcePort:     8080 + i,
				TargetHost:     "127.0.0.1",
				TargetPort:     9090 + i,
				UserID:         userID,
			}
			_, err := service.CreatePortMapping(mapping)
			require.NoError(t, err)
		}

		// 获取用户的映射
		mappings, err := service.GetUserPortMappings(userID)
		require.NoError(t, err)
		assert.Len(t, mappings, 3)
	})

	t.Run("获取空用户的端口映射", func(t *testing.T) {
		service, _, _ := setupPortMappingServiceTest(t)

		mappings, err := service.GetUserPortMappings("empty-user")
		require.NoError(t, err)
		assert.Empty(t, mappings)
	})
}

func TestPortMappingService_GetPortMappingByDomain(t *testing.T) {
	t.Run("通过域名获取HTTP映射", func(t *testing.T) {
		service, mappingRepo, _ := setupPortMappingServiceTest(t)

		// 创建HTTP映射
		mapping := &models.PortMapping{
			ListenClientID: 12345678,
			TargetClientID: 87654321,
			Protocol:       models.ProtocolHTTP,
			SourcePort:     80,
			TargetHost:     "localhost",
			TargetPort:     8080,
			HTTPSubdomain:  "myapp",
			HTTPBaseDomain: "tunnel.example.com",
		}
		created, err := service.CreatePortMapping(mapping)
		require.NoError(t, err)

		// 添加到全局列表
		err = mappingRepo.AddMappingToList(created)
		require.NoError(t, err)

		// 通过域名查找
		found, err := service.GetPortMappingByDomain("myapp.tunnel.example.com")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, created.ID, found.ID)
	})

	t.Run("域名不存在", func(t *testing.T) {
		service, _, _ := setupPortMappingServiceTest(t)

		_, err := service.GetPortMappingByDomain("nonexistent.tunnel.example.com")
		assert.Error(t, err)
	})
}
