# Usage Examples

## Overview

This document provides comprehensive examples of how to use tunnox-core in various scenarios.

## Basic Server Setup

### Simple TCP Server

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()
    
    // Create protocol manager
    pm := protocol.NewProtocolManager(ctx)
    
    // Create and register TCP adapter
    tcpAdapter := protocol.NewTcpAdapter(ctx, nil)
    pm.Register(tcpAdapter)
    
    // Start all adapters
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("Failed to start adapters:", err)
    }
    
    log.Println("Server started on :8080")
    
    // Wait for shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
    pm.CloseAll()
    log.Println("Server stopped")
}
```

### Server with Cloud Control

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "tunnox-core/internal/cloud"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()
    
    // Initialize cloud control
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    
    // Create protocol manager
    pm := protocol.NewProtocolManager(ctx)
    
    // Create and register TCP adapter
    tcpAdapter := protocol.NewTcpAdapter(ctx, nil)
    pm.Register(tcpAdapter)
    
    // Start all adapters
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("Failed to start adapters:", err)
    }
    
    log.Println("Server started with cloud control")
    
    // Wait for shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
    pm.CloseAll()
    cloudControl.Stop()
    log.Println("Server stopped")
}
```

## Cloud Control Operations

### User Management

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud"
)

func userManagementExample() {
    ctx := context.Background()
    
    // Create cloud control
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Create a user
    user := &cloud.User{
        Username: "john_doe",
        Email:    "john@example.com",
        Status:   cloud.UserStatusActive,
    }
    
    if err := cloudControl.CreateUser(ctx, user); err != nil {
        log.Fatal("Failed to create user:", err)
    }
    log.Printf("Created user: %s", user.ID)
    
    // Get user
    retrievedUser, err := cloudControl.GetUser(ctx, user.ID)
    if err != nil {
        log.Fatal("Failed to get user:", err)
    }
    log.Printf("Retrieved user: %s", retrievedUser.Username)
    
    // Update user
    retrievedUser.Email = "john.updated@example.com"
    if err := cloudControl.UpdateUser(ctx, retrievedUser); err != nil {
        log.Fatal("Failed to update user:", err)
    }
    log.Println("Updated user email")
    
    // List users
    users, err := cloudControl.ListUsers(ctx, "")
    if err != nil {
        log.Fatal("Failed to list users:", err)
    }
    log.Printf("Total users: %d", len(users))
    
    // Delete user
    if err := cloudControl.DeleteUser(ctx, user.ID); err != nil {
        log.Fatal("Failed to delete user:", err)
    }
    log.Println("Deleted user")
}
```

### Client Management

```go
package main

import (
    "context"
    "log"
    "time"
    "tunnox-core/internal/cloud"
)

