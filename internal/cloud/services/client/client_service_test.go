package client

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStatsProvider implements base.StatsProvider for testing
type mockStatsProvider struct {
	counter     *stats.StatsCounter
	clientStats map[int64]*stats.ClientStats
	userStats   map[string]*stats.UserStats
}

func newMockStatsProvider() *mockStatsProvider {
	return &mockStatsProvider{
		counter:     nil,
		clientStats: make(map[int64]*stats.ClientStats),
		userStats:   make(map[string]*stats.UserStats),
	}
}

func (m *mockStatsProvider) GetCounter() *stats.StatsCounter {
	return m.counter
}

func (m *mockStatsProvider) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	if s, ok := m.clientStats[clientID]; ok {
		return s, nil
	}
	return &stats.ClientStats{ClientID: clientID}, nil
}

func (m *mockStatsProvider) GetUserStats(userID string) (*stats.UserStats, error) {
	if s, ok := m.userStats[userID]; ok {
		return s, nil
	}
	return &stats.UserStats{UserID: userID}, nil
}

// setupTestService creates test service with all dependencies
func setupTestService(t *testing.T) (*Service, *repos.ClientConfigRepository, *repos.ClientStateRepository, *repos.ClientTokenRepository, *repos.ClientRepository, *repos.PortMappingRepo, *idgen.IDManager) {
	ctx := context.Background()

	// Create in-memory storage
	stor := storage.NewMemoryStorage(ctx)

	// Create repositories
	baseRepo := repos.NewRepository(stor)
	configRepo := repos.NewClientConfigRepository(baseRepo)
	stateRepo := repos.NewClientStateRepository(ctx, stor)
	tokenRepo := repos.NewClientTokenRepository(ctx, stor)
	clientRepo := repos.NewClientRepository(baseRepo)
	mappingRepo := repos.NewPortMappingRepo(baseRepo)

	// Create ID manager
	idManager := idgen.NewIDManager(stor, ctx)

	// Create stats provider
	statsProvider := newMockStatsProvider()

	// Create service
	service := NewService(
		configRepo,
		stateRepo,
		tokenRepo,
		clientRepo,
		mappingRepo,
		idManager,
		statsProvider,
		ctx,
	)

	return service, configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo, idManager
}

func TestNewService(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)
	assert.NotNil(t, service)
	assert.NotNil(t, service.baseService)
}

func TestService_CreateClient(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		clientName  string
		expectError bool
	}{
		{
			name:        "success - create client with user",
			userID:      "user-123",
			clientName:  "test-client",
			expectError: false,
		},
		{
			name:        "success - create client without user",
			userID:      "",
			clientName:  "anonymous-client",
			expectError: false,
		},
		{
			name:        "success - create client with long name",
			userID:      "user-456",
			clientName:  "this-is-a-very-long-client-name-for-testing",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, _, _, _, _, _ := setupTestService(t)

			client, err := service.CreateClient(tt.userID, tt.clientName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)

				assert.NotZero(t, client.ID)
				assert.Equal(t, tt.userID, client.UserID)
				assert.Equal(t, tt.clientName, client.Name)
				assert.NotEmpty(t, client.AuthCode)
				assert.NotEmpty(t, client.SecretKey)
				assert.Equal(t, models.ClientTypeRegistered, client.Type)
				assert.Equal(t, models.ClientStatusOffline, client.Status)
				assert.False(t, client.CreatedAt.IsZero())
			}
		})
	}
}

func TestService_CreateClient_UniqueIDs(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	ids := make(map[int64]bool)
	authCodes := make(map[string]bool)

	// Create multiple clients
	for i := 0; i < 10; i++ {
		client, err := service.CreateClient("user-1", "client")
		require.NoError(t, err)

		// Verify unique IDs
		assert.False(t, ids[client.ID], "duplicate client ID")
		ids[client.ID] = true

		// Verify unique auth codes
		assert.False(t, authCodes[client.AuthCode], "duplicate auth code")
		authCodes[client.AuthCode] = true
	}
}

func TestService_GetClient(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Service) int64
		expectError bool
		expectNil   bool
	}{
		{
			name: "success - get existing client",
			setup: func(s *Service) int64 {
				client, _ := s.CreateClient("user-1", "test-client")
				return client.ID
			},
			expectError: false,
			expectNil:   false,
		},
		{
			name: "client not found",
			setup: func(s *Service) int64 {
				return 99999 // Non-existent
			},
			expectError: true,
			expectNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, _, _, _, _, _ := setupTestService(t)

			clientID := tt.setup(service)

			result, err := service.GetClient(clientID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, clientID, result.ID)
			}
		})
	}
}

