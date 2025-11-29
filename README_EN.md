# Tunnox Core

<div align="center">

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)
![Status](https://img.shields.io/badge/Status-Alpha-orange?style=flat-square)

**Enterprise-Grade NAT Traversal and Port Mapping Platform**

A high-performance tunnel solution designed for distributed network environments, supporting multiple transport protocols and flexible deployment models.

[中文文档](README.md) | [Architecture](docs/ARCHITECTURE_DESIGN_V2.2.md) | [API Documentation](docs/MANAGEMENT_API.md)

</div>

---

## Introduction

Tunnox Core is a NAT traversal platform kernel developed in Go, providing secure and stable remote access capabilities. The project adopts a layered architecture design, supports multiple transport protocols including TCP, WebSocket, UDP, and QUIC, and can flexibly adapt to different network environments and business scenarios.

### Key Features

- **Multi-Protocol Transport**: TCP, WebSocket, UDP, and QUIC support
- **End-to-End Encryption**: AES-256-GCM encryption for secure data transmission
- **Data Compression**: Gzip compression to reduce bandwidth consumption
- **Traffic Control**: Token bucket algorithm for precise bandwidth limiting
- **SOCKS5 Proxy**: Support for SOCKS5 protocol for flexible network proxying
- **Distributed Architecture**: Cluster deployment with gRPC inter-node communication
- **Real-Time Configuration**: Push configuration changes through control connections
- **Anonymous Access**: Support anonymous clients for lower barriers to entry
- **Interactive CLI**: Comprehensive command-line interface with connection code generation and port mapping management
- **Connection Code System**: One-time connection codes for simplified tunnel establishment
- **Elegant Startup Display**: Beautiful runtime information display on server startup

### Use Cases

**Remote Access**
- Access home NAS, development machines, databases remotely
- Temporarily share local services with teams or clients

**IoT Device Management**
- Remote monitoring and control of industrial equipment
- Unified access for smart home devices

**Development & Debugging**
- Expose local services for external testing
- Webhook receiving and debugging

**Enterprise Applications**
- Interconnect branch office networks
- Secure integration with third-party systems

---

## Technical Architecture

### Transport Protocols

Tunnox supports four transport protocols, allowing flexible selection based on network conditions:

| Protocol | Characteristics | Use Cases |
|----------|----------------|-----------|
| **TCP** | Stable, reliable, good compatibility | Traditional networks, database connections |
| **WebSocket** | HTTP compatible, strong firewall traversal | Enterprise networks, CDN acceleration |
| **UDP** | Low latency, connectionless | Real-time applications, gaming services |
| **QUIC** | Multiplexing, built-in encryption | Mobile networks, unstable networks |

### Core Components

**Protocol Adapter Layer**
- Unified protocol adapter interface for transparent protocol switching
- Each protocol listens on independent ports without interference

**Session Management Layer**
- Connection lifecycle management with keepalive
- Support for both anonymous and registered clients

**Stream Processing Layer**
- StreamProcessor provides unified packet read/write interface
- Supports chainable transformers: compression, encryption, rate limiting

**Data Forwarding Layer**
- Transparent forwarding mode, server doesn't parse business data
- Support for cross-node bridge forwarding

**Cloud Control Layer**
- Management API provides RESTful interface
- Real-time configuration push without client restart

### Data Flow

```
Client A (Source)
    ↓ [Compress + Encrypt]
  Server
    ↓ [Transparent Forward]
Client B (Target)
    ↓ [Decompress + Decrypt]
  Target Service
```

Clients handle compression and encryption, while the server only performs transparent forwarding, reducing server computational overhead and improving forwarding efficiency.

---

## Quick Start

### Requirements

- Go 1.24 or higher
- Docker (optional, for testing environment)

### Build

```bash
# Clone repository
git clone https://github.com/your-org/tunnox-core.git
cd tunnox-core

# Install dependencies
go mod download

# Build server
go build -o bin/tunnox-server ./cmd/server

# Build client
go build -o bin/tunnox-client ./cmd/client
```

### Run

**1. Start Server**

```bash
./bin/tunnox-server -config config.yaml
```

Default listening ports:
- TCP: 7001
- WebSocket: 7000 (path: `/_tunnox`)
- QUIC: 7003
- Management API: 9000

**2. Start Client**

```bash
# Start client (interactive CLI)
./bin/tunnox-client -config client.yaml
```

The client will enter an interactive command-line interface with the following commands:

```bash
tunnox> help                    # Show help
tunnox> connect                 # Connect to server
tunnox> generate-code           # Generate connection code (target)
tunnox> use-code <code>         # Use connection code to create mapping (source)
tunnox> list-codes              # List all connection codes
tunnox> list-mappings           # List all port mappings
tunnox> status                  # Show connection status
tunnox> exit                    # Exit CLI
```

**3. Create Mapping Using Connection Code (Recommended)**

**Target Client**:
```bash
tunnox> generate-code
# Interactive protocol selection (TCP/UDP/SOCKS5)
# Enter target address (e.g., 192.168.1.10:8080)
# Connection code generated, e.g., abc-def-123
```

**Source Client**:
```bash
tunnox> use-code abc-def-123
# Enter local listen address (e.g., 127.0.0.1:8080)
# Mapping created successfully
```

**4. Access Service**

```bash
# Access target service through mapping
mysql -h 127.0.0.1 -P 8080 -u user -p
```

**Or create mapping via Management API**:

```bash
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "tcp",
    "source_port": 8080,
    "target_host": "localhost",
    "target_port": 3306,
    "enable_compression": true,
    "enable_encryption": true
  }'
```

### Configuration Examples

**Server Configuration (server.yaml)**

```yaml
server:
  host: "0.0.0.0"
  port: 7000
  
  protocols:
    tcp:
      enabled: true
      port: 7001
    websocket:
      enabled: true
      port: 7000
    quic:
      enabled: true
      port: 7003

log:
  level: "info"
  format: "text"

cloud:
  type: "built_in"
  built_in:
    jwt_secret_key: "your-secret-key"
```

**Client Configuration (client.yaml)**

```yaml
# Anonymous mode
anonymous: true
device_id: "my-device"

# Or registered mode
client_id: 10000001
auth_token: "your-token"

server:
  address: "server.example.com:7001"
  protocol: "tcp"  # tcp/websocket/udp/quic
```

---

## Core Features

### Port Mapping

Support for multiple protocol mappings:

- **TCP Mapping**: Databases, SSH, RDP, and other TCP services
- **UDP Mapping**: DNS, gaming services, real-time applications, and other UDP services
- **HTTP Mapping**: Web services, API endpoints
- **SOCKS5 Proxy**: Global proxy supporting any protocol

### Data Processing

**Compression**
- Gzip compression with configurable levels (1-9)
- Automatic skip for already compressed data

**Encryption**
- AES-256-GCM encryption algorithm
- Independent keys per mapping
- Automatic key negotiation and distribution

**Traffic Control**
- Token bucket algorithm for bandwidth limiting
- Support for burst traffic handling
- Per-mapping configuration

### Client Management

**Anonymous Mode**
- No registration required, automatic device ID assignment
- Suitable for temporary use and quick testing

**Registered Mode**
- JWT Token authentication
- Multi-client management support
- Quota and permission control

### Cluster Deployment

**Node Communication**
- gRPC connection pool for efficient inter-node communication
- Support for cross-node data forwarding

**Message Broadcasting**
- Redis Pub/Sub or memory mode
- Real-time configuration synchronization

**Storage Abstraction**
- Memory Storage: Single-node deployment
- Redis Storage: Cluster caching
- Hybrid Storage: Redis + Remote gRPC

---

## Technical Highlights

### 1. Unified Protocol Abstraction

The `ProtocolAdapter` interface unifies handling logic for different transport protocols. Adding new protocols only requires implementing the interface.

### 2. Stream Processing Architecture

`StreamProcessor` provides a unified packet read/write interface, supporting chainable composition of compression, encryption, and rate limiting transformers for flexible data processing pipelines.

### 3. Transparent Forwarding Mode

The server doesn't parse business data, only performs transparent forwarding, reducing CPU overhead. Compression and encryption are handled by clients, ensuring end-to-end security.

### 4. Persistent Session Management

Connectionless protocols like UDP and QUIC implement connection semantics through session management, supporting `StreamProcessor`'s packet protocol.

### 5. Resource Lifecycle Management

Hierarchical resource cleanup based on the `dispose` pattern ensures proper release of connections, streams, and sessions, preventing memory leaks.

### 6. Real-Time Configuration Push

Configuration changes are pushed through control connections. Clients don't need to poll or restart, with configuration taking effect in under 100ms.

---

## Project Structure

```
tunnox-core/
├── cmd/                      # Application entry points
│   ├── server/              # Server
│   └── client/              # Client
├── internal/                # Internal implementation
│   ├── protocol/            # Protocol adapter layer
│   │   ├── adapter/         # TCP/WebSocket/UDP/QUIC adapters
│   │   └── session/         # Session management
│   ├── stream/              # Stream processing layer
│   │   ├── compression/     # Compression
│   │   ├── encryption/      # Encryption
│   │   └── transform/       # Stream transformers
│   ├── client/              # Client implementation
│   │   └── mapping/         # Mapping handlers
│   ├── cloud/               # Cloud control
│   │   ├── managers/        # Business managers
│   │   ├── repos/           # Data repositories
│   │   └── services/        # Business services
│   ├── bridge/              # Cluster communication
│   ├── broker/              # Message broadcasting
│   ├── api/                 # Management API
│   └── core/                # Core components
│       ├── storage/         # Storage abstraction
│       ├── dispose/         # Resource management
│       └── idgen/           # ID generation
├── docs/                    # Documentation
└── test-env/                # Test environment
```

---

## Development Status

### Implemented Features

**Transport Protocols** ✅
- Complete implementation of TCP, WebSocket, UDP, QUIC
- Protocol adapter framework and unified interface

**Stream Processing System** ✅
- Packet protocol and StreamProcessor
- Gzip compression (Level 1-9)
- AES-256-GCM encryption
- Token bucket rate limiting

**Client Features** ✅
- TCP/HTTP/SOCKS5/UDP mapping handlers
- Multi-protocol transport support
- Auto-reconnect and keepalive
- Interactive CLI interface
- Connection code generation and usage
- Port mapping management (list, view, delete)
- Tabular data display

**Server Features** ✅
- Session management and connection routing
- Transparent data forwarding
- Real-time configuration push
- Elegant startup information display
- Log file output (no console pollution)

**Authentication System** ✅
- JWT Token authentication
- Anonymous client support
- Client claiming mechanism

**Management API** ✅
- RESTful interface
- User, client, mapping management
- Statistics and monitoring endpoints
- Connection code management endpoints

**Quota Management** ✅
- User quota model (client count, connections, bandwidth, storage)
- Quota checking and enforcement
- Connection code and mapping count limits

**Monitoring System** ✅
- System metrics collection (CPU, memory, goroutines)
- Resource monitoring and statistics
- Basic metrics endpoints

**Cluster Support** ✅
- gRPC node communication
- Redis/Memory message broadcasting
- Cross-node data forwarding

**Development Toolchain** ✅
- Version management and automated releases
- GitHub Actions CI/CD
- Unified version information management

### In Development

**Monitoring System Enhancement** 🔄
- Prometheus integration and visualization in development
- Richer metrics export

**Web Management UI** 📋
- Planned as a separate project

---

## Performance Characteristics

### Transport Performance

Performance data based on local test environment (Docker Nginx):

| Scenario | Latency | Notes |
|----------|---------|-------|
| TCP Direct | 2.2ms | Baseline |
| TCP + Compression | 2.3ms | Gzip Level 6 |
| TCP + Compression + Encryption | 2.4ms | AES-256-GCM |
| WebSocket | 2.5ms | Through Nginx proxy |
| QUIC | 2.3ms | 0-RTT connection |

### Resource Usage

- **Memory**: ~100KB per connection
- **CPU**: < 5% in transparent forwarding mode
- **Concurrent Connections**: 10K+ per node

### Optimization Techniques

- **Memory Pool**: Buffer reuse to reduce GC pressure
- **Zero-Copy**: Minimize memory allocation and data copying
- **Stream Processing**: Read-write streaming to reduce memory footprint
- **Connection Pooling**: gRPC connection pool to reduce handshake overhead

---

## Deployment

### Single Node Deployment

Suitable for small-scale use or testing:

```bash
# Start server
./tunnox-server -config server.yaml

# Start client
./tunnox-client -config client.yaml
```

### Cluster Deployment

Suitable for production and large-scale use:

**Infrastructure Requirements**
- Kubernetes cluster
- Redis Cluster (message broadcasting)
- PostgreSQL/MySQL (optional, persistent storage)

**Deployment Architecture**
```
LoadBalancer (80/443)
    ↓
Tunnox Server Pods (multiple replicas)
    ↓
Redis Cluster (sessions and messages)
    ↓
Remote Storage (gRPC)
```

Detailed deployment documentation: [docs/ARCHITECTURE_DESIGN_V2.2.md](docs/ARCHITECTURE_DESIGN_V2.2.md)

### Docker Deployment

```bash
# Build images
docker build -t tunnox-server -f Dockerfile.server .
docker build -t tunnox-client -f Dockerfile.client .

# Run server
docker run -d \
  -p 7000:7000 \
  -p 7001:7001 \
  -p 7003:7003 \
  -p 9000:9000 \
  -v ./config.yaml:/app/config.yaml \
  tunnox-server

# Run client
docker run -d \
  -v ./client.yaml:/app/client.yaml \
  tunnox-client
```

---

## Usage Examples

### Example 1: MySQL Database Mapping

**Scenario**: Access remote MySQL database

```bash
# 1. Create mapping
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "tcp",
    "source_port": 13306,
    "target_host": "localhost",
    "target_port": 3306,
    "enable_compression": true,
    "enable_encryption": true
  }'

# 2. Connect to database
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### Example 2: Web Service Mapping

**Scenario**: Temporarily share local web service

```bash
# 1. Create mapping
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "tcp",
    "source_port": 8080,
    "target_host": "localhost",
    "target_port": 3000
  }'

