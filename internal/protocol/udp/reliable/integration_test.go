package reliable

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestPair creates a connected client-server pair for testing
func setupTestPair(t *testing.T) (*Session, *Session, func()) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create server listener
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenUDP("udp", serverAddr)
	require.NoError(t, err)

	// Create server dispatcher
	serverDispatcher := NewPacketDispatcher(serverConn, logger)
	serverDispatcher.Start()

	// Create client connection
	clientConn, err := net.DialUDP("udp", nil, serverConn.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)

	// Create client dispatcher
	clientDispatcher := NewPacketDispatcher(clientConn, logger)
	clientDispatcher.Start()

	// Create client transport
	transport, err := NewClientTransport(clientConn, serverConn.LocalAddr().(*net.UDPAddr), clientDispatcher, logger)
	require.NoError(t, err)

	// Accept server session
	serverSession, err := serverDispatcher.Accept()
	require.NoError(t, err)

	cleanup := func() {
		transport.Close()
		serverSession.Close()
		serverDispatcher.Stop()
		clientDispatcher.Stop()
		serverConn.Close()
		clientConn.Close()
	}

	return transport.session, serverSession, cleanup
}

func TestIntegration_SmallDataTransfer(t *testing.T) {
	client, server, cleanup := setupTestPair(t)
	defer cleanup()

	// Send small data
	testData := []byte("Hello, UDP Reliable Transport!")

	var wg sync.WaitGroup
	wg.Add(1)

	// Receive in goroutine
	var received []byte
	var recvErr error
	go func() {
		defer wg.Done()
		buf := make([]byte, len(testData))
		n, err := io.ReadFull(server, buf)
		if err != nil {
			recvErr = err
			return
		}
		received = buf[:n]
	}()

	// Send data
	n, err := client.Write(testData)
	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Wait for receive
	wg.Wait()
	assert.NoError(t, recvErr)
	assert.Equal(t, testData, received)
}

func TestIntegration_LargeDataTransfer(t *testing.T) {
	client, server, cleanup := setupTestPair(t)
	defer cleanup()

	// Generate 1MB of random data
	dataSize := 1 * 1024 * 1024
	testData := make([]byte, dataSize)
	_, err := rand.Read(testData)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	// Receive in goroutine
	var received []byte
	var recvErr error
	go func() {
		defer wg.Done()
		buf := make([]byte, dataSize)
		n, err := io.ReadFull(server, buf)
		if err != nil {
			recvErr = err
			return
		}
		received = buf[:n]
	}()

	// Send data
	n, err := client.Write(testData)
	assert.NoError(t, err)
	assert.Equal(t, dataSize, n)

	// Wait for receive with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for data transfer")
	}

	assert.NoError(t, recvErr)
	assert.Equal(t, dataSize, len(received))
	assert.True(t, bytes.Equal(testData, received))
}

func TestIntegration_BidirectionalTransfer(t *testing.T) {
	client, server, cleanup := setupTestPair(t)
	defer cleanup()

	clientData := []byte("Hello from client!")
	serverData := []byte("Hello from server!")

	var wg sync.WaitGroup
	wg.Add(2)

	// Client receives from server
	var clientReceived []byte
	go func() {
		defer wg.Done()
		buf := make([]byte, len(serverData))
		io.ReadFull(client, buf)
		clientReceived = buf
	}()

	// Server receives from client
	var serverReceived []byte
	go func() {
		defer wg.Done()
		buf := make([]byte, len(clientData))
		io.ReadFull(server, buf)
		serverReceived = buf
	}()

	// Send data both ways
	client.Write(clientData)
	server.Write(serverData)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for bidirectional transfer")
	}

	assert.Equal(t, clientData, serverReceived)
	assert.Equal(t, serverData, clientReceived)
}

func TestIntegration_MultipleChunks(t *testing.T) {
	client, server, cleanup := setupTestPair(t)
	defer cleanup()

	// Send multiple chunks
	numChunks := 10
	chunkSize := 10 * 1024 // 10KB per chunk
	totalSize := numChunks * chunkSize

	testData := make([]byte, totalSize)
	_, err := rand.Read(testData)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var received []byte
	go func() {
		defer wg.Done()
		buf := make([]byte, totalSize)
		io.ReadFull(server, buf)
		received = buf
	}()

	// Send in chunks
	for i := 0; i < numChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		_, err := client.Write(testData[start:end])
		assert.NoError(t, err)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for chunked transfer")
	}

	assert.Equal(t, totalSize, len(received))
	assert.True(t, bytes.Equal(testData, received))
}

