package registry

import (
	"context"
	"testing"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage/memory"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	r := NewRegistry(c)

	assert.NotNil(t, r)
	assert.NotNil(t, r.container)
	assert.NotNil(t, r.baseService)
}

func TestRegistry_Container(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	r := NewRegistry(c)

	result := r.Container()
	assert.Equal(t, c, result)
}

func TestRegistry_WrapResolveError(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	r := NewRegistry(c)

	err := assert.AnError
	wrapped := r.wrapResolveError(err, "test_service")

	assert.Error(t, wrapped)
	assert.Contains(t, wrapped.Error(), "test_service")
}

func TestRegisterInfrastructureServices(t *testing.T) {
	tests := []struct {
		name        string
		config      *configs.ControlConfig
		makeStorage func(context.Context) *memory.Storage
		factories   *ManagerFactories
		expectError bool
	}{
		{
			name:   "success with all dependencies",
			config: &configs.ControlConfig{},
			makeStorage: func(ctx context.Context) *memory.Storage {
				return memory.New(ctx)
			},
			factories: &ManagerFactories{
				NewJWTProvider: func(config any, storage any, parentCtx context.Context) base.JWTProvider {
					return &mockJWTProvider{}
				},
			},
			expectError: false,
		},
		{
			name:        "nil storage",
			config:      &configs.ControlConfig{},
			makeStorage: nil,
			factories:   &ManagerFactories{},
			expectError: false, // No error on register, error on resolve
		},
		{
			name:   "nil config",
			config: nil,
			makeStorage: func(ctx context.Context) *memory.Storage {
				return memory.New(ctx)
			},
			factories:   &ManagerFactories{},
			expectError: false, // No error on register, error on resolve
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c := container.NewContainer(ctx)

			var stor *memory.Storage
			if tt.makeStorage != nil {
				stor = tt.makeStorage(ctx)
			}

			err := RegisterInfrastructureServices(c, tt.config, stor, tt.factories, ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRegisterInfrastructureServices_ResolveStorage(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Resolve storage
	storageInstance, err := c.Resolve("storage")
	require.NoError(t, err)
	assert.NotNil(t, storageInstance)
}

func TestRegisterInfrastructureServices_ResolveConfig(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{
		NodeID: "test-node",
	}

	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Resolve config
	configInstance, err := c.Resolve("config")
	require.NoError(t, err)
	assert.NotNil(t, configInstance)

	resolvedConfig, ok := configInstance.(*configs.ControlConfig)
	require.True(t, ok)
	assert.Equal(t, "test-node", resolvedConfig.NodeID)
}

func TestRegisterInfrastructureServices_ResolveIDManager(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Resolve ID manager
	idManagerInstance, err := c.Resolve("id_manager")
	require.NoError(t, err)
	assert.NotNil(t, idManagerInstance)

	idManager, ok := idManagerInstance.(*idgen.IDManager)
	require.True(t, ok)
	assert.NotNil(t, idManager)
}

func TestRegisterInfrastructureServices_ResolveRepositories(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Test all base repositories
	testCases := []struct {
		name     string
		expected interface{}
	}{
		{"repository", (*repos.Repository)(nil)},
		{"user_repository", (*repos.UserRepository)(nil)},
		{"client_repository", (*repos.ClientRepository)(nil)},
		{"mapping_repository", (*repos.PortMappingRepo)(nil)},
		{"node_repository", (*repos.NodeRepository)(nil)},
		{"connection_repository", (*repos.ConnectionRepo)(nil)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instance, err := c.Resolve(tc.name)
			require.NoError(t, err, "failed to resolve %s", tc.name)
			assert.NotNil(t, instance, "%s should not be nil", tc.name)
		})
	}
}

func TestRegisterInfrastructureServices_ResolveClientRepositories(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Test client-specific repositories
	testCases := []string{
		"client_config_repository",
		"client_state_repository",
		"client_token_repository",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			instance, err := c.Resolve(tc)
			require.NoError(t, err, "failed to resolve %s", tc)
			assert.NotNil(t, instance, "%s should not be nil", tc)
		})
	}
}

func TestRegisterInfrastructureServices_ResolveJWTManager(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	jwtProviderCalled := false
	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			jwtProviderCalled = true
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Resolve JWT manager
	jwtManagerInstance, err := c.Resolve("jwt_manager")
	require.NoError(t, err)
	assert.NotNil(t, jwtManagerInstance)
	assert.True(t, jwtProviderCalled)
}

func TestRegisterInfrastructureServices_ResolveStatsManager(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Resolve stats manager
	statsManagerInstance, err := c.Resolve("stats_manager")
	require.NoError(t, err)
	assert.NotNil(t, statsManagerInstance)

	// Should implement StatsProvider
	_, ok := statsManagerInstance.(base.StatsProvider)
	assert.True(t, ok)
}

func TestRegisterBusinessServices(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	// First register infrastructure
	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Then register business services with mock constructors
	constructors := &ServiceConstructors{
		NewUserService: func(userRepo *repos.UserRepository, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "user"}
		},
		NewClientService: func(configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo interface{}, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "client"}
		},
		NewPortMappingService: func(mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "mapping"}
		},
		NewNodeService: func(nodeRepo *repos.NodeRepository, idManager *idgen.IDManager, parentCtx context.Context) interface{} {
			return &mockService{name: "node"}
		},
		NewAuthService: func(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository, jwtProvider base.JWTProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "auth"}
		},
		NewAnonymousService: func(clientRepo *repos.ClientRepository, configRepo *repos.ClientConfigRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) interface{} {
			return &mockService{name: "anonymous"}
		},
		NewConnectionService: func(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) interface{} {
			return &mockService{name: "connection"}
		},
		NewStatsService: func(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, nodeRepo *repos.NodeRepository, parentCtx context.Context) interface{} {
			return &mockService{name: "stats"}
		},
	}

	err = RegisterBusinessServices(c, constructors, ctx)
	require.NoError(t, err)
}

