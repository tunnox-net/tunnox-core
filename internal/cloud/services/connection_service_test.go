package services

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupConnectionServiceTest 创建连接服务测试所需的依赖
func setupConnectionServiceTest(t *testing.T) (ConnectionService, *repos.ConnectionRepo, context.Context) {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := repos.NewRepository(stor)
	connRepo := repos.NewConnectionRepo(ctx, repo)
	idManager := idgen.NewIDManager(stor, ctx)

	service := NewConnectionService(connRepo, idManager, ctx)
	return service, connRepo, ctx
}

func TestConnectionService_RegisterConnection(t *testing.T) {
	tests := []struct {
		name        string
		mappingID   string
		connInfo    *models.ConnectionInfo
		expectError bool
		validate    func(t *testing.T, connRepo *repos.ConnectionRepo, connInfo *models.ConnectionInfo)
	}{
		{
			name:      "注册新连接",
			mappingID: "mapping-001",
			connInfo: &models.ConnectionInfo{
				ClientID: 12345678,
				SourceIP: "192.168.1.100",
				Status:   "",
			},
			expectError: false,
			validate: func(t *testing.T, connRepo *repos.ConnectionRepo, connInfo *models.ConnectionInfo) {
				assert.NotEmpty(t, connInfo.ConnID)
				assert.Equal(t, "mapping-001", connInfo.MappingID)
				assert.Equal(t, int64(12345678), connInfo.ClientID)
				assert.Equal(t, "active", connInfo.Status)
				assert.NotZero(t, connInfo.EstablishedAt)
			},
		},
		{
			name:      "注册多个连接到同一映射",
			mappingID: "mapping-002",
			connInfo: &models.ConnectionInfo{
				ClientID: 87654321,
				SourceIP: "192.168.1.200",
			},
			expectError: false,
			validate: func(t *testing.T, connRepo *repos.ConnectionRepo, connInfo *models.ConnectionInfo) {
				assert.NotEmpty(t, connInfo.ConnID)
				assert.Equal(t, "mapping-002", connInfo.MappingID)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, connRepo, _ := setupConnectionServiceTest(t)

			err := service.RegisterConnection(tc.mappingID, tc.connInfo)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.validate != nil {
				tc.validate(t, connRepo, tc.connInfo)
			}
		})
	}
}

func TestConnectionService_UnregisterConnection(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, service ConnectionService) string // 返回 connID
		expectError bool
	}{
		{
			name: "注销存在的连接",
			setupFunc: func(t *testing.T, service ConnectionService) string {
				connInfo := &models.ConnectionInfo{
					ClientID: 12345678,
					SourceIP: "192.168.1.100",
				}
				err := service.RegisterConnection("mapping-001", connInfo)
				require.NoError(t, err)
				return connInfo.ConnID
			},
			expectError: false,
		},
		{
			name: "注销不存在的连接",
			setupFunc: func(t *testing.T, service ConnectionService) string {
				return "non-existent-conn"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, connRepo, _ := setupConnectionServiceTest(t)

			connID := tc.setupFunc(t, service)
			err := service.UnregisterConnection(connID)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// 验证连接已被删除
			_, err = connRepo.GetConnection(connID)
			assert.Error(t, err)
		})
	}
}

func TestConnectionService_GetConnections(t *testing.T) {
	t.Run("获取映射的连接列表", func(t *testing.T) {
		service, _, _ := setupConnectionServiceTest(t)

		mappingID := "test-mapping"

		// 注册多个连接
		for i := 0; i < 3; i++ {
			connInfo := &models.ConnectionInfo{
				ClientID: int64(12345678 + i),
				SourceIP: "192.168.1.100",
			}
			err := service.RegisterConnection(mappingID, connInfo)
			require.NoError(t, err)
		}

		// 获取连接列表
		connections, err := service.GetConnections(mappingID)
		require.NoError(t, err)
		assert.Len(t, connections, 3)
	})

	t.Run("获取空映射的连接列表", func(t *testing.T) {
		service, _, _ := setupConnectionServiceTest(t)

		connections, err := service.GetConnections("empty-mapping")
		require.NoError(t, err)
		assert.Empty(t, connections)
	})
}

func TestConnectionService_GetClientConnections(t *testing.T) {
	t.Run("获取客户端的连接列表", func(t *testing.T) {
		service, _, _ := setupConnectionServiceTest(t)

		clientID := int64(12345678)

		// 注册多个连接到不同的映射
		for i := 0; i < 3; i++ {
			connInfo := &models.ConnectionInfo{
				ClientID: clientID,
				SourceIP: "192.168.1.100",
			}
			mappingID := "mapping-" + string(rune('A'+i))
			err := service.RegisterConnection(mappingID, connInfo)
			require.NoError(t, err)
		}

		// 获取客户端的连接列表
		connections, err := service.GetClientConnections(clientID)
		require.NoError(t, err)
		assert.Len(t, connections, 3)
	})

	t.Run("获取无连接客户端的连接列表", func(t *testing.T) {
		service, _, _ := setupConnectionServiceTest(t)

		connections, err := service.GetClientConnections(99999999)
		require.NoError(t, err)
		assert.Empty(t, connections)
	})
}

func TestConnectionService_UpdateConnectionStats(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T, service ConnectionService) string
		bytesSent     int64
		bytesReceived int64
		expectError   bool
	}{
		{
			name: "更新连接统计",
			setupFunc: func(t *testing.T, service ConnectionService) string {
				connInfo := &models.ConnectionInfo{
					ClientID: 12345678,
					SourceIP: "192.168.1.100",
				}
				err := service.RegisterConnection("mapping-001", connInfo)
				require.NoError(t, err)
				return connInfo.ConnID
			},
			bytesSent:     1024,
			bytesReceived: 2048,
			expectError:   false,
		},
		{
			name: "更新不存在连接的统计",
			setupFunc: func(t *testing.T, service ConnectionService) string {
				return "non-existent-conn"
			},
			bytesSent:     1024,
			bytesReceived: 2048,
			expectError:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, connRepo, _ := setupConnectionServiceTest(t)

			connID := tc.setupFunc(t, service)
			err := service.UpdateConnectionStats(connID, tc.bytesSent, tc.bytesReceived)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// 验证统计已更新
			conn, err := connRepo.GetConnection(connID)
			require.NoError(t, err)
			assert.Equal(t, tc.bytesSent, conn.BytesSent)
			assert.Equal(t, tc.bytesReceived, conn.BytesReceived)
			assert.True(t, time.Since(conn.LastActivity) < 5*time.Second)
		})
	}
}
