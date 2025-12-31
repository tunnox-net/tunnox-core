package auth

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJWTProvider implements base.JWTProvider for testing
type mockJWTProvider struct {
	onGenerateTokenPair   func(ctx context.Context, client interface{}) (base.JWTTokenResult, error)
	onValidateAccessToken func(ctx context.Context, token string) (base.JWTClaimsResult, error)
	onValidateRefreshToken func(ctx context.Context, refreshToken string) (base.RefreshTokenClaimsResult, error)
	onRefreshAccessToken  func(ctx context.Context, refreshToken string, client interface{}) (base.JWTTokenResult, error)
	onRevokeToken         func(ctx context.Context, tokenID string) error
}

func (m *mockJWTProvider) GenerateTokenPair(ctx context.Context, client interface{}) (base.JWTTokenResult, error) {
	if m.onGenerateTokenPair != nil {
		return m.onGenerateTokenPair(ctx, client)
	}
	return &mockJWTTokenResult{
		token:        "mock-access-token",
		refreshToken: "mock-refresh-token",
		expiresAt:    time.Now().Add(1 * time.Hour),
		clientId:     123,
		tokenID:      "mock-token-id",
	}, nil
}

func (m *mockJWTProvider) ValidateAccessToken(ctx context.Context, token string) (base.JWTClaimsResult, error) {
	if m.onValidateAccessToken != nil {
		return m.onValidateAccessToken(ctx, token)
	}
	return &mockJWTClaimsResult{
		clientID:   123,
		userID:     "user-1",
		clientType: "registered",
		nodeID:     "node-1",
	}, nil
}

func (m *mockJWTProvider) ValidateRefreshToken(ctx context.Context, refreshToken string) (base.RefreshTokenClaimsResult, error) {
	if m.onValidateRefreshToken != nil {
		return m.onValidateRefreshToken(ctx, refreshToken)
	}
	return &mockRefreshTokenClaimsResult{
		clientID: 123,
		tokenID:  "mock-token-id",
	}, nil
}

func (m *mockJWTProvider) RefreshAccessToken(ctx context.Context, refreshToken string, client interface{}) (base.JWTTokenResult, error) {
	if m.onRefreshAccessToken != nil {
		return m.onRefreshAccessToken(ctx, refreshToken, client)
	}
	return &mockJWTTokenResult{
		token:        "new-access-token",
		refreshToken: "new-refresh-token",
		expiresAt:    time.Now().Add(1 * time.Hour),
		clientId:     123,
		tokenID:      "new-token-id",
	}, nil
}

func (m *mockJWTProvider) RevokeToken(ctx context.Context, tokenID string) error {
	if m.onRevokeToken != nil {
		return m.onRevokeToken(ctx, tokenID)
	}
	return nil
}

// mockJWTTokenResult implements base.JWTTokenResult
type mockJWTTokenResult struct {
	token        string
	refreshToken string
	expiresAt    time.Time
	clientId     int64
	tokenID      string
}

func (m *mockJWTTokenResult) GetToken() string        { return m.token }
func (m *mockJWTTokenResult) GetRefreshToken() string { return m.refreshToken }
func (m *mockJWTTokenResult) GetExpiresAt() time.Time { return m.expiresAt }
func (m *mockJWTTokenResult) GetClientId() int64      { return m.clientId }
func (m *mockJWTTokenResult) GetTokenID() string      { return m.tokenID }

// mockJWTClaimsResult implements base.JWTClaimsResult
type mockJWTClaimsResult struct {
	clientID   int64
	userID     string
	clientType string
	nodeID     string
}

func (m *mockJWTClaimsResult) GetClientID() int64     { return m.clientID }
func (m *mockJWTClaimsResult) GetUserID() string      { return m.userID }
func (m *mockJWTClaimsResult) GetClientType() string  { return m.clientType }
func (m *mockJWTClaimsResult) GetNodeID() string      { return m.nodeID }

// mockRefreshTokenClaimsResult implements base.RefreshTokenClaimsResult
type mockRefreshTokenClaimsResult struct {
	clientID int64
	tokenID  string
}

func (m *mockRefreshTokenClaimsResult) GetClientID() int64 { return m.clientID }
func (m *mockRefreshTokenClaimsResult) GetTokenID() string { return m.tokenID }