# 2. Access service
curl http://localhost:8080
```

### Example 3: SOCKS5 Proxy

**Scenario**: Access internal services through SOCKS5

```bash
# 1. Create SOCKS5 mapping
curl -X POST http://localhost:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "source_client_id": 10000001,
    "target_client_id": 10000002,
    "protocol": "socks5",
    "source_port": 1080
  }'

# 2. Use SOCKS5 proxy
curl --socks5 localhost:1080 http://internal-service:8080
```

---

## Configuration

### Server Configuration

```yaml
server:
  host: "0.0.0.0"
  port: 7000
  
  # Protocol configuration
  protocols:
    tcp:
      enabled: true
      port: 7001
    websocket:
      enabled: true
      port: 7000
    udp:
      enabled: false
      port: 7002
    quic:
      enabled: true
      port: 7003

# Logging
log:
  level: "info"        # debug/info/warn/error
  format: "text"       # text/json
  output: "file"       # Only "file" supported (logs to file, no console output)
  file: "logs/server.log"

# Cloud control
cloud:
  type: "built_in"     # built_in/external
  built_in:
    jwt_secret_key: "your-secret-key"
    jwt_expiration: 3600
    cleanup_interval: 300

# Message broker
message_broker:
  type: "memory"       # memory/redis
  node_id: "node-001"

