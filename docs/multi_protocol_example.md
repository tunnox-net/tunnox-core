# Multi-Protocol Adapter Example

This document demonstrates how to use TCP, WebSocket, UDP, and QUIC adapters in tunnox-core.

## Overview

tunnox-core supports multiple protocol adapters. All adapters implement the unified `Adapter` interface and can be integrated with `ConnectionSession` for business logic handling.

## Supported Protocols

1. **TCP Adapter** - Reliable connection based on TCP
2. **WebSocket Adapter** - HTTP upgrade connection based on WebSocket
3. **UDP Adapter** - Packet-based transmission using UDP
4. **QUIC Adapter** - Modern transport protocol based on QUIC

## Basic Usage

### 1. Create All Adapters

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
    
    // Create CloudControlAPI
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
    // Create ConnectionSession
    session := &protocol.ConnectionSession{
        CloudApi: cloudControl,
    }
    session.SetCtx(ctx, session.onClose)
    
    // Create protocol manager
    pm := protocol.NewManager(ctx)
    
    // Create and register all adapters
    tcpAdapter := protocol.NewTcpAdapter(ctx, session)
    wsAdapter := protocol.NewWebSocketAdapter(ctx, session)
    udpAdapter := protocol.NewUdpAdapter(ctx, session)
    quicAdapter := protocol.NewQuicAdapter(ctx, session)
    
    pm.Register(tcpAdapter)
    pm.Register(wsAdapter)
    pm.Register(udpAdapter)
    pm.Register(quicAdapter)
    
    // Set listen addresses
    tcpAdapter.ListenFrom(":8080")
    wsAdapter.ListenFrom(":8081")
    udpAdapter.ListenFrom(":8082")
    quicAdapter.ListenFrom(":8083")
    
    // Start all adapters
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("Failed to start adapters:", err)
    }
    
    log.Println("Multi-protocol server started:")
    log.Println("  TCP:     :8080")
    log.Println("  WebSocket: :8081")
    log.Println("  UDP:     :8082")
    log.Println("  QUIC:    :8083")
    
    // Wait for signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
    pm.CloseAll()
    log.Println("Server stopped")
}
```

### 2. Client Connection Example

```go
// TCP client
client := protocol.NewTcpAdapter(ctx, nil)
if err := client.ConnectTo("localhost:8080"); err != nil {
    log.Printf("TCP connection failed: %v", err)
}

// WebSocket client
wsClient := protocol.NewWebSocketAdapter(ctx, nil)
if err := wsClient.ConnectTo("ws://localhost:8081"); err != nil {
    log.Printf("WebSocket connection failed: %v", err)
}

// UDP client
udpClient := protocol.NewUdpAdapter(ctx, nil)
if err := udpClient.ConnectTo("localhost:8082"); err != nil {
    log.Printf("UDP connection failed: %v", err)
}

// QUIC client
quicClient := protocol.NewQuicAdapter(ctx, nil)
if err := quicClient.ConnectTo("localhost:8083"); err != nil {
    log.Printf("QUIC connection failed: %v", err)
}
```

## Protocol Feature Comparison

| Protocol   | Reliability | Performance | Firewall Friendly | Latency | Typical Use Cases         |
|------------|-------------|-------------|-------------------|---------|--------------------------|
| TCP        | High        | Medium      | Good              | Medium  | File transfer, DB        |
| WebSocket  | High        | Medium      | Excellent         | Medium  | Web, real-time comm      |
| UDP        | Low         | High        | Good              | Low     | Games, streaming, DNS    |
| QUIC       | High        | High        | Medium            | Low     | Modern web, mobile apps  |

## Connection Handling Flow

All adapters follow the same connection handling flow:

1. **Accept connection**: Adapter accepts a new connection
2. **Call AcceptConnection**: Calls `session.AcceptConnection(reader, writer)`
3. **Business logic**: ConnectionSession handles business logic
4. **Resource cleanup**: Resources are automatically cleaned up when the connection closes

```go
// All adapters call the same interface
func (t *TcpAdapter) handleConn(conn net.Conn) {
    t.session.AcceptConnection(conn, conn)
}

func (w *WebSocketAdapter) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
    wrapper := &WebSocketConnWrapper{conn: conn}
    w.session.AcceptConnection(wrapper, wrapper)
}

func (u *UdpAdapter) handlePacket(data []byte, addr net.Addr) {
    virtualConn := &UdpVirtualConn{data: data, addr: addr, conn: u.conn}
    u.session.AcceptConnection(virtualConn, virtualConn)
}

func (q *QuicAdapter) handleStream(stream *quic.Stream) {
    wrapper := &QuicStreamWrapper{stream: *stream}
    q.session.AcceptConnection(wrapper, wrapper)
}
```

## Configuration Options

### TCP Adapter
- Supports standard TCP address format: `host:port`
- Automatic connection lifecycle management
- Supports long and short connections

### WebSocket Adapter
- Supports `ws://` and `wss://` protocols
- Automatic HTTP upgrade
- Built-in heartbeat
- Supports binary and text messages

### UDP Adapter
- Supports standard UDP address format: `host:port`
- Packet-level processing
- Suitable for connectionless communication

### QUIC Adapter
- Supports standard QUIC address format: `host:port`
- Auto-generates TLS certificates (development)
- Supports multiplexing
- Built-in congestion control

## Error Handling

```go
// Unified error handling pattern
if err := adapter.Start(ctx); err != nil {
    log.Printf("Failed to start %s adapter: %v", adapter.Name(), err)
    // Handle error
}

// Connection error handling
if err := adapter.ConnectTo(serverAddr); err != nil {
    log.Printf("Failed to connect to %s server: %v", adapter.Name(), err)
    // Retry or use backup server
}
```

## Performance Considerations

1. **TCP**: Suitable for reliable transmission
2. **WebSocket**: Suitable for bidirectional web communication
3. **UDP**: Suitable for latency-sensitive scenarios
4. **QUIC**: Suitable for modern networks, especially mobile

## Security Considerations

1. **TCP**: Can be used with TLS
2. **WebSocket**: Supports WSS (WebSocket Secure)
3. **UDP**: Application-layer encryption recommended
4. **QUIC**: Built-in TLS 1.3 support

## Testing

Run tests for all adapters:

```bash
# Test all adapters
go test ./tests -v -run "Test.*Adapter"

# Test specific adapter
go test ./tests -v -run "TestTcpAdapter"
go test ./tests -v -run "TestWebSocketAdapter"
go test ./tests -v -run "TestUdpAdapter"
go test ./tests -v -run "TestQuicAdapter"
```

## Summary

With the unified `Adapter` interface and `ConnectionSession` integration, tunnox-core provides flexible multi-protocol support. Developers can choose the appropriate protocol as needed, and business logic is fully reusable.
