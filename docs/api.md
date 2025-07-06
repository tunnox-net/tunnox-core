# API Documentation

## Overview

This document provides comprehensive API documentation for Tunnox Core, covering all public interfaces, data structures, and usage examples. The API is designed around the **Manager Pattern** with clear separation of concerns and consistent error handling.

## 🏗️ Core Interfaces

### CloudControlAPI

The main interface for cloud control operations, acting as a bus that delegates to specialized managers:

```go
type CloudControlAPI interface {
    // Node Management
    NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error)
    NodeUnregister(req *models.NodeUnregisterRequest) error
    NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error)

    // Authentication
    Authenticate(req *models.AuthRequest) (*models.AuthResponse, error)
    ValidateToken(token string) (*models.AuthResponse, error)

    // User Management
    CreateUser(username, email string) (*models.User, error)
    GetUser(userID string) (*models.User, error)
    UpdateUser(user *models.User) error
    DeleteUser(userID string) error
    ListUsers(userType models.UserType) ([]*models.User, error)

    // Client Management
    CreateClient(userID, clientName string) (*models.Client, error)
    GetClient(clientID int64) (*models.Client, error)
    TouchClient(clientID int64)
    UpdateClient(client *models.Client) error
    DeleteClient(clientID int64) error
    UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error
    ListClients(userID string, clientType models.ClientType) ([]*models.Client, error)
    ListUserClients(userID string) ([]*models.Client, error)
    GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)

    // Port Mapping Management
    CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error)
    GetUserPortMappings(userID string) ([]*models.PortMapping, error)
    GetPortMapping(mappingID string) (*models.PortMapping, error)
    UpdatePortMapping(mapping *models.PortMapping) error
    DeletePortMapping(mappingID string) error
    UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error
    UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error
    ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error)

    // Anonymous Management
    GenerateAnonymousCredentials() (*models.Client, error)
    GetAnonymousClient(clientID int64) (*models.Client, error)
    ListAnonymousClients() ([]*models.Client, error)
    DeleteAnonymousClient(clientID int64) error
    CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error)
    GetAnonymousMappings() ([]*models.PortMapping, error)
    CleanupExpiredAnonymous() error

    // Node Service Management
    GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error)
    GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error)

    // Statistics
    GetUserStats(userID string) (*stats.UserStats, error)
    GetClientStats(clientID int64) (*stats.ClientStats, error)
    GetSystemStats() (*stats.SystemStats, error)
    GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error)
    GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error)

    // Search
    SearchUsers(keyword string) ([]*models.User, error)
    SearchClients(keyword string) ([]*models.Client, error)
    SearchPortMappings(keyword string) ([]*models.PortMapping, error)

    // Connection Management
    RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error
    UnregisterConnection(connID string) error
    GetConnections(mappingID string) ([]*models.ConnectionInfo, error)
    GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error)
    UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error

    // JWT Token Management
    GenerateJWTToken(clientID int64) (*JWTTokenInfo, error)
    RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error)
    ValidateJWTToken(token string) (*JWTTokenInfo, error)
    RevokeJWTToken(token string) error

    // Resource Management
    Close() error
}
```

## 🔧 Manager APIs

### JWTManager

Handles JWT token generation, validation, and caching:

```go
type JWTManager struct {
    config *ControlConfig
    cache  *TokenCacheManager
    utils.Dispose
}

// Methods
func (m *JWTManager) GenerateTokenPair(ctx context.Context, client *models.Client) (*JWTTokenInfo, error)
func (m *JWTManager) ValidateAccessToken(ctx context.Context, tokenString string) (*JWTClaims, error)
func (m *JWTManager) ValidateRefreshToken(ctx context.Context, refreshTokenString string) (*RefreshTokenClaims, error)
func (m *JWTManager) RefreshAccessToken(ctx context.Context, refreshTokenString string, client *models.Client) (*JWTTokenInfo, error)
func (m *JWTManager) RevokeToken(ctx context.Context, tokenID string) error
```

### StatsManager

Provides comprehensive statistics and analytics:

```go
type StatsManager struct {
    userRepo    *repos.UserRepository
    clientRepo  *repos.ClientRepository
    mappingRepo *repos.PortMappingRepo
    nodeRepo    *repos.NodeRepository
    utils.Dispose
}

// Methods
func (sm *StatsManager) GetUserStats(userID string) (*stats.UserStats, error)
func (sm *StatsManager) GetClientStats(clientID int64) (*stats.ClientStats, error)
func (sm *StatsManager) GetSystemStats() (*stats.SystemStats, error)
func (sm *StatsManager) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error)
func (sm *StatsManager) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error)
```

