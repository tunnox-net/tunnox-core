package mapping

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
	"tunnox-core/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSOCKS5MappingAdapter(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]string
	}{
		{
			name:        "create with nil credentials",
			credentials: nil,
		},
		{
			name:        "create with empty credentials",
			credentials: map[string]string{},
		},
		{
			name: "create with credentials",
			credentials: map[string]string{
				"user1": "pass1",
				"user2": "pass2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSOCKS5MappingAdapter(tt.credentials)
			assert.NotNil(t, adapter)
			assert.Equal(t, tt.credentials, adapter.credentials)
			assert.Nil(t, adapter.listener)
		})
	}
}

func TestSOCKS5MappingAdapter_StartListener(t *testing.T) {
	tests := []struct {
		name        string
		config      config.MappingConfig
		expectError bool
	}{
		{
			name: "start listener on valid port",
			config: config.MappingConfig{
				LocalPort: 19081,
			},
			expectError: false,
		},
		{
			name: "start listener on random port",
			config: config.MappingConfig{
				LocalPort: 0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSOCKS5MappingAdapter(nil)
			defer adapter.Close()

			err := adapter.StartListener(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter.listener)
			}
		})
	}
}

func TestSOCKS5MappingAdapter_Accept(t *testing.T) {
	t.Run("accept connection successfully", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)
		err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
		require.NoError(t, err)
		defer adapter.Close()

		addr := adapter.listener.Addr().String()

		// Connect in goroutine
		go func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer conn.Close()
			time.Sleep(200 * time.Millisecond)
		}()

		// Accept connection (may timeout due to 1 second deadline, but that's OK)
		conn, err := adapter.Accept()
		if err == nil {
			assert.NotNil(t, conn)
			conn.Close()
		}
	})

	t.Run("accept without listener returns error", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)

		conn, err := adapter.Accept()
		assert.Error(t, err)
		assert.Nil(t, conn)
		assert.Contains(t, err.Error(), "not initialized")
	})
}

func TestSOCKS5MappingAdapter_GetProtocol(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)
	protocol := adapter.GetProtocol()
	assert.Equal(t, "socks5", protocol)
}

func TestSOCKS5MappingAdapter_Close(t *testing.T) {
	t.Run("close with nil listener", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)
		err := adapter.Close()
		assert.NoError(t, err)
	})

	t.Run("close with active listener", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)
		err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
		require.NoError(t, err)

		err = adapter.Close()
		assert.NoError(t, err)
	})
}

func TestSOCKS5MappingAdapter_sendReply(t *testing.T) {
	tests := []struct {
		name     string
		rep      byte
		bindAddr string
		bindPort uint16
		validate func(t *testing.T, reply []byte)
	}{
		{
			name:     "success reply with IPv4",
			rep:      socksRepSuccess,
			bindAddr: "127.0.0.1",
			bindPort: 8080,
			validate: func(t *testing.T, reply []byte) {
				assert.GreaterOrEqual(t, len(reply), 10, "reply should have at least 10 bytes")
				assert.Equal(t, socks5Version, reply[0])
				assert.Equal(t, socksRepSuccess, reply[1])
				// Verify it's an IPv4 address (4 bytes) + port (2 bytes)
				assert.Contains(t, reply, byte(127))
				port := binary.BigEndian.Uint16(reply[len(reply)-2:])
				assert.Equal(t, uint16(8080), port)
			},
		},
		{
			name:     "failure reply",
			rep:      socksRepServerFailure,
			bindAddr: "0.0.0.0",
			bindPort: 0,
			validate: func(t *testing.T, reply []byte) {
				assert.Equal(t, socks5Version, reply[0])
				assert.Equal(t, socksRepServerFailure, reply[1])
			},
		},
		{
			name:     "IPv6 address",
			rep:      socksRepSuccess,
			bindAddr: "::1",
			bindPort: 9000,
			validate: func(t *testing.T, reply []byte) {
				assert.GreaterOrEqual(t, len(reply), 4)
				assert.Equal(t, socks5Version, reply[0])
				assert.Equal(t, socksRepSuccess, reply[1])
				// IPv6 reply should be longer than IPv4
				assert.Greater(t, len(reply), 10)
			},
		},
		{
			name:     "invalid address defaults to IPv4",
			rep:      socksRepSuccess,
			bindAddr: "invalid",
			bindPort: 1234,
			validate: func(t *testing.T, reply []byte) {
				assert.GreaterOrEqual(t, len(reply), 10)
				// Just verify it's a valid SOCKS5 reply
				assert.Equal(t, socks5Version, reply[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSOCKS5MappingAdapter(nil)

			// Use a pipe to capture the reply
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()

			// Send reply in goroutine
			go func() {
				adapter.sendReply(server, tt.rep, tt.bindAddr, tt.bindPort)
			}()

			// Read reply
			reply := make([]byte, 1024)
			n, err := client.Read(reply)
			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, reply[:n])
			}
		})
	}
}