func clientManagementExample() {
    ctx := context.Background()
    
    // Create cloud control
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Create a user first
    user := &cloud.User{
        Username: "client_user",
        Email:    "client@example.com",
        Status:   cloud.UserStatusActive,
    }
    cloudControl.CreateUser(ctx, user)
    
    // Register a client
    client := &cloud.Client{
        UserID:   user.ID,
        Name:     "my-client",
        Status:   cloud.ClientStatusOnline,
        LastSeen: time.Now(),
    }
    
    if err := cloudControl.RegisterClient(ctx, client); err != nil {
        log.Fatal("Failed to register client:", err)
    }
    log.Printf("Registered client: %s", client.ID)
    
    // Get client
    retrievedClient, err := cloudControl.GetClient(ctx, client.ID)
    if err != nil {
        log.Fatal("Failed to get client:", err)
    }
    log.Printf("Retrieved client: %s", retrievedClient.Name)
    
    // Update client status
    retrievedClient.Status = cloud.ClientStatusOffline
    retrievedClient.LastSeen = time.Now()
    if err := cloudControl.UpdateClient(ctx, retrievedClient); err != nil {
        log.Fatal("Failed to update client:", err)
    }
    log.Println("Updated client status")
    
    // List user clients
    clients, err := cloudControl.ListUserClients(ctx, user.ID)
    if err != nil {
        log.Fatal("Failed to list clients:", err)
    }
    log.Printf("User has %d clients", len(clients))
    
    // Cleanup
    cloudControl.DeleteClient(ctx, client.ID)
    cloudControl.DeleteUser(ctx, user.ID)
}
```

### Port Mapping

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud"
)

func portMappingExample() {
    ctx := context.Background()
    
    // Create cloud control
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Create user and client
    user := &cloud.User{
        Username: "mapping_user",
        Email:    "mapping@example.com",
        Status:   cloud.UserStatusActive,
    }
    cloudControl.CreateUser(ctx, user)
    
    client := &cloud.Client{
        UserID: user.ID,
        Name:   "mapping-client",
        Status: cloud.ClientStatusOnline,
    }
    cloudControl.RegisterClient(ctx, client)
    
    // Create port mapping
    mapping := &cloud.PortMapping{
        UserID:     user.ID,
        ClientID:   client.ID,
        LocalPort:  8080,
        RemotePort: 80,
        Protocol:   "tcp",
        Status:     cloud.MappingStatusActive,
        TrafficStats: &cloud.TrafficStats{
            BytesSent:     0,
            BytesReceived: 0,
        },
    }
    
    if err := cloudControl.CreatePortMapping(ctx, mapping); err != nil {
        log.Fatal("Failed to create mapping:", err)
    }
    log.Printf("Created mapping: %s", mapping.ID)
    
    // Get mapping
    retrievedMapping, err := cloudControl.GetPortMapping(ctx, mapping.ID)
    if err != nil {
        log.Fatal("Failed to get mapping:", err)
    }
    log.Printf("Retrieved mapping: %d -> %d", retrievedMapping.LocalPort, retrievedMapping.RemotePort)
    
    // Update mapping
    retrievedMapping.Status = cloud.MappingStatusInactive
    if err := cloudControl.UpdatePortMapping(ctx, retrievedMapping); err != nil {
        log.Fatal("Failed to update mapping:", err)
    }
    log.Println("Updated mapping status")
    
    // List user mappings
    mappings, err := cloudControl.ListUserMappings(ctx, user.ID)
    if err != nil {
        log.Fatal("Failed to list mappings:", err)
    }
    log.Printf("User has %d mappings", len(mappings))
    
    // Cleanup
    cloudControl.DeletePortMapping(ctx, mapping.ID)
    cloudControl.DeleteClient(ctx, client.ID)
    cloudControl.DeleteUser(ctx, user.ID)
}
```

## Authentication

### User Authentication

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud"
)