### NodeManager

Manages node registration, health monitoring, and service discovery:

```go
type NodeManager struct {
    nodeRepo *repos.NodeRepository
    utils.Dispose
}

// Methods
func (nm *NodeManager) GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error)
func (nm *NodeManager) GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error)
```

### AnonymousManager

Handles anonymous user and temporary mapping management:

```go
type AnonymousManager struct {
    clientRepo  *repos.ClientRepository
    mappingRepo *repos.PortMappingRepo
    idGen       *distributed.DistributedIDGenerator
    utils.Dispose
}

// Methods
func (am *AnonymousManager) GenerateAnonymousCredentials() (*models.Client, error)
func (am *AnonymousManager) GetAnonymousClient(clientID int64) (*models.Client, error)
func (am *AnonymousManager) ListAnonymousClients() ([]*models.Client, error)
func (am *AnonymousManager) DeleteAnonymousClient(clientID int64) error
func (am *AnonymousManager) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error)
func (am *AnonymousManager) GetAnonymousMappings() ([]*models.PortMapping, error)
func (am *AnonymousManager) CleanupExpiredAnonymous() error
```

### SearchManager

Provides search capabilities across users, clients, and mappings:

```go
type SearchManager struct {
    userRepo    *repos.UserRepository
    clientRepo  *repos.ClientRepository
    mappingRepo *repos.PortMappingRepo
    utils.Dispose
}

// Methods
func (sm *SearchManager) SearchUsers(keyword string) ([]*models.User, error)
func (sm *SearchManager) SearchClients(keyword string) ([]*models.Client, error)
func (sm *SearchManager) SearchPortMappings(keyword string) ([]*models.PortMapping, error)
```

### ConnectionManager

Tracks and manages active connections:

```go
type ConnectionManager struct {
    connRepo *repos.ConnectionRepo
    idGen    *distributed.DistributedIDGenerator
    utils.Dispose
}

// Methods
func (cm *ConnectionManager) RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error
func (cm *ConnectionManager) UnregisterConnection(connID string) error
func (cm *ConnectionManager) GetConnections(mappingID string) ([]*models.ConnectionInfo, error)
func (cm *ConnectionManager) GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error)
func (cm *ConnectionManager) UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error
```

### ConfigManager

Manages dynamic configuration updates and watchers:

```go
type ConfigManager struct {
    storage  storages.Storage
    config   *ControlConfig
    mu       sync.RWMutex
    watchers []ConfigWatcher
    utils.Dispose
}

// Methods
func (cm *ConfigManager) GetConfig() *ControlConfig
func (cm *ConfigManager) UpdateConfig(ctx context.Context, newConfig *ControlConfig) error
func (cm *ConfigManager) LoadConfig(ctx context.Context) error
func (cm *ConfigManager) AddWatcher(watcher ConfigWatcher)
func (cm *ConfigManager) RemoveWatcher(watcher ConfigWatcher)
```

### CleanupManager

Handles scheduled cleanup tasks and resource management:

```go
type CleanupManager struct {
    storage storages.Storage
    lock    distributed.DistributedLock
    mu      sync.Mutex
    utils.Dispose
}

// Methods
func (cm *CleanupManager) RegisterCleanupTask(ctx context.Context, taskType string, interval time.Duration) error
func (cm *CleanupManager) AcquireCleanupTask(ctx context.Context, taskType string) (*CleanupTask, bool, error)
func (cm *CleanupManager) CompleteCleanupTask(ctx context.Context, taskType string, err error) error
func (cm *CleanupManager) GetCleanupTasks(ctx context.Context) ([]*CleanupTask, error)
```

## 📊 Data Structures

### Core Models

#### User

```go
type User struct {
    ID        string             `json:"id"`
    Username  string             `json:"username"`
    Email     string             `json:"email"`
    Status    UserStatus         `json:"status"`
    CreatedAt time.Time          `json:"created_at"`
    UpdatedAt time.Time          `json:"updated_at"`
}

type UserStatus string

const (
    UserStatusActive   UserStatus = "active"
    UserStatusInactive UserStatus = "inactive"
    UserStatusSuspended UserStatus = "suspended"
)
```

