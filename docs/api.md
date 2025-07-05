# API Documentation

## Overview

This document describes the public APIs and interfaces provided by tunnox-core.

## Core Interfaces

### ProtocolAdapter

The main interface for protocol adapters.

```go
type ProtocolAdapter interface {
    Start(ctx context.Context) error
    Close() error
    IsClosed() bool
    SetCtx(parent context.Context, onClose func())
    Ctx() context.Context
    Name() string
    Addr() string
}
```

**Methods:**
- `Start(ctx)`: Start the protocol adapter
- `Close()`: Close the adapter and release resources
- `IsClosed()`: Check if adapter is closed
- `SetCtx(parent, onClose)`: Set context and cleanup function
- `Ctx()`: Get current context
- `Name()`: Get adapter name
- `Addr()`: Get listening address

### PackageStreamer

Interface for data stream operations.

```go
type PackageStreamer interface {
    ReadPacket() (*packet.TransferPacket, int, error)
    WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error)
    ReadExact(length int) ([]byte, error)
    WriteExact(data []byte) error
    Close()
}
```

**Methods:**
- `ReadPacket()`: Read a complete packet
- `WritePacket(pkt, compress, rateLimit)`: Write packet with options
- `ReadExact(length)`: Read exact number of bytes
- `WriteExact(data)`: Write exact data
- `Close()`: Close the stream

## Cloud Control APIs

### CloudControlAPI

Main interface for cloud control operations.

```go
type CloudControlAPI interface {
    // User management
    CreateUser(ctx context.Context, user *User) error
    GetUser(ctx context.Context, userId string) (*User, error)
    UpdateUser(ctx context.Context, user *User) error
    DeleteUser(ctx context.Context, userId string) error
    ListUsers(ctx context.Context, filter string) ([]*User, error)
    
    // Client management
    RegisterClient(ctx context.Context, client *Client) error
    GetClient(ctx context.Context, clientId string) (*Client, error)
    UpdateClient(ctx context.Context, client *Client) error
    DeleteClient(ctx context.Context, clientId string) error
    ListUserClients(ctx context.Context, userId string) ([]*Client, error)
    
    // Port mapping
    CreatePortMapping(ctx context.Context, mapping *PortMapping) error
    GetPortMapping(ctx context.Context, mappingId string) (*PortMapping, error)
    UpdatePortMapping(ctx context.Context, mapping *PortMapping) error
    DeletePortMapping(ctx context.Context, mappingId string) error
    ListUserMappings(ctx context.Context, userId string) ([]*PortMapping, error)
    
    // Authentication
    AuthenticateUser(ctx context.Context, credentials *UserCredentials) (*AuthResult, error)
    RefreshToken(ctx context.Context, refreshToken string) (*AuthResult, error)
    ValidateToken(ctx context.Context, token string) (*TokenInfo, error)
    
    // System operations
    GetSystemStats(ctx context.Context) (*SystemStats, error)
    Start() error
    Stop() error
}
```

## Data Structures

### User

```go
type User struct {
    ID          string    `json:"id"`
    Username    string    `json:"username"`
    Email       string    `json:"email"`
    Status      UserStatus `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### Client

```go
type Client struct {
    ID          string      `json:"id"`
    UserID      string      `json:"user_id"`
    Name        string      `json:"name"`
    Status      ClientStatus `json:"status"`
    LastSeen    time.Time   `json:"last_seen"`
    CreatedAt   time.Time   `json:"created_at"`
    UpdatedAt   time.Time   `json:"updated_at"`
}
```

### PortMapping

```go
type PortMapping struct {
    ID              string           `json:"id"`
    UserID          string           `json:"user_id"`
    ClientID        string           `json:"client_id"`
    LocalPort       int              `json:"local_port"`
    RemotePort      int              `json:"remote_port"`
    Protocol        string           `json:"protocol"`
    Status          MappingStatus    `json:"status"`
    TrafficStats    *TrafficStats    `json:"traffic_stats"`
    CreatedAt       time.Time        `json:"created_at"`
    UpdatedAt       time.Time        `json:"updated_at"`
}
```

### TransferPacket

```go
type TransferPacket struct {
    PacketType    Type
    CommandPacket *CommandPacket
}
```

### CommandPacket

```go
type CommandPacket struct {
    CommandType CommandType
    Token       string
    SenderId    string
    ReceiverId  string
    CommandBody string
}
```

## Usage Examples