# Management API
management_api:
  enabled: true
  listen_addr: ":9000"
  auth:
    type: "bearer"
    bearer_token: "your-api-key"
```

### Client Configuration

```yaml
# Anonymous mode (recommended for testing)
anonymous: true
device_id: "my-device-001"

# Registered mode (recommended for production)
client_id: 10000001
auth_token: "your-jwt-token"

# Server configuration
server:
  address: "server.example.com:7001"
  protocol: "tcp"      # tcp/websocket/udp/quic
```

### Mapping Configuration

Mappings are created dynamically through Management API:

```json
{
  "source_client_id": 10000001,
  "target_client_id": 10000002,
  "protocol": "tcp",
  "source_port": 8080,
  "target_host": "localhost",
  "target_port": 3306,
  "enable_compression": true,
  "compression_level": 6,
  "enable_encryption": true,
  "encryption_method": "aes-256-gcm",
  "bandwidth_limit": 10485760
}
```

---

## Management API

Tunnox provides a complete RESTful API for management:

### Endpoint Overview

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/users` | POST | Create user |
| `/api/v1/clients` | GET/POST | Manage clients |
| `/api/v1/mappings` | GET/POST/DELETE | Manage port mappings |
| `/api/v1/stats` | GET | Get statistics |
| `/api/v1/nodes` | GET | Query node status |