#### Client

```go
type Client struct {
    ID          int64              `json:"id"`
    UserID      string             `json:"user_id"`
    Name        string             `json:"name"`
    AuthCode    string             `json:"auth_code"`
    SecretKey   string             `json:"secret_key"`
    Status      ClientStatus       `json:"status"`
    Type        ClientType         `json:"type"`
    NodeID      string             `json:"node_id,omitempty"`
    Config      configs.ClientConfig `json:"config"`
    LastSeen    *time.Time         `json:"last_seen,omitempty"`
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`
}

type ClientStatus string

const (
    ClientStatusOnline  ClientStatus = "online"
    ClientStatusOffline ClientStatus = "offline"
    ClientStatusError   ClientStatus = "error"
)

type ClientType string

const (
    ClientTypeRegistered ClientType = "registered"
    ClientTypeAnonymous  ClientType = "anonymous"
)
```

#### PortMapping

```go
type PortMapping struct {
    ID             string           `json:"id"`
    UserID         string           `json:"user_id"`
    SourceClientID int64            `json:"source_client_id"`
    TargetClientID int64            `json:"target_client_id"`
    Protocol       Protocol         `json:"protocol"`
    SourcePort     int              `json:"source_port"`
    TargetPort     int              `json:"target_port"`
    Status         MappingStatus    `json:"status"`
    Type           MappingType      `json:"type"`
    TrafficStats   stats.TrafficStats `json:"traffic_stats"`
    CreatedAt      time.Time        `json:"created_at"`
    UpdatedAt      time.Time        `json:"updated_at"`
}

type MappingStatus string

const (
    MappingStatusActive   MappingStatus = "active"
    MappingStatusInactive MappingStatus = "inactive"
    MappingStatusError    MappingStatus = "error"
)

type MappingType string

const (
    MappingTypeStandard  MappingType = "standard"
    MappingTypeAnonymous MappingType = "anonymous"
)
```

#### ConnectionInfo

```go
type ConnectionInfo struct {
    ID            string    `json:"id"`
    MappingID     string    `json:"mapping_id"`
    ClientID      int64     `json:"client_id"`
    RemoteAddr    string    `json:"remote_addr"`
    LocalAddr     string    `json:"local_addr"`
    Protocol      Protocol  `json:"protocol"`
    Status        string    `json:"status"`
    BytesSent     int64     `json:"bytes_sent"`
    BytesReceived int64     `json:"bytes_received"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```

### JWT Structures

#### JWTTokenInfo

```go
type JWTTokenInfo struct {
    Token        string    `json:"token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
    ClientId     int64     `json:"client_id"`
    TokenID      string    `json:"token_id"`
}
```

#### JWTClaims

```go
type JWTClaims struct {
    ClientID   int64  `json:"client_id,string"`
    UserID     string `json:"user_id,omitempty"`
    ClientType string `json:"client_type"`
    NodeID     string `json:"node_id,omitempty"`
    jwt.RegisteredClaims
}
```

### Statistics Structures

#### UserStats

```go
type UserStats struct {
    UserID           string    `json:"user_id"`
    TotalClients     int       `json:"total_clients"`
    OnlineClients    int       `json:"online_clients"`
    TotalMappings    int       `json:"total_mappings"`
    ActiveMappings   int       `json:"active_mappings"`
    TotalTraffic     int64     `json:"total_traffic"`
    TotalConnections int64     `json:"total_connections"`
    LastActive       time.Time `json:"last_active"`
}
```

#### ClientStats

```go
type ClientStats struct {
    ClientID         int64     `json:"client_id"`
    UserID           string    `json:"user_id"`
    TotalMappings    int       `json:"total_mappings"`
    ActiveMappings   int       `json:"active_mappings"`
    TotalTraffic     int64     `json:"total_traffic"`
    TotalConnections int64     `json:"total_connections"`
    Uptime           int64     `json:"uptime"`
    LastSeen         time.Time `json:"last_seen"`
}
```

#### SystemStats

```go
type SystemStats struct {
    TotalUsers       int   `json:"total_users"`
    TotalClients     int   `json:"total_clients"`
    OnlineClients    int   `json:"online_clients"`
    TotalMappings    int   `json:"total_mappings"`
    ActiveMappings   int   `json:"active_mappings"`
    TotalNodes       int   `json:"total_nodes"`
    OnlineNodes      int   `json:"online_nodes"`
    TotalTraffic     int64 `json:"total_traffic"`
    TotalConnections int64 `json:"total_connections"`
    AnonymousUsers   int   `json:"anonymous_users"`
}
```

### Configuration Structures

#### ControlConfig

```go
type ControlConfig struct {
    // API Configuration
    APIEndpoint string        `json:"api_endpoint"`
    APIKey      string        `json:"api_key,omitempty"`
    APISecret   string        `json:"api_secret,omitempty"`
    Timeout     time.Duration `json:"timeout"`
    
    // Node Configuration
    NodeID      string        `json:"node_id,omitempty"`
    NodeName    string        `json:"node_name,omitempty"`
    UseBuiltIn  bool          `json:"use_built_in"`
    
    // JWT Configuration
    JWTSecretKey      string        `json:"jwt_secret_key"`
    JWTExpiration     time.Duration `json:"jwt_expiration"`
    RefreshExpiration time.Duration `json:"refresh_expiration"`
    JWTIssuer         string        `json:"jwt_issuer"`
}
```

#### ClientConfig

```go
type ClientConfig struct {
    EnableCompression bool     `json:"enable_compression"`
    BandwidthLimit    int64    `json:"bandwidth_limit"`
    MaxConnections    int      `json:"max_connections"`
    AllowedPorts      []int    `json:"allowed_ports"`
    BlockedPorts      []int    `json:"blocked_ports"`
    AutoReconnect     bool     `json:"auto_reconnect"`
    HeartbeatInterval int      `json:"heartbeat_interval"`
}
```

## 🔄 Usage Examples

### Basic Cloud Control Setup

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/storages"
)

func main() {
    // Create configuration
    config := managers.DefaultConfig()
    
    // Create storage backend
    storage := storages.NewMemoryStorage(context.Background())
    
    // Create cloud control instance
    cloudControl := managers.NewCloudControl(config, storage)
    
    // Start the service
    cloudControl.Start()
    defer cloudControl.Close()
    
    // Your application logic here...
}
```