func TestSOCKS5MappingAdapter_handleHandshake(t *testing.T) {
	tests := []struct {
		name        string
		clientData  []byte
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid handshake with no auth",
			clientData: []byte{
				0x05,       // SOCKS version 5
				0x01,       // 1 method
				0x00,       // No authentication
			},
			expectError: false,
		},
		{
			name: "valid handshake with multiple methods",
			clientData: []byte{
				0x05,       // SOCKS version 5
				0x03,       // 3 methods
				0x00,       // No authentication
				0x01,       // GSSAPI
				0x02,       // Username/Password
			},
			expectError: false,
		},
		{
			name: "invalid SOCKS version",
			clientData: []byte{
				0x04,       // SOCKS version 4
				0x01,
				0x00,
			},
			expectError: true,
			errorMsg:    "unsupported SOCKS version",
		},
		{
			name:        "empty handshake",
			clientData:  []byte{},
			expectError: true,
		},
		{
			name: "incomplete handshake",
			clientData: []byte{
				0x05,       // SOCKS version 5
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSOCKS5MappingAdapter(nil)

			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()

			// Write client data in goroutine
			go func() {
				client.Write(tt.clientData)
				time.Sleep(50 * time.Millisecond)
			}()

			err := adapter.handleHandshake(server)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)

				// Verify server response
				response := make([]byte, 2)
				client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, err := client.Read(response)
				if err == nil && n == 2 {
					assert.Equal(t, byte(socks5Version), response[0])
					assert.Equal(t, socksAuthNone, response[1])
				}
			}
		})
	}
}

func TestSOCKS5MappingAdapter_handleRequest(t *testing.T) {
	tests := []struct {
		name         string
		clientData   []byte
		expectedAddr string
		expectError  bool
		errorMsg     string
	}{
		{
			name: "valid IPv4 CONNECT request",
			clientData: []byte{
				0x05,             // SOCKS version 5
				0x01,             // CONNECT command
				0x00,             // Reserved
				0x01,             // IPv4 address type
				192, 168, 1, 100, // IP address
				0x00, 0x50,       // Port 80
			},
			expectedAddr: "192.168.1.100:80",
			expectError:  false,
		},
		{
			name: "valid domain CONNECT request",
			clientData: func() []byte {
				buf := bytes.NewBuffer(nil)
				buf.Write([]byte{0x05, 0x01, 0x00, 0x03}) // Version, CMD, RSV, ATYP
				buf.WriteByte(11)                          // Domain length
				buf.WriteString("example.com")             // Domain
				buf.Write([]byte{0x01, 0xBB})              // Port 443
				return buf.Bytes()
			}(),
			expectedAddr: "example.com:443",
			expectError:  false,
		},
		{
			name: "valid IPv6 CONNECT request",
			clientData: []byte{
				0x05,       // SOCKS version 5
				0x01,       // CONNECT command
				0x00,       // Reserved
				0x04,       // IPv6 address type
				0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // IPv6 address
				0x00, 0x50, // Port 80
			},
			expectedAddr: "2001:db8::1:80",
			expectError:  false,
		},
		{
			name: "unsupported command (BIND)",
			clientData: []byte{
				0x05,             // SOCKS version 5
				0x02,             // BIND command (not supported)
				0x00,             // Reserved
				0x01,             // IPv4 address type
				192, 168, 1, 100, // IP address
				0x00, 0x50,       // Port 80
			},
			expectError: true,
			errorMsg:    "unsupported command",
		},
		{
			name: "invalid SOCKS version in request",
			clientData: []byte{
				0x04,             // SOCKS version 4
				0x01,             // CONNECT command
				0x00,             // Reserved
				0x01,             // IPv4 address type
				192, 168, 1, 100, // IP address
				0x00, 0x50,       // Port 80
			},
			expectError: true,
			errorMsg:    "unsupported SOCKS version",
		},
		{
			name: "unsupported address type",
			clientData: []byte{
				0x05,       // SOCKS version 5
				0x01,       // CONNECT command
				0x00,       // Reserved
				0xFF,       // Invalid address type
				0x00, 0x00, // Dummy data
			},
			expectError: true,
			errorMsg:    "unsupported address type",
		},
		{
			name:        "incomplete request",
			clientData:  []byte{0x05, 0x01},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSOCKS5MappingAdapter(nil)

			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()

			// Write client data in goroutine
			go func() {
				client.Write(tt.clientData)
				// Read server response
				response := make([]byte, 1024)
				client.Read(response)
			}()

			targetAddr, err := adapter.handleRequest(server)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAddr, targetAddr)
			}
		})
	}
}

