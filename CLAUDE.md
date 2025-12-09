# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Tunnox Core** is an enterprise-grade internal network penetration and port mapping platform built in Go. It provides secure, high-performance remote access capabilities with support for multiple transport protocols (TCP, WebSocket, UDP, QUIC, HTTP Poll) and flexible deployment modes.

The project follows a layered architecture with protocol abstraction, session management, stream processing (compression/encryption/rate-limiting), transparent data forwarding, and cloud-based management.

## Development Commands

### Build

```bash
# Build both server and client (with version info)
./scripts/build.sh

# Or build manually:
go build -o bin/tunnox-server ./cmd/server
go build -o bin/tunnox-client ./cmd/client

# Build with specific output location
go build -o server ./cmd/server
go build -o client ./cmd/client
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run tests with coverage
go test ./... -cover

# Run tests for specific package
go test ./internal/stream/... -v
go test ./internal/protocol/... -v

# Run tests with race detection
go test -race ./...

# Run benchmarks
go test -bench=. -benchmem ./internal/stream/...
```

### Running

```bash
# Start server with config
./bin/tunnox-server -config config.yaml

# Start client
./bin/tunnox-client -config client-config.yaml

# Start server without config (uses defaults)
./bin/tunnox-server
```

### Protocol Buffer Generation

```bash
# Regenerate protobuf files
./scripts/gen_proto.sh
```

### Integration Tests

```bash
# Start test environment (MySQL, Redis, Nginx in Docker)
cd test-env
docker-compose up -d

# Run port mapping tests
./test-port-mapping.sh
./test-tcp-mapping.sh
```

## Architecture Overview

### Layered Architecture

1. **Protocol Adapter Layer** (`internal/protocol/adapter/`, `internal/protocol/registry/`)
   - Unified `ProtocolAdapter` interface for all transport protocols
   - Implementations: TCP, WebSocket, UDP, QUIC, HTTP Poll
   - Each protocol has independent listener and manages its own connections
   - Protocol registry for dynamic protocol management

2. **Session Management Layer** (`internal/protocol/session/`)
   - Connection lifecycle management with heartbeat keep-alive
   - Supports both anonymous and registered clients
   - Session persistence for connectionless protocols (UDP, QUIC)
   - Client ID allocation: 100-199M (registered), 200-299M (anonymous), 600-999M (managed)

3. **Stream Processing Layer** (`internal/stream/`)
   - `StreamProcessor` provides unified packet read/write interface
   - Chainable transformers: compression → encryption → rate limiting
   - Compression: Gzip with levels 1-9
   - Encryption: AES-256-GCM with per-mapping keys
   - Rate limiting: Token bucket algorithm

4. **Data Forwarding Layer** (`internal/client/mapping/`)
   - Transparent forwarding mode - server doesn't parse business data
   - Mapping handlers: TCP, HTTP, SOCKS5, UDP
   - End-to-end encryption and compression handled by clients

5. **Cloud Management Layer** (`internal/cloud/`)
   - **Managers**: High-level business logic (user, client, mapping management)
   - **Services**: Core business services (auth, quota, connection code)
   - **Repos**: Data access layer (storage abstraction)
   - **Models**: Domain entities (User, Client, Mapping, etc.)

6. **Cluster Communication Layer** (`internal/bridge/`)
   - gRPC connection pool for inter-node communication
   - Supports cross-node data forwarding
   - Node discovery and health checking

7. **Message Broadcasting Layer** (`internal/broker/`)
   - Abstraction for pub/sub messaging
   - Implementations: Memory (single node), Redis (cluster)
   - Used for real-time config push and cluster coordination

### Key Packages

- `cmd/server/` - Server entry point
- `cmd/client/` - Client entry point with interactive CLI
- `internal/api/` - Management REST API (port 9000)
- `internal/protocol/httppoll/` - HTTP Poll protocol implementation (firewall bypass)
- `internal/protocol/udp/` - UDP protocol with session management
- `internal/core/storage/` - Storage abstraction (memory/redis/hybrid)
- `internal/core/dispose/` - Hierarchical resource cleanup management
- `internal/core/idgen/` - ID generator (snowflake-based)
- `internal/core/metrics/` - Metrics collection (CPU, memory, goroutines)
- `internal/security/` - JWT authentication and token management
- `internal/health/` - Health check system
- `internal/packet/` - Packet protocol definitions

## Configuration

### Server Configuration (config.yaml)