### User and Client Management

```go
func userManagementExample(cloudControl managers.CloudControlAPI) error {
    // Create a user
    user, err := cloudControl.CreateUser("john_doe", "john@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    
    // Create a client for the user
    client, err := cloudControl.CreateClient(user.ID, "my-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    
    // Update client status
    err = cloudControl.UpdateClientStatus(client.ID, models.ClientStatusOnline, "node-001")
    if err != nil {
        return fmt.Errorf("failed to update client status: %w", err)
    }
    
    log.Printf("Created user: %s, client: %d", user.ID, client.ID)
    return nil
}
```

### JWT Token Management

```go
func jwtTokenExample(cloudControl managers.CloudControlAPI, clientID int64) error {
    // Generate JWT token
    tokenInfo, err := cloudControl.GenerateJWTToken(clientID)
    if err != nil {
        return fmt.Errorf("failed to generate token: %w", err)
    }
    
    // Validate token
    validatedToken, err := cloudControl.ValidateJWTToken(tokenInfo.Token)
    if err != nil {
        return fmt.Errorf("failed to validate token: %w", err)
    }
    
    log.Printf("Generated token for client: %d", validatedToken.ClientId)
    
    // Refresh token when needed
    newTokenInfo, err := cloudControl.RefreshJWTToken(tokenInfo.RefreshToken)
    if err != nil {
        return fmt.Errorf("failed to refresh token: %w", err)
    }
    
    log.Printf("Refreshed token for client: %d", newTokenInfo.ClientId)
    return nil
}
```

### Port Mapping Management

```go
func portMappingExample(cloudControl managers.CloudControlAPI, userID string, clientID int64) error {
    // Create port mapping
    mapping := &models.PortMapping{
        UserID:         userID,
        SourceClientID: clientID,
        TargetClientID: clientID,
        Protocol:       models.ProtocolTCP,
        SourcePort:     8080,
        TargetPort:     80,
        Status:         models.MappingStatusActive,
        Type:           models.MappingTypeStandard,
    }
    
    createdMapping, err := cloudControl.CreatePortMapping(mapping)
    if err != nil {
        return fmt.Errorf("failed to create mapping: %w", err)
    }
    
    // Update mapping status
    err = cloudControl.UpdatePortMappingStatus(createdMapping.ID, models.MappingStatusInactive)
    if err != nil {
        return fmt.Errorf("failed to update mapping status: %w", err)
    }
    
    // Get user's port mappings
    mappings, err := cloudControl.GetUserPortMappings(userID)
    if err != nil {
        return fmt.Errorf("failed to get user mappings: %w", err)
    }
    
    log.Printf("User has %d port mappings", len(mappings))
    return nil
}
```

