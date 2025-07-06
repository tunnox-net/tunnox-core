# Usage Examples

## Overview

This document provides comprehensive examples of how to use Tunnox Core in various scenarios. The examples demonstrate the **Manager Pattern** architecture, proper resource management, and best practices for building scalable cloud-controlled applications.

## 🚀 Quick Start Examples

### Basic Server Setup

#### Simple Cloud Control Server

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
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
    
    log.Println("Cloud control server started")
    
    // Wait for shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
}
```

#### Server with Built-in Cloud Control

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "tunnox-core/internal/cloud/managers"
)

func main() {
    // Create configuration
    config := managers.DefaultConfig()
    
    // Create built-in cloud control (uses memory storage)
    cloudControl := managers.NewBuiltinCloudControl(config)
    
    // Start the service
    cloudControl.Start()
    defer cloudControl.Close()
    
    log.Println("Built-in cloud control server started")
    
    // Wait for shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
}
```

## 👥 User Management Examples

### Complete User Lifecycle

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func userLifecycleExample(cloudControl managers.CloudControlAPI) error {
    // 1. Create a user
    user, err := cloudControl.CreateUser("john_doe", "john@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    log.Printf("Created user: %s (%s)", user.Username, user.ID)
    
    // 2. Get user details
    retrievedUser, err := cloudControl.GetUser(user.ID)
    if err != nil {
        return fmt.Errorf("failed to get user: %w", err)
    }
    log.Printf("Retrieved user: %s", retrievedUser.Username)
    
    // 3. Update user information
    retrievedUser.Email = "john.updated@example.com"
    err = cloudControl.UpdateUser(retrievedUser)
    if err != nil {
        return fmt.Errorf("failed to update user: %w", err)
    }
    log.Println("Updated user email")
    
    // 4. List all users
    users, err := cloudControl.ListUsers(models.UserTypeActive)
    if err != nil {
        return fmt.Errorf("failed to list users: %w", err)
    }
    log.Printf("Total active users: %d", len(users))
    
    // 5. Delete user (cleanup)
    err = cloudControl.DeleteUser(user.ID)
    if err != nil {
        return fmt.Errorf("failed to delete user: %w", err)
    }
    log.Println("Deleted user")
    
    return nil
}
```

### Bulk User Operations

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func bulkUserOperationsExample(cloudControl managers.CloudControlAPI) error {
    // Create multiple users
    userData := []struct {
        username string
        email    string
    }{
        {"alice", "alice@example.com"},
        {"bob", "bob@example.com"},
        {"charlie", "charlie@example.com"},
    }
    
    var users []*models.User
    
    for _, data := range userData {
        user, err := cloudControl.CreateUser(data.username, data.email)
        if err != nil {
            return fmt.Errorf("failed to create user %s: %w", data.username, err)
        }
        users = append(users, user)
        log.Printf("Created user: %s", user.Username)
    }
    
    // List all users
    allUsers, err := cloudControl.ListUsers("")
    if err != nil {
        return fmt.Errorf("failed to list users: %w", err)
    }
    
    log.Printf("Total users in system: %d", len(allUsers))
    
    // Cleanup - delete all created users
    for _, user := range users {
        err := cloudControl.DeleteUser(user.ID)
        if err != nil {
            log.Printf("Warning: failed to delete user %s: %v", user.Username, err)
        } else {
            log.Printf("Deleted user: %s", user.Username)
        }
    }
    
    return nil
}
```

## 🔧 Client Management Examples

### Client Registration and Management

```go
package main

import (
    "context"
    "log"
    "time"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func clientManagementExample(cloudControl managers.CloudControlAPI) error {
    // 1. Create a user first
    user, err := cloudControl.CreateUser("client_user", "client@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID) // Cleanup
    
    // 2. Create multiple clients for the user
    clientNames := []string{"desktop-client", "mobile-client", "server-client"}
    var clients []*models.Client
    
    for _, name := range clientNames {
        client, err := cloudControl.CreateClient(user.ID, name)
        if err != nil {
            return fmt.Errorf("failed to create client %s: %w", name, err)
        }
        clients = append(clients, client)
        log.Printf("Created client: %s (ID: %d)", client.Name, client.ID)
    }
    
    // 3. Update client status
    for _, client := range clients {
        err := cloudControl.UpdateClientStatus(client.ID, models.ClientStatusOnline, "node-001")
        if err != nil {
            return fmt.Errorf("failed to update client status: %w", err)
        }
        log.Printf("Updated client %d status to online", client.ID)
    }
    
    // 4. Touch client (update last seen)
    for _, client := range clients {
        cloudControl.TouchClient(client.ID)
    }
    
    // 5. List user's clients
    userClients, err := cloudControl.ListUserClients(user.ID)
    if err != nil {
        return fmt.Errorf("failed to list user clients: %w", err)
    }
    log.Printf("User has %d clients", len(userClients))
    
    // 6. Get client details
    for _, client := range clients {
        retrievedClient, err := cloudControl.GetClient(client.ID)
        if err != nil {
            return fmt.Errorf("failed to get client %d: %w", client.ID, err)
        }
        log.Printf("Client %d: %s, Status: %s", 
            retrievedClient.ID, retrievedClient.Name, retrievedClient.Status)
    }
    
    // 7. Get client port mappings
    for _, client := range clients {
        mappings, err := cloudControl.GetClientPortMappings(client.ID)
        if err != nil {
            return fmt.Errorf("failed to get client mappings: %w", err)
        }
        log.Printf("Client %d has %d port mappings", client.ID, len(mappings))
    }
    
    // 8. Cleanup - delete clients
    for _, client := range clients {
        err := cloudControl.DeleteClient(client.ID)
        if err != nil {
            return fmt.Errorf("failed to delete client %d: %w", client.ID, err)
        }
        log.Printf("Deleted client: %d", client.ID)
    }
    
    return nil
}
```

### Client Status Monitoring

```go
package main

import (
    "context"
    "log"
    "time"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func clientStatusMonitoringExample(cloudControl managers.CloudControlAPI) error {
    // Create user and client
    user, err := cloudControl.CreateUser("monitor_user", "monitor@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    client, err := cloudControl.CreateClient(user.ID, "monitor-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    defer cloudControl.DeleteClient(client.ID)
    
    // Simulate client status changes
    statuses := []models.ClientStatus{
        models.ClientStatusOnline,
        models.ClientStatusOffline,
        models.ClientStatusError,
        models.ClientStatusOnline,
    }
    
    for i, status := range statuses {
        err := cloudControl.UpdateClientStatus(client.ID, status, "node-001")
        if err != nil {
            return fmt.Errorf("failed to update status: %w", err)
        }
        
        // Touch client to update last seen
        cloudControl.TouchClient(client.ID)
        
        log.Printf("Updated client %d status to %s", client.ID, status)
        
        // Get updated client info
        updatedClient, err := cloudControl.GetClient(client.ID)
        if err != nil {
            return fmt.Errorf("failed to get client: %w", err)
        }
        
        log.Printf("Client %d: Status=%s, LastSeen=%v", 
            updatedClient.ID, updatedClient.Status, updatedClient.LastSeen)
        
        if i < len(statuses)-1 {
            time.Sleep(1 * time.Second) // Simulate time passing
        }
    }
    
    return nil
}
```

## 🌐 Port Mapping Examples

### Port Mapping Lifecycle

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func portMappingLifecycleExample(cloudControl managers.CloudControlAPI) error {
    // 1. Create user and client
    user, err := cloudControl.CreateUser("mapping_user", "mapping@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    client, err := cloudControl.CreateClient(user.ID, "mapping-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    defer cloudControl.DeleteClient(client.ID)
    
    // 2. Create port mapping
    mapping := &models.PortMapping{
        UserID:         user.ID,
        SourceClientID: client.ID,
        TargetClientID: client.ID,
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
    log.Printf("Created mapping: %s", createdMapping.ID)
    
    // 3. Get mapping details
    retrievedMapping, err := cloudControl.GetPortMapping(createdMapping.ID)
    if err != nil {
        return fmt.Errorf("failed to get mapping: %w", err)
    }
    log.Printf("Retrieved mapping: %s -> %s:%d", 
        retrievedMapping.ID, retrievedMapping.Protocol, retrievedMapping.TargetPort)
    
    // 4. Update mapping status
    err = cloudControl.UpdatePortMappingStatus(createdMapping.ID, models.MappingStatusInactive)
    if err != nil {
        return fmt.Errorf("failed to update mapping status: %w", err)
    }
    log.Printf("Updated mapping %s status to inactive", createdMapping.ID)
    
    // 5. Update mapping statistics
    stats := &stats.TrafficStats{
        BytesSent:     1024 * 1024, // 1MB
        BytesReceived: 512 * 1024,  // 512KB
        Connections:   10,
    }
    
    err = cloudControl.UpdatePortMappingStats(createdMapping.ID, stats)
    if err != nil {
        return fmt.Errorf("failed to update mapping stats: %w", err)
    }
    log.Printf("Updated mapping %s statistics", createdMapping.ID)
    
    // 6. List user's port mappings
    userMappings, err := cloudControl.GetUserPortMappings(user.ID)
    if err != nil {
        return fmt.Errorf("failed to get user mappings: %w", err)
    }
    log.Printf("User has %d port mappings", len(userMappings))
    
    // 7. Delete mapping
    err = cloudControl.DeletePortMapping(createdMapping.ID)
    if err != nil {
        return fmt.Errorf("failed to delete mapping: %w", err)
    }
    log.Printf("Deleted mapping: %s", createdMapping.ID)
    
    return nil
}
```

### Multiple Protocol Mappings

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func multipleProtocolMappingsExample(cloudControl managers.CloudControlAPI) error {
    // Create user and client
    user, err := cloudControl.CreateUser("multi_protocol_user", "multi@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    client, err := cloudControl.CreateClient(user.ID, "multi-protocol-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    defer cloudControl.DeleteClient(client.ID)
    
    // Create mappings for different protocols
    mappings := []struct {
        protocol   models.Protocol
        sourcePort int
        targetPort int
        name       string
    }{
        {models.ProtocolTCP, 8080, 80, "HTTP"},
        {models.ProtocolTCP, 8443, 443, "HTTPS"},
        {models.ProtocolUDP, 53, 53, "DNS"},
        {models.ProtocolTCP, 22, 22, "SSH"},
    }
    
    var createdMappings []*models.PortMapping
    
    for _, mappingData := range mappings {
        mapping := &models.PortMapping{
            UserID:         user.ID,
            SourceClientID: client.ID,
            TargetClientID: client.ID,
            Protocol:       mappingData.protocol,
            SourcePort:     mappingData.sourcePort,
            TargetPort:     mappingData.targetPort,
            Status:         models.MappingStatusActive,
            Type:           models.MappingTypeStandard,
        }
        
        createdMapping, err := cloudControl.CreatePortMapping(mapping)
        if err != nil {
            return fmt.Errorf("failed to create %s mapping: %w", mappingData.name, err)
        }
        
        createdMappings = append(createdMappings, createdMapping)
        log.Printf("Created %s mapping: %s (%s:%d -> %d)", 
            mappingData.name, createdMapping.ID, mappingData.protocol, 
            mappingData.sourcePort, mappingData.targetPort)
    }
    
    // List all mappings
    allMappings, err := cloudControl.ListPortMappings(models.MappingTypeStandard)
    if err != nil {
        return fmt.Errorf("failed to list mappings: %w", err)
    }
    log.Printf("Total standard mappings: %d", len(allMappings))
    
    // Cleanup
    for _, mapping := range createdMappings {
        err := cloudControl.DeletePortMapping(mapping.ID)
        if err != nil {
            log.Printf("Warning: failed to delete mapping %s: %v", mapping.ID, err)
        } else {
            log.Printf("Deleted mapping: %s", mapping.ID)
        }
    }
    
    return nil
}
```

## 🔐 JWT Authentication Examples

### Complete JWT Workflow

```go
package main

import (
    "context"
    "log"
    "time"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func jwtAuthenticationExample(cloudControl managers.CloudControlAPI) error {
    // 1. Create user and client
    user, err := cloudControl.CreateUser("jwt_user", "jwt@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    client, err := cloudControl.CreateClient(user.ID, "jwt-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    defer cloudControl.DeleteClient(client.ID)
    
    // 2. Generate JWT token
    tokenInfo, err := cloudControl.GenerateJWTToken(client.ID)
    if err != nil {
        return fmt.Errorf("failed to generate token: %w", err)
    }
    log.Printf("Generated token for client %d", tokenInfo.ClientId)
    log.Printf("Token expires at: %v", tokenInfo.ExpiresAt)
    
    // 3. Validate token
    validatedToken, err := cloudControl.ValidateJWTToken(tokenInfo.Token)
    if err != nil {
        return fmt.Errorf("failed to validate token: %w", err)
    }
    log.Printf("Token validated for client: %d", validatedToken.ClientId)
    
    // 4. Simulate token refresh
    time.Sleep(1 * time.Second) // Simulate time passing
    
    newTokenInfo, err := cloudControl.RefreshJWTToken(tokenInfo.RefreshToken)
    if err != nil {
        return fmt.Errorf("failed to refresh token: %w", err)
    }
    log.Printf("Refreshed token for client: %d", newTokenInfo.ClientId)
    log.Printf("New token expires at: %v", newTokenInfo.ExpiresAt)
    
    // 5. Validate new token
    newValidatedToken, err := cloudControl.ValidateJWTToken(newTokenInfo.Token)
    if err != nil {
        return fmt.Errorf("failed to validate new token: %w", err)
    }
    log.Printf("New token validated for client: %d", newValidatedToken.ClientId)
    
    // 6. Revoke token
    err = cloudControl.RevokeJWTToken(newTokenInfo.Token)
    if err != nil {
        return fmt.Errorf("failed to revoke token: %w", err)
    }
    log.Printf("Revoked token: %s", newTokenInfo.TokenID)
    
    // 7. Try to validate revoked token (should fail)
    _, err = cloudControl.ValidateJWTToken(newTokenInfo.Token)
    if err == nil {
        return fmt.Errorf("expected token validation to fail after revocation")
    }
    log.Printf("Token validation failed as expected after revocation: %v", err)
    
    return nil
}
```

### Token Management for Multiple Clients

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func multiClientTokenExample(cloudControl managers.CloudControlAPI) error {
    // Create user
    user, err := cloudControl.CreateUser("multi_token_user", "multi@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    // Create multiple clients
    clientNames := []string{"desktop", "mobile", "server"}
    var clients []*models.Client
    var tokens []*managers.JWTTokenInfo
    
    for _, name := range clientNames {
        client, err := cloudControl.CreateClient(user.ID, name)
        if err != nil {
            return fmt.Errorf("failed to create client %s: %w", name, err)
        }
        defer cloudControl.DeleteClient(client.ID)
        clients = append(clients, client)
        
        // Generate token for each client
        tokenInfo, err := cloudControl.GenerateJWTToken(client.ID)
        if err != nil {
            return fmt.Errorf("failed to generate token for client %s: %w", name, err)
        }
        tokens = append(tokens, tokenInfo)
        
        log.Printf("Generated token for %s client (ID: %d)", name, client.ID)
    }
    
    // Validate all tokens
    for i, token := range tokens {
        validatedToken, err := cloudControl.ValidateJWTToken(token.Token)
        if err != nil {
            return fmt.Errorf("failed to validate token for client %s: %w", clientNames[i], err)
        }
        log.Printf("Validated token for %s client: %d", clientNames[i], validatedToken.ClientId)
    }
    
    // Refresh tokens
    for i, token := range tokens {
        newToken, err := cloudControl.RefreshJWTToken(token.RefreshToken)
        if err != nil {
            return fmt.Errorf("failed to refresh token for client %s: %w", clientNames[i], err)
        }
        log.Printf("Refreshed token for %s client: %d", clientNames[i], newToken.ClientId)
        
        // Update token reference
        tokens[i] = newToken
    }
    
    // Revoke all tokens
    for i, token := range tokens {
        err := cloudControl.RevokeJWTToken(token.Token)
        if err != nil {
            return fmt.Errorf("failed to revoke token for client %s: %w", clientNames[i], err)
        }
        log.Printf("Revoked token for %s client", clientNames[i])
    }
    
    return nil
}
```

## 📊 Statistics and Analytics Examples

### Comprehensive Statistics Collection

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func statisticsCollectionExample(cloudControl managers.CloudControlAPI) error {
    // Create test data
    user, err := cloudControl.CreateUser("stats_user", "stats@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    client, err := cloudControl.CreateClient(user.ID, "stats-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    defer cloudControl.DeleteClient(client.ID)
    
    // Create some mappings with traffic
    mappings := []struct {
        protocol   models.Protocol
        sourcePort int
        targetPort int
        traffic    int64
    }{
        {models.ProtocolTCP, 8080, 80, 1024 * 1024 * 10}, // 10MB
        {models.ProtocolTCP, 8443, 443, 1024 * 1024 * 5},  // 5MB
        {models.ProtocolUDP, 53, 53, 1024 * 512},          // 512KB
    }
    
    var createdMappings []*models.PortMapping
    
    for _, mappingData := range mappings {
        mapping := &models.PortMapping{
            UserID:         user.ID,
            SourceClientID: client.ID,
            TargetClientID: client.ID,
            Protocol:       mappingData.protocol,
            SourcePort:     mappingData.sourcePort,
            TargetPort:     mappingData.targetPort,
            Status:         models.MappingStatusActive,
            Type:           models.MappingTypeStandard,
        }
        
        createdMapping, err := cloudControl.CreatePortMapping(mapping)
        if err != nil {
            return fmt.Errorf("failed to create mapping: %w", err)
        }
        defer cloudControl.DeletePortMapping(createdMapping.ID)
        createdMappings = append(createdMappings, createdMapping)
        
        // Update mapping with traffic stats
        stats := &stats.TrafficStats{
            BytesSent:     mappingData.traffic,
            BytesReceived: mappingData.traffic / 2,
            Connections:   10,
        }
        
        err = cloudControl.UpdatePortMappingStats(createdMapping.ID, stats)
        if err != nil {
            return fmt.Errorf("failed to update mapping stats: %w", err)
        }
    }
    
    // 1. Get user statistics
    userStats, err := cloudControl.GetUserStats(user.ID)
    if err != nil {
        return fmt.Errorf("failed to get user stats: %w", err)
    }
    
    log.Printf("User Statistics:")
    log.Printf("  Total Clients: %d", userStats.TotalClients)
    log.Printf("  Online Clients: %d", userStats.OnlineClients)
    log.Printf("  Total Mappings: %d", userStats.TotalMappings)
    log.Printf("  Active Mappings: %d", userStats.ActiveMappings)
    log.Printf("  Total Traffic: %d bytes", userStats.TotalTraffic)
    log.Printf("  Total Connections: %d", userStats.TotalConnections)
    log.Printf("  Last Active: %v", userStats.LastActive)
    
    // 2. Get client statistics
    clientStats, err := cloudControl.GetClientStats(client.ID)
    if err != nil {
        return fmt.Errorf("failed to get client stats: %w", err)
    }
    
    log.Printf("Client Statistics:")
    log.Printf("  Client ID: %d", clientStats.ClientID)
    log.Printf("  Total Mappings: %d", clientStats.TotalMappings)
    log.Printf("  Active Mappings: %d", clientStats.ActiveMappings)
    log.Printf("  Total Traffic: %d bytes", clientStats.TotalTraffic)
    log.Printf("  Total Connections: %d", clientStats.TotalConnections)
    log.Printf("  Uptime: %d seconds", clientStats.Uptime)
    log.Printf("  Last Seen: %v", clientStats.LastSeen)
    
    // 3. Get system statistics
    systemStats, err := cloudControl.GetSystemStats()
    if err != nil {
        return fmt.Errorf("failed to get system stats: %w", err)
    }
    
    log.Printf("System Statistics:")
    log.Printf("  Total Users: %d", systemStats.TotalUsers)
    log.Printf("  Total Clients: %d", systemStats.TotalClients)
    log.Printf("  Online Clients: %d", systemStats.OnlineClients)
    log.Printf("  Total Mappings: %d", systemStats.TotalMappings)
    log.Printf("  Active Mappings: %d", systemStats.ActiveMappings)
    log.Printf("  Total Nodes: %d", systemStats.TotalNodes)
    log.Printf("  Online Nodes: %d", systemStats.OnlineNodes)
    log.Printf("  Total Traffic: %d bytes", systemStats.TotalTraffic)
    log.Printf("  Total Connections: %d", systemStats.TotalConnections)
    log.Printf("  Anonymous Users: %d", systemStats.AnonymousUsers)
    
    // 4. Get traffic statistics
    trafficStats, err := cloudControl.GetTrafficStats("24h")
    if err != nil {
        return fmt.Errorf("failed to get traffic stats: %w", err)
    }
    log.Printf("Traffic Statistics (24h): %d data points", len(trafficStats))
    
    // 5. Get connection statistics
    connectionStats, err := cloudControl.GetConnectionStats("24h")
    if err != nil {
        return fmt.Errorf("failed to get connection stats: %w", err)
    }
    log.Printf("Connection Statistics (24h): %d data points", len(connectionStats))
    
    return nil
}
```

## 🔍 Search and Discovery Examples

### Advanced Search Operations

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func searchAndDiscoveryExample(cloudControl managers.CloudControlAPI) error {
    // Create test data
    user1, err := cloudControl.CreateUser("alice_smith", "alice@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user1: %w", err)
    }
    defer cloudControl.DeleteUser(user1.ID)
    
    user2, err := cloudControl.CreateUser("bob_jones", "bob@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user2: %w", err)
    }
    defer cloudControl.DeleteUser(user2.ID)
    
    client1, err := cloudControl.CreateClient(user1.ID, "alice-desktop")
    if err != nil {
        return fmt.Errorf("failed to create client1: %w", err)
    }
    defer cloudControl.DeleteClient(client1.ID)
    
    client2, err := cloudControl.CreateClient(user2.ID, "bob-mobile")
    if err != nil {
        return fmt.Errorf("failed to create client2: %w", err)
    }
    defer cloudControl.DeleteClient(client2.ID)
    
    // Create mappings
    mapping1 := &models.PortMapping{
        UserID:         user1.ID,
        SourceClientID: client1.ID,
        TargetClientID: client1.ID,
        Protocol:       models.ProtocolTCP,
        SourcePort:     8080,
        TargetPort:     80,
        Status:         models.MappingStatusActive,
        Type:           models.MappingTypeStandard,
    }
    
    mapping2 := &models.PortMapping{
        UserID:         user2.ID,
        SourceClientID: client2.ID,
        TargetClientID: client2.ID,
        Protocol:       models.ProtocolTCP,
        SourcePort:     8443,
        TargetPort:     443,
        Status:         models.MappingStatusActive,
        Type:           models.MappingTypeStandard,
    }
    
    createdMapping1, err := cloudControl.CreatePortMapping(mapping1)
    if err != nil {
        return fmt.Errorf("failed to create mapping1: %w", err)
    }
    defer cloudControl.DeletePortMapping(createdMapping1.ID)
    
    createdMapping2, err := cloudControl.CreatePortMapping(mapping2)
    if err != nil {
        return fmt.Errorf("failed to create mapping2: %w", err)
    }
    defer cloudControl.DeletePortMapping(createdMapping2.ID)
    
    // 1. Search users
    log.Println("Searching users...")
    
    users, err := cloudControl.SearchUsers("alice")
    if err != nil {
        return fmt.Errorf("failed to search users: %w", err)
    }
    log.Printf("Found %d users matching 'alice'", len(users))
    for _, user := range users {
        log.Printf("  - %s (%s)", user.Username, user.Email)
    }
    
    users, err = cloudControl.SearchUsers("bob")
    if err != nil {
        return fmt.Errorf("failed to search users: %w", err)
    }
    log.Printf("Found %d users matching 'bob'", len(users))
    for _, user := range users {
        log.Printf("  - %s (%s)", user.Username, user.Email)
    }
    
    // 2. Search clients
    log.Println("Searching clients...")
    
    clients, err := cloudControl.SearchClients("desktop")
    if err != nil {
        return fmt.Errorf("failed to search clients: %w", err)
    }
    log.Printf("Found %d clients matching 'desktop'", len(clients))
    for _, client := range clients {
        log.Printf("  - %s (ID: %d)", client.Name, client.ID)
    }
    
    clients, err = cloudControl.SearchClients("mobile")
    if err != nil {
        return fmt.Errorf("failed to search clients: %w", err)
    }
    log.Printf("Found %d clients matching 'mobile'", len(clients))
    for _, client := range clients {
        log.Printf("  - %s (ID: %d)", client.Name, client.ID)
    }
    
    // 3. Search port mappings
    log.Println("Searching port mappings...")
    
    mappings, err := cloudControl.SearchPortMappings("8080")
    if err != nil {
        return fmt.Errorf("failed to search mappings: %w", err)
    }
    log.Printf("Found %d mappings matching '8080'", len(mappings))
    for _, mapping := range mappings {
        log.Printf("  - %s (%s:%d -> %d)", 
            mapping.ID, mapping.Protocol, mapping.SourcePort, mapping.TargetPort)
    }
    
    mappings, err = cloudControl.SearchPortMappings("443")
    if err != nil {
        return fmt.Errorf("failed to search mappings: %w", err)
    }
    log.Printf("Found %d mappings matching '443'", len(mappings))
    for _, mapping := range mappings {
        log.Printf("  - %s (%s:%d -> %d)", 
            mapping.ID, mapping.Protocol, mapping.SourcePort, mapping.TargetPort)
    }
    
    return nil
}
```

## 🎭 Anonymous User Examples

### Anonymous User Management

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func anonymousUserExample(cloudControl managers.CloudControlAPI) error {
    // 1. Generate anonymous credentials
    anonymousClient, err := cloudControl.GenerateAnonymousCredentials()
    if err != nil {
        return fmt.Errorf("failed to generate anonymous credentials: %w", err)
    }
    defer cloudControl.DeleteAnonymousClient(anonymousClient.ID)
    
    log.Printf("Generated anonymous client: %d (%s)", 
        anonymousClient.ID, anonymousClient.Name)
    log.Printf("  Auth Code: %s", anonymousClient.AuthCode)
    log.Printf("  Type: %s", anonymousClient.Type)
    
    // 2. Get anonymous client details
    retrievedClient, err := cloudControl.GetAnonymousClient(anonymousClient.ID)
    if err != nil {
        return fmt.Errorf("failed to get anonymous client: %w", err)
    }
    log.Printf("Retrieved anonymous client: %d", retrievedClient.ID)
    
    // 3. Create anonymous mapping
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
    defer cloudControl.DeletePortMapping(mapping.ID)
    
    log.Printf("Created anonymous mapping: %s", mapping.ID)
    log.Printf("  Protocol: %s", mapping.Protocol)
    log.Printf("  Port: %d -> %d", mapping.SourcePort, mapping.TargetPort)
    log.Printf("  Type: %s", mapping.Type)
    
    // 4. List all anonymous clients
    anonymousClients, err := cloudControl.ListAnonymousClients()
    if err != nil {
        return fmt.Errorf("failed to list anonymous clients: %w", err)
    }
    log.Printf("Total anonymous clients: %d", len(anonymousClients))
    
    // 5. List all anonymous mappings
    anonymousMappings, err := cloudControl.GetAnonymousMappings()
    if err != nil {
        return fmt.Errorf("failed to get anonymous mappings: %w", err)
    }
    log.Printf("Total anonymous mappings: %d", len(anonymousMappings))
    
    // 6. Cleanup expired anonymous resources
    err = cloudControl.CleanupExpiredAnonymous()
    if err != nil {
        return fmt.Errorf("failed to cleanup anonymous resources: %w", err)
    }
    log.Println("Cleaned up expired anonymous resources")
    
    return nil
}
```

## 🔗 Connection Management Examples

### Connection Tracking

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/models"
)

func connectionTrackingExample(cloudControl managers.CloudControlAPI) error {
    // Create user, client, and mapping
    user, err := cloudControl.CreateUser("conn_user", "conn@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    defer cloudControl.DeleteUser(user.ID)
    
    client, err := cloudControl.CreateClient(user.ID, "conn-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    defer cloudControl.DeleteClient(client.ID)
    
    mapping := &models.PortMapping{
        UserID:         user.ID,
        SourceClientID: client.ID,
        TargetClientID: client.ID,
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
    defer cloudControl.DeletePortMapping(createdMapping.ID)
    
    // Register connections
    connections := []struct {
        remoteAddr string
        localAddr  string
        protocol   models.Protocol
    }{
        {"192.168.1.100:54321", "127.0.0.1:8080", models.ProtocolTCP},
        {"192.168.1.101:54322", "127.0.0.1:8080", models.ProtocolTCP},
        {"192.168.1.102:54323", "127.0.0.1:8080", models.ProtocolTCP},
    }
    
    var connIDs []string
    
    for _, connData := range connections {
        connInfo := &models.ConnectionInfo{
            MappingID:  createdMapping.ID,
            ClientID:   client.ID,
            RemoteAddr: connData.remoteAddr,
            LocalAddr:  connData.localAddr,
            Protocol:   connData.protocol,
            Status:     "active",
        }
        
        err := cloudControl.RegisterConnection(createdMapping.ID, connInfo)
        if err != nil {
            return fmt.Errorf("failed to register connection: %w", err)
        }
        
        // Get the connection ID (in real implementation, this would be returned)
        // For this example, we'll simulate it
        connID := fmt.Sprintf("conn-%s-%s", createdMapping.ID, connData.remoteAddr)
        connIDs = append(connIDs, connID)
        
        log.Printf("Registered connection: %s", connID)
    }
    
    // Get connections for mapping
    mappingConnections, err := cloudControl.GetConnections(createdMapping.ID)
    if err != nil {
        return fmt.Errorf("failed to get mapping connections: %w", err)
    }
    log.Printf("Mapping %s has %d connections", createdMapping.ID, len(mappingConnections))
    
    // Get connections for client
    clientConnections, err := cloudControl.GetClientConnections(client.ID)
    if err != nil {
        return fmt.Errorf("failed to get client connections: %w", err)
    }
    log.Printf("Client %d has %d connections", client.ID, len(clientConnections))
    
    // Update connection statistics
    for i, connID := range connIDs {
        bytesSent := int64(1024 * (i + 1))
        bytesReceived := int64(512 * (i + 1))
        
        err := cloudControl.UpdateConnectionStats(connID, bytesSent, bytesReceived)
        if err != nil {
            return fmt.Errorf("failed to update connection stats: %w", err)
        }
        
        log.Printf("Updated connection %s stats: sent=%d, received=%d", 
            connID, bytesSent, bytesReceived)
    }
    
    // Unregister connections
    for _, connID := range connIDs {
        err := cloudControl.UnregisterConnection(connID)
        if err != nil {
            return fmt.Errorf("failed to unregister connection: %w", err)
        }
        log.Printf("Unregistered connection: %s", connID)
    }
    
    return nil
}
```

## 🎯 Best Practices

### Error Handling

```go
func bestPracticeErrorHandling(cloudControl managers.CloudControlAPI) error {
    // Always use proper error handling
    user, err := cloudControl.CreateUser("best_practice_user", "best@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    
    // Use defer for cleanup
    defer func() {
        if err := cloudControl.DeleteUser(user.ID); err != nil {
            log.Printf("Warning: failed to cleanup user: %v", err)
        }
    }()
    
    // Handle specific error types
    client, err := cloudControl.CreateClient(user.ID, "best-client")
    if err != nil {
        // Check for specific error types if needed
        return fmt.Errorf("failed to create client: %w", err)
    }
    
    // Continue with operations...
    return nil
}
```

### Resource Management

```go
func bestPracticeResourceManagement(cloudControl managers.CloudControlAPI) error {
    // Create resources with proper cleanup
    var resources []string
    
    // Helper function for cleanup
    cleanup := func() {
        for _, resource := range resources {
            // Cleanup logic here
            log.Printf("Cleaned up resource: %s", resource)
        }
    }
    
    // Ensure cleanup happens
    defer cleanup()
    
    // Create resources
    user, err := cloudControl.CreateUser("resource_user", "resource@example.com")
    if err != nil {
        return fmt.Errorf("failed to create user: %w", err)
    }
    resources = append(resources, fmt.Sprintf("user:%s", user.ID))
    
    client, err := cloudControl.CreateClient(user.ID, "resource-client")
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }
    resources = append(resources, fmt.Sprintf("client:%d", client.ID))
    
    // Continue with operations...
    return nil
}
```

### Configuration Management

```go
func bestPracticeConfiguration(cloudControl managers.CloudControlAPI) error {
    // Use environment variables for configuration
    config := managers.DefaultConfig()
    
    // Override with environment variables
    if endpoint := os.Getenv("TUNNOX_API_ENDPOINT"); endpoint != "" {
        config.APIEndpoint = endpoint
    }
    
    if secretKey := os.Getenv("TUNNOX_JWT_SECRET_KEY"); secretKey != "" {
        config.JWTSecretKey = secretKey
    }
    
    // Use the configuration
    log.Printf("Using API endpoint: %s", config.APIEndpoint)
    
    return nil
}
```

---

## 📚 Additional Resources

- **[API Documentation](api.md)** - Complete API reference
- **[Architecture Guide](architecture.md)** - System architecture overview
- **[Configuration Guide](cmd/server/config/README.md)** - Configuration options
- **[Contributing Guide](CONTRIBUTING.md)** - Development guidelines 