func authenticationExample() {
    ctx := context.Background()
    
    // Create cloud control
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Create a user
    user := &cloud.User{
        Username: "auth_user",
        Email:    "auth@example.com",
        Status:   cloud.UserStatusActive,
    }
    cloudControl.CreateUser(ctx, user)
    
    // Authenticate user
    credentials := &cloud.UserCredentials{
        Username: "auth_user",
        Password: "password123",
    }
    
    authResult, err := cloudControl.AuthenticateUser(ctx, credentials)
    if err != nil {
        log.Fatal("Authentication failed:", err)
    }
    log.Printf("Authentication successful, token: %s", authResult.AccessToken)
    
    // Validate token
    tokenInfo, err := cloudControl.ValidateToken(ctx, authResult.AccessToken)
    if err != nil {
        log.Fatal("Token validation failed:", err)
    }
    log.Printf("Token valid for user: %s", tokenInfo.UserID)
    
    // Refresh token
    newAuthResult, err := cloudControl.RefreshToken(ctx, authResult.RefreshToken)
    if err != nil {
        log.Fatal("Token refresh failed:", err)
    }
    log.Printf("Token refreshed: %s", newAuthResult.AccessToken)
    
    // Cleanup
    cloudControl.DeleteUser(ctx, user.ID)
}
```

## Custom Protocol Adapter

### Creating a Custom Adapter

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net"
    "tunnox-core/internal/protocol"
    "tunnox-core/internal/stream"
    "tunnox-core/internal/utils"
)

// CustomAdapter implements ProtocolAdapter interface
type CustomAdapter struct {
    protocol.BaseAdapter
    listener   net.Listener
    active     bool
}

func NewCustomAdapter(addr string, parentCtx context.Context) *CustomAdapter {
    adapter := &CustomAdapter{}
    adapter.SetName("custom")
    adapter.SetAddr(addr)
    adapter.SetCtx(parentCtx, adapter.onClose)
    return adapter
}

func (c *CustomAdapter) Start(ctx context.Context) error {
    ln, err := net.Listen("tcp", c.Addr())
    if err != nil {
        return fmt.Errorf("failed to listen: %w", err)
    }
    c.listener = ln
    c.active = true
    
    go c.acceptLoop()
    return nil
}

func (c *CustomAdapter) acceptLoop() {
    for c.active {
        conn, err := c.listener.Accept()
        if err != nil {
            if !c.IsClosed() {
                log.Printf("Accept error: %v", err)
            }
            return
        }
        go c.handleConn(conn)
    }
}

func (c *CustomAdapter) handleConn(conn net.Conn) {
    defer conn.Close()
    
    ctx, cancel := context.WithCancel(c.Ctx())
    defer cancel()
    
    // Create package stream
    ps := stream.NewPackageStream(conn, conn, ctx)
    defer ps.Close()
    
    // Handle connection (implement your custom logic here)
    log.Printf("Custom adapter handling connection from %s", conn.RemoteAddr())
    
    // Example: echo server
    for {
        data, err := ps.ReadExact(1024)
        if err != nil {
            break
        }
        
        // Echo back
        if err := ps.WriteExact(data); err != nil {
            break
        }
    }
}

func (c *CustomAdapter) Close() error {
    c.active = false
    if c.listener != nil {
        c.listener.Close()
    }
    c.Dispose.Close()
    return nil
}

func (c *CustomAdapter) onClose() {
    c.Close()
}

func main() {
    ctx := context.Background()
    
    // Create protocol manager
    pm := protocol.NewProtocolManager(ctx)
    
    // Register custom adapter
    customAdapter := NewCustomAdapter(":8081", ctx)
    pm.Register(customAdapter)
    
    // Start all adapters
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("Failed to start adapters:", err)
    }
    
    log.Println("Custom adapter server started on :8081")
    
    // Keep running
    select {}
}
```

## Error Handling Examples

### Comprehensive Error Handling

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "log"
    "tunnox-core/internal/cloud"
)

func errorHandlingExample() {
    ctx := context.Background()
    
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Example: Handle user not found
    _, err := cloudControl.GetUser(ctx, "non-existent-id")
    if err != nil {
        if errors.Is(err, cloud.ErrUserNotFound) {
            log.Println("User not found - this is expected")
        } else {
            log.Fatal("Unexpected error:", err)
        }
    }
    
    // Example: Handle client operations
    client := &cloud.Client{
        UserID: "non-existent-user",
        Name:   "test-client",
        Status: cloud.ClientStatusOnline,
    }
    
    err = cloudControl.RegisterClient(ctx, client)
    if err != nil {
        if errors.Is(err, cloud.ErrUserNotFound) {
            log.Println("Cannot register client for non-existent user")
        } else {
            log.Fatal("Unexpected error:", err)
        }
    }
    
    // Example: Handle authentication errors
    credentials := &cloud.UserCredentials{
        Username: "invalid-user",
        Password: "wrong-password",
    }
    
    _, err = cloudControl.AuthenticateUser(ctx, credentials)
    if err != nil {
        if errors.Is(err, cloud.ErrAuthenticationFailed) {
            log.Println("Authentication failed - this is expected")
        } else {
            log.Fatal("Unexpected authentication error:", err)
        }
    }
}

