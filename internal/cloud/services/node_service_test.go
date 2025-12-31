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

// setupNodeServiceTest 创建测试所需的依赖
func setupNodeServiceTest(t *testing.T) (NodeService, *repos.NodeRepository, *idgen.IDManager, context.Context) {
	ctx := context.Background()
	stor := storage.NewMemoryStorage(ctx)
	repo := repos.NewRepository(stor)
	nodeRepo := repos.NewNodeRepository(repo)
	idManager := idgen.NewIDManager(stor, ctx)

	service := NewNodeService(nodeRepo, idManager, ctx)
	return service, nodeRepo, idManager, ctx
}

func TestNodeService_NodeRegister(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, nodeRepo *repos.NodeRepository) // 测试前设置
		request     *models.NodeRegisterRequest
		expectError bool
		validate    func(t *testing.T, resp *models.NodeRegisterResponse)
	}{
		{
			name: "新节点注册成功",
			request: &models.NodeRegisterRequest{
				Address: "192.168.1.100:8000",
				Version: "1.0.0",
				Meta:    map[string]string{"region": "us-west"},
			},
			expectError: false,
			validate: func(t *testing.T, resp *models.NodeRegisterResponse) {
				assert.True(t, resp.Success)
				assert.NotEmpty(t, resp.NodeID)
				assert.Equal(t, "Node registered successfully", resp.Message)
			},
		},
		{
			name: "已存在节点更新信息",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) {
				// 先创建一个节点
				existingNode := &models.Node{
					ID:        "existing-node-001",
					Name:      "Existing Node",
					Address:   "192.168.1.50:8000",
					Meta:      map[string]string{},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				err := nodeRepo.CreateNode(existingNode)
				require.NoError(t, err)
			},
			request: &models.NodeRegisterRequest{
				NodeID:  "existing-node-001",
				Address: "192.168.1.100:8000",
				Version: "1.0.0",
				Meta:    map[string]string{"region": "us-east"},
			},
			expectError: false,
			validate: func(t *testing.T, resp *models.NodeRegisterResponse) {
				assert.True(t, resp.Success)
				assert.Equal(t, "existing-node-001", resp.NodeID)
				assert.Equal(t, "Node updated successfully", resp.Message)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, nodeRepo, _, _ := setupNodeServiceTest(t)

			if tc.setupFunc != nil {
				tc.setupFunc(t, nodeRepo)
			}

			resp, err := service.NodeRegister(tc.request)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)

			if tc.validate != nil {
				tc.validate(t, resp)
			}
		})
	}
}

func TestNodeService_NodeUnregister(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, nodeRepo *repos.NodeRepository) string // 返回要注销的nodeID
		expectError bool
	}{
		{
			name: "注销存在的节点",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) string {
				node := &models.Node{
					ID:        "node-to-delete",
					Name:      "Node to Delete",
					Address:   "192.168.1.100:8000",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				err := nodeRepo.CreateNode(node)
				require.NoError(t, err)
				return "node-to-delete"
			},
			expectError: false,
		},
		{
			name: "注销不存在的节点",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) string {
				return "non-existent-node"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, nodeRepo, _, _ := setupNodeServiceTest(t)

			nodeID := tc.setupFunc(t, nodeRepo)
			req := &models.NodeUnregisterRequest{NodeID: nodeID}

			err := service.NodeUnregister(req)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// 验证节点已被删除
			_, err = nodeRepo.GetNode(nodeID)
			assert.Error(t, err)
		})
	}
}

func TestNodeService_NodeHeartbeat(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, nodeRepo *repos.NodeRepository) string
		newAddress  string
		expectError bool
	}{
		{
			name: "心跳更新成功",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) string {
				node := &models.Node{
					ID:        "heartbeat-node",
					Name:      "Heartbeat Node",
					Address:   "192.168.1.100:8000",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				err := nodeRepo.CreateNode(node)
				require.NoError(t, err)
				return "heartbeat-node"
			},
			newAddress:  "192.168.1.200:8000",
			expectError: false,
		},
		{
			name: "心跳节点不存在",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) string {
				return "non-existent-node"
			},
			newAddress:  "192.168.1.200:8000",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, nodeRepo, _, _ := setupNodeServiceTest(t)

			nodeID := tc.setupFunc(t, nodeRepo)
			req := &models.NodeHeartbeatRequest{
				NodeID:  nodeID,
				Address: tc.newAddress,
				Time:    time.Now(),
			}

			resp, err := service.NodeHeartbeat(req)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.True(t, resp.Success)

			// 验证地址已更新
			updatedNode, err := nodeRepo.GetNode(nodeID)
			require.NoError(t, err)
			assert.Equal(t, tc.newAddress, updatedNode.Address)
		})
	}
}

func TestNodeService_GetNodeServiceInfo(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, nodeRepo *repos.NodeRepository) string
		expectError bool
	}{
		{
			name: "获取存在节点信息",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) string {
				node := &models.Node{
					ID:        "info-node",
					Name:      "Info Node",
					Address:   "192.168.1.100:8000",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				err := nodeRepo.CreateNode(node)
				require.NoError(t, err)
				return "info-node"
			},
			expectError: false,
		},
		{
			name: "获取不存在节点信息",
			setupFunc: func(t *testing.T, nodeRepo *repos.NodeRepository) string {
				return "non-existent-node"
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, nodeRepo, _, _ := setupNodeServiceTest(t)

			nodeID := tc.setupFunc(t, nodeRepo)
			info, err := service.GetNodeServiceInfo(nodeID)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, nodeID, info.NodeID)
			assert.Equal(t, "192.168.1.100:8000", info.Address)
		})
	}
}

func TestNodeService_GetAllNodeServiceInfo(t *testing.T) {
	t.Run("获取所有节点信息", func(t *testing.T) {
		service, nodeRepo, _, _ := setupNodeServiceTest(t)

		// 创建多个节点
		nodes := []*models.Node{
			{
				ID:        "node-1",
				Name:      "Node 1",
				Address:   "192.168.1.1:8000",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        "node-2",
				Name:      "Node 2",
				Address:   "192.168.1.2:8000",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		for _, node := range nodes {
			err := nodeRepo.CreateNode(node)
			require.NoError(t, err)
			err = nodeRepo.AddNodeToList(node)
			require.NoError(t, err)
		}

		infos, err := service.GetAllNodeServiceInfo()
		require.NoError(t, err)
		assert.Len(t, infos, 2)

		// 验证返回的节点信息
		nodeIDs := make(map[string]bool)
		for _, info := range infos {
			nodeIDs[info.NodeID] = true
		}
		assert.True(t, nodeIDs["node-1"])
		assert.True(t, nodeIDs["node-2"])
	})

	t.Run("空节点列表", func(t *testing.T) {
		service, _, _, _ := setupNodeServiceTest(t)

		infos, err := service.GetAllNodeServiceInfo()
		require.NoError(t, err)
		assert.Empty(t, infos)
	})
}