### Authentication

```bash
# Bearer Token authentication
curl -H "Authorization: Bearer your-api-key" \
  http://localhost:9000/api/v1/stats
```

Detailed API documentation: [docs/MANAGEMENT_API.md](docs/MANAGEMENT_API.md)

---

## Testing

### Unit Tests

```bash
# Run all tests
go test ./... -v

# Run specific package tests
go test ./internal/stream/... -v

# Test coverage
go test ./... -cover
```

### Integration Tests

The project provides a complete test environment:

```bash
cd test-env

# Start test services (MySQL, Redis, Nginx, etc.)
docker-compose up -d

# Run test scripts
./test-port-mapping.sh
```

### Performance Tests

```bash
# Benchmark tests
go test -bench=. -benchmem ./internal/stream/...

# Race condition tests
go test -race ./...
```

---

## Roadmap

### v1.0.0 (Current)

- [x] Core architecture design
- [x] Four transport protocol support
- [x] Stream processing system
- [x] TCP/UDP/HTTP/SOCKS5 port mapping
- [x] Management API
- [x] Anonymous clients
- [x] Interactive CLI interface
- [x] Connection code system
- [x] Server startup information display
- [x] Version management and CI/CD
- [x] Quota management system
- [x] Basic monitoring and statistics

### v1.1.0 (Planned)