### Creating a TCP Adapter

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()
    
    // Create TCP adapter
    	tcpAdapter := protocol.NewTcpAdapter(ctx, nil)
    
    // Register with protocol manager
    pm := protocol.NewProtocolManager(ctx)
    pm.Register(tcpAdapter)
    
    // Start all adapters
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal(err)
    }
    
    // ... your application logic ...
    
    // Cleanup
    pm.CloseAll()
}
```

### Using Cloud Control

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud"
)

func main() {
    // Create cloud control instance
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    
    // Start cloud control
    cloudControl.Start()
    
    ctx := context.Background()
    
    // Create a user
    user := &cloud.User{
        Username: "testuser",
        Email:    "test@example.com",
        Status:   cloud.UserStatusActive,
    }
    
    if err := cloudControl.CreateUser(ctx, user); err != nil {
        log.Fatal(err)
    }
    
    // Register a client
    client := &cloud.Client{
        UserID: user.ID,
        Name:   "test-client",
        Status: cloud.ClientStatusOnline,
    }
    
    if err := cloudControl.RegisterClient(ctx, client); err != nil {
        log.Fatal(err)
    }
    
    // Create port mapping
    mapping := &cloud.PortMapping{
        UserID:     user.ID,
        ClientID:   client.ID,
        LocalPort:  8080,
        RemotePort: 80,
        Protocol:   "tcp",
        Status:     cloud.MappingStatusActive,
    }
    
    if err := cloudControl.CreatePortMapping(ctx, mapping); err != nil {
        log.Fatal(err)
    }
    
    // Get system stats
    stats, err := cloudControl.GetSystemStats(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Total users: %d", stats.TotalUsers)
    log.Printf("Total clients: %d", stats.TotalClients)
    log.Printf("Total mappings: %d", stats.TotalMappings)
}
```

### Working with PackageStream

```go
package main

import (
    "context"
    "log"
    "net"
    "tunnox-core/internal/packet"
    "tunnox-core/internal/stream"
)

func handleConnection(conn net.Conn) {
    ctx := context.Background()
    
    // Create package stream
    ps := stream.NewPackageStream(conn, conn, ctx)
    defer ps.Close()
    
    // Read packets
    for {
        pkt, _, err := ps.ReadPacket()
        if err != nil {
            log.Printf("Read error: %v", err)
            return
        }
        
        // Process packet based on type
        if pkt.PacketType.IsHeartbeat() {
            // Handle heartbeat
            log.Println("Received heartbeat")
        } else if pkt.PacketType.IsJsonCommand() && pkt.CommandPacket != nil {
            // Handle command packet
            log.Printf("Received command: %v", pkt.CommandPacket.CommandType)
        }
    }
}
```

## Error Handling

### Common Error Types

```go
// Connection errors
var ErrConnectionClosed = errors.New("connection closed")
var ErrConnectionTimeout = errors.New("connection timeout")

// Protocol errors
var ErrInvalidPacketType = errors.New("invalid packet type")
var ErrInvalidCommandType = errors.New("invalid command type")

// Business errors
var ErrUserNotFound = errors.New("user not found")
var ErrClientNotFound = errors.New("client not found")
var ErrMappingNotFound = errors.New("mapping not found")

// Authentication errors
var ErrAuthenticationFailed = errors.New("authentication failed")
var ErrInvalidToken = errors.New("invalid token")
var ErrTokenExpired = errors.New("token expired")
```

### Error Handling Example

```go
func processRequest(ctx context.Context, cloudControl cloud.CloudControlAPI, userId string) error {
    user, err := cloudControl.GetUser(ctx, userId)
    if err != nil {
        if errors.Is(err, cloud.ErrUserNotFound) {
            return fmt.Errorf("user %s not found", userId)
        }
        return fmt.Errorf("failed to get user: %w", err)
    }
    
    // Process user data...
    return nil
}
```

## Configuration

### CloudControlConfig

```go
type CloudControlConfig struct {
    APIEndpoint       string        `json:"api_endpoint"`
    Timeout           time.Duration `json:"timeout"`
    UseBuiltIn        bool          `json:"use_built_in"`
    JWTSecretKey      string        `json:"jwt_secret_key"`
    JWTExpiration     time.Duration `json:"jwt_expiration"`
    RefreshExpiration time.Duration `json:"refresh_expiration"`
    JWTIssuer         string        `json:"jwt_issuer"`
}
```

### Default Configuration

```go
func DefaultConfig() *CloudControlConfig {
    return &CloudControlConfig{
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

## Best Practices

### Resource Management

1. Always use `defer` for cleanup operations
2. Implement Dispose interface for custom components
3. Use context for cancellation and timeouts
4. Handle errors appropriately

### Performance

1. Use buffer pools for memory efficiency
2. Implement rate limiting for bandwidth control
3. Use compression for large data transfers
4. Monitor resource usage

### Security

1. Validate all input data
2. Use secure token management
3. Implement proper authentication
4. Log security events

### Testing

1. Write unit tests for all components
2. Test error conditions
3. Use mocks for external dependencies
4. Test resource cleanup 