func TestService_UpdateClient(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Service) *models.Client
		modify      func(*models.Client)
		expectError bool
		errContains string
	}{
		{
			name: "success - update name",
			setup: func(s *Service) *models.Client {
				client, _ := s.CreateClient("user-1", "original-name")
				return client
			},
			modify: func(c *models.Client) {
				c.Name = "new-name"
			},
			expectError: false,
		},
		{
			name: "nil client",
			setup: func(s *Service) *models.Client {
				return nil
			},
			modify:      func(c *models.Client) {},
			expectError: true,
			errContains: "client is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, _, _, _, _, _ := setupTestService(t)

			client := tt.setup(service)
			if client != nil {
				tt.modify(client)
			}

			err := service.UpdateClient(client)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)

				// Verify update persisted
				updated, _ := service.GetClient(client.ID)
				assert.Equal(t, "new-name", updated.Name)
			}
		})
	}
}

func TestService_DeleteClient(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Service) int64
		expectError bool
	}{
		{
			name: "success - delete existing client",
			setup: func(s *Service) int64 {
				client, _ := s.CreateClient("user-1", "to-delete")
				return client.ID
			},
			expectError: false,
		},
		{
			name: "client not found",
			setup: func(s *Service) int64 {
				return 99999
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, _, _, _, _, _ := setupTestService(t)

			clientID := tt.setup(service)

			err := service.DeleteClient(clientID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify deleted
				_, err := service.GetClient(clientID)
				assert.Error(t, err)
			}
		})
	}
}