- Multi-protocol listener config (TCP/WebSocket/UDP/QUIC/HTTP Poll)
- Log configuration (level, format, rotation)
- Cloud management (built-in or external)
- Storage backend (memory/redis/hybrid)
- Message broker (memory/redis)
- Management API settings
- JWT secret and expiration

### Client Configuration (client-config.yaml)

- Anonymous mode or registered mode (client_id + auth_token)
- Server address and protocol selection
- Device ID for anonymous clients

## Project Structure Insights

### Protocol Implementations

Each protocol adapter implements the same interface but has protocol-specific features:

- **TCP**: Direct socket connection, most stable
- **WebSocket**: HTTP-compatible, best firewall penetration
- **UDP**: Connectionless, requires session management for packet protocol
- **QUIC**: Built-in encryption, 0-RTT connection, multiplexing
- **HTTP Poll**: Long-polling for restrictive firewall environments

### Stream Processing Pipeline

StreamProcessor uses a packet-based protocol with flags:
- Compression flag (0x01)
- Encryption flag (0x02)
- Data transformations are applied in order: compress → encrypt → transmit

### Resource Management

The codebase uses a consistent `dispose` pattern for resource cleanup:
- All major components implement disposal interfaces
- Hierarchical cleanup ensures proper resource release
- Prevents memory leaks in long-running connections

### Client Types and ID Ranges

- **100-199M**: Registered clients (托管客户端) - pre-registered by users
- **200-299M**: Anonymous clients (匿名客户端) - auto-assigned, can be claimed later
- **600-999M**: Managed clients (托管客户端) - claimed from anonymous pool

### Management API

REST API on port 9000 provides:
- User management (`/api/v1/users`)
- Client management (`/api/v1/clients`)
- Port mapping management (`/api/v1/mappings`)
- Connection code system (`/api/v1/connection-codes`)
- Statistics and monitoring (`/api/v1/stats`)
- Bearer token authentication

### Connection Code System

Simplified mapping creation workflow:
1. Target client generates connection code (specifies protocol + target address)
2. Source client uses code to create mapping (specifies local listen address)
3. Server coordinates the mapping creation between clients

## Testing Patterns

### Unit Tests

- Test files follow `*_test.go` naming convention
- Use `github.com/stretchr/testify` for assertions
- Mock implementations for interfaces (especially Storage, Broker)
- Table-driven tests for multiple scenarios

### Integration Tests

- Docker Compose setup in `test-env/` provides MySQL, Redis, Nginx
- Test scripts verify end-to-end port mapping scenarios
- Uses real protocol implementations but isolated environment

## Development Guidelines

### Protocol Development

When adding new protocols:
1. Implement `ProtocolAdapter` interface in `internal/protocol/adapter/`
2. Register protocol in `internal/protocol/registry/`
3. Add protocol enum to `internal/constants/`
4. Update server config to support new protocol

### Stream Transformers

When adding new stream transformers:
1. Implement `StreamTransformer` interface in `internal/stream/transform/`
2. Add to transformer chain in `StreamProcessor`
3. Define packet flags if needed
4. Update configuration model

### Storage Providers

When adding new storage backends:
1. Implement `Storage` interface in `internal/core/storage/`
2. Add factory method in storage factory
3. Update config enum and validation

### Error Handling

The codebase uses typed errors (`internal/core/errors/`):
- Use `ErrNotFound`, `ErrAlreadyExists`, `ErrInvalidParameter` etc.
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Log errors at appropriate levels before returning

## Important Context

### Current Branch and Recent Work

The current branch is `feature/udp` with recent focus on:
- HTTP Poll protocol support for firewall bypass scenarios
- Configuration validation and error handling improvements
- Memory storage and cleanup manager logic
- Connection retrieval method unification

### Logging Behavior

- Server logs write to file by default, not console (`logs/server.log`)
- Client logs in interactive mode go to `~/.tunnox/client.log`
- Console output is kept clean for CLI interaction
- Use appropriate log levels: debug/info/warn/error

### Concurrency Patterns

- Heavy use of goroutines for connection handling
- Context-based cancellation throughout the codebase
- Sync primitives (RWMutex, WaitGroup) for state management
- Connection pools for gRPC bridge connections

### Version Management

- Version stored in `VERSION` file (e.g., "1.0.0")
- Build script injects version, git commit, and build time into binary
- Version info exposed via Management API and CLI commands