// Test helpers
func setupTestService(t *testing.T) (*Service, *repos.ClientRepository, *repos.NodeRepository, *mockJWTProvider) {
	ctx := context.Background()

	// Create in-memory storage
	stor := storage.NewMemoryStorage(ctx)

	// Create repositories
	baseRepo := repos.NewRepository(stor)
	clientRepo := repos.NewClientRepository(baseRepo)
	nodeRepo := repos.NewNodeRepository(baseRepo)

	// Create mock JWT provider
	jwtProvider := &mockJWTProvider{}

	// Create service
	service := NewService(clientRepo, nodeRepo, jwtProvider, ctx)

	return service, clientRepo, nodeRepo, jwtProvider
}

func createTestClient(t *testing.T, clientRepo *repos.ClientRepository, id int64, authCode, secretKey string, clientType models.ClientType) *models.Client {
	now := time.Now()
	client := &models.Client{
		ID:        id,
		UserID:    "user-1",
		Name:      "test-client",
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    models.ClientStatusOffline,
		Type:      clientType,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := clientRepo.CreateClient(client)
	require.NoError(t, err)
	return client
}

func createTestNode(t *testing.T, nodeRepo *repos.NodeRepository, nodeID, address string) *models.Node {
	now := time.Now()
	node := &models.Node{
		ID:        nodeID,
		Name:      "test-node",
		Address:   address,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := nodeRepo.CreateNode(node)
	require.NoError(t, err)
	return node
}

func TestNewService(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	assert.NotNil(t, service)
}

func TestService_Authenticate(t *testing.T) {
	tests := []struct {
		name          string
		setupClient   func(*repos.ClientRepository) *models.Client
		setupNode     func(*repos.NodeRepository) *models.Node
		setupJWT      func(*mockJWTProvider)
		request       *models.AuthRequest
		expectSuccess bool
		expectMessage string
	}{
		{
			name: "successful authentication with auth code",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        123,
					UserID:    "user-1",
					Name:      "test-client",
					AuthCode:  "valid-auth-code",
					SecretKey: "valid-secret",
					Status:    models.ClientStatusOffline,
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {},
			request: &models.AuthRequest{
				ClientID:  123,
				AuthCode:  "valid-auth-code",
				SecretKey: "valid-secret",
				NodeID:    "",
				Version:   "1.0.0",
				IPAddress: "192.168.1.1",
			},
			expectSuccess: true,
			expectMessage: "Authentication successful",
		},
		{
			name: "anonymous client authentication with secret key",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        456,
					Name:      "anonymous-client",
					AuthCode:  "anon-auth-code",
					SecretKey: "anon-secret-key",
					Status:    models.ClientStatusOffline,
					Type:      models.ClientTypeAnonymous,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {},
			request: &models.AuthRequest{
				ClientID:  456,
				AuthCode:  "anon-secret-key", // Anonymous clients can use SecretKey as AuthCode
				SecretKey: "",
				NodeID:    "",
				Version:   "1.0.0",
				IPAddress: "192.168.1.2",
			},
			expectSuccess: true,
			expectMessage: "Authentication successful",
		},
		{
			name: "client not found",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {},
			request: &models.AuthRequest{
				ClientID: 999,
				AuthCode: "invalid",
			},
			expectSuccess: false,
			expectMessage: "Client not found",
		},
		{
			name: "invalid auth code",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        789,
					AuthCode:  "correct-code",
					SecretKey: "secret",
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {},
			request: &models.AuthRequest{
				ClientID: 789,
				AuthCode: "wrong-code",
			},
			expectSuccess: false,
			expectMessage: "Invalid auth code",
		},
		{
			name: "invalid secret key",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        100,
					AuthCode:  "auth-code",
					SecretKey: "correct-secret",
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {},
			request: &models.AuthRequest{
				ClientID:  100,
				AuthCode:  "auth-code",
				SecretKey: "wrong-secret",
			},
			expectSuccess: false,
			expectMessage: "Invalid secret key",
		},
		{
			name: "JWT generation failure",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        200,
					AuthCode:  "auth-code",
					SecretKey: "secret",
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onGenerateTokenPair = func(ctx context.Context, client interface{}) (base.JWTTokenResult, error) {
					return nil, assert.AnError
				}
			},
			request: &models.AuthRequest{
				ClientID:  200,
				AuthCode:  "auth-code",
				SecretKey: "",
			},
			expectSuccess: false,
			expectMessage: "Failed to generate token",
		},
		{
			name: "with node info",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        300,
					AuthCode:  "auth-code",
					SecretKey: "secret",
					Type:      models.ClientTypeRegistered,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupNode: func(repo *repos.NodeRepository) *models.Node {
				now := time.Now()
				node := &models.Node{
					ID:        "node-1",
					Name:      "test-node",
					Address:   "10.0.0.1:8000",
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateNode(node)
				return node
			},
			setupJWT: func(jp *mockJWTProvider) {},
			request: &models.AuthRequest{
				ClientID:  300,
				AuthCode:  "auth-code",
				NodeID:    "node-1",
				Version:   "2.0.0",
				IPAddress: "192.168.1.100",
			},
			expectSuccess: true,
			expectMessage: "Authentication successful",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, nodeRepo, jwtProvider := setupTestService(t)

			tt.setupClient(clientRepo)
			tt.setupNode(nodeRepo)
			tt.setupJWT(jwtProvider)

			response, err := service.Authenticate(tt.request)

			// Authenticate should not return error (errors are in response)
			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, tt.expectSuccess, response.Success)
			assert.Contains(t, response.Message, tt.expectMessage)

			if tt.expectSuccess {
				assert.NotEmpty(t, response.Token)
				assert.NotNil(t, response.Client)
			}
		})
	}
}

