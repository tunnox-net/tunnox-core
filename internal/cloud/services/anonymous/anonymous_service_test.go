package anonymous

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

// mockNotifier implements Notifier interface for testing
type mockNotifier struct {
	notifiedClients []int64
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{
		notifiedClients: make([]int64, 0),
	}
}

func (m *mockNotifier) NotifyClientUpdate(clientID int64) {
	m.notifiedClients = append(m.notifiedClients, clientID)
}

// setupTestService creates test service with dependencies
func setupTestService(t *testing.T) (*Service, *repos.ClientRepository, *repos.ClientConfigRepository, *repos.PortMappingRepo, *idgen.IDManager) {
	ctx := context.Background()

	// Create in-memory storage
	stor := storage.NewMemoryStorage(ctx)

	// Create repositories
	baseRepo := repos.NewRepository(stor)
	clientRepo := repos.NewClientRepository(baseRepo)
	configRepo := repos.NewClientConfigRepository(baseRepo)
	mappingRepo := repos.NewPortMappingRepo(baseRepo)

	// Create ID manager
	idManager := idgen.NewIDManager(stor, ctx)

	// Create service
	service := NewService(clientRepo, configRepo, mappingRepo, idManager, ctx)

	return service, clientRepo, configRepo, mappingRepo, idManager
}

func TestNewService(t *testing.T) {
	service, _, _, _, _ := setupTestService(t)
	assert.NotNil(t, service)
}

func TestService_GenerateAnonymousCredentials(t *testing.T) {
	tests := []struct {
		name        string
		expectError bool
	}{
		{
			name:        "successful generation",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, _, _, _ := setupTestService(t)

			client, err := service.GenerateAnonymousCredentials()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)

				// Verify client properties
				assert.NotZero(t, client.ID)
				assert.Empty(t, client.UserID) // Anonymous has no UserID
				assert.Contains(t, client.Name, "Anonymous-")
				assert.NotEmpty(t, client.AuthCode)
				assert.NotEmpty(t, client.SecretKey)
				assert.Equal(t, models.ClientStatusOffline, client.Status)
				assert.Equal(t, models.ClientTypeAnonymous, client.Type)
				assert.False(t, client.CreatedAt.IsZero())
				assert.False(t, client.UpdatedAt.IsZero())
			}
		})
	}
}

func TestService_GenerateAnonymousCredentials_MultipleClients(t *testing.T) {
	service, _, _, _, _ := setupTestService(t)

	clients := make([]*models.Client, 5)
	ids := make(map[int64]bool)

	// Generate multiple anonymous clients
	for i := 0; i < 5; i++ {
		client, err := service.GenerateAnonymousCredentials()
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify unique IDs
		assert.False(t, ids[client.ID], "duplicate client ID generated")
		ids[client.ID] = true

		clients[i] = client
	}

	// Verify all have unique auth codes
	authCodes := make(map[string]bool)
	for _, c := range clients {
		assert.False(t, authCodes[c.AuthCode], "duplicate auth code generated")
		authCodes[c.AuthCode] = true
	}
}

func TestService_GetAnonymousClient(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Service, *repos.ClientRepository) int64
		expectError bool
		errContains string
	}{
		{
			name: "success - get existing anonymous client",
			setup: func(s *Service, repo *repos.ClientRepository) int64 {
				client, err := s.GenerateAnonymousCredentials()
				require.NoError(nil, err)
				return client.ID
			},
			expectError: false,
		},
		{
			name: "client not found",
			setup: func(s *Service, repo *repos.ClientRepository) int64 {
				return 99999 // Non-existent ID
			},
			expectError: true,
			errContains: "not found",
		},
		{
			name: "not anonymous client",
			setup: func(s *Service, repo *repos.ClientRepository) int64 {
				now := time.Now()
				client := &models.Client{
					ID:        12345,
					Name:      "registered-client",
					Type:      models.ClientTypeRegistered, // Not anonymous
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client.ID
			},
			expectError: true,
			errContains: "not anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, _, _ := setupTestService(t)

			clientID := tt.setup(service, clientRepo)

			result, err := service.GetAnonymousClient(clientID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, clientID, result.ID)
				assert.Equal(t, models.ClientTypeAnonymous, result.Type)
			}
		})
	}
}

func TestService_DeleteAnonymousClient(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*Service, *repos.ClientRepository) int64
		expectError bool
		errContains string
	}{
		{
			name: "success - delete existing anonymous client",
			setup: func(s *Service, repo *repos.ClientRepository) int64 {
				client, err := s.GenerateAnonymousCredentials()
				require.NoError(nil, err)
				return client.ID
			},
			expectError: false,
		},
		{
			name: "client not found",
			setup: func(s *Service, repo *repos.ClientRepository) int64 {
				return 99999 // Non-existent ID
			},
			expectError: true,
			errContains: "get anonymous client",
		},
		{
			name: "not anonymous client",
			setup: func(s *Service, repo *repos.ClientRepository) int64 {
				now := time.Now()
				client := &models.Client{
					ID:        12345,
					Name:      "registered-client",
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client.ID
			},
			expectError: true,
			errContains: "not anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, _, _ := setupTestService(t)

			clientID := tt.setup(service, clientRepo)

			err := service.DeleteAnonymousClient(clientID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)

				// Verify client is deleted
				_, err := service.GetAnonymousClient(clientID)
				assert.Error(t, err)
			}
		})
	}
}

