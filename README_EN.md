# Tunnox Core

<div align="center">

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)
![Status](https://img.shields.io/badge/Status-Alpha-orange?style=flat-square)

**Enterprise-Grade NAT Traversal and Port Mapping Platform**

A high-performance tunnel solution designed for distributed network environments, supporting multiple transport protocols and flexible deployment models.

[中文文档](README.md) | [Quick Start](docs/QuickStart_EN.md) | [Architecture](docs/ARCHITECTURE_DESIGN_V2.2.md) | [API Documentation](docs/MANAGEMENT_API.md)

</div>

---

## Introduction

Tunnox Core is a NAT traversal tool developed in Go, providing secure and stable remote access capabilities. The project adopts a layered architecture design, supports multiple transport protocols including TCP, WebSocket, UDP, and QUIC, and can flexibly adapt to different network environments and business scenarios.

**Design Philosophy**: Tunnox Core can be used as a standalone tool directly (without external storage and management platform), or integrated as a platform kernel into larger systems.

### Key Features

- **Zero Dependencies**: No database, Redis, or other external storage required, ready to use out of the box
- **Multi-Protocol Transport**: TCP, WebSocket, KCP, QUIC support
- **End-to-End Encryption**: AES-256-GCM encryption for secure data transmission
- **Data Compression**: Gzip compression to reduce bandwidth consumption
- **Traffic Control**: Token bucket algorithm for precise bandwidth limiting
- **SOCKS5 Proxy**: Support for SOCKS5 protocol for flexible network proxying with dynamic target addresses
- **HTTP Domain Proxy**: Support for accessing HTTP services in target network via HTTP proxy
- **Anonymous Access**: Support anonymous clients, no registration required
- **Interactive CLI**: Comprehensive command-line interface with connection code generation and port mapping management
- **Connection Code System**: One-time connection codes for simplified tunnel establishment
- **Auto-Connect**: Client supports multi-protocol auto-connection, automatically selects best available protocol
- **Flexible Deployment**: Support standalone deployment (memory storage) and cluster deployment (Redis + gRPC)

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

Tunnox supports five transport protocols, allowing flexible selection based on network conditions:

| Protocol | Characteristics | Use Cases |
|----------|----------------|-----------|
| **TCP** | Stable, reliable, good compatibility | Traditional networks, database connections |
| **WebSocket** | HTTP compatible, strong firewall traversal | Enterprise networks, CDN acceleration |
| **KCP** | UDP-based, low latency, fast retransmission | Real-time applications, gaming, unstable networks |
| **QUIC** | Multiplexing, built-in encryption, 0-RTT connection | Mobile networks, high-performance scenarios |

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

### Simplest Usage (No Configuration File Required)

Tunnox Core is designed for zero-configuration startup, with no need for databases, Redis, or other external dependencies.

**Requirements**:
- Go 1.24 or higher (only for compilation)
- Or use pre-compiled binaries directly

**1. Build**

```bash
# Clone repository
git clone https://github.com/your-org/tunnox-core.git
cd tunnox-core

# Build server and client
go build -o bin/tunnox-server ./cmd/server
go build -o bin/tunnox-client ./cmd/client
```

**2. Start Server (Zero Configuration)**

```bash
# Start directly with default configuration (memory storage, no external dependencies)
./bin/tunnox-server

# Or specify a config file
./bin/tunnox-server -config config.yaml
```

Default listening ports:
- TCP: 8000
- WebSocket: 8443
- KCP: 8000 (UDP-based)
- QUIC: 443

Logs output to: `~/logs/server.log`

**3. Start Client (Anonymous Mode)**

```bash
# Interactive mode (recommended)
./bin/tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous

# Daemon mode
./bin/tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous -daemon
```

The client will automatically connect to the server without requiring account registration.

**4. Create Tunnel Using Connection Code (Simplest Way)**

In interactive mode, use connection codes to quickly establish tunnels:

**Target (machine with the service)**:
```bash
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3306
✅ Connection code generated: abc-def-123
```

**Source (machine that needs to access the service)**:
```bash
tunnox> use-code abc-def-123
Local Listen Address: 127.0.0.1:13306
✅ Mapping created successfully
```

**5. Access Service**

```bash
# Now you can access the remote service through local port
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### Common Commands

Client interactive CLI supports the following commands:

```bash
tunnox> help                    # Show help
tunnox> status                  # Show connection status
tunnox> generate-code           # Generate connection code (target)
tunnox> use-code <code>         # Use connection code to create mapping (source)
tunnox> list-codes              # List all connection codes
tunnox> list-mappings           # List all port mappings
tunnox> delete-mapping <id>     # Delete mapping
tunnox> exit                    # Exit CLI
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
- Complete implementation of TCP, WebSocket, KCP, QUIC
- Protocol adapter framework and unified interface
- Client multi-protocol auto-connection feature

**Stream Processing System** ✅
- Packet protocol and StreamProcessor
- Gzip compression (Level 1-9)
- AES-256-GCM encryption
- Token bucket rate limiting

**Client Features** ✅
- TCP/HTTP/SOCKS5 mapping handlers
- SOCKS5 proxy support (dynamic target addresses)
- HTTP domain proxy support
- Multi-protocol transport support (TCP/WebSocket/KCP/QUIC)
- Multi-protocol auto-connection (automatically selects best available protocol)
- Auto-reconnect and keepalive
- Interactive CLI interface
- Connection code generation and usage
- Port mapping management (list, view, delete)
- Configuration hot-reload (server pushes config changes)
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

### Standalone Deployment (Recommended for Personal and Small Teams)

Standalone deployment requires no external dependencies, using memory storage:

```bash
# 1. Start server (zero configuration)
./tunnox-server

# 2. Start client (anonymous mode)
./tunnox-client -s server-ip:8000 -p tcp -anonymous
```

**Features**:
- No database, Redis, or other external storage required
- Simple configuration, ready to use out of the box
- Suitable for personal use, small team collaboration, temporary testing

### Cluster Deployment (For Production Environments)

For high availability and horizontal scaling, deploy in cluster mode:

**Infrastructure Requirements**:
- Redis Cluster (for session sharing and message broadcasting)
- Load Balancer (optional, for multi-node load balancing)

**Server Configuration**:

```yaml
# Cluster mode configuration
storage:
  type: "redis"
  redis:
    address: "redis-cluster:6379"
    password: "your-password"

message_broker:
  type: "redis"
  redis:
    address: "redis-cluster:6379"
    password: "your-password"
  # node_id is automatically allocated at server startup (node-0001 to node-1000)
  # No manual configuration needed
```

**Deployment Architecture**:
```
Clients
  ↓
Load Balancer (optional)
  ↓
Tunnox Server Nodes 1, 2, 3...
  ↓
Redis Cluster (sessions and messages)
```

**Notes**:
- Node ID is automatically allocated by NodeIDAllocator at server startup (range: node-0001 to node-1000)
- Uses distributed lock mechanism to ensure ID uniqueness
- After node crash, ID is automatically released after 90 seconds
- Can be manually specified via environment variable `NODE_ID` or `MESSAGE_BROKER_NODE_ID` (mainly for testing)

### Docker Deployment

```bash
# Build images
docker build -t tunnox-server -f Dockerfile .
docker build -t tunnox-client -f Dockerfile.client .

# Run server (standalone mode)
docker run -d \
  -p 8000:8000 \
  -p 8443:8443 \
  --name tunnox-server \
  tunnox-server

# Run client
docker run -d \
  -e SERVER_ADDRESS="server-ip:8000" \
  -e PROTOCOL="tcp" \
  -e ANONYMOUS="true" \
  --name tunnox-client \
  tunnox-client
```

---

## Usage Examples

### Example 1: MySQL Database Mapping

**Scenario**: Access MySQL database on remote server from local machine

**Steps**:

1. Start target client on remote server (machine with MySQL):
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
```

2. Generate connection code on target:
```bash
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3306
✅ Connection code: mysql-abc-123
```

3. Start source client on local machine:
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
```

4. Use connection code on source:
```bash
tunnox> use-code mysql-abc-123
Local Listen Address: 127.0.0.1:13306
✅ Mapping created
```

5. Connect to database:
```bash
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### Example 2: Web Service Mapping

**Scenario**: Temporarily share local development web service with colleagues for testing

**Steps**:

1. Start target client on development machine:
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3000
✅ Connection code: web-xyz-456
```

2. Colleague starts source client on their machine:
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> use-code web-xyz-456
Local Listen Address: 127.0.0.1:8080
✅ Mapping created
```

3. Colleague accesses the service:
```bash
curl http://localhost:8080
# Or open http://localhost:8080 in browser
```

### Example 3: SOCKS5 Proxy

**Scenario**: Access multiple internal network services through SOCKS5 proxy

**Steps**:

1. Start target client on internal network machine:
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> generate-code
Select Protocol: 3 (SOCKS5)
✅ Connection code: socks-def-789
```

2. Start source client on external network machine:
```bash
./tunnox-client -s server-ip:8000 -p tcp -anonymous
tunnox> use-code socks-def-789
Local Listen Address: 127.0.0.1:1080
✅ SOCKS5 proxy created
```

3. Use SOCKS5 proxy to access internal services:
```bash
# Access any internal service through proxy
curl --socks5 localhost:1080 http://192.168.1.100:8080
curl --socks5 localhost:1080 http://internal-api.local/api/data

# Configure browser to use SOCKS5 proxy: 127.0.0.1:1080
```

---

## Configuration

**Server configuration is optional**, default values are used when no config file is provided.

**Minimal Server Configuration (config.yaml)**

```yaml
server:
  protocols:
    tcp:
      enabled: true
      port: 8000
    websocket:
      enabled: true
      port: 8443
    kcp:
      enabled: true
      port: 8000
    quic:
      enabled: true
      port: 443

log:
  level: "info"
  output: "file"
  file: "~/logs/server.log"

# Use built-in storage, no external dependencies
cloud:
  type: "built_in"
  built_in:
    jwt_secret_key: "change-this-in-production"

# Use memory storage, no Redis required
storage:
  type: "memory"

message_broker:
  type: "memory"
```

**Client Configuration (client-config.yaml)**

```yaml
# Anonymous mode (recommended for quick testing)
anonymous: true
device_id: "my-device"

server:
  address: "127.0.0.1:8000"
  protocol: "tcp"  # tcp/websocket/kcp/quic

log:
  level: "info"
  output: "file"
  file: "/tmp/tunnox-client.log"
```

**Command-line parameters have higher priority than config file**, you can use command-line parameters to override config:

```bash
# Use command-line parameters, no config file needed
./bin/tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous -device my-device

# Supported protocols: tcp, websocket, kcp, quic
./bin/tunnox-client -s 127.0.0.1:8000 -p kcp -anonymous
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

**Q: Do I need a database or Redis?**

A: No. Tunnox Core uses memory storage by default and can start with zero dependencies. If you need cluster deployment or persistence, you can optionally configure Redis.

**Q: How to quickly test?**

A: The simplest way is to start the server and two clients on the same machine:
```bash
# Terminal 1: Start server
./tunnox-server

# Terminal 2: Start target client
./tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous

# Terminal 3: Start source client
./tunnox-client -s 127.0.0.1:8000 -p tcp -anonymous

# Then use connection codes to establish tunnels
```

**Q: What's the difference between anonymous mode and registered mode?**

A: Anonymous mode requires no registration, uses device ID to connect, suitable for quick testing and personal use. Registered mode requires JWT Token, supports quota management and permission control, suitable for teams and production environments.

**Q: Which protocols are supported?**

A: TCP, WebSocket, KCP, QUIC. TCP (stable), KCP (low latency), or QUIC (high performance) are recommended.

**Q: How to choose transport protocol?**

A: 
- **TCP**: Most stable, good compatibility, recommended for database connections and daily use
- **WebSocket**: Can traverse HTTP proxies and firewalls, suitable for enterprise networks
- **KCP**: UDP-based, low latency, fast retransmission, suitable for real-time applications and gaming
- **QUIC**: Multiplexing, 0-RTT connection, suitable for mobile and high-performance scenarios

**Q: How is the performance?**

A: In transparent forwarding mode, a single node can support 10K+ concurrent connections with < 5ms added latency. Actual performance depends on hardware and network conditions.

**Q: Is IPv6 supported?**

A: Yes, all protocol adapters support both IPv4 and IPv6.

**Q: How is security ensured?**

A: Provides end-to-end AES-256-GCM encryption, JWT authentication, and fine-grained permission control. Encryption and authentication are recommended for production environments.

**Q: Can it be used commercially?**

A: Yes, the project uses the MIT License, allowing commercial use and secondary development.

**Q: Where are logs written?**

A: Logs are written to files by default, not polluting the console. Server: `~/logs/server.log`, Client: `/tmp/tunnox-client.log`. Can be modified via config file or command-line parameters.

**Q: How to deploy in production?**

A: It's recommended to run the client in daemon mode (`-daemon` parameter), configure system service (systemd) for automatic startup and restart. Server deployment is recommended using Docker or Kubernetes.

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