func TestService_ValidateToken(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		setupClient   func(*repos.ClientRepository) *models.Client
		setupJWT      func(*mockJWTProvider)
		expectSuccess bool
		expectMessage string
	}{
		{
			name:  "valid token",
			token: "valid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				expiresAt := now.Add(1 * time.Hour)
				client := &models.Client{
					ID:             123,
					JWTToken:       "valid-token",
					TokenExpiresAt: &expiresAt,
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:      func(jp *mockJWTProvider) {},
			expectSuccess: true,
			expectMessage: "Token valid",
		},
		{
			name:  "invalid token - JWT validation fails",
			token: "invalid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onValidateAccessToken = func(ctx context.Context, token string) (base.JWTClaimsResult, error) {
					return nil, assert.AnError
				}
			},
			expectSuccess: false,
			expectMessage: "Invalid token",
		},
		{
			name:  "client not found",
			token: "orphan-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT:      func(jp *mockJWTProvider) {},
			expectSuccess: false,
			expectMessage: "Client not found",
		},
		{
			name:  "token mismatch",
			token: "mismatched-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				expiresAt := now.Add(1 * time.Hour)
				client := &models.Client{
					ID:             123,
					JWTToken:       "different-token", // Different token stored
					TokenExpiresAt: &expiresAt,
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:      func(jp *mockJWTProvider) {},
			expectSuccess: false,
			expectMessage: "Token mismatch",
		},
		{
			name:  "token expired",
			token: "expired-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				expiredAt := now.Add(-1 * time.Hour) // Expired
				client := &models.Client{
					ID:             123,
					JWTToken:       "expired-token",
					TokenExpiresAt: &expiredAt,
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:      func(jp *mockJWTProvider) {},
			expectSuccess: false,
			expectMessage: "Token expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, jwtProvider := setupTestService(t)

			tt.setupClient(clientRepo)
			tt.setupJWT(jwtProvider)

			response, err := service.ValidateToken(tt.token)

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, tt.expectSuccess, response.Success)
			assert.Contains(t, response.Message, tt.expectMessage)
		})
	}
}

func TestService_GenerateJWTToken(t *testing.T) {
	tests := []struct {
		name        string
		clientID    int64
		setupClient func(*repos.ClientRepository) *models.Client
		setupJWT    func(*mockJWTProvider)
		expectError bool
	}{
		{
			name:     "success",
			clientID: 123,
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        123,
					Name:      "test-client",
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: false,
		},
		{
			name:     "client not found",
			clientID: 999,
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: true,
		},
		{
			name:     "JWT generation error",
			clientID: 456,
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        456,
					Name:      "test-client",
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onGenerateTokenPair = func(ctx context.Context, client interface{}) (base.JWTTokenResult, error) {
					return nil, assert.AnError
				}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, jwtProvider := setupTestService(t)

			tt.setupClient(clientRepo)
			tt.setupJWT(jwtProvider)

			result, err := service.GenerateJWTToken(tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.NotEmpty(t, result.AccessToken)
				assert.NotEmpty(t, result.RefreshToken)
				assert.Equal(t, "Bearer", result.TokenType)
			}
		})
	}
}

