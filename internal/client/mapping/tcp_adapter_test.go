package mapping

import (
	"io"
	"net"
	"testing"
	"time"
	"tunnox-core/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTCPMappingAdapter(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	assert.NotNil(t, adapter)
	assert.Nil(t, adapter.listener)
}

func TestTCPMappingAdapter_StartListener(t *testing.T) {
	tests := []struct {
		name        string
		config      config.MappingConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "start listener on valid port",
			config: config.MappingConfig{
				LocalPort: 18081,
			},
			expectError: false,
		},
		{
			name: "start listener on another valid port",
			config: config.MappingConfig{
				LocalPort: 18082,
			},
			expectError: false,
		},
		{
			name: "start listener on port 0 (random port)",
			config: config.MappingConfig{
				LocalPort: 0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewTCPMappingAdapter()
			defer adapter.Close()

			err := adapter.StartListener(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter.listener)
			}
		})
	}
}

func TestTCPMappingAdapter_Accept(t *testing.T) {
	t.Run("accept connection successfully", func(t *testing.T) {
		adapter := NewTCPMappingAdapter()
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
			time.Sleep(100 * time.Millisecond)
		}()

		// Accept connection
		conn, err := adapter.Accept()
		require.NoError(t, err)
		assert.NotNil(t, conn)
		defer conn.Close()
	})

	t.Run("accept without listener returns error", func(t *testing.T) {
		adapter := NewTCPMappingAdapter()

		conn, err := adapter.Accept()
		assert.Error(t, err)
		assert.Nil(t, conn)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("accept on closed listener returns error", func(t *testing.T) {
		adapter := NewTCPMappingAdapter()
		err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
		require.NoError(t, err)

		// Close listener immediately
		adapter.Close()

		conn, err := adapter.Accept()
		assert.Error(t, err)
		assert.Nil(t, conn)
	})
}

func TestTCPMappingAdapter_PrepareConnection(t *testing.T) {
	adapter := NewTCPMappingAdapter()

	// Create a mock connection
	mockConn := &mockReadWriteCloser{}

	// TCP PrepareConnection should always return nil (no-op)
	err := adapter.PrepareConnection(mockConn)
	assert.NoError(t, err)
}

func TestTCPMappingAdapter_GetProtocol(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	protocol := adapter.GetProtocol()
	assert.Equal(t, "tcp", protocol)
}

func TestTCPMappingAdapter_Close(t *testing.T) {
	t.Run("close with nil listener", func(t *testing.T) {
		adapter := NewTCPMappingAdapter()
		err := adapter.Close()
		assert.NoError(t, err)
	})

	t.Run("close with active listener", func(t *testing.T) {
		adapter := NewTCPMappingAdapter()
		err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
		require.NoError(t, err)

		err = adapter.Close()
		assert.NoError(t, err)

		// Verify listener is closed
		assert.NotNil(t, adapter.listener)
		_, err = adapter.listener.Accept()
		assert.Error(t, err)
	})

	t.Run("close multiple times", func(t *testing.T) {
		adapter := NewTCPMappingAdapter()
		err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
		require.NoError(t, err)

		err = adapter.Close()
		assert.NoError(t, err)

		// Second close should not panic
		err = adapter.Close()
		assert.Error(t, err) // Closing already closed listener returns error
	})
}

func TestTCPMappingAdapter_Integration(t *testing.T) {
	// Full integration test: start listener, accept connection, transfer data
	adapter := NewTCPMappingAdapter()
	err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
	require.NoError(t, err)
	defer adapter.Close()

	addr := adapter.listener.Addr().String()

	// Client writes data
	testData := []byte("Hello, TCP Mapping!")
	receivedData := make(chan []byte, 1)

	// Accept connection in goroutine
	go func() {
		conn, err := adapter.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Prepare connection (should be no-op)
		err = adapter.PrepareConnection(conn)
		if err != nil {
			return
		}

		// Read data
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		receivedData <- buf[:n]
	}()

	// Give accept time to start
	time.Sleep(10 * time.Millisecond)

	// Client connects and sends data
	clientConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer clientConn.Close()

	_, err = clientConn.Write(testData)
	require.NoError(t, err)

	// Wait for data
	select {
	case data := <-receivedData:
		assert.Equal(t, testData, data)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for data")
	}
}

func TestTCPMappingAdapter_MultipleConnections(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	err := adapter.StartListener(config.MappingConfig{LocalPort: 0})
	require.NoError(t, err)
	defer adapter.Close()

	addr := adapter.listener.Addr().String()

	// Accept multiple connections
	numConnections := 3
	accepted := make(chan bool, numConnections)

	go func() {
		for i := 0; i < numConnections; i++ {
			conn, err := adapter.Accept()
			if err != nil {
				return
			}
			conn.Close()
			accepted <- true
		}
	}()

	// Create multiple client connections
	for i := 0; i < numConnections; i++ {
		conn, err := net.Dial("tcp", addr)
		require.NoError(t, err)
		conn.Close()
	}

	// Wait for all connections to be accepted
	for i := 0; i < numConnections; i++ {
		select {
		case <-accepted:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatalf("Timeout waiting for connection %d", i)
		}
	}
}

// Mock types for testing

type mockReadWriteCloser struct {
	data []byte
}

func (m *mockReadWriteCloser) Read(p []byte) (n int, err error) {
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n = copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockReadWriteCloser) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}