func TestIntegration_ConcurrentConnections(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create server
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenUDP("udp", serverAddr)
	require.NoError(t, err)
	defer serverConn.Close()

	serverDispatcher := NewPacketDispatcher(serverConn, logger)
	serverDispatcher.Start()
	defer serverDispatcher.Stop()

	numConnections := 5
	var wg sync.WaitGroup
	wg.Add(numConnections * 2) // client + server for each connection

	// Start server acceptor
	serverSessions := make([]*Session, 0, numConnections)
	var serverMu sync.Mutex
	go func() {
		for i := 0; i < numConnections; i++ {
			session, err := serverDispatcher.Accept()
			if err != nil {
				return
			}
			serverMu.Lock()
			serverSessions = append(serverSessions, session)
			serverMu.Unlock()

			// Handle server side
			go func(s *Session) {
				defer wg.Done()
				buf := make([]byte, 100)
				n, _ := s.Read(buf)
				s.Write(buf[:n]) // Echo back
			}(session)
		}
	}()

	// Create clients
	for i := 0; i < numConnections; i++ {
		go func(id int) {
			defer wg.Done()

			clientConn, err := net.DialUDP("udp", nil, serverConn.LocalAddr().(*net.UDPAddr))
			if err != nil {
				t.Errorf("client %d: dial failed: %v", id, err)
				return
			}
			defer clientConn.Close()

			clientDispatcher := NewPacketDispatcher(clientConn, logger)
			clientDispatcher.Start()
			defer clientDispatcher.Stop()

			transport, err := NewClientTransport(clientConn, serverConn.LocalAddr().(*net.UDPAddr), clientDispatcher, logger)
			if err != nil {
				t.Errorf("client %d: transport failed: %v", id, err)
				return
			}
			defer transport.Close()

			// Send and receive
			testData := []byte("Hello from client " + string(rune('0'+id)))
			transport.Write(testData)

			buf := make([]byte, 100)
			n, _ := transport.Read(buf)
			if !bytes.Equal(testData, buf[:n]) {
				t.Errorf("client %d: data mismatch", id)
			}
		}(i)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for concurrent connections")
	}

	// Cleanup server sessions
	serverMu.Lock()
	for _, s := range serverSessions {
		s.Close()
	}
	serverMu.Unlock()
}

func TestIntegration_FlowControl(t *testing.T) {
	client, server, cleanup := setupTestPair(t)
	defer cleanup()

	// Send data larger than flow control window
	dataSize := 100 * 1024 // 100KB
	testData := make([]byte, dataSize)
	_, err := rand.Read(testData)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	var received []byte
	go func() {
		defer wg.Done()
		buf := make([]byte, dataSize)
		io.ReadFull(server, buf)
		received = buf
	}()

	// Send data
	_, err = client.Write(testData)
	assert.NoError(t, err)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for flow controlled transfer")
	}

	assert.True(t, bytes.Equal(testData, received))
}

// BenchmarkDataTransfer benchmarks data transfer throughput
func BenchmarkDataTransfer(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Setup
	serverAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	serverConn, _ := net.ListenUDP("udp", serverAddr)
	defer serverConn.Close()

	serverDispatcher := NewPacketDispatcher(serverConn, logger)
	serverDispatcher.Start()
	defer serverDispatcher.Stop()

	clientConn, _ := net.DialUDP("udp", nil, serverConn.LocalAddr().(*net.UDPAddr))
	defer clientConn.Close()

	clientDispatcher := NewPacketDispatcher(clientConn, logger)
	clientDispatcher.Start()
	defer clientDispatcher.Stop()

	transport, _ := NewClientTransport(clientConn, serverConn.LocalAddr().(*net.UDPAddr), clientDispatcher, logger)
	defer transport.Close()

	server, _ := serverDispatcher.Accept()
	defer server.Close()

	// Prepare test data
	dataSize := 64 * 1024 // 64KB
	testData := make([]byte, dataSize)
	rand.Read(testData)

	// Start receiver
	go func() {
		buf := make([]byte, dataSize)
		for {
			_, err := server.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	b.ResetTimer()
	b.SetBytes(int64(dataSize))

	for i := 0; i < b.N; i++ {
		transport.Write(testData)
	}
}