### Statistics and Analytics

```go
func statisticsExample(cloudControl managers.CloudControlAPI, userID string, clientID int64) error {
    // Get user statistics
    userStats, err := cloudControl.GetUserStats(userID)
    if err != nil {
        return fmt.Errorf("failed to get user stats: %w", err)
    }
    
    log.Printf("User %s: %d clients, %d mappings, %d bytes traffic", 
        userID, userStats.TotalClients, userStats.TotalMappings, userStats.TotalTraffic)
    
    // Get client statistics
    clientStats, err := cloudControl.GetClientStats(clientID)
    if err != nil {
        return fmt.Errorf("failed to get client stats: %w", err)
    }
    
    log.Printf("Client %d: %d mappings, %d bytes traffic, uptime: %d seconds", 
        clientID, clientStats.TotalMappings, clientStats.TotalTraffic, clientStats.Uptime)
    
    // Get system statistics
    systemStats, err := cloudControl.GetSystemStats()
    if err != nil {
        return fmt.Errorf("failed to get system stats: %w", err)
    }
    
    log.Printf("System: %d users, %d clients, %d nodes, %d bytes total traffic", 
        systemStats.TotalUsers, systemStats.TotalClients, systemStats.TotalNodes, systemStats.TotalTraffic)
    
    return nil
}
```

### Anonymous User Management

```go
func anonymousUserExample(cloudControl managers.CloudControlAPI) error {
    // Generate anonymous credentials
    anonymousClient, err := cloudControl.GenerateAnonymousCredentials()
    if err != nil {
        return fmt.Errorf("failed to generate anonymous credentials: %w", err)
    }
    
    // Create anonymous mapping
    mapping, err := cloudControl.CreateAnonymousMapping(
        anonymousClient.ID, 
        anonymousClient.ID, 
        models.ProtocolTCP, 
        8080, 
        80,
    )
    if err != nil {
        return fmt.Errorf("failed to create anonymous mapping: %w", err)
    }
    
    log.Printf("Created anonymous client: %d, mapping: %s", anonymousClient.ID, mapping.ID)
    
    // Cleanup expired anonymous resources
    err = cloudControl.CleanupExpiredAnonymous()
    if err != nil {
        return fmt.Errorf("failed to cleanup anonymous resources: %w", err)
    }
    
    return nil
}
```

### Search Functionality

```go
func searchExample(cloudControl managers.CloudControlAPI, keyword string) error {
    // Search users
    users, err := cloudControl.SearchUsers(keyword)
    if err != nil {
        return fmt.Errorf("failed to search users: %w", err)
    }
    
    log.Printf("Found %d users matching '%s'", len(users), keyword)
    
    // Search clients
    clients, err := cloudControl.SearchClients(keyword)
    if err != nil {
        return fmt.Errorf("failed to search clients: %w", err)
    }
    
    log.Printf("Found %d clients matching '%s'", len(clients), keyword)
    
    // Search port mappings
    mappings, err := cloudControl.SearchPortMappings(keyword)
    if err != nil {
        return fmt.Errorf("failed to search mappings: %w", err)
    }
    
    log.Printf("Found %d mappings matching '%s'", len(mappings), keyword)
    
    return nil
}
```

## 🛡️ Error Handling

### Error Types

```go
// Domain-specific errors
type UserNotFoundError struct {
    UserID string
}

type ClientAlreadyExistsError struct {
    ClientID int64
}

type TokenExpiredError struct {
    TokenID string
}

type MappingNotFoundError struct {
    MappingID string
}

// Infrastructure errors
type StorageError struct {
    Operation string
    Key       string
    Cause     error
}

type DistributedLockError struct {
    LockKey string
    Cause   error
}
```

### Error Handling Best Practices