- [ ] Prometheus monitoring integration and visualization
- [ ] Performance optimization and stress testing
- [ ] Richer metrics export

### v1.2.0 (Future)

- [ ] Web management UI
- [ ] Client SDKs (Go/Python/Rust)
- [ ] Plugin system
- [ ] Additional protocol support

### v2.0.0 (Long-term)

- [ ] Production-grade stability
- [ ] Complete documentation and examples
- [ ] Commercial support
- [ ] Community ecosystem

---

## Contributing

We welcome all forms of contribution:

- **Code Contributions**: Bug fixes, feature additions, performance optimization
- **Documentation**: Improve docs, add examples, translations
- **Issue Reports**: Bug reports, feature suggestions
- **Test Cases**: Add tests, improve coverage

### Contribution Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Create a Pull Request

### Code Standards

- Follow official Go coding conventions
- Use `gofmt` to format code
- Add necessary comments and documentation
- Ensure tests pass

---

## Client CLI Usage

### Main Commands

| Command | Description | Example |
|---------|-------------|---------|
| `help` | Show help information | `help generate-code` |
| `connect` | Connect to server | `connect` |
| `status` | Show connection status | `status` |
| `generate-code` | Generate connection code (target) | `generate-code` |
| `list-codes` | List all connection codes | `list-codes` |
| `use-code <code>` | Use connection code to create mapping (source) | `use-code abc-def-123` |
| `list-mappings` | List all port mappings | `list-mappings` |
| `show-mapping <id>` | Show mapping details | `show-mapping mapping-001` |
| `delete-mapping <id>` | Delete mapping | `delete-mapping mapping-001` |
| `config` | Configuration management | `config list` |
| `exit` | Exit CLI | `exit` |

### Connection Code Workflow

1. **Target Client Generates Connection Code**:
   ```bash
   tunnox> generate-code
   Select Protocol: TCP/UDP/SOCKS5
   Target Address: 192.168.1.10:8080
   ✅ Connection code generated: abc-def-123
   ```

2. **Source Client Uses Connection Code**:
   ```bash
   tunnox> use-code abc-def-123
   Local Listen Address: 127.0.0.1:8080
   ✅ Mapping created successfully
   ```

3. **View Mapping Status**:
   ```bash
   tunnox> list-mappings
   # Display table of all mappings
   ```

---

## FAQ

**Q: What's the difference between Tunnox and frp/ngrok?**

A: Tunnox focuses more on scalability and commercial readiness in its architecture, with built-in cloud control management, quota management, multi-protocol support, and cluster deployment capabilities. frp is more suitable for personal use, while ngrok is a closed-source commercial product.

**Q: Which operating systems are supported?**

A: Linux, macOS, Windows, and Docker container deployment are all supported.

**Q: How is the performance?**

A: In transparent forwarding mode, a single node can support 10K+ concurrent connections with < 5ms added latency. Actual performance depends on hardware and network conditions.

**Q: Is IPv6 supported?**

A: Yes, all protocol adapters support both IPv4 and IPv6.

**Q: How is security ensured?**

A: Provides end-to-end AES-256-GCM encryption, JWT authentication, and fine-grained permission control. Encryption and authentication are recommended for production environments.

**Q: Can it be used commercially?**

A: Yes, the project uses the MIT License, allowing commercial use and secondary development.

**Q: Where are logs written?**

A: Both server and client logs are written to files by default, not to console. Server log path: `logs/server.log`, client log path: `~/.tunnox/client.log` (interactive mode) or `/var/log/tunnox-client.log` (daemon mode).

---

## License

This project is licensed under the [MIT License](LICENSE).

---

## Contact

- **Project Home**: [GitHub Repository](https://github.com/your-org/tunnox-core)
- **Issue Tracker**: [GitHub Issues](https://github.com/your-org/tunnox-core/issues)
- **Documentation**: [docs/](docs/)

---

<div align="center">

**If this project helps you, please give it a Star ⭐**

</div>
