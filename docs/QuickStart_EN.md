# Tunnox Core Quick Start Guide

This guide will help you get started with Tunnox Core in 5 minutes, with no external dependencies required.

## Core Concepts

Tunnox Core is a NAT traversal tool consisting of three components:

- **Server**: Deployed on a machine with public IP, responsible for forwarding data
- **Target Client**: Deployed on the machine with the service (e.g., database server)
- **Listen Client**: Deployed on the machine that needs to access the service (e.g., your laptop)

**How it works**:
```
Listen Client (local) → Server (public) → Target Client (internal) → Target Service
```

## Quick Start

### Step 1: Build

```bash
git clone https://github.com/your-org/tunnox-core.git
cd tunnox-core

# Build server and client
go build -o bin/tunnox-server ./cmd/server
go build -o bin/tunnox-client ./cmd/client
```

### Step 2: Start Server

On a machine with public IP:

```bash
./bin/tunnox-server
```

The server will start automatically, listening on:
- TCP: 8000
- WebSocket: 8443
- KCP: 8000 (UDP-based)
- QUIC: 443

### Step 3: Start Target Client

On the machine with the service (e.g., internal database server):

```bash
./bin/tunnox-client -s <server-ip>:8000 -p tcp -anonymous
```


After entering the interactive interface, generate a connection code:

```bash
tunnox> generate-code
Select Protocol: 1 (TCP)
Target Address: localhost:3306
✅ Connection code generated: mysql-abc-123
```

### Step 4: Start Source Client

On your local machine:

```bash
./bin/tunnox-client -s <server-ip>:8000 -p tcp -anonymous
```

After entering the interactive interface, use the connection code:

```bash
tunnox> use-code mysql-abc-123
Local Listen Address: 127.0.0.1:13306
✅ Mapping created successfully
```

### Step 5: Access Service

Now you can access the remote service through the local port:

```bash
mysql -h 127.0.0.1 -P 13306 -u root -p
```

## Common Use Cases

### Use Case 1: Remote Access to Home NAS

1. Start target client on NAS
2. Generate connection code (target address: localhost:5000)
3. Start source client on external machine
4. Use connection code to create mapping
5. Access NAS through local port

### Use Case 2: Temporarily Share Local Development Web Service

1. Start target client on development machine (target address: localhost:3000)
2. Generate connection code and share with colleagues
3. Colleagues use connection code to create mapping
4. Colleagues access your service through local port

### Use Case 3: Access Internal Network via SOCKS5 Proxy

1. Start target client on internal network machine
2. Generate SOCKS5 connection code (select SOCKS5 protocol)
3. Use connection code on external machine
4. Configure browser or application to use SOCKS5 proxy (127.0.0.1:1080)

## Daemon Mode

If you need to run the client in the background (without interactive interface):

```bash
# Start in daemon mode
./bin/tunnox-client -s <server-ip>:8000 -p tcp -anonymous -daemon
```

## Configuration File (Optional)

If you don't want to enter command-line parameters every time, you can create a config file:

**client-config.yaml**:
```yaml
anonymous: true
device_id: "my-device"

server:
  address: "server-ip:8000"
  protocol: "tcp"

log:
  level: "info"
  output: "file"
  file: "/tmp/tunnox-client.log"
```

Start with config file:
```bash
./bin/tunnox-client -config client-config.yaml
```

## FAQ

**Q: Do I need to install a database or Redis?**
A: No, Tunnox Core uses memory storage by default, zero dependencies required.

**Q: How to choose transport protocol?**
A: TCP is most stable, recommended for daily use; KCP has low latency, suitable for real-time applications; QUIC has better performance, suitable for mobile networks; WebSocket can traverse firewalls.

**Q: How long is the connection code valid?**
A: Default 24 hours, automatically expires after use.

**Q: How to view logs?**
A: Server logs: `~/logs/server.log`, Client logs: `/tmp/tunnox-client.log`

**Q: How to stop the service?**
A: Type `exit` in interactive mode, or press Ctrl+C.

## Next Steps

- See [README_EN.md](../README_EN.md) for more features
- See [ARCHITECTURE_DESIGN_V2.2.md](ARCHITECTURE_DESIGN_V2.2.md) for architecture design
- See [MANAGEMENT_API.md](MANAGEMENT_API.md) for API documentation

## Support

- GitHub Issues: https://github.com/your-org/tunnox-core/issues
- Documentation: https://github.com/your-org/tunnox-core/docs