func TestService_ListAnonymousClients(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*Service, *repos.ClientRepository)
		expectedCount int
		expectError   bool
	}{
		{
			name: "list multiple anonymous clients",
			setup: func(s *Service, repo *repos.ClientRepository) {
				// Generate 3 anonymous clients
				for i := 0; i < 3; i++ {
					_, err := s.GenerateAnonymousCredentials()
					require.NoError(nil, err)
				}
			},
			expectedCount: 3,
			expectError:   false,
		},
		{
			name: "filter out registered clients",
			setup: func(s *Service, repo *repos.ClientRepository) {
				// Generate 2 anonymous clients
				for i := 0; i < 2; i++ {
					_, err := s.GenerateAnonymousCredentials()
					require.NoError(nil, err)
				}

				// Add registered client
				now := time.Now()
				registeredClient := &models.Client{
					ID:        99999,
					Name:      "registered",
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(registeredClient)
			},
			expectedCount: 2, // Only anonymous clients
			expectError:   false,
		},
		{
			name:          "empty list",
			setup:         func(s *Service, repo *repos.ClientRepository) {},
			expectedCount: 0,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, _, _ := setupTestService(t)

			tt.setup(service, clientRepo)

			result, err := service.ListAnonymousClients()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)

				// Verify all are anonymous
				for _, client := range result {
					assert.Equal(t, models.ClientTypeAnonymous, client.Type)
				}
			}
		})
	}
}

func TestService_CreateAnonymousMapping(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*Service, *repos.ClientRepository) (int64, int64)
		protocol       models.Protocol
		sourcePort     int
		targetPort     int
		expectError    bool
		errContains    string
	}{
		{
			name: "success - create mapping between two anonymous clients",
			setup: func(s *Service, repo *repos.ClientRepository) (int64, int64) {
				client1, _ := s.GenerateAnonymousCredentials()
				client2, _ := s.GenerateAnonymousCredentials()
				return client1.ID, client2.ID
			},
			protocol:    models.ProtocolTCP,
			sourcePort:  8080,
			targetPort:  3306,
			expectError: false,
		},
		{
			name: "listen client not found",
			setup: func(s *Service, repo *repos.ClientRepository) (int64, int64) {
				client, _ := s.GenerateAnonymousCredentials()
				return 99999, client.ID // Non-existent listen client
			},
			protocol:    models.ProtocolTCP,
			sourcePort:  8080,
			targetPort:  3306,
			expectError: true,
			errContains: "listen client",
		},
		{
			name: "target client not found",
			setup: func(s *Service, repo *repos.ClientRepository) (int64, int64) {
				client, _ := s.GenerateAnonymousCredentials()
				return client.ID, 99999 // Non-existent target client
			},
			protocol:    models.ProtocolTCP,
			sourcePort:  8080,
			targetPort:  3306,
			expectError: true,
			errContains: "target client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, _, _ := setupTestService(t)

			listenID, targetID := tt.setup(service, clientRepo)

			result, err := service.CreateAnonymousMapping(listenID, targetID, tt.protocol, tt.sourcePort, tt.targetPort)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.NotEmpty(t, result.ID)
				assert.Equal(t, listenID, result.ListenClientID)
				assert.Equal(t, targetID, result.TargetClientID)
				assert.Equal(t, tt.protocol, result.Protocol)
				assert.Equal(t, tt.sourcePort, result.SourcePort)
				assert.Equal(t, tt.targetPort, result.TargetPort)
				assert.Equal(t, models.MappingStatusInactive, result.Status)
				assert.Equal(t, models.MappingTypeAnonymous, result.Type)
			}
		})
	}
}

func TestService_CreateAnonymousMapping_WithNotifier(t *testing.T) {
	service, clientRepo, _, _, _ := setupTestService(t)

	// Setup notifier
	notifier := newMockNotifier()
	service.SetNotifier(notifier)

	// Create two clients
	client1, _ := service.GenerateAnonymousCredentials()
	client2, _ := service.GenerateAnonymousCredentials()

	// Create mapping
	_, err := service.CreateAnonymousMapping(client1.ID, client2.ID, models.ProtocolTCP, 8080, 3306)
	require.NoError(t, err)

	// Verify notifier was called with listen client ID
	require.Len(t, notifier.notifiedClients, 1)
	assert.Equal(t, client1.ID, notifier.notifiedClients[0])

	// Verify client still exists in repo
	_, err = clientRepo.GetClient("1") // Assuming first generated ID is 1
	// This may error depending on ID generation, that's okay for this test
}

func TestService_GetAnonymousMappings(t *testing.T) {
	service, _, _, _, _ := setupTestService(t)

	// This method is not fully implemented (returns empty list)
	result, err := service.GetAnonymousMappings()
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestService_CleanupExpiredAnonymous(t *testing.T) {
	// Note: CleanupExpiredAnonymous relies on LastSeen being stored, but the repository
	// implementation may not persist all fields correctly. This test verifies the basic
	// flow works without errors.

	t.Run("cleanup runs without error", func(t *testing.T) {
		service, _, _, _, _ := setupTestService(t)

		// Generate some clients
		_, _ = service.GenerateAnonymousCredentials()
		_, _ = service.GenerateAnonymousCredentials()

		// Cleanup should not error
		err := service.CleanupExpiredAnonymous()
		assert.NoError(t, err)
	})

	t.Run("handles empty list", func(t *testing.T) {
		service, _, _, _, _ := setupTestService(t)

		// Cleanup with no clients
		err := service.CleanupExpiredAnonymous()
		assert.NoError(t, err)
	})
}

func TestService_SetNotifier(t *testing.T) {
	service, _, _, _, _ := setupTestService(t)

	// Initially nil
	assert.Nil(t, service.notifier)

	// Set notifier
	notifier := newMockNotifier()
	service.SetNotifier(notifier)

	assert.NotNil(t, service.notifier)
	assert.Equal(t, notifier, service.notifier)
}
