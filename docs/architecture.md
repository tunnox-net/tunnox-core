# Architecture Design

## Overview

tunnox-core is built with a layered architecture that emphasizes maintainability, extensibility, and resource management. The system is designed around the concept of a Dispose tree, where all resources are managed hierarchically for safe and graceful shutdown.

## Core Design Principles

### 1. Dispose Tree Resource Management

All components that require resource cleanup implement the `utils.Dispose` interface, forming a hierarchical tree structure:

```
Server (Root)
├── ProtocolManager
│   ├── TcpAdapter
│   │   ├── ConnectionSession
│   │   │   └── PackageStream
│   │   └── ConnectionSession
│   └── FutureAdapters (HTTP, WebSocket, etc.)
└── CloudControl
    ├── UserRepository
    ├── ClientRepository
    ├── MappingRepository
    └── NodeRepository
```

**Benefits:**
- Automatic cascading cleanup when parent is disposed
- Prevents resource leaks
- Clear ownership hierarchy
- Thread-safe disposal

### 2. Layered Protocol Adapter Architecture

The protocol layer is designed for extensibility and clean separation of concerns:

```
ProtocolManager
├── ProtocolAdapter Interface
│   ├── BaseAdapter (common functionality)
│   ├── TcpAdapter (TCP implementation)
│   ├── HttpAdapter (future)
│   └── WebSocketAdapter (future)
```

**Key Features:**
- Unified interface for all protocol adapters
- Hot-plug capability
- Independent lifecycle management
- Consistent error handling

### 3. Session-Based Connection Management

Each connection is managed through a dedicated session:

```
ConnectionSession
├── PackageStream (data transport)
├── Command Handlers (business logic)
└── Resource Management (Dispose integration)
```

## Component Details

### Protocol Layer

#### ProtocolAdapter Interface
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

#### TcpAdapter Implementation
- Listens on specified TCP port
- Creates ConnectionSession for each connection
- Manages connection lifecycle
- Integrates with Dispose tree

### Stream Layer

#### PackageStream
- Thread-safe data transport
- Supports compression and rate limiting
- Memory pool optimization
- Context-aware operations

#### Features
- **Compression**: Gzip compression for data efficiency
- **Rate Limiting**: Token bucket algorithm for bandwidth control
- **Buffer Management**: Memory pool for performance
- **Error Handling**: Comprehensive error types and recovery

### Cloud Control Layer

#### Repository Pattern
Each entity type has its own repository:
- UserRepository: User management
- ClientRepository: Client registration and status
- MappingRepository: Port mapping configuration
- NodeRepository: Node management

#### Built-in Storage
- Memory-based storage for development
- Extensible to Redis, PostgreSQL, etc.
- Transaction support
- Automatic cleanup

## Data Flow

### Connection Establishment
1. Client connects to TcpAdapter
2. TcpAdapter creates ConnectionSession
3. ConnectionSession creates PackageStream
4. PackageStream handles data transport
5. All components integrated into Dispose tree

### Packet Processing
1. PackageStream reads TransferPacket
2. ConnectionSession dispatches by CommandType
3. Command handlers process business logic
4. Response sent back through PackageStream

### Resource Cleanup
1. Server.Close() triggers Dispose tree cleanup
2. All child components automatically disposed
3. Resources released in correct order
4. No resource leaks

## Error Handling

### Error Types
- **Connection Errors**: Network-related issues
- **Protocol Errors**: Invalid packet format
- **Business Errors**: Authentication, authorization
- **System Errors**: Resource exhaustion, configuration

### Recovery Strategies
- Automatic reconnection for transient errors
- Graceful degradation for non-critical failures
- Comprehensive logging for debugging
- Circuit breaker pattern for stability

## Performance Considerations

### Memory Management
- Buffer pools for efficient memory usage
- Zero-copy operations where possible
- Automatic garbage collection optimization

### Concurrency
- Thread-safe operations throughout
- Connection pooling for scalability
- Non-blocking I/O operations

### Scalability
- Horizontal scaling through multiple nodes
- Load balancing support
- Stateless design where possible

## Security

### Authentication
- JWT-based token authentication
- Token refresh mechanism
- Secure key management

### Authorization
- Role-based access control
- Resource-level permissions
- Audit logging

### Data Protection
- Encryption support (planned)
- Secure communication channels
- Input validation and sanitization

## Future Extensibility

### Protocol Support
- HTTP/HTTPS adapter
- WebSocket adapter
- Custom protocol adapters

### Storage Backends
- Redis integration
- PostgreSQL support
- Distributed storage

### Monitoring and Observability
- Metrics collection
- Distributed tracing
- Health checks

## Development Guidelines

### Code Organization
- Clear separation of concerns
- Interface-driven design
- Comprehensive unit tests
- Documentation for all public APIs

### Resource Management
- All resources must implement Dispose
- Proper error handling and cleanup
- Memory leak prevention
- Performance monitoring

### Testing Strategy
- Unit tests for all components
- Integration tests for workflows
- Performance benchmarks
- Resource leak detection 