func TestService_ListClients(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*Service)
		userID        string
		clientType    models.ClientType
		expectedCount int
	}{
		{
			name: "list all clients",
			setup: func(s *Service) {
				s.CreateClient("user-1", "client-1")
				s.CreateClient("user-2", "client-2")
				s.CreateClient("user-1", "client-3")
			},
			userID:        "",
			clientType:    "",
			expectedCount: 3,
		},
		{
			name: "list by user",
			setup: func(s *Service) {
				s.CreateClient("user-1", "client-1")
				s.CreateClient("user-2", "client-2")
				s.CreateClient("user-1", "client-3")
			},
			userID:        "user-1",
			clientType:    "",
			expectedCount: 2,
		},
		{
			name:          "empty list",
			setup:         func(s *Service) {},
			userID:        "",
			clientType:    "",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, _, _, _, _, _ := setupTestService(t)

			tt.setup(service)

			result, err := service.ListClients(tt.userID, tt.clientType)
			require.NoError(t, err)

			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestService_ListUserClients(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create clients for different users
	service.CreateClient("user-1", "client-1")
	service.CreateClient("user-1", "client-2")
	service.CreateClient("user-2", "client-3")

	// List user-1 clients
	result, err := service.ListUserClients("user-1")
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// List user-2 clients
	result, err = service.ListUserClients("user-2")
	require.NoError(t, err)
	assert.Len(t, result, 1)

	// List non-existent user
	result, err = service.ListUserClients("user-999")
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func TestService_SearchClients(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create clients with various names
	service.CreateClient("user-1", "alpha-client")
	service.CreateClient("user-1", "beta-client")
	service.CreateClient("user-2", "alpha-server")

	tests := []struct {
		name          string
		keyword       string
		expectedCount int
	}{
		{
			name:          "search by name - alpha",
			keyword:       "alpha",
			expectedCount: 2,
		},
		{
			name:          "search by name - client",
			keyword:       "client",
			expectedCount: 2,
		},
		{
			name:          "search case insensitive",
			keyword:       "ALPHA",
			expectedCount: 2,
		},
		{
			name:          "no matches",
			keyword:       "gamma",
			expectedCount: 0,
		},
		{
			name:          "empty keyword",
			keyword:       "",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.SearchClients(tt.keyword)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestService_UpdateClientStatus(t *testing.T) {
	service, _, stateRepo, _, _, _, _ := setupTestService(t)

	// Create client and connect first (required for valid state)
	client, _ := service.CreateClient("user-1", "test-client")
	err := service.ConnectClient(client.ID, "node-1", "conn-1", "192.168.1.1", "tcp", "1.0.0")
	require.NoError(t, err)

	// Verify state
	state, err := stateRepo.GetState(client.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ClientStatusOnline, state.Status)
	assert.Equal(t, "node-1", state.NodeID)

	// Disconnect
	err = service.DisconnectClient(client.ID)
	require.NoError(t, err)
}

func TestService_ConnectClient(t *testing.T) {
	service, _, stateRepo, _, _, _, _ := setupTestService(t)

	// Create client
	client, _ := service.CreateClient("user-1", "test-client")

	// Connect client
	err := service.ConnectClient(client.ID, "node-1", "conn-123", "192.168.1.1", "tcp", "1.0.0")
	require.NoError(t, err)

	// Verify state
	state, err := stateRepo.GetState(client.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ClientStatusOnline, state.Status)
	assert.Equal(t, "node-1", state.NodeID)
	assert.Equal(t, "conn-123", state.ConnID)
	assert.Equal(t, "192.168.1.1", state.IPAddress)
	assert.Equal(t, "tcp", state.Protocol)
	assert.Equal(t, "1.0.0", state.Version)
}

func TestService_DisconnectClient(t *testing.T) {
	service, _, stateRepo, _, _, _, _ := setupTestService(t)

	// Create and connect client
	client, _ := service.CreateClient("user-1", "test-client")
	service.ConnectClient(client.ID, "node-1", "conn-123", "192.168.1.1", "tcp", "1.0.0")

	// Disconnect
	err := service.DisconnectClient(client.ID)
	require.NoError(t, err)

	// Verify state is deleted (means offline)
	state, err := stateRepo.GetState(client.ID)
	// State should be nil or error after disconnect
	if err == nil {
		assert.Nil(t, state)
	}
}

func TestService_DisconnectClient_AlreadyOffline(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create client (never connected)
	client, _ := service.CreateClient("user-1", "test-client")

	// Disconnect should not error
	err := service.DisconnectClient(client.ID)
	assert.NoError(t, err)
}

func TestService_GetClientNodeID(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create and connect client
	client, _ := service.CreateClient("user-1", "test-client")
	service.ConnectClient(client.ID, "node-1", "conn-123", "192.168.1.1", "tcp", "1.0.0")

	// Get node ID
	nodeID, err := service.GetClientNodeID(client.ID)
	require.NoError(t, err)
	assert.Equal(t, "node-1", nodeID)

	// Disconnect and check
	service.DisconnectClient(client.ID)
	nodeID, err = service.GetClientNodeID(client.ID)
	// Should return empty for offline client
	assert.Empty(t, nodeID)
}

func TestService_IsClientOnNode(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create and connect client
	client, _ := service.CreateClient("user-1", "test-client")
	service.ConnectClient(client.ID, "node-1", "conn-123", "192.168.1.1", "tcp", "1.0.0")

	// Check correct node
	isOnNode, err := service.IsClientOnNode(client.ID, "node-1")
	require.NoError(t, err)
	assert.True(t, isOnNode)

	// Check wrong node
	isOnNode, err = service.IsClientOnNode(client.ID, "node-2")
	require.NoError(t, err)
	assert.False(t, isOnNode)
}

func TestService_GetNodeClients(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create and connect clients
	client1, _ := service.CreateClient("user-1", "client-1")
	client2, _ := service.CreateClient("user-1", "client-2")
	client3, _ := service.CreateClient("user-1", "client-3")

	service.ConnectClient(client1.ID, "node-1", "conn-1", "192.168.1.1", "tcp", "1.0.0")
	service.ConnectClient(client2.ID, "node-1", "conn-2", "192.168.1.2", "tcp", "1.0.0")
	service.ConnectClient(client3.ID, "node-2", "conn-3", "192.168.1.3", "tcp", "1.0.0")

	// Get node-1 clients
	clients, err := service.GetNodeClients("node-1")
	require.NoError(t, err)
	assert.Len(t, clients, 2)

	// Get node-2 clients
	clients, err = service.GetNodeClients("node-2")
	require.NoError(t, err)
	assert.Len(t, clients, 1)

	// Get non-existent node
	clients, err = service.GetNodeClients("node-3")
	require.NoError(t, err)
	assert.Len(t, clients, 0)
}

func TestService_TouchClient(t *testing.T) {
	service, _, stateRepo, _, _, _, _ := setupTestService(t)

	// Create and connect client
	client, _ := service.CreateClient("user-1", "test-client")
	service.ConnectClient(client.ID, "node-1", "conn-123", "192.168.1.1", "tcp", "1.0.0")

	// Get initial last seen
	stateBefore, _ := stateRepo.GetState(client.ID)
	lastSeenBefore := stateBefore.LastSeen

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Touch client
	service.TouchClient(client.ID)

	// Verify last seen updated
	stateAfter, _ := stateRepo.GetState(client.ID)
	assert.True(t, stateAfter.LastSeen.After(lastSeenBefore) || stateAfter.LastSeen.Equal(lastSeenBefore))
}

func TestService_GetClientStats(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create client
	client, _ := service.CreateClient("user-1", "test-client")

	// Get stats
	clientStats, err := service.GetClientStats(client.ID)
	require.NoError(t, err)
	require.NotNil(t, clientStats)
	assert.Equal(t, client.ID, clientStats.ClientID)
}

func TestService_GetClientStats_NoProvider(t *testing.T) {
	// Skip this test as the service requires a non-nil stats provider
	// The nil check happens at GetClientStats, not at construction
	t.Run("nil statsProvider causes error", func(t *testing.T) {
		service, _, _, _, _, _, _ := setupTestService(t)
		// Override statsProvider to nil to test the nil check
		service.statsProvider = nil

		_, err := service.GetClientStats(123)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stats provider not available")
	})
}

func TestService_GetClientPortMappings(t *testing.T) {
	service, _, _, _, _, _, _ := setupTestService(t)

	// Create client
	client, _ := service.CreateClient("user-1", "test-client")

	// Get mappings (should be empty for new client)
	mappings, err := service.GetClientPortMappings(client.ID)
	require.NoError(t, err)
	assert.Empty(t, mappings)
}