// Custom error handler
func handleCloudError(err error, operation string) error {
    if err == nil {
        return nil
    }
    
    switch {
    case errors.Is(err, cloud.ErrUserNotFound):
        return fmt.Errorf("user not found during %s: %w", operation, err)
    case errors.Is(err, cloud.ErrClientNotFound):
        return fmt.Errorf("client not found during %s: %w", operation, err)
    case errors.Is(err, cloud.ErrMappingNotFound):
        return fmt.Errorf("mapping not found during %s: %w", operation, err)
    case errors.Is(err, cloud.ErrAuthenticationFailed):
        return fmt.Errorf("authentication failed during %s: %w", operation, err)
    case errors.Is(err, cloud.ErrInvalidToken):
        return fmt.Errorf("invalid token during %s: %w", operation, err)
    default:
        return fmt.Errorf("unexpected error during %s: %w", operation, err)
    }
}
```

## Performance Optimization

### Using Buffer Pools

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/utils"
)

func bufferPoolExample() {
    ctx := context.Background()
    
    // Create buffer manager
    bufferMgr := utils.NewBufferManager(ctx)
    defer bufferMgr.Close()
    
    // Allocate buffers
    buffer1 := bufferMgr.Allocate(1024)
    buffer2 := bufferMgr.Allocate(2048)
    
    log.Printf("Allocated buffers: %d bytes, %d bytes", len(buffer1), len(buffer2))
    
    // Use buffers
    copy(buffer1, []byte("Hello, World!"))
    copy(buffer2, []byte("This is a larger buffer"))
    
    // Release buffers back to pool
    bufferMgr.Release(buffer1)
    bufferMgr.Release(buffer2)
    
    log.Println("Buffers released back to pool")
}
```

### Rate Limiting Example

```go
package main

import (
    "context"
    "log"
    "time"
    "tunnox-core/internal/stream"
)

func rateLimitingExample() {
    ctx := context.Background()
    
    // Create token bucket with 1000 bytes per second
    tokenBucket, err := stream.NewTokenBucket(1000, ctx)
    if err != nil {
        log.Fatal("Failed to create token bucket:", err)
    }
    defer tokenBucket.Close()
    
    // Simulate data transfer
    dataSizes := []int{100, 200, 300, 400, 500}
    
    for _, size := range dataSizes {
        start := time.Now()
        
        // Wait for tokens
        err := tokenBucket.WaitForTokens(size)
        if err != nil {
            log.Printf("Rate limit error: %v", err)
            continue
        }
        
        duration := time.Since(start)
        log.Printf("Transferred %d bytes in %v", size, duration)
    }
}
```

## Testing Examples

### Unit Test Example

```go
package main

import (
    "context"
    "testing"
    "tunnox-core/internal/cloud"
)

func TestUserCreation(t *testing.T) {
    ctx := context.Background()
    
    // Create cloud control for testing
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Test user creation
    user := &cloud.User{
        Username: "testuser",
        Email:    "test@example.com",
        Status:   cloud.UserStatusActive,
    }
    
    err := cloudControl.CreateUser(ctx, user)
    if err != nil {
        t.Fatalf("Failed to create user: %v", err)
    }
    
    if user.ID == "" {
        t.Error("User ID should be generated")
    }
    
    // Test user retrieval
    retrievedUser, err := cloudControl.GetUser(ctx, user.ID)
    if err != nil {
        t.Fatalf("Failed to get user: %v", err)
    }
    
    if retrievedUser.Username != user.Username {
        t.Errorf("Expected username %s, got %s", user.Username, retrievedUser.Username)
    }
    
    // Cleanup
    cloudControl.DeleteUser(ctx, user.ID)
}

func TestUserNotFound(t *testing.T) {
    ctx := context.Background()
    
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Test getting non-existent user
    _, err := cloudControl.GetUser(ctx, "non-existent-id")
    if err == nil {
        t.Error("Expected error for non-existent user")
    }
    
    if !errors.Is(err, cloud.ErrUserNotFound) {
        t.Errorf("Expected ErrUserNotFound, got %v", err)
    }
}
```

## Best Practices Summary

1. **Always use context**: Pass context through all operations for cancellation and timeouts
2. **Implement Dispose**: All custom components should implement the Dispose interface
3. **Handle errors properly**: Use error wrapping and type checking
4. **Use defer for cleanup**: Ensure resources are properly released
5. **Test thoroughly**: Write unit tests for all components
6. **Monitor performance**: Use buffer pools and rate limiting appropriately
7. **Log appropriately**: Add logging for debugging and monitoring
8. **Validate input**: Always validate user input and configuration 