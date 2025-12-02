package session

import (
	"net"
	"time"

	"tunnox-core/internal/stream"
)

// HTTPPollTunnelConnection HTTP 长轮询协议的隧道连接实现
type HTTPPollTunnelConnection struct {
	connectionID string
	clientID     int64
	mappingID    string
	tunnelID     string
	stream       stream.PackageStreamer

	state    ConnectionStateManager
	timeout  ConnectionTimeoutManager
	error    ConnectionErrorHandler
	reuse    ConnectionReuseStrategy
}

// NewHTTPPollTunnelConnection 创建 HTTP 长轮询隧道连接
func NewHTTPPollTunnelConnection(
	connectionID string,
	clientID int64,
	mappingID string,
	tunnelID string,
	stream stream.PackageStreamer,
) *HTTPPollTunnelConnection {
	state := NewHTTPPollConnectionState(connectionID)
	timeout := NewHTTPPollConnectionTimeout(30*time.Second, 30*time.Second, 60*time.Second)
	errorHandler := NewHTTPPollConnectionError()
	reuse := NewHTTPPollConnectionReuse()

	return &HTTPPollTunnelConnection{
		connectionID: connectionID,
		clientID:     clientID,
		mappingID:    mappingID,
		tunnelID:     tunnelID,
		stream:       stream,
		state:        state,
		timeout:      timeout,
		error:        errorHandler,
		reuse:        reuse,
	}
}

func (c *HTTPPollTunnelConnection) GetConnectionID() string {
	return c.connectionID
}

func (c *HTTPPollTunnelConnection) GetClientID() int64 {
	return c.clientID
}

func (c *HTTPPollTunnelConnection) GetMappingID() string {
	return c.mappingID
}

func (c *HTTPPollTunnelConnection) GetTunnelID() string {
	return c.tunnelID
}

func (c *HTTPPollTunnelConnection) GetProtocol() string {
	return "httppoll"
}

func (c *HTTPPollTunnelConnection) GetStream() stream.PackageStreamer {
	return c.stream
}

func (c *HTTPPollTunnelConnection) GetNetConn() net.Conn {
	return nil
}

func (c *HTTPPollTunnelConnection) ConnectionState() ConnectionStateManager {
	return c.state
}

func (c *HTTPPollTunnelConnection) ConnectionTimeout() ConnectionTimeoutManager {
	return c.timeout
}

func (c *HTTPPollTunnelConnection) ConnectionError() ConnectionErrorHandler {
	return c.error
}

func (c *HTTPPollTunnelConnection) ConnectionReuse() ConnectionReuseStrategy {
	return c.reuse
}

func (c *HTTPPollTunnelConnection) Close() error {
	if c.stream != nil {
		c.stream.Close()
	}
	c.state.SetState(StateClosed)
	return nil
}

func (c *HTTPPollTunnelConnection) IsClosed() bool {
	return c.state.IsClosed()
}

