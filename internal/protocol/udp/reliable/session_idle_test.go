package reliable

import (
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSession_LastActivityUpdate tests that lastActivity is updated correctly
func TestSession_LastActivityUpdate(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create UDP connection
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	remoteAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}

	// Create session
	session := NewSession(conn, remoteAddr, 1, 1, true, logger)
	defer session.Close()

	// Get initial activity time
	initialActivity := session.getLastActivity()
	time.Sleep(100 * time.Millisecond)

	// Update activity
	session.updateActivity()

	// Verify activity was updated
	newActivity := session.getLastActivity()
	assert.True(t, newActivity.After(initialActivity),
		"Activity time should be updated")

	timeDiff := newActivity.Sub(initialActivity)
	assert.True(t, timeDiff >= 100*time.Millisecond,
		"Activity time difference should be at least 100ms")
}

// TestSession_IdleTimeoutDetection tests the idle timeout detection logic
func TestSession_IdleTimeoutDetection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create UDP connection
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	remoteAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}

	// Create session
	session := NewSession(conn, remoteAddr, 1, 1, true, logger)
	defer session.Close()

	// Test 1: Fresh session should not be idle
	idleTime := time.Since(session.getLastActivity())
	idleTimeoutDuration := time.Duration(SessionIdleTimeout) * time.Millisecond
	assert.True(t, idleTime < idleTimeoutDuration,
		"Fresh session should not be considered idle")

	// Test 2: Update activity and verify it's still not idle
	time.Sleep(100 * time.Millisecond)
	session.updateActivity()
	idleTime = time.Since(session.getLastActivity())
	assert.True(t, idleTime < idleTimeoutDuration,
		"Recently updated session should not be considered idle")

	// Test 3: Verify the timeout constant is reasonable (15 minutes)
	expectedTimeout := 15 * 60 * 1000 // 15 minutes in milliseconds
	assert.Equal(t, expectedTimeout, SessionIdleTimeout,
		"Session idle timeout should be 15 minutes")

	// Test 4: Verify keepalive interval is reasonable (30 seconds)
	expectedKeepAlive := 30 * 1000 // 30 seconds in milliseconds
	assert.Equal(t, expectedKeepAlive, KeepAliveInterval,
		"KeepAlive interval should be 30 seconds")
}

// TestSession_ActivityUpdateOnSend tests that activity is updated when sending data
func TestSession_ActivityUpdateOnSend(t *testing.T) {
	client, _, cleanup := setupTestPair(t)
	defer cleanup()

	// Get initial activity time
	initialActivity := client.getLastActivity()
	time.Sleep(100 * time.Millisecond)

	// Send data (this should update activity)
	testData := []byte("test data")
	_, err := client.Write(testData)
	require.NoError(t, err)

	// Wait a bit for the write to be processed
	time.Sleep(50 * time.Millisecond)

	// Verify activity was updated
	newActivity := client.getLastActivity()
	assert.True(t, newActivity.After(initialActivity),
		"Activity should be updated after sending data")
}

// TestSession_ActivityUpdateOnReceive tests that activity is updated when receiving data
func TestSession_ActivityUpdateOnReceive(t *testing.T) {
	client, server, cleanup := setupTestPair(t)
	defer cleanup()

	// Get initial activity time of server
	initialActivity := server.getLastActivity()
	time.Sleep(100 * time.Millisecond)

	// Client sends data to server
	testData := []byte("test data")
	_, err := client.Write(testData)
	require.NoError(t, err)

	// Server reads data (this should update activity)
	buf := make([]byte, len(testData))
	n, err := server.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Verify server activity was updated
	newActivity := server.getLastActivity()
	assert.True(t, newActivity.After(initialActivity),
		"Activity should be updated after receiving data")
}