func TestRegisterBusinessServices_ResolveServices(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	// Register infrastructure
	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Register business services
	constructors := createMockConstructors()
	err = RegisterBusinessServices(c, constructors, ctx)
	require.NoError(t, err)

	// Test resolving all business services
	testCases := []string{
		"user_service",
		"client_service",
		"mapping_service",
		"node_service",
		"auth_service",
		"anonymous_service",
		"connection_service",
		"stats_service",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			instance, err := c.Resolve(tc)
			require.NoError(t, err, "failed to resolve %s", tc)
			assert.NotNil(t, instance, "%s should not be nil", tc)

			// Verify it's the mock service
			mockSvc, ok := instance.(*mockService)
			require.True(t, ok, "%s should be mockService", tc)
			assert.NotEmpty(t, mockSvc.name)
		})
	}
}

func TestRegisterBusinessServices_MissingConstructor(t *testing.T) {
	ctx := context.Background()
	c := container.NewContainer(ctx)
	stor := memory.New(ctx)
	config := &configs.ControlConfig{}

	// Register infrastructure
	err := RegisterInfrastructureServices(c, config, stor, &ManagerFactories{
		NewJWTProvider: func(cfg any, storage any, parentCtx context.Context) base.JWTProvider {
			return &mockJWTProvider{}
		},
	}, ctx)
	require.NoError(t, err)

	// Register with nil constructor
	constructors := &ServiceConstructors{
		NewUserService: nil, // Missing constructor
	}

	err = RegisterBusinessServices(c, constructors, ctx)
	require.NoError(t, err) // No error on register

	// Error on resolve
	_, err = c.Resolve("user_service")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not provided")
}

// Mock types
type mockJWTProvider struct{}

func (m *mockJWTProvider) GenerateTokenPair(ctx context.Context, client interface{}) (base.JWTTokenResult, error) {
	return nil, nil
}
func (m *mockJWTProvider) ValidateAccessToken(ctx context.Context, token string) (base.JWTClaimsResult, error) {
	return nil, nil
}
func (m *mockJWTProvider) ValidateRefreshToken(ctx context.Context, refreshToken string) (base.RefreshTokenClaimsResult, error) {
	return nil, nil
}
func (m *mockJWTProvider) RefreshAccessToken(ctx context.Context, refreshToken string, client interface{}) (base.JWTTokenResult, error) {
	return nil, nil
}
func (m *mockJWTProvider) RevokeToken(ctx context.Context, tokenID string) error {
	return nil
}

type mockService struct {
	name string
}

func createMockConstructors() *ServiceConstructors {
	return &ServiceConstructors{
		NewUserService: func(userRepo *repos.UserRepository, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "user"}
		},
		NewClientService: func(configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo interface{}, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "client"}
		},
		NewPortMappingService: func(mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "mapping"}
		},
		NewNodeService: func(nodeRepo *repos.NodeRepository, idManager *idgen.IDManager, parentCtx context.Context) interface{} {
			return &mockService{name: "node"}
		},
		NewAuthService: func(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository, jwtProvider base.JWTProvider, parentCtx context.Context) interface{} {
			return &mockService{name: "auth"}
		},
		NewAnonymousService: func(clientRepo *repos.ClientRepository, configRepo *repos.ClientConfigRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) interface{} {
			return &mockService{name: "anonymous"}
		},
		NewConnectionService: func(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) interface{} {
			return &mockService{name: "connection"}
		},
		NewStatsService: func(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, nodeRepo *repos.NodeRepository, parentCtx context.Context) interface{} {
			return &mockService{name: "stats"}
		},
	}
}
