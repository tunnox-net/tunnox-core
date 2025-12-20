package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPLongPollingConn_Basic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 注意：这个测试需要真实的服务器，所以暂时跳过
	// 在实际环境中，应该使用 mock HTTP 服务器
	t.Skip("Skipping test that requires real HTTP server")

	conn, err := NewHTTPLongPollingConn(ctx, "https://example.com", 123, "test-token", "test-instance-id", "")
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn)
	assert.Equal(t, "httppoll", conn.LocalAddr().Network())
}

func TestHTTPLongPollingConn_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := NewHTTPLongPollingConn(ctx, "https://example.com", 123, "test-token", "test-instance-id", "")
	require.NoError(t, err)

	// 关闭连接
	err = conn.Close()
	assert.NoError(t, err)

	// 再次关闭应该不会出错
	err = conn.Close()
	assert.NoError(t, err)
}

func TestDialHTTPLongPolling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 注意：这个测试需要真实的服务器，所以暂时跳过
	t.Skip("Skipping test that requires real HTTP server")

	conn, err := dialHTTPLongPolling(ctx, "https://example.com", 123, "test-token", "test-instance-id", "")
	if err == nil {
		conn.Close()
	}
}
