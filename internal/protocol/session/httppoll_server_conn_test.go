package session

import (
	"context"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerHTTPLongPollingConn_Creation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := NewServerHTTPLongPollingConn(ctx, 123)
	require.NotNil(t, conn)
	assert.Equal(t, int64(123), conn.GetClientID())
	assert.NotNil(t, conn.LocalAddr())
	assert.NotNil(t, conn.RemoteAddr())
}

func TestServerHTTPLongPollingConn_PushData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := NewServerHTTPLongPollingConn(ctx, 123)
	require.NotNil(t, conn)

	// 测试 PushData（现在接收 Base64 字符串）
	testData := []byte("push data")
	encodedData := base64.StdEncoding.EncodeToString(testData)
	err := conn.PushData(encodedData)
	require.NoError(t, err)

	// 测试从 Read 读取数据（应该返回解码后的原始数据）
	readBuf := make([]byte, 100)
	n, err := conn.Read(readBuf)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, readBuf[:n])
}

func TestServerHTTPLongPollingConn_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := NewServerHTTPLongPollingConn(ctx, 123)
	require.NotNil(t, conn)

	// 关闭连接
	err := conn.Close()
	require.NoError(t, err)

	// 测试关闭后读取应该返回 EOF
	readBuf := make([]byte, 100)
	_, err = conn.Read(readBuf)
	assert.Equal(t, io.EOF, err)

	// 测试关闭后写入应该返回错误
	_, err = conn.Write([]byte("test"))
	assert.Equal(t, io.ErrClosedPipe, err)
}

func TestServerHTTPLongPollingConn_PollDataTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := NewServerHTTPLongPollingConn(ctx, 123)
	require.NotNil(t, conn)

	// 测试超时
	pollCtx, pollCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer pollCancel()

	_, err := conn.PollData(pollCtx)
	assert.Equal(t, context.DeadlineExceeded, err)
}