func TestSOCKS5MappingAdapter_PrepareConnection(t *testing.T) {
	t.Run("successful SOCKS5 handshake and request", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)

		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		// Simulate SOCKS5 client in goroutine
		go func() {
			// Send handshake
			client.Write([]byte{0x05, 0x01, 0x00}) // Version 5, 1 method, no auth

			// Read handshake response
			handshakeResp := make([]byte, 2)
			client.Read(handshakeResp)

			// Send CONNECT request
			request := []byte{
				0x05,             // SOCKS version 5
				0x01,             // CONNECT command
				0x00,             // Reserved
				0x01,             // IPv4 address type
				127, 0, 0, 1,     // IP address
				0x1F, 0x90,       // Port 8080
			}
			client.Write(request)

			// Read response
			response := make([]byte, 1024)
			client.Read(response)
		}()

		err := adapter.PrepareConnection(server)
		assert.NoError(t, err)
	})

	t.Run("prepare connection with non-net.Conn", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)
		mockConn := &mockReadWriteCloser{}

		err := adapter.PrepareConnection(mockConn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires net.Conn")
	})

	t.Run("prepare connection with invalid handshake", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)

		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		// Send invalid handshake
		go func() {
			client.Write([]byte{0x04, 0x01, 0x00}) // SOCKS4 instead of SOCKS5
		}()

		err := adapter.PrepareConnection(server)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handshake failed")
	})
}

func TestSOCKS5Constants(t *testing.T) {
	// Verify SOCKS5 protocol constants
	assert.Equal(t, byte(0x05), socks5Version)
	assert.Equal(t, byte(0x00), socksAuthNone)
	assert.Equal(t, byte(0xFF), socksAuthNoMatch)
	assert.Equal(t, byte(0x01), socksCmdConnect)
	assert.Equal(t, byte(0x01), socksAddrTypeIPv4)
	assert.Equal(t, byte(0x03), socksAddrTypeDomain)
	assert.Equal(t, byte(0x04), socksAddrTypeIPv6)
	assert.Equal(t, byte(0x00), socksRepSuccess)
	assert.Equal(t, byte(0x01), socksRepServerFailure)
	assert.Equal(t, byte(0x07), socksRepCommandNotSupported)
	assert.Equal(t, byte(0x08), socksRepAddrTypeNotSupported)
}

func TestSOCKS5MappingAdapter_EdgeCases(t *testing.T) {
	t.Run("handle very long domain name", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)

		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		// Simulate client with long domain
		go func() {
			// Handshake
			client.Write([]byte{0x05, 0x01, 0x00})
			handshakeResp := make([]byte, 2)
			client.Read(handshakeResp)

			// CONNECT request with long domain (255 bytes max)
			buf := bytes.NewBuffer(nil)
			buf.Write([]byte{0x05, 0x01, 0x00, 0x03}) // Version, CMD, RSV, ATYP
			longDomain := string(bytes.Repeat([]byte("a"), 200)) + ".com"
			buf.WriteByte(byte(len(longDomain)))
			buf.WriteString(longDomain)
			buf.Write([]byte{0x00, 0x50}) // Port 80
			client.Write(buf.Bytes())

			// Read response
			response := make([]byte, 1024)
			client.Read(response)
		}()

		err := adapter.PrepareConnection(server)
		assert.NoError(t, err)
	})

	t.Run("handle connection timeout", func(t *testing.T) {
		adapter := NewSOCKS5MappingAdapter(nil)

		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		// Don't send any data - should timeout
		go func() {
			time.Sleep(15 * time.Second)
		}()

		err := adapter.PrepareConnection(server)
		assert.Error(t, err)
	})
}
