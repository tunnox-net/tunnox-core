package session

import (
	"net"
	"time"

	"tunnox-core/internal/stream"
)

// TCPTunnelConnection TCP 协议的隧道连接实现
type TCPTunnelConnection struct {
	connID    string
	conn      net.Conn
	clientID  int64
	mappingID string
	tunnelID  string
	stream    stream.PackageStreamer

	state    ConnectionStateManager
	timeout  ConnectionTimeoutManager
	error    ConnectionErrorHandler
	reuse    ConnectionReuseStrategy
}

// NewTCPTunnelConnection 创建 TCP 隧道连接
func NewTCPTunnelConnection(
	connID string,
	conn net.Conn,
	clientID int64,
	mappingID string,
	tunnelID string,
	stream stream.PackageStreamer,
) *TCPTunnelConnection {
	state := NewTCPConnectionState(conn)
	timeout := NewTCPConnectionTimeout(conn, 30*time.Second, 30*time.Second, 60*time.Second)
	errorHandler := NewTCPConnectionError()
	reuse := NewTCPConnectionReuse(10)

	return &TCPTunnelConnection{
		connID:    connID,
		conn:      conn,
		clientID:  clientID,
		mappingID: mappingID,
		tunnelID:  tunnelID,
		stream:    stream,
		state:     state,
		timeout:   timeout,
		error:     errorHandler,
		reuse:     reuse,
	}
}

func (c *TCPTunnelConnection) GetConnectionID() string {
	return c.connID
}

func (c *TCPTunnelConnection) GetClientID() int64 {
	return c.clientID
}

func (c *TCPTunnelConnection) GetMappingID() string {
	return c.mappingID
}

func (c *TCPTunnelConnection) GetTunnelID() string {
	return c.tunnelID
}

func (c *TCPTunnelConnection) GetProtocol() string {
	return "tcp"
}

func (c *TCPTunnelConnection) GetStream() stream.PackageStreamer {
	return c.stream
}

func (c *TCPTunnelConnection) GetNetConn() net.Conn {
	return c.conn
}

func (c *TCPTunnelConnection) ConnectionState() ConnectionStateManager {
	return c.state
}

func (c *TCPTunnelConnection) ConnectionTimeout() ConnectionTimeoutManager {
	return c.timeout
}

func (c *TCPTunnelConnection) ConnectionError() ConnectionErrorHandler {
	return c.error
}

func (c *TCPTunnelConnection) ConnectionReuse() ConnectionReuseStrategy {
	return c.reuse
}

func (c *TCPTunnelConnection) Close() error {
	var errs []error
	if c.stream != nil {
		c.stream.Close()
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	c.state.SetState(StateClosed)
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (c *TCPTunnelConnection) IsClosed() bool {
	return c.state.IsClosed()
}