```go
func handleErrors(cloudControl managers.CloudControlAPI) {
    // Always check for errors
    user, err := cloudControl.GetUser("user-123")
    if err != nil {
        if errors.Is(err, &UserNotFoundError{}) {
            log.Printf("User not found: user-123")
            return
        }
        log.Printf("Failed to get user: %v", err)
        return
    }
    
    // Use proper error wrapping
    client, err := cloudControl.CreateClient(user.ID, "my-client")
    if err != nil {
        log.Printf("Failed to create client for user %s: %w", user.ID, err)
        return
    }
    
    // Handle cleanup on errors
    defer func() {
        if err != nil {
            // Cleanup on error
            cloudControl.DeleteClient(client.ID)
        }
    }()
    
    // Continue with operations...
}
```

## 🔧 Configuration

### Default Configuration

```go
func DefaultConfig() *ControlConfig {
    return &ControlConfig{
        APIEndpoint:       "http://localhost:8080",
        Timeout:           30 * time.Second,
        UseBuiltIn:        true,
        JWTSecretKey:      "your-secret-key",
        JWTExpiration:     24 * time.Hour,
        RefreshExpiration: 7 * 24 * time.Hour,
        JWTIssuer:         "tunnox",
    }
}
```

### Environment-based Configuration

```go
func loadConfigFromEnv() *ControlConfig {
    config := DefaultConfig()
    
    if endpoint := os.Getenv("TUNNOX_API_ENDPOINT"); endpoint != "" {
        config.APIEndpoint = endpoint
    }
    
    if secretKey := os.Getenv("TUNNOX_JWT_SECRET_KEY"); secretKey != "" {
        config.JWTSecretKey = secretKey
    }
    
    if nodeID := os.Getenv("TUNNOX_NODE_ID"); nodeID != "" {
        config.NodeID = nodeID
    }
    
    return config
}
```

## 🚀 Performance Considerations

### Resource Management

1. **Always use `defer` for cleanup operations**
2. **Implement Dispose interface for custom components**
3. **Use context for cancellation and timeouts**
4. **Handle errors appropriately**

### Caching Strategies

1. **Use token caching for performance**
2. **Implement connection pooling**
3. **Cache frequently accessed data**
4. **Monitor cache hit rates**

### Scalability

1. **Use distributed locks for coordination**
2. **Implement proper connection limits**
3. **Monitor resource usage**
4. **Use load balancing where appropriate**

## 🔒 Security Best Practices

### Token Management

1. **Validate all tokens before use**
2. **Implement proper token expiration**
3. **Use secure token storage**
4. **Log security events**

### Input Validation

1. **Validate all input data**
2. **Sanitize user inputs**
3. **Use parameterized queries**
4. **Implement rate limiting**

### Access Control

1. **Implement role-based access control**
2. **Use resource-level permissions**
3. **Audit all operations**
4. **Monitor for suspicious activity**

## 🧪 Testing

### Unit Testing

```go
func TestUserManagement(t *testing.T) {
    // Create test storage
    storage := storages.NewMemoryStorage(context.Background())
    
    // Create cloud control with test storage
    config := managers.DefaultConfig()
    cloudControl := managers.NewCloudControl(config, storage)
    
    // Test user creation
    user, err := cloudControl.CreateUser("testuser", "test@example.com")
    assert.NoError(t, err)
    assert.NotNil(t, user)
    assert.Equal(t, "testuser", user.Username)
    
    // Test user retrieval
    retrievedUser, err := cloudControl.GetUser(user.ID)
    assert.NoError(t, err)
    assert.Equal(t, user.ID, retrievedUser.ID)
}
```

### Integration Testing

```go
func TestEndToEndWorkflow(t *testing.T) {
    // Setup test environment
    storage := storages.NewMemoryStorage(context.Background())
    config := managers.DefaultConfig()
    cloudControl := managers.NewCloudControl(config, storage)
    
    // Test complete workflow
    user, _ := cloudControl.CreateUser("testuser", "test@example.com")
    client, _ := cloudControl.CreateClient(user.ID, "test-client")
    token, _ := cloudControl.GenerateJWTToken(client.ID)
    
    // Validate the workflow
    assert.NotEmpty(t, user.ID)
    assert.NotEmpty(t, client.ID)
    assert.NotEmpty(t, token.Token)
}
```

---

## 📚 Additional Resources

- **[Architecture Guide](architecture.md)** - Detailed architecture overview
- **[Examples](examples.md)** - Comprehensive usage examples
- **[Configuration Guide](cmd/server/config/README.md)** - Configuration options
- **[Contributing Guide](CONTRIBUTING.md)** - Development guidelines 