func TestService_RefreshJWTToken(t *testing.T) {
	tests := []struct {
		name         string
		refreshToken string
		setupClient  func(*repos.ClientRepository) *models.Client
		setupJWT     func(*mockJWTProvider)
		expectError  bool
	}{
		{
			name:         "success",
			refreshToken: "valid-refresh-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:           123,
					Name:         "test-client",
					RefreshToken: "valid-refresh-token",
					CreatedAt:    now,
					UpdatedAt:    now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: false,
		},
		{
			name:         "invalid refresh token",
			refreshToken: "invalid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onValidateRefreshToken = func(ctx context.Context, refreshToken string) (base.RefreshTokenClaimsResult, error) {
					return nil, assert.AnError
				}
			},
			expectError: true,
		},
		{
			name:         "client not found after token validation",
			refreshToken: "orphan-refresh-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil // Client doesn't exist
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: true,
		},
		{
			name:         "refresh error",
			refreshToken: "valid-but-fail-refresh",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        123,
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onRefreshAccessToken = func(ctx context.Context, refreshToken string, client interface{}) (base.JWTTokenResult, error) {
					return nil, assert.AnError
				}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, jwtProvider := setupTestService(t)

			tt.setupClient(clientRepo)
			tt.setupJWT(jwtProvider)

			result, err := service.RefreshJWTToken(tt.refreshToken)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.NotEmpty(t, result.AccessToken)
			}
		})
	}
}

func TestService_ValidateJWTToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		setupClient func(*repos.ClientRepository) *models.Client
		setupJWT    func(*mockJWTProvider)
		expectError bool
	}{
		{
			name:  "success",
			token: "valid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				expiresAt := now.Add(1 * time.Hour)
				client := &models.Client{
					ID:             123,
					JWTToken:       "valid-token",
					RefreshToken:   "refresh-token",
					TokenExpiresAt: &expiresAt,
					CreatedAt:      now,
					UpdatedAt:      now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: false,
		},
		{
			name:  "invalid token",
			token: "invalid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onValidateAccessToken = func(ctx context.Context, token string) (base.JWTClaimsResult, error) {
					return nil, assert.AnError
				}
			},
			expectError: true,
		},
		{
			name:  "client not found",
			token: "orphan-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, jwtProvider := setupTestService(t)

			tt.setupClient(clientRepo)
			tt.setupJWT(jwtProvider)

			result, err := service.ValidateJWTToken(tt.token)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.token, result.AccessToken)
				assert.Equal(t, "Bearer", result.TokenType)
			}
		})
	}
}

func TestService_RevokeJWTToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		setupClient func(*repos.ClientRepository) *models.Client
		setupJWT    func(*mockJWTProvider)
		expectError bool
	}{
		{
			name:  "success",
			token: "valid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        123,
					JWTToken:  "valid-token",
					TokenID:   "token-id-123",
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT:    func(jp *mockJWTProvider) {},
			expectError: false,
		},
		{
			name:  "invalid token",
			token: "invalid-token",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				return nil
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onValidateAccessToken = func(ctx context.Context, token string) (base.JWTClaimsResult, error) {
					return nil, assert.AnError
				}
			},
			expectError: true,
		},
		{
			name:  "revoke error",
			token: "valid-but-revoke-fails",
			setupClient: func(repo *repos.ClientRepository) *models.Client {
				now := time.Now()
				client := &models.Client{
					ID:        123,
					TokenID:   "token-id",
					CreatedAt: now,
					UpdatedAt: now,
				}
				_ = repo.CreateClient(client)
				return client
			},
			setupJWT: func(jp *mockJWTProvider) {
				jp.onRevokeToken = func(ctx context.Context, tokenID string) error {
					return assert.AnError
				}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, clientRepo, _, jwtProvider := setupTestService(t)

			tt.setupClient(clientRepo)
			tt.setupJWT(jwtProvider)

			err := service.RevokeJWTToken(tt.token)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestJWTTokenInfo(t *testing.T) {
	info := JWTTokenInfo{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		TokenType:    "Bearer",
		ClientID:     123,
	}

	assert.Equal(t, "access", info.AccessToken)
	assert.Equal(t, "refresh", info.RefreshToken)
	assert.Equal(t, "Bearer", info.TokenType)
	assert.Equal(t, int64(123), info.ClientID)
}
